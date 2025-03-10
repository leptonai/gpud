// Package pci tracks the PCI devices and their Access Control Services (ACS) status.
package pci

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/pci/id"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/query"
)

var _ components.Component = &component{}

type component struct {
	cfg         Config
	rootCtx     context.Context
	cancel      context.CancelFunc
	poller      query.Poller
	eventBucket eventstore.Bucket
}

func New(ctx context.Context, cfg Config, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(id.Name)
	if err != nil {
		return nil, err
	}

	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg, eventBucket)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, id.Name)

	return &component{
		cfg:         cfg,
		rootCtx:     ctx,
		cancel:      ccancel,
		poller:      getDefaultPoller(),
		eventBucket: eventBucket,
	}, nil
}

func (c *component) Name() string { return id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return nil, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(id.Name)

	c.eventBucket.Close()

	return nil
}
