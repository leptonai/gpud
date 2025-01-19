// Package fd tracks the number of file descriptors used on the host.
package fd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/leptonai/gpud/components"
	fd_id "github.com/leptonai/gpud/components/fd/id"
	"github.com/leptonai/gpud/components/fd/metrics"
	fd_state "github.com/leptonai/gpud/components/fd/state"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/poller"
	poller_log "github.com/leptonai/gpud/pkg/poller/log"
	"github.com/leptonai/gpud/pkg/process"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(ctx context.Context, cfg Config) (components.Component, error) {
	cctx, ccancel := context.WithCancel(ctx)

	cfg.PollerConfig.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)
	getDefaultPoller().Start(cctx, fd_id.Name)

	c := &component{
		rootCtx: ctx,
		cancel:  ccancel,
		cfg:     cfg,
		poller:  getDefaultPoller(),
	}

	if runtime.GOOS == "linux" && process.CommandExists("dmesg") && os.Geteuid() == 0 {
		if err := setDefaultLogPoller(ctx, cfg.PollerConfig); err != nil {
			return nil, err
		}
		c.logPoller = getDefaultLogPoller()
		c.logPoller.Start(cctx, fd_id.Name)
	}

	return c, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx   context.Context
	cancel    context.CancelFunc
	cfg       Config
	poller    poller.Poller
	logPoller poller_log.Poller
	gatherer  prometheus.Gatherer
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
	EventKeyErrorVFSFileMaxLimitReachedUnixSeconds = "unix_seconds"
	EventKeyErrorVFSFileMaxLimitReachedLogLine     = "log_line"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	if c.logPoller == nil {
		return nil, nil
	}

	evs, err := fd_state.ReadEvents(ctx, c.cfg.PollerConfig.State.DBRO, fd_state.WithSince(since))
	if err != nil {
		return nil, err
	}

	events := make([]components.Event, 0)
	for _, ev := range evs {
		events = append(events, components.Event{
			Time:    metav1.Time{Time: time.Unix(ev.UnixSeconds, 0)},
			Name:    ev.EventType,
			Type:    components.EventTypeCritical,
			Message: "VFS file-max limit reached",
			ExtraInfo: map[string]string{
				EventKeyErrorVFSFileMaxLimitReachedUnixSeconds: strconv.FormatInt(ev.UnixSeconds, 10),
				EventKeyErrorVFSFileMaxLimitReachedLogLine:     ev.EventDetails,
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
	_ = c.poller.Stop(fd_id.Name)

	if c.logPoller != nil && runtime.GOOS == "linux" && process.CommandExists("dmesg") && os.Geteuid() == 0 {
		_ = c.logPoller.Stop(fd_id.Name)
	}

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return metrics.Register(reg, dbRW, dbRO, tableName)
}
