package nvml

import (
	"context"
	"fmt"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/log"
)

func GPMSupportedByDevice(dev device.Device) (bool, error) {
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlGpmFunctions.html#group__nvmlGpmFunctions_1gdfd08d875be65f0532201913da9b8890
	gpuQuerySupport, ret := dev.GpmQueryDeviceSupport()

	// may fail due to "Argument version mismatch"
	// with driver mismatch
	if IsNotSupportError(ret) {
		return false, nil
	}
	// Version mismatch errors are considered as not supported errors
	// since they indicate that the NVML library is not compatible
	// with the corresponding API call.
	if IsVersionMismatchError(ret) {
		return false, nil
	}
	if IsGPULostError(ret) {
		return false, ErrGPULost
	}
	if ret != nvml.SUCCESS {
		return false, fmt.Errorf("could not query GPM support: %v", nvml.ErrorString(ret))
	}
	return gpuQuerySupport.IsSupportedDevice != 0, nil
}

// GPMMetrics contains the GPM metrics for a device.
type GPMMetrics struct {
	// Time is the time the metrics were collected.
	Time metav1.Time `json:"time"`

	// Device UUID that these GPM metrics belong to.
	UUID string `json:"uuid"`

	// The duration of the sample.
	SampleDuration metav1.Duration `json:"sample_duration"`

	// The metrics.
	Metrics map[nvml.GpmMetricId]float64 `json:"metrics"`
}

// Returns the map from the metrics ID to the value for this device.
// Don't call these in parallel for multiple devices.
// It "SIGSEGV: segmentation violation" in cgo execution.
// Returns nil if it's not supported.
// ref. https://github.com/NVIDIA/go-nvml/blob/main/examples/gpm-metrics/main.go
func GetGPMMetrics(ctx context.Context, dev device.Device, sampleDuration time.Duration, metricIDs ...nvml.GpmMetricId) (map[nvml.GpmMetricId]float64, error) {
	if len(metricIDs) == 0 {
		return nil, fmt.Errorf("no metric IDs provided")
	}
	if len(metricIDs) > 98 {
		return nil, fmt.Errorf("too many metric IDs provided (%d > 98)", len(metricIDs))
	}

	sample1, ret := nvml.GpmSampleAlloc()
	if IsNotSupportError(ret) {
		return nil, nil
	}
	// Version mismatch errors are considered as not supported errors
	// since they indicate that the NVML library is not compatible
	// with the corresponding API call.
	if IsVersionMismatchError(ret) {
		return nil, nil
	}
	if IsGPULostError(ret) {
		return nil, ErrGPULost
	}
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

	log.Logger.Debugw("waiting for sample interval")
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(sampleDuration):
		log.Logger.Debugw("waited for sample interval")
	}

	if ret := dev.GpmSampleGet(sample2); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("could not get sample: %v", nvml.ErrorString(ret))
	}

	gpmMetric := nvml.GpmMetricsGetType{
		NumMetrics: uint32(len(metricIDs)),
		Sample1:    sample1,
		Sample2:    sample2,
		Metrics:    [210]nvml.GpmMetric{},
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
