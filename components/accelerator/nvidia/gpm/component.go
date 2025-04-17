// Package gpm tracks the NVIDIA per-GPU GPM metrics.
package gpm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	gonvml "github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
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

	nvmlInstance        nvml.InstanceV2
	getGPMSupportedFunc func(dev device.Device) (bool, error)
	getGPMMetricsFunc   func(ctx context.Context, dev device.Device) (map[gonvml.GpmMetricId]float64, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		nvmlInstance:        gpudInstance.NVMLInstance,
		getGPMSupportedFunc: nvidianvml.GPMSupportedByDevice,
		getGPMMetricsFunc: func(ctx2 context.Context, dev device.Device) (map[gonvml.GpmMetricId]float64, error) {
			return nvidianvml.GetGPMMetrics(
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
	log.Logger.Infow("checking nvidia gpm metric")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil || !c.nvmlInstance.NVMLExists() {
		d.reason = "NVIDIA NVML is not loaded"
		d.health = apiv1.StateTypeHealthy
		return d
	}

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		supported, err := c.getGPMSupportedFunc(dev)
		if err != nil {
			d.err = err
			d.health = apiv1.StateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting GPM supported for device %s", uuid)
			return d
		}

		if !supported {
			d.GPMSupported = false
			d.health = apiv1.StateTypeHealthy
			d.reason = "GPM not supported"
			return d
		}

		metrics, err := c.getGPMMetricsFunc(c.ctx, dev)
		if err != nil {
			d.err = err
			d.health = apiv1.StateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting GPM metrics for device %s", uuid)
			return d
		}

		now := metav1.Time{Time: time.Now().UTC()}
		d.GPMMetrics = append(d.GPMMetrics, nvidianvml.GPMMetrics{
			Time:           now,
			UUID:           uuid,
			SampleDuration: metav1.Duration{Duration: sampleDuration},
			Metrics:        metrics,
		})

		for metricID, metricValue := range metrics {
			recordGPMMetricByID(metricID, uuid, metricValue)
		}
	}

	d.health = apiv1.StateTypeHealthy
	d.reason = fmt.Sprintf("all %d GPU(s) were checked, no GPM issue found", len(devs))

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	GPMSupported bool                    `json:"gpm_supported,omitempty"`
	GPMMetrics   []nvidianvml.GPMMetrics `json:"gpm_metrics,omitempty"`

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
	if len(d.GPMMetrics) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.SetHeader([]string{"GPU UUID", "Metric", "Value"})
	for _, metric := range d.GPMMetrics {
		for metricID, metricValue := range metric.Metrics {
			table.Append([]string{metric.UUID, fmt.Sprintf("%v", metricID), fmt.Sprintf("%f", metricValue)})
		}
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
				Health: apiv1.StateTypeHealthy,
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
