// Package fd tracks the number of file descriptors used on the host.
package fd

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	fd_id "github.com/leptonai/gpud/components/fd/id"
	"github.com/leptonai/gpud/components/fd/metrics"
	"github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/query"
)

func New(ctx context.Context, cfg Config, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(fd_id.Name)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	logLineProcessor, err := dmesg.NewLogLineProcessor(cctx, Match, eventBucket)
	if err != nil {
		ccancel()
		return nil, err
	}

	kmsgWatcher, err := kmsg.StartWatch(Match)
	if err != nil {
		ccancel()
		return nil, err
	}

	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)
	getDefaultPoller().Start(cctx, cfg.Query, fd_id.Name)

	return &component{
		rootCtx:          ctx,
		cancel:           ccancel,
		logLineProcessor: logLineProcessor,
		eventBucket:      eventBucket,
		kmsgWatcher:      kmsgWatcher,
		poller:           getDefaultPoller(),
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx          context.Context
	cancel           context.CancelFunc
	logLineProcessor *dmesg.LogLineProcessor
	eventBucket      eventstore.Bucket
	poller           query.Poller
	gatherer         prometheus.Gatherer

	// experimental
	kmsgWatcher kmsg.Watcher
}

func (c *component) Name() string { return fd_id.Name }

func (c *component) Start() error { return nil }

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

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventBucket.Get(ctx, since)
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
	c.cancel()

	// safe to call stop multiple times
	c.poller.Stop(fd_id.Name)

	c.logLineProcessor.Close()
	c.eventBucket.Close()

	if c.kmsgWatcher != nil {
		c.kmsgWatcher.Close()
	}

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return metrics.Register(reg, dbRW, dbRO, tableName)
}
