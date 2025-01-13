// Package fd tracks the number of file descriptors used on the host.
package fd

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/leptonai/gpud/components"
	common_dmesg "github.com/leptonai/gpud/components/common/dmesg"
	fd_id "github.com/leptonai/gpud/components/fd/id"
	"github.com/leptonai/gpud/components/fd/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"github.com/prometheus/client_golang/prometheus"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, fd_id.Name)

	common_dmesg.SetDefaultLogPoller(ctx, cfg.Query.State.DBRW, cfg.Query.State.DBRO)
	common_dmesg.GetDefaultLogPoller().Start(cctx, cfg.Query, common_dmesg.Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  getDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx  context.Context
	cancel   context.CancelFunc
	poller   query.Poller
	gatherer prometheus.Gatherer
}

func (c *component) Name() string { return fd_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", fd_id.Name)
		return []components.State{
			{
				Name:    fd_id.Name,
				Healthy: true,
				Reason:  query.ErrNoData.Error(),
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if last.Error != nil {
		return []components.State{
			{
				Name:    fd_id.Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Name:    fd_id.Name,
				Healthy: true,
				Reason:  "no output",
			},
		}, nil
	}

	output, ok := last.Output.(*Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T", last.Output)
	}
	return output.States()
}

const (
	EventNameErrorVFSFileMaxLimitReached = "error_vfs_file_max_limit_reached"

	EventKeyErrorVFSFileMaxLimitReachedUnixSeconds = "unix_seconds"
	EventKeyErrorVFSFileMaxLimitReachedLogLine     = "log_line"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	logItems, err := common_dmesg.GetDefaultLogPoller().Find(since)
	if err != nil {
		return nil, err
	}

	events := make([]components.Event, 0)
	for _, logItem := range logItems {
		if logItem.Error != nil {
			continue
		}
		if logItem.Matched == nil {
			continue
		}
		if logItem.Matched.Name != common_dmesg.EventFileDescriptorVFSFileMaxLimitReached {
			continue
		}

		events = append(events, components.Event{
			Time:    logItem.Time,
			Name:    EventNameErrorVFSFileMaxLimitReached,
			Type:    components.EventTypeCritical,
			Message: "VFS file-max limit reached",
			ExtraInfo: map[string]string{
				EventKeyErrorVFSFileMaxLimitReachedUnixSeconds: strconv.FormatInt(logItem.Time.Unix(), 10),
				EventKeyErrorVFSFileMaxLimitReachedLogLine:     logItem.Line,
			},
		})
	}

	return events, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	allocatedFileHandles, err := metrics.ReadAllocatedFileHandles(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read allocated file handles: %w", err)
	}
	runningPIDs, err := metrics.ReadRunningPIDs(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read running pids: %w", err)
	}
	limits, err := metrics.ReadLimits(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read limits: %w", err)
	}
	allocatedPercents, err := metrics.ReadAllocatedFileHandlesPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read allocated percents: %w", err)
	}
	usedPercents, err := metrics.ReadUsedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used percents: %w", err)
	}

	ms := make([]components.Metric, 0, len(allocatedFileHandles)+len(runningPIDs)+len(limits)+len(allocatedPercents)+len(usedPercents))
	for _, m := range allocatedFileHandles {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range runningPIDs {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range limits {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range allocatedPercents {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range usedPercents {
		ms = append(ms, components.Metric{Metric: m})
	}

	return ms, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(fd_id.Name)

	common_dmesg.GetDefaultLogPoller().Stop(common_dmesg.Name)

	c.cancel()
	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return metrics.Register(reg, dbRW, dbRO, tableName)
}
