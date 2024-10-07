// Package library provides a component that returns healthy if and only if all the specified libraries exist.
package library

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/file"
)

const Name = "library"

func New(libs []string) components.Component {
	libraries := make(map[string]any)
	for _, lib := range libs {
		libraries[lib] = struct{}{}
	}
	return &component{libraries: libraries}
}

var _ components.Component = (*component)(nil)

type component struct {
	libraries map[string]any
}

func (c *component) Name() string { return Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	unhealthy := []components.State{}
	for lib := range c.libraries {
		resolved, err := file.FindLibrary(lib)
		if err != nil {
			return nil, err
		}

		if resolved == "" {
			unhealthy = append(unhealthy, components.State{
				Name: Name,

				// TODO: mark it as unhealthy once stable
				Healthy: true,

				Reason: fmt.Sprintf("library %q does not exist", lib),
			})
			continue
		}

		log.Logger.Debugw("found library", "library", lib, "resolved", resolved)
	}
	if len(unhealthy) > 0 {
		return unhealthy, nil
	}

	return []components.State{
		{
			Name:    Name,
			Healthy: true,
			Reason:  "all libraries exist",
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
