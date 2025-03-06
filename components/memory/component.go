// Package memory tracks the memory usage of the host.
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	memory_id "github.com/leptonai/gpud/components/memory/id"
	"github.com/leptonai/gpud/components/memory/metrics"
	"github.com/leptonai/gpud/pkg/dmesg"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/query"

	"github.com/prometheus/client_golang/prometheus"
)

func New(ctx context.Context, cfg Config) (components.Component, error) {
	eventsStore, err := events_db.NewStore(
		cfg.Query.State.DBRW,
		cfg.Query.State.DBRO,
		events_db.CreateDefaultTableName(memory_id.Name),
		3*24*time.Hour,
	)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	logLineProcessor, err := dmesg.NewLogLineProcessor(cctx, Match, eventsStore)
	if err != nil {
		ccancel()
		return nil, err
	}

	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)
	getDefaultPoller().Start(cctx, cfg.Query, memory_id.Name)

	kmsgWatcher, err := kmsg.StartWatch(Match)
	if err != nil {
		ccancel()
		return nil, err
	}

	return &component{
		ctx:              cctx,
		cancel:           ccancel,
		poller:           getDefaultPoller(),
		cfg:              cfg,
		logLineProcessor: logLineProcessor,
		eventsStore:      eventsStore,
		kmsgWatcher:      kmsgWatcher,
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	ctx              context.Context
	cancel           context.CancelFunc
	poller           query.Poller
	cfg              Config
	logLineProcessor *dmesg.LogLineProcessor
	eventsStore      events_db.Store
	gatherer         prometheus.Gatherer

	// experimental
	kmsgWatcher kmsg.Watcher
}

func (c *component) Name() string { return memory_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", memory_id.Name)
		return []components.State{
			{
				Name:    memory_id.Name,
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
				Name:    memory_id.Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Name:    memory_id.Name,
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
	return c.eventsStore.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	totalBytes, err := metrics.ReadTotalBytes(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read total bytes: %w", err)
	}
	usedBytes, err := metrics.ReadUsedBytes(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used bytes: %w", err)
	}
	usedPercents, err := metrics.ReadUsedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used bytes percents: %w", err)
	}

	ms := make([]components.Metric, 0, len(totalBytes)+len(usedBytes)+len(usedPercents))
	for _, m := range totalBytes {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range usedBytes {
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
	c.poller.Stop(memory_id.Name)

	c.logLineProcessor.Close()
	c.eventsStore.Close()

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
