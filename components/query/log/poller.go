package log

import (
	"context"
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
	// If none, it returns all events that are already filtered
	// by the default filters in the configuration.
	Find(since time.Time, selectFilters ...*query_log_common.Filter) ([]Item, error)

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

	Error error `json:"error,omitempty"`
}

type poller struct {
	query.Poller

	cfg query_log_config.Config

	tailLogger             query_log_tail.Streamer
	tailFileSeekInfoMu     sync.RWMutex
	tailFileSeekInfo       tail.SeekInfo
	tailFileSeekInfoSyncer func(ctx context.Context, file string, seekInfo tail.SeekInfo) `json:"-"`

	bufferedItemsMu sync.RWMutex
	bufferedItems   []Item
}

func New(ctx context.Context, cfg query_log_config.Config, parseTime query_log_common.ParseTimeFunc) (Poller, error) {
	return newPoller(ctx, cfg, parseTime)
}

func newPoller(ctx context.Context, cfg query_log_config.Config, parseTime query_log_common.ParseTimeFunc) (*poller, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.SetDefaultsIfNotSet()

	options := []query_log_tail.OpOption{
		query_log_tail.WithSelectFilter(cfg.SelectFilters...),
		query_log_tail.WithRejectFilter(cfg.RejectFilters...),
		query_log_tail.WithParseTime(parseTime),
	}

	var tailLogger query_log_tail.Streamer
	var err error
	if cfg.File != "" {
		tailLogger, err = query_log_tail.NewFromFile(cfg.File, cfg.SeekInfo, options...)
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
	)

	return pl, nil
}

func (pl *poller) pollSync(ctx context.Context) {
	for line := range pl.tailLogger.Line() {
		item := Item{
			Time:    metav1.Time{Time: line.Time},
			Line:    line.Text,
			Matched: line.MatchedFilter,
			Error:   line.Err,
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

func (pl *poller) TailScan(ctx context.Context, opts ...query_log_tail.OpOption) ([]Item, error) {
	items := make([]Item, 0)
	processMatchedFunc := func(line []byte, time time.Time, matchedFilter *query_log_common.Filter) {
		items = append(items, Item{
			Time:    metav1.Time{Time: time},
			Line:    string(line),
			Matched: matchedFilter,
		})
	}

	options := []query_log_tail.OpOption{
		query_log_tail.WithProcessMatched(processMatchedFunc),
	}
	if pl.cfg.File != "" {
		options = append(options, query_log_tail.WithFile(pl.cfg.File))
	}
	if len(pl.cfg.Commands) > 0 {
		options = append(options, query_log_tail.WithCommands(pl.cfg.Commands))
	}
	if pl.cfg.Scan != nil && pl.cfg.Scan.File != "" {
		options = append(options, query_log_tail.WithFile(pl.cfg.Scan.File))
	}
	if pl.cfg.Scan != nil && len(pl.cfg.Scan.Commands) > 0 {
		options = append(options, query_log_tail.WithCommands(pl.cfg.Scan.Commands))
	}
	if len(pl.cfg.SelectFilters) > 0 {
		options = append(options, query_log_tail.WithSelectFilter(pl.cfg.SelectFilters...))
	}
	if _, err := query_log_tail.Scan(
		ctx,
		append(options, opts...)...,
	); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}

	return items, nil
}

func (pl *poller) Find(since time.Time, selectFilters ...*query_log_common.Filter) ([]Item, error) {
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

			var matchedFilter *query_log_common.Filter
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

		var matchedFilter *query_log_common.Filter
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

func (pl *poller) SeekInfo() tail.SeekInfo {
	pl.tailFileSeekInfoMu.RLock()
	defer pl.tailFileSeekInfoMu.RUnlock()
	return pl.tailFileSeekInfo
}
