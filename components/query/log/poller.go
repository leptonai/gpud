package log

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components/query"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
	query_log_tail "github.com/leptonai/gpud/components/query/log/tail"
	"github.com/leptonai/gpud/log"

	"github.com/nxadm/tail"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ query.Poller = (*poller)(nil)

var _ Poller = (*poller)(nil)

// Poller implements the log file poller.
// The underlying poller is a tail.Tail but with poll mode enabled.
// Poll is better when there are multiple consumers (e.g., multiple log tailers)
// reading from the same file.
type Poller interface {
	query.Poller

	// Config returns the config used to start the log poller.
	// This is useful for debugging and logging.
	LogConfig() query_log_config.Config

	// Returns the file name that this poller watches on.
	File() string
	// Returns the commands that this poller is running.
	Commands() [][]string

	// Tails the last N lines without polling, just by reading the file
	// from the end of the file.
	// Thus, the returned items are sorted by the time from new to old.
	//
	// Useful for investing the old dmesg logs.
	// Use this to backfill events for the old logs.
	//
	// If select filter is none, it returns all events
	// that are already filtered by the default filters
	// in the configuration.
	TailScan(ctx context.Context, opts ...query_log_tail.OpOption) ([]Item, error)

	// Returns all the events for the given "since" time.
	// If the log poller configures filters, it only returns the events that match the filters.
	// Returns `github.com/leptonai/gpud/components/query.ErrNoData` if there is no event found.
	Find(since time.Time) ([]Item, error)

	// Returns the last seek info.
	SeekInfo() tail.SeekInfo
}

// Item is the basic unit of data that poller returns.
// If enabled, each result is persisted in the storage.
// It is converted from the underlying query poller.
type Item struct {
	Time metav1.Time `json:"time"`
	Line string      `json:"line"`

	// Matched filter that was applied to this item/line.
	Matched *query_log_common.Filter `json:"matched,omitempty"`

	Error *string `json:"error,omitempty"`
}

func (item Item) JSON() ([]byte, error) {
	return json.Marshal(item)
}

func ParseItemJSON(data []byte) (Item, error) {
	item := Item{}
	if err := json.Unmarshal(data, &item); err != nil {
		return Item{}, err
	}
	if item.Matched != nil && item.Matched.Regex != nil {
		if err := item.Matched.Compile(); err != nil {
			return Item{}, err
		}
	}
	return item, nil
}

type Items []Item

type poller struct {
	query.Poller

	cfg query_log_config.Config

	tailLogger query_log_tail.Streamer

	tailFileSeekInfoMu     sync.RWMutex
	tailFileSeekInfo       tail.SeekInfo
	tailFileSeekInfoSyncer func(ctx context.Context, file string, seekInfo tail.SeekInfo)

	bufferedItemsMu sync.RWMutex
	bufferedItems   []Item
}

func New(ctx context.Context, cfg query_log_config.Config, extractTime query_log_common.ExtractTimeFunc, processMatched query_log_common.ProcessMatchedFunc) (Poller, error) {
	return newPoller(ctx, cfg, extractTime, processMatched)
}

func newPoller(ctx context.Context, cfg query_log_config.Config, extractTime query_log_common.ExtractTimeFunc, processMatched query_log_common.ProcessMatchedFunc) (*poller, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.SetDefaultsIfNotSet()

	options := []query_log_tail.OpOption{
		query_log_tail.WithDedup(true),
		query_log_tail.WithSelectFilter(cfg.SelectFilters...),
		query_log_tail.WithRejectFilter(cfg.RejectFilters...),
		query_log_tail.WithExtractTime(extractTime),
		query_log_tail.WithProcessMatched(processMatched),
		query_log_tail.WithSkipEmptyLine(true),
	}

	if cfg.File != "" {
		options = append(options, query_log_tail.WithLabel("file", cfg.File))
	} else {
		for i, cmds := range cfg.Commands {
			options = append(options, query_log_tail.WithLabel(fmt.Sprintf("command-%d", i+1), strings.Join(cmds, " ")))
		}
	}

	var tailLogger query_log_tail.Streamer
	var err error
	if cfg.File != "" {
		tailLogger, err = query_log_tail.NewFromFile(ctx, cfg.File, cfg.SeekInfo, options...)
	} else {
		tailLogger, err = query_log_tail.NewFromCommand(ctx, cfg.Commands, options...)
	}
	if err != nil {
		return nil, err
	}

	pl := &poller{
		cfg:                    cfg,
		tailLogger:             tailLogger,
		tailFileSeekInfoSyncer: cfg.SeekInfoSyncer,
		bufferedItems:          make([]Item, 0, cfg.BufferSize),
	}
	go pl.pollSync(ctx)

	flushFunc := func(ctx context.Context) (any, error) {
		pl.bufferedItemsMu.Lock()
		defer pl.bufferedItemsMu.Unlock()
		copied := make([]Item, len(pl.bufferedItems))
		copy(copied, pl.bufferedItems)
		pl.bufferedItems = pl.bufferedItems[:0]
		return copied, nil
	}

	name := cfg.File
	if name == "" {
		for _, args := range cfg.Commands {
			if name != "" {
				name += ", "
			}
			name += strings.Join(args, " ")
		}
	}

	pl.Poller = query.New(
		name,
		cfg.Query,
		flushFunc,
		nil,
	)

	return pl, nil
}

// pollSync polls the log tail from the specified file or long-running commands
// and syncs the items to the buffered items.
// This only catches the realtime/latest and all the future logs.
func (pl *poller) pollSync(ctx context.Context) {
	for line := range pl.tailLogger.Line() {
		var errStr *string
		if line.Err != nil {
			s := line.Err.Error()
			errStr = &s
		}

		item := Item{
			Time:    metav1.Time{Time: line.Time},
			Line:    line.Text,
			Matched: line.MatchedFilter,
			Error:   errStr,
		}

		pl.bufferedItemsMu.Lock()
		pl.bufferedItems = append(pl.bufferedItems, item)
		pl.bufferedItemsMu.Unlock()

		pl.tailFileSeekInfoMu.Lock()
		pl.tailFileSeekInfo = line.SeekInfo
		if pl.tailFileSeekInfoSyncer != nil {
			pl.tailFileSeekInfoSyncer(ctx, pl.tailLogger.File(), pl.tailFileSeekInfo)
		}
		pl.tailFileSeekInfoMu.Unlock()
	}
}

func (pl *poller) LogConfig() query_log_config.Config {
	return pl.cfg
}

func (pl *poller) File() string {
	return pl.tailLogger.File()
}

func (pl *poller) Commands() [][]string {
	return pl.tailLogger.Commands()
}

// This only catches the realtime/latest and all the future logs.
// Returns `github.com/leptonai/gpud/components/query.ErrNoData` if there is no event found.
func (pl *poller) Find(since time.Time) ([]Item, error) {
	// 1. filter the already flushed/in-queue ones
	polledItems, err := pl.Poller.All(since)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0)
	for _, item := range polledItems {
		if item.Error != nil {
			continue
		}
		if item.Output == nil {
			log.Logger.Warnw("polled item has no output (without an error)", "item", item)
			continue
		}

		itemsFromPollerOutput := item.Output.([]Item)
		items = append(items, itemsFromPollerOutput...)
	}

	pl.bufferedItemsMu.RLock()
	defer pl.bufferedItemsMu.RUnlock()

	// 2. filter the buffered ones
	// if not empty, buffered ones have not been flushed by the poller
	// thus not returned by the poller all events
	for _, item := range pl.bufferedItems {
		if !since.IsZero() && item.Time.Time.Before(since) {
			continue
		}
		items = append(items, item)
	}

	return items, nil
}

func (pl *poller) SeekInfo() tail.SeekInfo {
	pl.tailFileSeekInfoMu.RLock()
	defer pl.tailFileSeekInfoMu.RUnlock()
	return pl.tailFileSeekInfo
}
