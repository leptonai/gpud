package customplugins

import (
	"context"
	"fmt"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

// NewInitFunc creates a new component initializer for the given plugin spec.
func (spec *Spec) NewInitFunc() components.InitFunc {
	if spec == nil {
		return nil
	}
	return func(gpudInstance *components.GPUdInstance) (components.Component, error) {
		cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
		c := &component{
			ctx:    cctx,
			cancel: ccancel,
			spec:   spec,
		}
		return c, nil
	}
}

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	spec *Spec

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

var _ CustomPluginRegisteree = &component{}

func (c *component) IsCustomPlugin() bool {
	return true
}

func (c *component) Spec() Spec {
	if c == nil || c.spec == nil {
		return Spec{}
	}
	return *c.spec
}

var _ components.Deregisterable = &component{}

func (c *component) CanDeregister() bool {
	return true
}

func (c *component) Name() string { return c.spec.ComponentName() }

func (c *component) Start() error {
	log.Logger.Infow("starting custom plugin", "type", c.spec.Type, "component", c.Name(), "plugin", c.spec.PluginName)

	itv := c.spec.Interval.Duration
	// either periodic check is disabled or interval is too short
	if itv < time.Second {
		_ = c.Check()
		return nil
	}

	go func() {
		ticker := time.NewTicker(itv)
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

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking custom plugin", "type", c.spec.Type, "component", c.Name(), "plugin", c.spec.PluginName)

	cr := &checkResult{
		componentName: c.Name(),
		pluginName:    c.spec.PluginName,
		ts:            time.Now().UTC(),
		extraInfo:     make(map[string]string),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.spec.HealthStatePlugin == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no state plugin defined"
		return cr
	}

	cctx, ccancel := context.WithTimeout(c.ctx, c.spec.Timeout.Duration)
	defer ccancel()

	cr.out, cr.exitCode, cr.err = c.spec.HealthStatePlugin.executeAllSteps(cctx)
	cr.output = string(cr.out)

	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error executing state plugin -- %s (output: %s)", cr.err, string(cr.out))
		return cr
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "ok"
	log.Logger.Debugw("successfully executed plugin", "exitCode", cr.exitCode, "output", string(cr.out))

	// either custom parser (jsonpath) or default parser
	if len(cr.out) > 0 && c.spec.HealthStatePlugin.Parser != nil {
		var matchResults map[string]extractedField
		matchResults, cr.err = c.spec.HealthStatePlugin.Parser.extractExtraInfo(cr.out)
		if cr.err != nil {
			log.Logger.Errorw("error extracting extra info", "error", cr.err)

			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "failed to parse plugin output"
			return cr
		}

		if len(matchResults) > 0 {
			for k, result := range matchResults {
				cr.extraInfo[k] = result.fieldValue

				if !result.matched {
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.reason = fmt.Sprintf("cannot find the matching value for %q", k)
				}
			}
		}
	}

	return cr
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	return lastCheckResult.getLastHealthStates(c.Name(), c.spec.PluginName)
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	out      []byte
	exitCode int32

	componentName string
	pluginName    string

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
	// extra info extracted from the output
	extraInfo map[string]string

	// output of the last check commands
	output string
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	return string(cr.out) + "\n\n" + fmt.Sprintf("(exit code %d)", cr.exitCode)
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

func (cr *checkResult) getLastHealthStates(componentName string, pluginName string) apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Component: componentName,
				Name:      pluginName,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Component: cr.componentName,
		Name:      cr.pluginName,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
		ExtraInfo: cr.extraInfo,
	}
	return apiv1.HealthStates{state}
}

var _ components.CheckResultDebugger = &checkResult{}

func (cr *checkResult) Debug() string {
	if cr == nil {
		return ""
	}
	return cr.output
}
