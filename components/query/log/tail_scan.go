package log

import (
	"context"
	"time"

	query_log_common "github.com/leptonai/gpud/components/query/log/common"
	query_log_tail "github.com/leptonai/gpud/components/query/log/tail"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TailScan tails the last N lines without polling, just by reading the file.
// This only catches the old logs, not the future ones.
func (pl *poller) TailScan(ctx context.Context, opts ...query_log_tail.OpOption) ([]Item, error) {
	tailOpts := &query_log_tail.Op{}
	if err := tailOpts.ApplyOpts(opts); err != nil {
		return nil, err
	}

	items := make([]Item, 0)
	processMatchedFunc := func(line []byte, time time.Time, matchedFilter *query_log_common.Filter) {
		items = append(items, Item{
			Time:    metav1.Time{Time: time},
			Line:    string(line),
			Matched: matchedFilter,
		})

		if tailOpts.ProcessMatched != nil {
			tailOpts.ProcessMatched(line, time, matchedFilter)
		}
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
