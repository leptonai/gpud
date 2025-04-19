// Package nvlink monitors the NVIDIA per-GPU nvlink devices.
package nvlink

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

const Name = "accelerator-nvidia-nvlink"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance  nvidianvml.InstanceV2
	getNVLinkFunc func(uuid string, dev device.Device) (nvidianvml.NVLink, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:           cctx,
		cancel:        ccancel,
		nvmlInstance:  gpudInstance.NVMLInstance,
		getNVLinkFunc: nvidianvml.GetNVLink,
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
	log.Logger.Infow("checking nvidia gpu nvlink")

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
		nvLink, err := c.getNVLinkFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting nvlink for device", "uuid", uuid, "error", err)

			d.err = err
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting nvlink for device %s", uuid)
			return d
		}

		d.NVLinks = append(d.NVLinks, nvLink)

		if nvLink.States.AllFeatureEnabled() {
			metricFeatureEnabled.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(1.0))
		} else {
			metricFeatureEnabled.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(0.0))
		}
		metricReplayErrors.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(nvLink.States.TotalRelayErrors()))
		metricRecoveryErrors.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(nvLink.States.TotalRecoveryErrors()))
		metricCRCErrors.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(nvLink.States.TotalCRCErrors()))
	}

	d.health = apiv1.HealthStateTypeHealthy
	d.reason = fmt.Sprintf("all %d GPU(s) were checked, no nvlink issue found", len(devs))

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	NVLinks []nvidianvml.NVLink `json:"nvlinks,omitempty"`

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
	if len(d.NVLinks) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"UUID", "NVLink Enabled", "NVLink Supported"})
	for _, nvlink := range d.NVLinks {
		table.Append([]string{nvlink.UUID, fmt.Sprintf("%t", nvlink.States.AllFeatureEnabled()), fmt.Sprintf("%t", nvlink.Supported)})
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
