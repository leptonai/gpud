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
	getAllModules  func() ([]string, error)
	modulesToCheck []string

	lastMu   sync.RWMutex
	lastData *Data
}

func New(modulesToCheck []string) components.Component {
	return &component{
		getAllModules:  getAllModules,
		modulesToCheck: modulesToCheck,
	}
}

func (c *component) Name() string { return Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates(c.modulesToCheck)
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	return nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
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

	d.LoadedModules, d.err = c.getAllModules()
	if d.err != nil {
		return
	}
	if len(d.LoadedModules) > 0 {
		d.loadedModules = make(map[string]struct{})
		for _, module := range d.LoadedModules {
			d.loadedModules[module] = struct{}{}
		}
	}
}

type Data struct {
	LoadedModules []string            `json:"loaded_modules"`
	loadedModules map[string]struct{} `json:"-"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
}

func (d *Data) getReason(modulesToCheck []string) string {
	if d == nil {
		return "no module data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to read modules -- %v", d.err)
	}
	if len(modulesToCheck) == 0 {
		return "no modules to check"
	}

	missingModules := []string{}
	for _, module := range modulesToCheck {
		if _, ok := d.loadedModules[module]; !ok {
			missingModules = append(missingModules, module)
		}
	}
	if len(missingModules) == 0 {
		return "all modules are loaded"
	}

	sort.Strings(missingModules)
	return fmt.Sprintf("missing modules: %q", missingModules)
}

func (d *Data) getHealth(modulesToCheck []string) (string, bool) {
	healthy := d == nil || d.err == nil || len(modulesToCheck) == 0
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}

	if len(modulesToCheck) > 0 {
		for _, module := range modulesToCheck {
			if _, ok := d.loadedModules[module]; !ok {
				healthy = false
				health = components.StateUnhealthy
				break
			}
		}
	}

	return health, healthy
}

func (d *Data) getStates(modulesToCheck []string) ([]components.State, error) {
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
		Reason: d.getReason(modulesToCheck),
	}
	state.Health, state.Healthy = d.getHealth(modulesToCheck)

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
