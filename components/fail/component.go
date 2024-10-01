// Package fail provides a component that always returns healthy: false for testing purposes
package fail

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/log"
)

const Name = "fail"

func New() components.Component {
	return &component{}
}

var _ components.Component = (*component)(nil)

type component struct{}

func (c *component) Name() string { return Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return []components.State{
		{
			Name:    Name,
			Healthy: false,
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
