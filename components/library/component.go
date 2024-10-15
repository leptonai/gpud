// Package library provides a component that returns healthy if and only if all the specified libraries exist.
package library

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/file"
)

const Name = "library"

type Config struct {
	Libraries  []string
	SearchDirs []string
}

func New(cfg Config) components.Component {
	libraries := make(map[string]any)
	for _, lib := range cfg.Libraries {
		libraries[lib] = struct{}{}
	}

	searchDirs := make(map[string]any)
	for _, dir := range cfg.SearchDirs {
		searchDirs[dir] = struct{}{}
	}
	searchOpts := []file.OpOption{}
	for dir := range searchDirs {
		searchOpts = append(searchOpts, file.WithSearchDirs(dir))
	}

	return &component{
		libraries:  libraries,
		searchDirs: searchDirs,
		searchOpts: searchOpts,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	libraries  map[string]any
	searchDirs map[string]any
	searchOpts []file.OpOption
}

func (c *component) Name() string { return Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	reasons := []string{}
	for lib := range c.libraries {
		resolved, err := file.FindLibrary(lib, c.searchOpts...)
		if err != nil {
			return nil, err
		}
		if resolved == "" {
			reasons = append(reasons, fmt.Sprintf("library %q does not exist", lib))
		}
		log.Logger.Debugw("found library", "library", lib, "resolved", resolved)
	}
	if len(reasons) == 0 {
		return []components.State{
			{
				Name:    Name,
				Healthy: true,
				Reason:  "all libraries exist",
			},
		}, nil
	}

	return []components.State{
		{
			Name:    Name,
			Healthy: false,
			Reason:  strings.Join(reasons, "; "),
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
