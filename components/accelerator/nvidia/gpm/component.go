// Package gpm tracks the NVIDIA per-GPU GPM metrics.
package gpm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	gonvml "github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

const (
	Name = "accelerator-nvidia-gpm"

	sampleDuration = 5 * time.Second
)

var _ components.Component = &component{}

var defaultGPMMetricIDs = []gonvml.GpmMetricId{
	// By default, it tracks the SM occupancy metrics, with nvml.GPM_METRIC_SM_OCCUPANCY,
	// nvml.GPM_METRIC_INTEGER_UTIL, nvml.GPM_METRIC_ANY_TENSOR_UTIL,
	// nvml.GPM_METRIC_DFMA_TENSOR_UTIL, nvml.GPM_METRIC_HMMA_TENSOR_UTIL,
	// nvml.GPM_METRIC_IMMA_TENSOR_UTIL, nvml.GPM_METRIC_FP64_UTIL,
	// nvml.GPM_METRIC_FP32_UTIL, nvml.GPM_METRIC_FP16_UTIL,
	//
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10641-L10643
	// NVML_GPM_METRIC_SM_OCCUPANCY is the percentage of warps that were active vs theoretical maximum (0.0 - 100.0).
	// NVML_GPM_METRIC_INTEGER_UTIL is the percentage of time the GPU's SMs were doing integer operations (0.0 - 100.0).
	// NVML_GPM_METRIC_ANY_TENSOR_UTIL is the percentage of time the GPU's SMs were doing ANY tensor operations (0.0 - 100.0).
	//
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10644-L10646
	// NVML_GPM_METRIC_DFMA_TENSOR_UTIL is the percentage of time the GPU's SMs were doing DFMA tensor operations (0.0 - 100.0).
	// NVML_GPM_METRIC_HMMA_TENSOR_UTIL is the percentage of time the GPU's SMs were doing HMMA tensor operations (0.0 - 100.0).
	// NVML_GPM_METRIC_IMMA_TENSOR_UTIL is the percentage of time the GPU's SMs were doing IMMA tensor operations (0.0 - 100.0).
	//
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10648-L10650
	// NVML_GPM_METRIC_FP64_UTIL is the percentage of time the GPU's SMs were doing non-tensor FP64 math (0.0 - 100.0).
	// NVML_GPM_METRIC_FP32_UTIL is the percentage of time the GPU's SMs were doing non-tensor FP32 math (0.0 - 100.0).
	// NVML_GPM_METRIC_FP16_UTIL is the percentage of time the GPU's SMs were doing non-tensor FP16 math (0.0 - 100.0).
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlGpmStructs.html#group__nvmlGpmStructs_1g168f5f2704ec9871110d22aa1879aec0
	gonvml.GPM_METRIC_SM_OCCUPANCY,
	gonvml.GPM_METRIC_INTEGER_UTIL,
	gonvml.GPM_METRIC_ANY_TENSOR_UTIL,
	gonvml.GPM_METRIC_DFMA_TENSOR_UTIL,
	gonvml.GPM_METRIC_HMMA_TENSOR_UTIL,
	gonvml.GPM_METRIC_IMMA_TENSOR_UTIL,
	gonvml.GPM_METRIC_FP64_UTIL,
	gonvml.GPM_METRIC_FP32_UTIL,
	gonvml.GPM_METRIC_FP16_UTIL,
}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getTimeNowFunc func() time.Time

	nvmlInstance        nvidianvml.Instance
	getGPMSupportedFunc func(dev device.Device) (bool, error)
	getGPMMetricsFunc   func(ctx context.Context, dev device.Device) (map[gonvml.GpmMetricId]float64, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:        gpudInstance.NVMLInstance,
		getGPMSupportedFunc: GPMSupportedByDevice,
		getGPMMetricsFunc: func(ctx2 context.Context, dev device.Device) (map[gonvml.GpmMetricId]float64, error) {
			return GetGPMMetrics(
				ctx2,
				dev,
				sampleDuration,
				defaultGPMMetricIDs...,
			)
		},
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
	log.Logger.Infow("checking nvidia gpm metric")

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

	devs := c.nvmlInstance.Devices()

	// First, check if all GPUs support GPM
	for uuid, dev := range devs {
		supported, err := c.getGPMSupportedFunc(dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting GPM supported"

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

		if !supported {
			cr.GPMSupported = false
			cr.health = apiv1.HealthStateTypeHealthy
			cr.reason = "GPM not supported"
			return cr
		}
	}

	// All GPUs support GPM, now collect metrics
	for uuid, dev := range devs {
		metrics, err := c.getGPMMetricsFunc(c.ctx, dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting GPM metrics"

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

		now := metav1.Time{Time: time.Now().UTC()}
		cr.GPMMetrics = append(cr.GPMMetrics, GPMMetrics{
			Time:           now,
			UUID:           uuid,
			SampleDuration: metav1.Duration{Duration: sampleDuration},
			Metrics:        metrics,
		})

		for metricID, metricValue := range metrics {
			recordGPMMetricByID(metricID, uuid, metricValue)
		}
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("all %d GPU(s) were checked, no GPM issue found", len(devs))

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	GPMSupported bool         `json:"gpm_supported,omitempty"`
	GPMMetrics   []GPMMetrics `json:"gpm_metrics,omitempty"`

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
	if len(cr.GPMMetrics) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.SetHeader([]string{"GPU UUID", "Metric", "Value"})
	for _, metric := range cr.GPMMetrics {
		for metricID, metricValue := range metric.Metrics {
			table.Append([]string{metric.UUID, fmt.Sprintf("%v", metricID), fmt.Sprintf("%f", metricValue)})
		}
	}
	table.Render()

	return buf.String()
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
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

	if len(cr.GPMMetrics) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
