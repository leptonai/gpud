package nvml

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/log"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Collects the GPM metrics for all the devices and returns the map from the device UUID to the metrics.
func (inst *instance) CollectGPMMetrics(ctx context.Context, sampleDuration time.Duration, metricIDs ...nvml.GpmMetricId) (map[string]map[nvml.GpmMetricId]float64, error) {
	if len(metricIDs) == 0 {
		return nil, fmt.Errorf("no metric IDs provided")
	}
	if len(metricIDs) > 98 {
		return nil, fmt.Errorf("too many metric IDs provided (%d > 98)", len(metricIDs))
	}
	for uuid, dev := range inst.devices {
		supported, err := GPMSupported(dev.device)
		if err != nil {
			return nil, err
		}
		if !supported {
			return nil, fmt.Errorf("device %s is not supported by GPM", uuid)
		}
	}

	type result struct {
		uuid    string
		metrics map[nvml.GpmMetricId]float64
		err     error
	}
	rsc := make(chan result, len(inst.devices))

	for _, dev := range inst.devices {
		go func(dev *DeviceInfo) {
			ms, err := GetGPMMetrics(ctx, dev.device, sampleDuration, metricIDs...)
			rsc <- result{
				uuid:    dev.UUID,
				metrics: ms,
				err:     err,
			}
		}(dev)
	}

	metrics := make(map[string]map[nvml.GpmMetricId]float64, len(inst.devices))
	for i := 0; i < len(inst.devices); i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-rsc:
			if res.err != nil {
				return nil, fmt.Errorf("device %q failed to get gpm metrics: %w", res.uuid, res.err)
			}
			metrics[res.uuid] = res.metrics
		}
	}
	return metrics, nil
}

func GPMSupported(dev device.Device) (bool, error) {
	gpuQuerySupport, ret := dev.GpmQueryDeviceSupport()
	if ret != nvml.SUCCESS {
		return false, fmt.Errorf("could not query GPM support: %v", nvml.ErrorString(ret))
	}
	return gpuQuerySupport.IsSupportedDevice != 0, nil
}

// Returns the map from the metrics ID to the value for this device.
// It blocks for the sample duration and returns the metrics.
func GetGPMMetrics(ctx context.Context, dev device.Device, sampleDuration time.Duration, metricIDs ...nvml.GpmMetricId) (map[nvml.GpmMetricId]float64, error) {
	if len(metricIDs) == 0 {
		return nil, fmt.Errorf("no metric IDs provided")
	}
	if len(metricIDs) > 98 {
		return nil, fmt.Errorf("too many metric IDs provided (%d > 98)", len(metricIDs))
	}

	sample1, ret := nvml.GpmSampleAlloc()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("could not allocate sample: %v", nvml.ErrorString(ret))
	}
	defer func() {
		_ = sample1.Free()
	}()

	sample2, ret := nvml.GpmSampleAlloc()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("could not allocate sample: %v", nvml.ErrorString(ret))
	}
	defer func() {
		_ = sample2.Free()
	}()

	if ret := dev.GpmSampleGet(sample1); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("could not get sample: %v", nvml.ErrorString(ret))
	}

	log.Logger.Debugw("waiting for sample duration", "duration", sampleDuration)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(sampleDuration):
		log.Logger.Debugw("waited for sample duration", "duration", sampleDuration)
	}

	if ret := dev.GpmSampleGet(sample2); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("could not get sample: %v", nvml.ErrorString(ret))
	}

	gpmMetric := nvml.GpmMetricsGetType{
		NumMetrics: uint32(len(metricIDs)),
		Sample1:    sample1,
		Sample2:    sample2,
		Metrics:    [98]nvml.GpmMetric{},
	}
	for i := range metricIDs {
		gpmMetric.Metrics[i].MetricId = uint32(metricIDs[i])
	}
	if ret = nvml.GpmMetricsGet(&gpmMetric); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get gpm metric: %v", nvml.ErrorString(ret))
	}
	if len(gpmMetric.Metrics) == len(metricIDs) {
		return nil, fmt.Errorf("expected %d metrics, got %d", len(metricIDs), len(gpmMetric.Metrics))
	}

	metrics := make(map[nvml.GpmMetricId]float64, len(metricIDs))
	for i := range metricIDs {
		metrics[metricIDs[i]] = gpmMetric.Metrics[i].Value
	}
	return metrics, nil
}
