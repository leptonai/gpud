// Package os queries the host OS information (e.g., kernel version).
package os

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	os_id "github.com/leptonai/gpud/components/os/id"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/query"
)

const (
	DefaultRetentionPeriod = eventstore.DefaultRetention
)

func New(ctx context.Context, cfg Config, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(os_id.Name)
	if err != nil {
		return nil, err
	}

	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg, eventBucket)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, os_id.Name)

	return &component{
		rootCtx:      ctx,
		cancel:       ccancel,
		poller:       getDefaultPoller(),
		eventsBucket: eventBucket,
		cfg:          cfg,
	}, nil
}

var _ components.Component = &component{}

type component struct {
	rootCtx      context.Context
	cancel       context.CancelFunc
	poller       query.Poller
	eventsBucket eventstore.Bucket
	cfg          Config
}

func (c *component) Name() string { return os_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", os_id.Name)
		return []components.State{
			{
				Name:    os_id.Name,
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
				Name:    os_id.Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Name:    os_id.Name,
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

// Returns the event in the descending order of timestamp (latest event first).
func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventsBucket.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(os_id.Name)

	c.eventsBucket.Close()

	return nil
}

var _ components.OutputProvider = (*component)(nil)

func (c *component) Output() (any, error) {
	last, err := c.poller.Last()
	if err != nil {
		return nil, err
	}
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", os_id.Name)
		return nil, query.ErrNoData
	}
	if last.Error != nil {
		return nil, last.Error
	}
	if last.Output == nil {
		return nil, nil
	}

	output, ok := last.Output.(*Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T, expected *os.Output", last.Output)
	}
	return output, nil
}
