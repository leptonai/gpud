// Package tailscale tracks the current tailscale status.
package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

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

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:                      cctx,
		cancel:                   cancel,
		checkDependencyInstalled: checkTailscaledInstalled,
		checkServiceActiveFunc: func() (bool, error) {
			return systemd.IsActive("tailscaled")
		},
	}
	return c
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			c.CheckOnce()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) HealthStates(ctx context.Context) (apiv1.HealthStates, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking tailscale")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	// assume "tailscaled" is not installed, thus not needed to check its activeness
	if c.checkDependencyInstalled == nil || !c.checkDependencyInstalled() {
		d.healthy = true
		d.reason = "tailscaled is not installed"
		return
	}

	// below are the checks in case "tailscaled" is installed, thus requires activeness checks
	if c.checkServiceActiveFunc != nil {
		d.TailscaledServiceActive, d.err = c.checkServiceActiveFunc()
		if !d.TailscaledServiceActive || d.err != nil {
			d.healthy = false
			d.reason = fmt.Sprintf("tailscaled installed but tailscaled service is not active or failed to check (error %v)", d.err)
			return
		}
	}

	d.healthy = true
	d.reason = "tailscaled service is active/running"
}

type Data struct {
	// TailscaledServiceActive is true if the tailscaled service is active.
	TailscaledServiceActive bool `json:"tailscaled_service_active"`

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

func (d *Data) getHealthStates() (apiv1.HealthStates, error) {
	if d == nil {
		return []apiv1.HealthState{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}, nil
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Health: apiv1.StateTypeHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateTypeUnhealthy
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.HealthState{state}, nil
}
