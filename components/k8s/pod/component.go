// Package pod tracks the current pods from the kubelet read-only port.
package pod

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
)

const Name = "k8s-pod"

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	GetDefaultPoller().Start(cctx, cfg.Query, Name)
	defaultPollerCloseOnce.Do(func() {
		close(defaultPollerc)
	})

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  GetDefaultPoller(),
		cfg:     cfg,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller

	cfg Config
}

func (c *component) Name() string { return Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err != nil {
		return nil, err
	}
	if last == nil { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", Name)
		return nil, nil
	}
	if last.Error != nil {
		return []components.State{
			{
				Name:    Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Name:    Name,
				Healthy: false,
				Reason:  "no output",
			},
		}, nil
	}

	output, ok := last.Output.(*Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T", last.Output)
	}
	return output.States(c.cfg)
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(Name)

	return nil
}
