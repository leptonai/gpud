// Package temperature tracks the NVIDIA per-GPU temperatures.
package temperature

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
)

const Name = "accelerator-nvidia-temperature"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getTimeNowFunc func() time.Time

	nvmlInstance       nvidianvml.Instance
	getTemperatureFunc func(uuid string, dev device.Device) (Temperature, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	if gpudInstance == nil {
		return nil, errors.New("gpud instance is nil")
	}
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:       gpudInstance.NVMLInstance,
		getTemperatureFunc: GetTemperature,
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
		ts: c.getTimeNowFunc(),
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
	// Check for NVML initialization errors first.
	// This handles cases like "error getting device handle for index 'N': Unknown Error"
	// which corresponds to nvidia-smi showing "Unable to determine the device handle for GPU".
	if err := c.nvmlInstance.InitError(); err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("NVML initialization error: %v", err)
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	marginThreshold := GetDefaultThresholdS()

	gpuTempThresholdExceeded := make([]string, 0)
	hbmTempThresholdExceeded := make([]string, 0)
	marginThresholdExceeded := make([]string, 0)

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		temp, err := c.getTemperatureFunc(uuid, dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting temperature"

			if errors.Is(err, nvmlerrors.ErrGPURequiresReset) {
				cr.reason = nvmlerrors.ErrGPURequiresReset.Error()
				cr.suggestedActions = &apiv1.SuggestedActions{
					Description: nvmlerrors.ErrGPURequiresReset.Error(),
					RepairActions: []apiv1.RepairActionType{
						apiv1.RepairActionTypeRebootSystem,
					},
				}
			}

			if errors.Is(err, nvmlerrors.ErrGPULost) {
				cr.reason = nvmlerrors.ErrGPULost.Error()
				cr.suggestedActions = &apiv1.SuggestedActions{
					Description: nvmlerrors.ErrGPULost.Error(),
					RepairActions: []apiv1.RepairActionType{
						apiv1.RepairActionTypeRebootSystem,
					},
				}
			}

			log.Logger.Warnw(cr.reason, "uuid", uuid, "error", cr.err)
			return cr
		}

		// nvmlDeviceGetMarginTemperature reports the thermal margin to the nearest slowdown
		// threshold as defined by NVML. NVML does not specify GPU core vs HBM; it is
		// whichever slowdown threshold is nearest (driver-defined).
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g42db93dc04fc99d253eadc2037a5232d
		if temp.ThresholdCelsiusSlowdown > 0 && temp.MarginTemperatureSupported {
			// margin left less than the threshold, indicating the GPU is approaching the slowdown threshold
			// e.g.,
			// 5°C margin left means the GPU is approaching the slowdown threshold at 82°C
			// when the slowdown threshold is 87°C
			if temp.ThresholdCelsiusSlowdownMargin <= marginThreshold.CelsiusSlowdownMargin {
				// e.g.,
				// 5°C margin left, when we set the threshold to 10°C
				// so that we can alert in advance before it's too late,
				// before the slowdown already happened
				marginThresholdExceeded = append(marginThresholdExceeded,
					fmt.Sprintf("%s has only %d °C margin left to slowdown (threshold %d °C)",
						uuid,
						temp.ThresholdCelsiusSlowdownMargin,
						marginThreshold.CelsiusSlowdownMargin,
					),
				)
			}
		}

		if temp.ThresholdCelsiusGPUMax > 0 && temp.CurrentCelsiusGPUCore > temp.ThresholdCelsiusGPUMax {
			gpuTempThresholdExceeded = append(gpuTempThresholdExceeded,
				fmt.Sprintf("%s current temperature is %d °C exceeding the threshold %d °C",
					uuid,
					temp.CurrentCelsiusGPUCore,
					temp.ThresholdCelsiusGPUMax,
				),
			)
		}

		// same logic as DCGM "VerifyHBMTemperature" that alerts "DCGM_FR_TEMP_VIOLATION",
		// use "DCGM_FI_DEV_MEM_MAX_OP_TEMP" to get the max HBM temperature threshold "NVML_TEMPERATURE_THRESHOLD_MEM_MAX"
		if temp.ThresholdCelsiusMemMax > 0 && temp.HBMTemperatureSupported && temp.CurrentCelsiusHBM > temp.ThresholdCelsiusMemMax {
			hbmTempThresholdExceeded = append(hbmTempThresholdExceeded,
				fmt.Sprintf("%s HBM temperature is %d °C exceeding the threshold %d °C",
					uuid,
					temp.CurrentCelsiusHBM,
					temp.ThresholdCelsiusMemMax,
				),
			)
		}

		cr.Temperatures = append(cr.Temperatures, temp)

		metricCurrentCelsius.With(prometheus.Labels{"uuid": uuid}).Set(float64(temp.CurrentCelsiusGPUCore))
		metricCurrentHBMCelsius.With(prometheus.Labels{"uuid": uuid}).Set(float64(temp.CurrentCelsiusHBM))
		metricThresholdSlowdownCelsius.With(prometheus.Labels{"uuid": uuid}).Set(float64(temp.ThresholdCelsiusSlowdown))
		metricThresholdMemMaxCelsius.With(prometheus.Labels{"uuid": uuid}).Set(float64(temp.ThresholdCelsiusMemMax))
		metricMarginCelsius.With(prometheus.Labels{"uuid": uuid}).Set(float64(temp.ThresholdCelsiusSlowdownMargin))

		slowdownPct, err := temp.GetUsedPercentSlowdown()
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting used percent for slowdown"
			log.Logger.Warnw(cr.reason, "uuid", uuid, "error", cr.err)
			return cr
		}
		metricSlowdownUsedPercent.With(prometheus.Labels{"uuid": uuid}).Set(slowdownPct)

		memMaxPct := 0.0
		if temp.ThresholdCelsiusMemMax > 0 && temp.HBMTemperatureSupported {
			memMaxPct = float64(temp.CurrentCelsiusHBM) / float64(temp.ThresholdCelsiusMemMax) * 100
		}
		metricMemMaxUsedPercent.With(prometheus.Labels{"uuid": uuid}).Set(memMaxPct)
	}

	if len(marginThresholdExceeded) > 0 {
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = fmt.Sprintf("margin threshold exceeded: %s", strings.Join(marginThresholdExceeded, ", "))
	} else if len(gpuTempThresholdExceeded) > 0 {
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = fmt.Sprintf("GPU temperature anomalies detected: %s", strings.Join(gpuTempThresholdExceeded, ", "))
	} else if len(hbmTempThresholdExceeded) > 0 {
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = fmt.Sprintf("HBM temperature anomalies detected: %s", strings.Join(hbmTempThresholdExceeded, ", "))
	} else {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("all %d GPU(s) were checked, no temperature issue found", len(devs))
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	Temperatures []Temperature `json:"temperatures,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the suggested actions for the last check
	suggestedActions *apiv1.SuggestedActions
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
	table.SetHeader([]string{"GPU UUID", "GPU Bus ID", "GPU temp", "HBM temp", "HBM temp threshold", "HBM used %", "Margin to slowdown"})
	for _, temp := range cr.Temperatures {
		hbmTemp := "n/a"
		if temp.HBMTemperatureSupported {
			hbmTemp = fmt.Sprintf("%d °C", temp.CurrentCelsiusHBM)
		}
		marginTemp := "n/a"
		if temp.MarginTemperatureSupported {
			marginTemp = fmt.Sprintf("%d °C", temp.ThresholdCelsiusSlowdownMargin)
		}
		table.Append([]string{
			temp.UUID,
			temp.BusID,
			fmt.Sprintf("%d °C", temp.CurrentCelsiusGPUCore),
			hbmTemp,
			fmt.Sprintf("%d °C", temp.ThresholdCelsiusMemMax),
			fmt.Sprintf("%s %%", temp.UsedPercentMemMax),
			marginTemp,
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

	// propagate suggested actions to health state if present
	if cr.suggestedActions != nil {
		state.SuggestedActions = cr.suggestedActions
	}

	if len(cr.Temperatures) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
