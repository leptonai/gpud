package log

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/poller"
	poller_log_common "github.com/leptonai/gpud/poller/log/common"
	poller_log_config "github.com/leptonai/gpud/poller/log/config"
	poller_log_tail "github.com/leptonai/gpud/poller/log/tail"

	"github.com/nxadm/tail"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ poller.Poller = (*pollerImpl)(nil)

var _ Poller = (*pollerImpl)(nil)

// Poller implements the log file poller.
// The underlying poller is a tail.Tail but with poll mode enabled.
// Poll is better when there are multiple consumers (e.g., multiple log tailers)
// reading from the same file.
type Poller interface {
	poller.Poller

	// Config returns the config used to start the log poller.
	// This is useful for debugging and logging.
	LogConfig() poller_log_config.Config

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
	TailScan(ctx context.Context, opts ...poller_log_tail.OpOption) ([]Item, error)

	// Returns all the events for the given "since" time.
	// If none, it returns all events that are already filtered
	// by the default filters in the configuration.
	// Returns `github.com/leptonai/gpud/poller.ErrNoData` if there is no event found.
	Find(since time.Time, selectFilters ...*poller_log_common.Filter) ([]Item, error)

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
	Matched *poller_log_common.Filter `json:"matched,omitempty"`

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

type pollerImpl struct {
	poller.Poller

	cfg poller_log_config.Config

	tailLogger poller_log_tail.Streamer

	tailFileSeekInfoMu     sync.RWMutex
	tailFileSeekInfo       tail.SeekInfo
	tailFileSeekInfoSyncer func(ctx context.Context, file string, seekInfo tail.SeekInfo)

	bufferedItemsMu sync.RWMutex
	bufferedItems   []Item
}

func New(ctx context.Context, cfg poller_log_config.Config, extractTime poller_log_common.ExtractTimeFunc, processMatched poller_log_common.ProcessMatchedFunc) (Poller, error) {
	return newPoller(ctx, cfg, extractTime, processMatched)
}

func newPoller(ctx context.Context, cfg poller_log_config.Config, extractTime poller_log_common.ExtractTimeFunc, processMatched poller_log_common.ProcessMatchedFunc) (*pollerImpl, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.SetDefaultsIfNotSet()

	options := []poller_log_tail.OpOption{
		poller_log_tail.WithDedup(true),
		poller_log_tail.WithSelectFilter(cfg.SelectFilters...),
		poller_log_tail.WithRejectFilter(cfg.RejectFilters...),
		poller_log_tail.WithExtractTime(extractTime),
		poller_log_tail.WithProcessMatched(processMatched),
		poller_log_tail.WithSkipEmptyLine(true),
	}

	if cfg.File != "" {
		options = append(options, poller_log_tail.WithLabel("file", cfg.File))
	} else {
		for i, cmds := range cfg.Commands {
			options = append(options, poller_log_tail.WithLabel(fmt.Sprintf("command-%d", i+1), strings.Join(cmds, " ")))
		}
	}

	var tailLogger poller_log_tail.Streamer
	var err error
	if cfg.File != "" {
		tailLogger, err = poller_log_tail.NewFromFile(ctx, cfg.File, cfg.SeekInfo, options...)
	} else {
		tailLogger, err = poller_log_tail.NewFromCommand(ctx, cfg.Commands, options...)
	}
	if err != nil {
		return nil, err
	}

	pl := &pollerImpl{
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

	pl.Poller = poller.New(
		name,
		cfg.PollerConfig,
		flushFunc,
		nil,
	)

	return pl, nil
}

// pollSync polls the log tail from the specified file or long-running commands
// and syncs the items to the buffered items.
// This only catches the realtime/latest and all the future logs.
func (pl *pollerImpl) pollSync(ctx context.Context) {
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

func (pl *pollerImpl) LogConfig() poller_log_config.Config {
	return pl.cfg
}

func (pl *pollerImpl) File() string {
	return pl.tailLogger.File()
}

func (pl *pollerImpl) Commands() [][]string {
	return pl.tailLogger.Commands()
}

// This only catches the realtime/latest and all the future logs.
// Returns `github.com/leptonai/gpud/poller.ErrNoData` if there is no event found.
func (pl *pollerImpl) Find(since time.Time, selectFilters ...*poller_log_common.Filter) ([]Item, error) {
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
		for _, item := range itemsFromPollerOutput {
			if len(selectFilters) == 0 {
				items = append(items, item)
				continue
			}

			var matchedFilter *poller_log_common.Filter
			for _, f := range selectFilters {
				matched, err := f.MatchString(item.Line)
				if err != nil {
					return nil, err
				}
				if matched {
					matchedFilter = f
					break
				}
			}

			if matchedFilter != nil {
				item.Matched = matchedFilter
				items = append(items, item)
			}
		}
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

		if len(selectFilters) == 0 {
			items = append(items, item)
			continue
		}

		var matchedFilter *poller_log_common.Filter
		for _, f := range selectFilters {
			matched, err := f.MatchString(item.Line)
			if err != nil {
				return nil, err
			}
			if matched {
				matchedFilter = f
				break
			}
		}

		if matchedFilter != nil {
			item.Matched = matchedFilter
			items = append(items, item)
		}
	}

	return items, nil
}

func (pl *pollerImpl) SeekInfo() tail.SeekInfo {
	pl.tailFileSeekInfoMu.RLock()
	defer pl.tailFileSeekInfoMu.RUnlock()
	return pl.tailFileSeekInfo
}
