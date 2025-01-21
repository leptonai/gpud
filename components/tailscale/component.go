// Package tailscale tracks the tailscale state (e.g., version) if available.
package tailscale

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	tailscale_id "github.com/leptonai/gpud/components/tailscale/id"
	"github.com/leptonai/gpud/log"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()

	return &component{
		rootCtx: ctx,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
}

func (c *component) Name() string { return tailscale_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return []components.State{
		{
			Name:    tailscale_id.Name,
			Healthy: true,
		},
	}, nil
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

	return nil
}
