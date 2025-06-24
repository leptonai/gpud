package customplugins

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

// NewInitFunc creates a new component initializer for the given plugin spec.
func (spec *Spec) NewInitFunc() components.InitFunc {
	if spec == nil {
		return nil
	}
	return func(gpudInstance *components.GPUdInstance) (components.Component, error) {
		healthStateSetter, err := pkgmetrics.RegisterHealthStateMetrics(spec.ComponentName())
		if err != nil {
			return nil, err
		}

		cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
		c := &component{
			ctx:               cctx,
			cancel:            ccancel,
			spec:              spec,
			healthStateSetter: healthStateSetter,
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

	healthStateSetter pkgmetrics.HealthStateSetter
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

func (c *component) Tags() []string {
	tags := []string{
		"custom-plugin",
		c.spec.ComponentName(),
	}

	if len(c.spec.Tags) > 0 {
		tags = append(tags, c.spec.Tags...)
	}

	return tags
}

func (c *component) IsSupported() bool {
	return true
}

func (c *component) Start() error {
	log.Logger.Infow("starting custom plugin", "type", c.spec.PluginType, "component", c.Name(), "plugin", c.spec.PluginName)

	if c.spec.RunMode == string(apiv1.RunModeTypeManual) {
		log.Logger.Infow("custom plugin is in manual mode, skipping start", "type", c.spec.PluginType, "component", c.Name(), "plugin", c.spec.PluginName)
		return nil
	}

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
	log.Logger.Infow("checking custom plugin", "type", c.spec.PluginType, "runMode", c.spec.RunMode, "component", c.Name(), "plugin", c.spec.PluginName)

	cr := &checkResult{
		componentName: c.Name(),
		pluginName:    c.spec.PluginName,
		ts:            time.Now().UTC(),
		runMode:       apiv1.RunModeType(c.spec.RunMode),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		if c.healthStateSetter != nil {
			c.healthStateSetter.Set(cr.health)
		}
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

	// either custom parser (jsonpath) or default parser
	// parse before processing the error/command failures
	// since we still want to process the output even if the plugin failed
	if len(cr.out) > 0 && c.spec.HealthStatePlugin.Parser != nil {
		extraInfo, exErr := c.spec.HealthStatePlugin.Parser.extractExtraInfo(cr.out, c.spec.PluginName, c.spec.RunMode)
		if exErr != nil {
			log.Logger.Errorw("error extracting extra info", "error", exErr)

			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "failed to parse plugin output"
			cr.err = exErr
			return cr
		}

		if len(extraInfo) > 0 {
			cr.extraInfo = make(map[string]string)
			suggestedActions := make(map[string]string)
			for k, data := range extraInfo {
				cr.extraInfo[k] = data.fieldValue

				if !data.expectMatched {
					log.Logger.Warnw("rule cannot find the matching value (marking unhealthy)",
						"component", c.Name(),
						"field", k,
						"value", data.fieldValue,
						"rule", data.expectRule,
					)

					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.reason = "unexpected plugin output"
				}

				if len(data.suggestedActions) > 0 {
					for actionName, desc := range data.suggestedActions {
						if prev := suggestedActions[actionName]; prev != "" {
							desc = fmt.Sprintf("%s, %s", prev, desc)
						}
						suggestedActions[actionName] = desc
					}
				}
			}
			if len(suggestedActions) > 0 {
				descriptions := make([]string, 0)
				repairActions := make([]apiv1.RepairActionType, 0)
				for actionName, desc := range suggestedActions {
					descriptions = append(descriptions, desc)
					repairActions = append(repairActions, apiv1.RepairActionType(actionName))
				}
				cr.suggestedActions = &apiv1.SuggestedActions{
					Description:   strings.Join(descriptions, "\n"),
					RepairActions: repairActions,
				}
			}
		}
	}

	// we still parsed the output above
	// even when the command/script had failed
	// e.g., command failed with non-zero exit code
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error executing state plugin (exit code: %d)", cr.exitCode)
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}

	// no invalid output found or no error occurred, thus healthy
	if cr.reason == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "ok"
		log.Logger.Debugw("successfully executed plugin", "exitCode", cr.exitCode, "output", string(cr.out))
	}

	return cr
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	if lastCheckResult == nil {
		return apiv1.HealthStates{
			{
				Time:          metav1.NewTime(time.Now().UTC()),
				Component:     c.Name(),
				ComponentType: apiv1.ComponentTypeCustomPlugin,

				// in case component/plugin name is too long
				// component name and plugin name are always equivalent
				// thus no need to redundantly display two here as [component name]/[plugin name]
				Name: "check",

				RunMode: apiv1.RunModeType(c.spec.RunMode),
				Health:  apiv1.HealthStateTypeHealthy,
				Reason:  "no data yet",
			},
		}
	}
	return lastCheckResult.HealthStates()
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
	// output of the last check commands
	out      []byte
	exitCode int32

	componentName string
	pluginName    string

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// runMode is the run mode of the last check
	runMode apiv1.RunModeType
	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
	// extra info extracted from the output
	extraInfo map[string]string
	// suggested actions extracted from the output
	suggestedActions *apiv1.SuggestedActions
}

func (cr *checkResult) ComponentName() string {
	return cr.componentName
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	return string(cr.out) + "\n" + fmt.Sprintf("(exit code %d)", cr.exitCode)
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
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

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:          metav1.NewTime(time.Now().UTC()),
				ComponentType: apiv1.ComponentTypeCustomPlugin,
				Health:        apiv1.HealthStateTypeHealthy,
				Reason:        "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:          metav1.NewTime(cr.ts),
		Component:     cr.componentName,
		ComponentType: apiv1.ComponentTypeCustomPlugin,

		// in case component/plugin name is too long
		// component name and plugin name are always equivalent
		// thus no need to redundantly display two here as [component name]/[plugin name]
		Name: "check",

		Reason:           cr.reason,
		Error:            cr.getError(),
		RunMode:          cr.runMode,
		Health:           cr.health,
		ExtraInfo:        cr.extraInfo,
		SuggestedActions: cr.suggestedActions,
		RawOutput:        string(cr.out),
	}

	// maximum length of the raw output is 4096 bytes
	if len(state.RawOutput) > 4096 {
		state.RawOutput = state.RawOutput[:4096]
	}

	return apiv1.HealthStates{state}
}

var _ components.CheckResultDebugger = &checkResult{}

func (cr *checkResult) Debug() string {
	if cr == nil {
		return ""
	}
	return string(cr.out)
}
