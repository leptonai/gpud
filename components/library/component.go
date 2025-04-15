// Package library provides a component that returns healthy if and only if all the specified libraries exist.
package library

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

// Name is the name of the library component.
const Name = "library"

var _ components.Component = &component{}

type component struct {
	libraries   map[string][]string
	searchDirs  map[string]any
	searchOpts  []file.OpOption
	findLibrary func(string, ...file.OpOption) (string, error)
}

type Config struct {
	Libraries  map[string][]string
	SearchDirs []string
}

func New(cfg Config) components.Component {
	searchDirs := make(map[string]any)
	for _, dir := range cfg.SearchDirs {
		searchDirs[dir] = struct{}{}
	}
	searchOpts := []file.OpOption{}
	for dir := range searchDirs {
		searchOpts = append(searchOpts, file.WithSearchDirs(dir))
	}

	return &component{
		libraries:   cfg.Libraries,
		searchDirs:  searchDirs,
		searchOpts:  searchOpts,
		findLibrary: file.FindLibrary,
	}
}

func (c *component) Name() string { return Name }

func (c *component) Start() error { return nil }

func (c *component) HealthStates(ctx context.Context) (apiv1.HealthStates, error) {
	reasons := []string{}
	for lib, alternatives := range c.libraries {
		opts := []file.OpOption{}
		opts = append(opts, c.searchOpts...)
		for _, alt := range alternatives {
			opts = append(opts, file.WithAlternativeLibraryName(alt))
		}
		resolved, err := c.findLibrary(lib, opts...)
		if resolved == "" && errors.Is(err, file.ErrLibraryNotFound) {
			reasons = append(reasons, fmt.Sprintf("library %q does not exist", lib))
			continue
		}
		if err != nil {
			return nil, err
		}
		log.Logger.Debugw("found library", "library", lib, "resolved", resolved)
	}

	if len(reasons) == 0 {
		return []apiv1.HealthState{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "all libraries exist",
			},
		}, nil
	}
	sort.Strings(reasons)

	return []apiv1.HealthState{
		{
			Name:   Name,
			Health: apiv1.StateTypeUnhealthy,
			Reason: strings.Join(reasons, "; "),
		},
	}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	return nil
}
