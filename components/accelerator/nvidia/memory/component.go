// Package memory tracks the NVIDIA per-GPU memory usage.
package memory

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

const Name = "accelerator-nvidia-memory"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance  nvml.InstanceV2
	getMemoryFunc func(uuid string, dev device.Device) (nvidianvml.Memory, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, nvmlInstance nvml.InstanceV2) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstance:  nvmlInstance,
		getMemoryFunc: nvidianvml.GetMemory,
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
	log.Logger.Infow("checking memory")
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
		mem, err := c.getMemoryFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting memory for device", "uuid", uuid, "error", err)
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting memory for device %s", uuid)
			return
		}
		d.Memories = append(d.Memories, mem)

		metricTotalBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(mem.TotalBytes))
		metricReservedBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(mem.ReservedBytes))
		metricUsedBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(mem.UsedBytes))
		metricFreeBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(mem.FreeBytes))

		usedPct, err := mem.GetUsedPercent()
		if err != nil {
			log.Logger.Errorw("error getting used percent for device", "uuid", uuid, "error", err)
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting used percent for device %s", uuid)
			return
		}
		metricUsedPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(usedPct)
	}

	d.healthy = true
	d.reason = fmt.Sprintf("all %d GPU(s) were checked, no memory issue found", len(devs))
}

type Data struct {
	Memories []nvidianvml.Memory `json:"memories,omitempty"`

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
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  components.StateHealthy,
	}
	if !d.healthy {
		state.Health = components.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
