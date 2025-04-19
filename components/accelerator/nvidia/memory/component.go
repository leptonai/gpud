// Package memory tracks the NVIDIA per-GPU memory usage.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-memory"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance  nvidianvml.InstanceV2
	getMemoryFunc func(uuid string, dev device.Device) (nvidianvml.Memory, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:           cctx,
		cancel:        ccancel,
		nvmlInstance:  gpudInstance.NVMLInstance,
		getMemoryFunc: nvidianvml.GetMemory,
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
	log.Logger.Infow("checking nvidia gpu memory")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = "NVIDIA NVML instance is nil"
		return d
	}
	if !c.nvmlInstance.NVMLExists() {
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = "NVIDIA NVML is not loaded"
		return d
	}

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		mem, err := c.getMemoryFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting memory for device", "uuid", uuid, "error", err)

			d.err = err
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting memory for device %s", uuid)
			return d
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
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting used percent for device %s", uuid)
			return d
		}
		metricUsedPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(usedPct)
	}

	d.health = apiv1.HealthStateTypeHealthy
	d.reason = fmt.Sprintf("all %d GPU(s) were checked, no memory issue found", len(devs))

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	Memories []nvidianvml.Memory `json:"memories,omitempty"`

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
	if len(d.Memories) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"GPU UUID", "Total", "Reserved", "Used", "Free", "Used %"})
	for _, mem := range d.Memories {
		table.Append([]string{
			mem.UUID,
			mem.TotalHumanized,
			mem.ReservedHumanized,
			mem.UsedHumanized,
			mem.FreeHumanized,
			mem.UsedPercent,
		})
	}
	table.Render()

	return buf.String()
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
				Health: apiv1.HealthStateTypeHealthy,
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
