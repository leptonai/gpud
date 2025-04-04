// Package clockspeed tracks the NVIDIA per-GPU clock speed.
package clockspeed

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-clock-speed"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstanceV2    nvml.InstanceV2
	getClockSpeedFunc func(uuid string, dev device.Device) (nvidianvml.ClockSpeed, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, nvmlInstanceV2 nvml.InstanceV2) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:               cctx,
		cancel:            ccancel,
		nvmlInstanceV2:    nvmlInstanceV2,
		getClockSpeedFunc: nvidianvml.GetClockSpeed,
	}
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

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking clock speed")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	devs := c.nvmlInstanceV2.Devices()
	for uuid, dev := range devs {
		clockSpeed, err := c.getClockSpeedFunc(uuid, dev)
		if err != nil {
			d.err = err
			return
		}

		graphicsMHz.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(clockSpeed.GraphicsMHz))
		memoryMHz.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(clockSpeed.MemoryMHz))

		d.ClockSpeeds = append(d.ClockSpeeds, clockSpeed)
	}
}

type Data struct {
	ClockSpeeds []nvidianvml.ClockSpeed `json:"clock_speeds,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error
}

func (d *Data) getReason() string {
	if d == nil {
		return "no clock speed data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get clock speed data -- %s", d.err)
	}

	return fmt.Sprintf("found %d GPU(s) for clock speed data", len(d.ClockSpeeds))
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}

	return health, healthy
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates() ([]components.State, error) {
	state := components.State{
		Name:   Name,
		Reason: d.getReason(),
		Error:  d.getError(),
	}
	state.Health, state.Healthy = d.getHealth()

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
