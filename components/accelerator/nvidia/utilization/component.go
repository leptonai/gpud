// Package utilization tracks the NVIDIA per-GPU utilization.
package utilization

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/prometheus/client_golang/prometheus"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-utilization"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance       nvidianvml.InstanceV2
	getUtilizationFunc func(uuid string, dev device.Device) (nvidianvml.Utilization, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, nvmlInstance nvidianvml.InstanceV2) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstance:       nvmlInstance,
		getUtilizationFunc: nvidianvml.GetUtilization,
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

func (c *component) States(ctx context.Context) ([]apiv1.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]apiv1.Event, error) {
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
	log.Logger.Infow("checking utilization")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		util, err := c.getUtilizationFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting utilization for device", "uuid", uuid, "error", err)
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting utilization for device %s", uuid)
			return
		}
		d.Utilizations = append(d.Utilizations, util)

		metricGPUUtilPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(util.GPUUsedPercent))
		metricMemoryUtilPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(util.MemoryUsedPercent))
	}

	d.healthy = true
	d.reason = fmt.Sprintf("all %d GPU(s) were checked, no utilization issue found", len(devs))
}

type Data struct {
	Utilizations []nvidianvml.Utilization `json:"utilizations,omitempty"`

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

func (d *Data) getStates() ([]apiv1.State, error) {
	if d == nil {
		return []apiv1.State{
			{
				Name:    Name,
				Health:  apiv1.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  apiv1.StateHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}
