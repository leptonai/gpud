// Package tailscale tracks the current tailscale status.
package tailscale

import (
	"context"
	"fmt"
	"sync"
	"time"

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

	checkServiceActive func(context.Context) (bool, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:                      cctx,
		cancel:                   cancel,
		checkDependencyInstalled: checkTailscaledInstalled,
		checkServiceActive: func(ctx context.Context) (bool, error) {
			return systemd.CheckServiceActive(ctx, "tailscaled")
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

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
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

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking docker containers")
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
		return
	}

	// below are the checks in case "tailscaled" is installed, thus requires activeness checks
	if c.checkServiceActive != nil {
		var err error
		cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
		d.TailscaledServiceActive, err = c.checkServiceActive(cctx)
		ccancel()
		if !d.TailscaledServiceActive || err != nil {
			d.err = fmt.Errorf("tailscaled is installed but tailscaled service is not active or failed to check (error %v)", err)
			return
		}
	}
}

type Data struct {
	// TailscaledServiceActive is true if the tailscaled service is active.
	TailscaledServiceActive bool `json:"tailscaled_service_active"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
}

func (d *Data) getReason() string {
	if d == nil {
		return "no tailscaled check yet"
	}

	if d.err != nil {
		return fmt.Sprintf("tailscaled check failed -- %s", d.err)
	}

	if d.TailscaledServiceActive {
		return "tailscaled service is active/running"
	}

	return "tailscaled service is not active"
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
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
		Reason: d.getReason(),
	}
	state.Health, state.Healthy = d.getHealth()
	return []components.State{state}, nil
}
