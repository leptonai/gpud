package nvml

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/log"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GPMEvent struct {
	Metrics []GPMMetrics
	Error   error
}

func GPMSupported(dev device.Device) (bool, error) {
	gpuQuerySupport, ret := dev.GpmQueryDeviceSupport()
	if ret != nvml.SUCCESS {
		return false, fmt.Errorf("could not query GPM support: %v", nvml.ErrorString(ret))
	}
	return gpuQuerySupport.IsSupportedDevice != 0, nil
}

func (inst *instance) GPMMetricsSupported() bool {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	return inst.gpmMetricsSupported
}

func (inst *instance) RecvGPMEvents() <-chan *GPMEvent {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	if inst.nvmlLib == nil {
		return nil
	}

	return inst.gpmEventCh
}

func (inst *instance) pollGPMEvents() {
	log.Logger.Debugw("polling gpm metrics events")
	for {
		select {
		case <-inst.rootCtx.Done():
			return
		default:
		}

		mss, err := inst.collectGPMMetrics()
		select {
		case <-inst.rootCtx.Done():
			return
		case inst.gpmEventCh <- &GPMEvent{
			Metrics: mss,
			Error:   err,
		}:
		default:
			log.Logger.Debugw("gpm event channel is full, skipping event")
		}
	}
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

// Collects the GPM metrics for all the devices and returns the map from the device UUID to the metrics.
func (inst *instance) collectGPMMetrics() ([]GPMMetrics, error) {
	if inst.gpmSampleInterval == 0 {
		return nil, errors.New("gpm sample interval is not set")
	}
	if len(inst.gpmMetricsIDs) == 0 {
		return nil, errors.New("no metric IDs provided")
	}
	if len(inst.gpmMetricsIDs) > 98 {
		return nil, fmt.Errorf("too many metric IDs provided (%d > 98)", len(inst.gpmMetricsIDs))
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
		m   GPMMetrics
		err error
	}
	rsc := make(chan result, len(inst.devices))
	for _, dev := range inst.devices {
		go func(dev *DeviceInfo) {
			ms, err := GetGPMMetrics(inst.rootCtx, dev.device, inst.gpmSampleInterval, inst.gpmMetricsIDs...)
			rsc <- result{
				m: GPMMetrics{
					UUID:           dev.UUID,
					SampleDuration: metav1.Duration{Duration: inst.gpmSampleInterval},
					Metrics:        ms,
				},
				err: err,
			}
		}(dev)
	}

	metrics := make([]GPMMetrics, 0, len(inst.devices))
	for i := 0; i < len(inst.devices); i++ {
		select {
		case <-inst.rootCtx.Done():
			return nil, inst.rootCtx.Err()
		case res := <-rsc:
			if res.err != nil {
				return nil, fmt.Errorf("device %q failed to get gpm metrics: %w", res.m.UUID, res.err)
			}
			metrics = append(metrics, res.m)
		}
	}

	now := time.Now().UTC()
	for i := range metrics {
		metrics[i].Time = metav1.NewTime(now)
	}
	return metrics, nil
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
