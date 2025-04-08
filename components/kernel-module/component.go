// Package kernelmodule provides a component that checks the kernel modules in Linux.
package kernelmodule

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

// Name is the name of the kernel module component.
const Name = "kernel-module"

var _ components.Component = &component{}

type component struct {
	getAllModulesFunc func() ([]string, error)
	modulesToCheck    []string

	lastMu   sync.RWMutex
	lastData *Data
}

func New(modulesToCheck []string) components.Component {
	return &component{
		getAllModulesFunc: getAllModules,
		modulesToCheck:    modulesToCheck,
	}
}

func (c *component) Name() string { return Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking info")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	d.LoadedModules, d.err = c.getAllModulesFunc()
	if d.err != nil {
		d.healthy = false
		d.reason = fmt.Sprintf("error getting all modules: %v", d.err)
		return
	}

	if len(d.LoadedModules) > 0 {
		d.loadedModules = make(map[string]struct{})
		for _, module := range d.LoadedModules {
			d.loadedModules[module] = struct{}{}
		}
	}

	missingModules := []string{}
	for _, module := range c.modulesToCheck {
		if _, ok := d.loadedModules[module]; !ok {
			missingModules = append(missingModules, module)
		}
	}
	sort.Strings(missingModules)

	if len(missingModules) == 0 {
		d.healthy = true
		d.reason = "all modules are loaded"
	} else {
		d.healthy = false
		d.reason = fmt.Sprintf("missing modules: %q", missingModules)
	}
}

type Data struct {
	LoadedModules []string            `json:"loaded_modules"`
	loadedModules map[string]struct{} `json:"-"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	healthy bool
	// tracks the reason of the last check
	reason string
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates() ([]components.State, error) {
	if d == nil {
		return []components.State{
			{
				Name:    Name,
				Health:  components.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := components.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  components.StateHealthy,
	}
	if !d.healthy {
		state.Health = components.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
