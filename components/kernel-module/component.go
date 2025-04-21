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

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
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
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.getLastHealthStates()
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

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	cr.LoadedModules, cr.err = c.getAllModulesFunc()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error getting all modules: %v", cr.err)
		return cr
	}

	if len(cr.LoadedModules) > 0 {
		cr.loadedModules = make(map[string]struct{})
		for _, module := range cr.LoadedModules {
			cr.loadedModules[module] = struct{}{}
		}
	}

	missingModules := []string{}
	for _, module := range c.modulesToCheck {
		if _, ok := cr.loadedModules[module]; !ok {
			missingModules = append(missingModules, module)
		}
	}
	sort.Strings(missingModules)

	if len(missingModules) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "all modules are loaded"
	} else {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("missing modules: %q", missingModules)
	}
	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	b, err := yaml.Marshal(cr)
	if err != nil {
		return fmt.Sprintf("error marshaling data: %v", err)
	}
	return string(b)
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthState() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) getLastHealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
