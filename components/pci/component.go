// Package pci tracks the PCI devices and their Access Control Services (ACS) status.
package pci

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	events_db "github.com/leptonai/gpud/components/db"
	"github.com/leptonai/gpud/components/pci/id"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
)

func New(ctx context.Context, cfg Config) (components.Component, error) {
	eventsStore, err := events_db.NewStore(
		cfg.Query.State.DBRW,
		cfg.Query.State.DBRO,
		events_db.CreateDefaultTableName(id.Name),
		3*24*time.Hour,
	)
	if err != nil {
		return nil, err
	}

	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg, eventsStore)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, id.Name)

	return &component{
		cfg:         cfg,
		rootCtx:     ctx,
		cancel:      ccancel,
		poller:      getDefaultPoller(),
		eventsStore: eventsStore,
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	cfg         Config
	rootCtx     context.Context
	cancel      context.CancelFunc
	poller      query.Poller
	eventsStore events_db.Store
}

func (c *component) Name() string { return id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return nil, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventsStore.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(id.Name)

	c.eventsStore.Close()

	return nil
}
