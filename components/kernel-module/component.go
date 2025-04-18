// Package kernelmodule provides a component that checks the kernel modules in Linux.
package kernelmodule

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

// Name is the name of the kernel module component.
const Name = "kernel-module"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getAllModulesFunc func() ([]string, error)
	modulesToCheck    []string

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		getAllModulesFunc: getAllModules,
		modulesToCheck:    gpudInstance.KernelModulesToCheck,
	}
	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getLastHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking kernel modules")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	d.LoadedModules, d.err = c.getAllModulesFunc()
	if d.err != nil {
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting all modules: %v", d.err)
		return d
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
		d.health = apiv1.StateTypeHealthy
		d.reason = "all modules are loaded"
	} else {
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("missing modules: %q", missingModules)
	}
	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	LoadedModules []string `json:"loaded_modules"`
	loadedModules map[string]struct{}

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (d *Data) String() string {
	if d == nil {
		return ""
	}

	b, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Sprintf("error marshaling data: %v", err)
	}
	return string(b)
}

func (d *Data) Summary() string {
	if d == nil {
		return ""
	}
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	return d.health
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getLastHealthStates() apiv1.HealthStates {
	if d == nil {
		return apiv1.HealthStates{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),
		Health: d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
