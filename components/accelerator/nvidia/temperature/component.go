// Package temperature tracks the NVIDIA per-GPU temperatures.
package temperature

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/prometheus/client_golang/prometheus"

	components "github.com/leptonai/gpud/api/v1"
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

func New(ctx context.Context, nvmlInstance nvidianvml.InstanceV2) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstance:       nvmlInstance,
		getTemperatureFunc: nvidianvml.GetTemperature,
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
	log.Logger.Infow("checking temperature")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	memThresholdExceeded := make([]string, 0)
	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		temp, err := c.getTemperatureFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting temperature for device", "uuid", uuid, "error", err)
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting temperature for device %s", uuid)
			return
		}
		d.Temperatures = append(d.Temperatures, temp)

		// same logic as DCGM "VerifyHBMTemperature" that alerts  "DCGM_FR_TEMP_VIOLATION",
		// use "DCGM_FI_DEV_MEM_MAX_OP_TEMP" to get the max HBM temperature threshold "NVML_TEMPERATURE_THRESHOLD_MEM_MAX"
		if temp.ThresholdCelsiusMemMax > 0 && temp.CurrentCelsiusGPUCore > temp.ThresholdCelsiusMemMax {
			memThresholdExceeded = append(memThresholdExceeded,
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
			d.healthy = false
			d.reason = fmt.Sprintf("error getting used percent for slowdown for device %s", uuid)
			return
		}
		metricSlowdownUsedPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(slowdownPct)
	}

	if len(memThresholdExceeded) == 0 {
		d.healthy = true
		d.reason = fmt.Sprintf("all %d GPU(s) were checked, no temperature issue found", len(devs))
	} else {
		d.healthy = false
		d.reason = fmt.Sprintf("exceeded HBM temperature thresholds: %s", strings.Join(memThresholdExceeded, ", "))
	}
}

type Data struct {
	Temperatures []nvidianvml.Temperature `json:"temperatures,omitempty"`

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
