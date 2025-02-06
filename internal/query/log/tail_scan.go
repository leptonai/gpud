package log

import (
	"context"
	"time"

	query_log_common "github.com/leptonai/gpud/internal/query/log/common"
	query_log_tail "github.com/leptonai/gpud/internal/query/log/tail"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TailScan tails the last N lines without polling, just by reading the file.
// This only catches the old logs, not the future ones.
func (pl *poller) TailScan(ctx context.Context, overwriteOpts ...query_log_tail.OpOption) ([]Item, error) {
	items := make([]Item, 0)
	defaultAppendMatchedFunc := func(time time.Time, line []byte, matchedFilter *query_log_common.Filter) {
		items = append(items, Item{
			Time:    metav1.Time{Time: time},
			Line:    string(line),
			Matched: matchedFilter,
		})
	}

	tailOverwriteOpts := &query_log_tail.Op{}
	_ = tailOverwriteOpts.ApplyOpts(overwriteOpts)
	procMatchedFunc := func(time time.Time, line []byte, matchedFilter *query_log_common.Filter) {
		if tailOverwriteOpts.ProcessMatched != nil {
			tailOverwriteOpts.ProcessMatched(time, line, matchedFilter)
		}
		defaultAppendMatchedFunc(time, line, matchedFilter)
	}

	// default options
	updatedOptions := []query_log_tail.OpOption{
		query_log_tail.WithProcessMatched(procMatchedFunc),
	}
	if pl.cfg.File != "" {
		updatedOptions = append(updatedOptions, query_log_tail.WithFile(pl.cfg.File))
	}
	if len(pl.cfg.Commands) > 0 {
		updatedOptions = append(updatedOptions, query_log_tail.WithCommands(pl.cfg.Commands))
	}
	if pl.cfg.Scan != nil && pl.cfg.Scan.File != "" {
		updatedOptions = append(updatedOptions, query_log_tail.WithFile(pl.cfg.Scan.File))
	}
	if pl.cfg.Scan != nil && len(pl.cfg.Scan.Commands) > 0 {
		updatedOptions = append(updatedOptions, query_log_tail.WithCommands(pl.cfg.Scan.Commands))
	}
	if len(pl.cfg.SelectFilters) > 0 {
		updatedOptions = append(updatedOptions, query_log_tail.WithSelectFilter(pl.cfg.SelectFilters...))
	}

	for _, opt := range overwriteOpts {
		// do not overwrite the process matched function since we need default items append operation
		tmp := &query_log_tail.Op{}
		_ = tmp.ApplyOpts([]query_log_tail.OpOption{opt})
		if tmp.ProcessMatched != nil {
			continue
		}

		// remaining ones can be overwritten
		updatedOptions = append(updatedOptions, opt)
	}

	if _, err := query_log_tail.Scan(ctx, updatedOptions...); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}

	return items, nil
}
