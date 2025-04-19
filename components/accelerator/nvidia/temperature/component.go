// Package temperature tracks the NVIDIA per-GPU temperatures.
package temperature

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

const Name = "accelerator-nvidia-temperature"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance       nvidianvml.InstanceV2
	getTemperatureFunc func(uuid string, dev device.Device) (nvidianvml.Temperature, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:                cctx,
		cancel:             ccancel,
		nvmlInstance:       gpudInstance.NVMLInstance,
		getTemperatureFunc: nvidianvml.GetTemperature,
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
	log.Logger.Infow("checking nvidia gpu temperature")

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

	tempThresholdExceeded := make([]string, 0)
	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		temp, err := c.getTemperatureFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting temperature for device", "uuid", uuid, "error", err)

			d.err = err
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting temperature for device %s", uuid)
			return d
		}
		d.Temperatures = append(d.Temperatures, temp)

		// same logic as DCGM "VerifyHBMTemperature" that alerts  "DCGM_FR_TEMP_VIOLATION",
		// use "DCGM_FI_DEV_MEM_MAX_OP_TEMP" to get the max HBM temperature threshold "NVML_TEMPERATURE_THRESHOLD_MEM_MAX"
		if temp.ThresholdCelsiusMemMax > 0 && temp.CurrentCelsiusGPUCore > temp.ThresholdCelsiusMemMax {
			tempThresholdExceeded = append(tempThresholdExceeded,
				fmt.Sprintf("%s current temperature is %d °C exceeding the HBM temperature threshold %d °C",
					uuid,
					temp.CurrentCelsiusGPUCore,
					temp.ThresholdCelsiusMemMax,
				),
			)
		}

		metricCurrentCelsius.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(temp.CurrentCelsiusGPUCore))
		metricThresholdSlowdownCelsius.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(temp.ThresholdCelsiusSlowdown))

		slowdownPct, err := temp.GetUsedPercentSlowdown()
		if err != nil {
			log.Logger.Errorw("error getting used percent for slowdown for device", "uuid", uuid, "error", err)

			d.err = err
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting used percent for slowdown for device %s", uuid)
			return d
		}
		metricSlowdownUsedPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(slowdownPct)
	}

	if len(tempThresholdExceeded) == 0 {
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = fmt.Sprintf("all %d GPU(s) were checked, no temperature issue found", len(devs))
	} else {
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = fmt.Sprintf("exceeded HBM temperature thresholds: %s", strings.Join(tempThresholdExceeded, ", "))
	}

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	Temperatures []nvidianvml.Temperature `json:"temperatures,omitempty"`

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
	if len(d.Temperatures) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetHeader([]string{"GPU UUID", "Current temp", "HBM temp threshold", "Used %"})
	for _, temp := range d.Temperatures {
		table.Append([]string{
			temp.UUID,
			fmt.Sprintf("%d °C", temp.CurrentCelsiusGPUCore),
			fmt.Sprintf("%d °C", temp.ThresholdCelsiusMemMax),
			fmt.Sprintf("%s %%", temp.UsedPercentMemMax),
		})
	}
	table.Render()

	return buf.String()
}

func (d *Data) Summary() string {
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
