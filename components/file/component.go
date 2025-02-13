// Package file provides a component that returns healthy if and only if all the specified files exist.
package file

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/leptonai/gpud/components"
	file_id "github.com/leptonai/gpud/components/file/id"
	"github.com/leptonai/gpud/pkg/log"
)

func New(filesToCheck []string) components.Component {
	return &component{filesToCheck: filesToCheck}
}

var _ components.Component = (*component)(nil)

type component struct {
	filesToCheck []string
}

func (c *component) Name() string { return file_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	unhealthy := []components.State{}
	for _, file := range c.filesToCheck {
		_, err := os.Stat(file)
		if os.IsNotExist(err) {
			unhealthy = append(unhealthy, components.State{
				Name:    file_id.Name,
				Healthy: false,
				Reason:  fmt.Sprintf("file %q does not exist", file),
			})
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	if len(unhealthy) > 0 {
		return unhealthy, nil
	}

	return []components.State{
		{
			Name:    file_id.Name,
			Healthy: true,
			Reason:  "all files exist",
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
