// Package kernelmodule provides a component that checks the kernel modules in Linux.
package kernelmodule

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	kernel_module_id "github.com/leptonai/gpud/components/kernel-module/id"
	"github.com/leptonai/gpud/pkg/log"
)

func New(modulesToCheck []string) components.Component {
	return &component{modulesToCheck: modulesToCheck}
}

var _ components.Component = &component{}

type component struct {
	modulesToCheck []string
}

func (c *component) Name() string { return kernel_module_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	b, err := os.ReadFile(DefaultEtcModulesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", DefaultEtcModulesPath, err)
	}
	modules, err := parseEtcModules(b)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %q: %w", DefaultEtcModulesPath, err)
	}
	if len(modules) == 0 && len(c.modulesToCheck) == 0 {
		return []components.State{
			{
				Name:    kernel_module_id.Name,
				Healthy: true,
				Reason:  "no module set in /etc/modules and no modules to check",
			},
		}, nil
	}

	modulesInJSON, err := json.Marshal(modules)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modules: %w", err)
	}
	state := components.State{
		Name:    kernel_module_id.Name,
		Healthy: true,
		Reason:  fmt.Sprintf("found %d modules in %q and %d module(s) to check", len(modules), DefaultEtcModulesPath, len(c.modulesToCheck)),
		ExtraInfo: map[string]string{
			"modules": string(modulesInJSON),
		},
	}

	modulesSet := map[string]struct{}{}
	for _, module := range modules {
		modulesSet[module] = struct{}{}
	}
	unhealthyReasons := []string{}
	for _, module := range c.modulesToCheck {
		if _, ok := modulesSet[module]; !ok {
			state.Healthy = false
			unhealthyReasons = append(unhealthyReasons, fmt.Sprintf("module %q not found in %q", module, DefaultEtcModulesPath))
		}
	}
	if len(unhealthyReasons) > 0 {
		state.Healthy = false
		state.Reason = strings.Join(unhealthyReasons, ", ")
	}

	return []components.State{state}, nil
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
