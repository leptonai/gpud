// Package fuse monitors the FUSE (Filesystem in Userspace).
package fuse

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	fuse_id "github.com/leptonai/gpud/components/fuse/id"
	"github.com/leptonai/gpud/components/fuse/metrics"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/query"
)

func New(ctx context.Context, cfg Config) (components.Component, error) {
	eventsStore, err := events_db.NewStore(
		cfg.Query.State.DBRW,
		cfg.Query.State.DBRO,
		events_db.CreateDefaultTableName(fuse_id.Name),
		events_db.DefaultRetention,
	)
	if err != nil {
		return nil, err
	}

	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg, eventsStore)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, fuse_id.Name)

	return &component{
		cfg:         cfg,
		ctx:         cctx,
		cancel:      ccancel,
		poller:      getDefaultPoller(),
		eventsStore: eventsStore,
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	cfg         Config
	ctx         context.Context
	cancel      context.CancelFunc
	poller      query.Poller
	eventsStore events_db.Store
	gatherer    prometheus.Gatherer
}

func (c *component) Name() string { return fuse_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", fuse_id.Name)
		return []components.State{
			{
				Name:    fuse_id.Name,
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
				Name:    fuse_id.Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}

	return []components.State{
		{
			Name:    fuse_id.Name,
			Healthy: true,
		},
	}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventsStore.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	congestedPercents, err := metrics.ReadConnectionsCongestedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read congested percents: %w", err)
	}
	maxBackgroundPercents, err := metrics.ReadConnectionsMaxBackgroundPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read max background percents: %w", err)
	}
	ms := make([]components.Metric, 0, len(congestedPercents)+len(maxBackgroundPercents))
	for _, m := range congestedPercents {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range maxBackgroundPercents {
		ms = append(ms, components.Metric{Metric: m})
	}

	return ms, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	// safe to call stop multiple times
	c.poller.Stop(fuse_id.Name)

	c.eventsStore.Close()

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return metrics.Register(reg, dbRW, dbRO, tableName)
}
