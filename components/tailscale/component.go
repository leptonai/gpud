// Package tailscale tracks the current tailscale status.
package tailscale

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/systemd"
)

// Name is the ID of the tailscale component.
const Name = "tailscale"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkDependencyInstalled func() bool
	checkServiceActiveFunc   func() (bool, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkDependencyInstalled: checkTailscaledInstalled,
		checkServiceActiveFunc: func() (bool, error) {
			return systemd.IsActive("tailscaled")
		},
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

func (c *component) Tags() []string {
	return []string{}
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking tailscale")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// assume "tailscaled" is not installed, thus not needed to check its activeness
	if c.checkDependencyInstalled == nil || !c.checkDependencyInstalled() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "tailscaled is not installed"
		return cr
	}

	// below are the checks in case "tailscaled" is installed, thus requires activeness checks
	if c.checkServiceActiveFunc != nil {
		cr.TailscaledServiceActive, cr.err = c.checkServiceActiveFunc()
		if !cr.TailscaledServiceActive || cr.err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "tailscaled installed but tailscaled service is not active or failed to check"
			log.Logger.Errorw(cr.reason, "error", cr.err)
			return cr
		}
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "tailscaled service is active/running"

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// TailscaledServiceActive is true if the tailscaled service is active.
	TailscaledServiceActive bool `json:"tailscaled_service_active"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	return cr.reason
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
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{"data": string(b)}
	return apiv1.HealthStates{state}
}
