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

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
)

const Name = "accelerator-nvidia-temperature"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance       nvidianvml.Instance
	getTemperatureFunc func(uuid string, dev device.Device) (nvidianvml.Temperature, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
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

func (c *component) Tags() []string {
	return []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}
}

func (c *component) IsSupported() bool {
	if c.nvmlInstance == nil {
		return false
	}
	return c.nvmlInstance.NVMLExists() && c.nvmlInstance.ProductName() != ""
}

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

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu temperature")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	tempThresholdExceeded := make([]string, 0)
	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		temp, err := c.getTemperatureFunc(uuid, dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting temperature"
			log.Logger.Warnw(cr.reason, "uuid", uuid, "error", cr.err)
			return cr
		}
		cr.Temperatures = append(cr.Temperatures, temp)

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

		metricCurrentCelsius.With(prometheus.Labels{"uuid": uuid}).Set(float64(temp.CurrentCelsiusGPUCore))
		metricThresholdSlowdownCelsius.With(prometheus.Labels{"uuid": uuid}).Set(float64(temp.ThresholdCelsiusSlowdown))

		slowdownPct, err := temp.GetUsedPercentSlowdown()
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting used percent for slowdown"
			log.Logger.Warnw(cr.reason, "uuid", uuid, "error", cr.err)
			return cr
		}
		metricSlowdownUsedPercent.With(prometheus.Labels{"uuid": uuid}).Set(slowdownPct)
	}

	if len(tempThresholdExceeded) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("all %d GPU(s) were checked, no temperature issue found", len(devs))
	} else {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("exceeded HBM temperature thresholds: %s", strings.Join(tempThresholdExceeded, ", "))
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if len(cr.Temperatures) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetHeader([]string{"GPU UUID", "GPU Bus ID", "Current temp", "HBM temp threshold", "Used %"})
	for _, temp := range cr.Temperatures {
		table.Append([]string{
			temp.UUID,
			temp.BusID,
			fmt.Sprintf("%d °C", temp.CurrentCelsiusGPUCore),
			fmt.Sprintf("%d °C", temp.ThresholdCelsiusMemMax),
			fmt.Sprintf("%s %%", temp.UsedPercentMemMax),
		})
	}
	table.Render()

	return buf.String()
}

func (cr *checkResult) Summary() string {
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

	if len(cr.Temperatures) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
