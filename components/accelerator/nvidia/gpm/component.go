// Package gpm tracks the NVIDIA per-GPU GPM metrics.
package gpm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	gonvml "github.com/NVIDIA/go-nvml/pkg/nvml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const (
	Name = "accelerator-nvidia-gpm"

	sampleDuration = 5 * time.Second
)

var (
	_ components.Component = &component{}

	defaultGPMMetricIDs = []gonvml.GpmMetricId{
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
)

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance        nvml.InstanceV2
	getGPMSupportedFunc func(dev device.Device) (bool, error)
	getGPMMetricsFunc   func(ctx context.Context, dev device.Device) (map[gonvml.GpmMetricId]float64, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, nvmlInstance nvml.InstanceV2) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:                 cctx,
		cancel:              ccancel,
		nvmlInstance:        nvmlInstance,
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
	log.Logger.Infow("checking clock speed")
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
		supported, err := c.getGPMSupportedFunc(dev)
		if err != nil {
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting GPM supported for device %s", uuid)
			return
		}

		if !supported {
			d.GPMSupported = false
			d.healthy = true
			d.reason = "GPM not supported"
			return
		}

		metrics, err := c.getGPMMetricsFunc(c.ctx, dev)
		if err != nil {
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting GPM metrics for device %s", uuid)
			return
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

	d.healthy = true
	d.reason = fmt.Sprintf("all %d GPU(s) were checked, no GPM issue found", len(devs))
}

type Data struct {
	GPMSupported bool                    `json:"gpm_supported,omitempty"`
	GPMMetrics   []nvidianvml.GPMMetrics `json:"gpm_metrics,omitempty"`

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
