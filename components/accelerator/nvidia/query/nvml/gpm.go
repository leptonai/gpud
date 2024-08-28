package nvml

import (
	"context"
	"errors"
	"fmt"
	"time"

	metrics_gpm "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/gpm"
	"github.com/leptonai/gpud/log"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Returns true if GPM is supported by all devices.
// Returns false if any device does not support GPM.
func GPMSupported() (bool, error) {
	nvmlLib := nvml.New()
	if ret := nvmlLib.Init(); ret != nvml.SUCCESS {
		return false, fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
	}
	log.Logger.Debugw("successfully initialized NVML")

	deviceLib := device.New(nvmlLib)
	devices, err := deviceLib.GetDevices()
	if err != nil {
		return false, err
	}

	for _, dev := range devices {
		supported, err := GPMSupportedByDevice(dev)
		if err != nil {
			return false, err
		}
		if !supported {
			return false, nil
		}
	}
	return true, nil
}

type GPMEvent struct {
	Metrics []GPMMetrics `json:"metrics"`
	Error   error        `json:"error"`
}

func (ev *GPMEvent) YAML() ([]byte, error) {
	return yaml.Marshal(ev)
}

func GPMSupportedByDevice(dev device.Device) (bool, error) {
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

	ticker := time.NewTicker(1)
	defer ticker.Stop()

	for {
		select {
		case <-inst.rootCtx.Done():
			return
		case <-ticker.C:
			ticker.Reset(inst.gpmPollInterval)
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
// Blocks for the duration of the sample interval.
func (inst *instance) collectGPMMetrics() ([]GPMMetrics, error) {
	if inst.gpmPollInterval == 0 {
		return nil, errors.New("gpm sample interval is not set")
	}
	if len(inst.gpmMetricsIDs) == 0 {
		return nil, errors.New("no metric IDs provided")
	}
	if len(inst.gpmMetricsIDs) > 98 {
		return nil, fmt.Errorf("too many metric IDs provided (%d > 98)", len(inst.gpmMetricsIDs))
	}
	for uuid, dev := range inst.devices {
		supported, err := GPMSupportedByDevice(dev.device)
		if err != nil {
			return nil, err
		}
		if !supported {
			return nil, fmt.Errorf("device %s is not supported by GPM", uuid)
		}
	}

	metrics := make([]GPMMetrics, 0, len(inst.devices))
	for _, dev := range inst.devices {
		ms, err := GetGPMMetrics(inst.rootCtx, dev.device, inst.gpmMetricsIDs...)
		if err != nil {
			return nil, fmt.Errorf("device %q failed to get gpm metrics: %w", dev.UUID, err)
		}
		metrics = append(metrics, GPMMetrics{
			UUID:           dev.UUID,
			SampleDuration: metav1.Duration{Duration: 5 * time.Second},
			Metrics:        ms,
		})
	}

	now := time.Now().UTC()
	metrics_gpm.SetLastUpdateUnixSeconds(float64(now.Unix()))

	for i, m := range metrics {
		metrics[i].Time = metav1.NewTime(now)

		gpuID := m.UUID
		for gpmMetricsID, v := range m.Metrics {
			switch gpmMetricsID {
			case nvml.GPM_METRIC_SM_OCCUPANCY:
				if err := metrics_gpm.SetGPUSMOccupancyPercent(inst.rootCtx, gpuID, v, now); err != nil {
					return nil, fmt.Errorf("failed to set gpm metric %v for gpu %s: %w", gpmMetricsID, gpuID, err)
				}
			default:
				log.Logger.Warnw("unsupported gpm metric id", "id", gpmMetricsID)
			}
		}
	}

	return metrics, nil
}

// Returns the map from the metrics ID to the value for this device.
// Don't call these in parallel for multiple devices.
// It "SIGSEGV: segmentation violation" in cgo execution.
// ref. https://github.com/NVIDIA/go-nvml/blob/main/examples/gpm-metrics/main.go
func GetGPMMetrics(ctx context.Context, dev device.Device, metricIDs ...nvml.GpmMetricId) (map[nvml.GpmMetricId]float64, error) {
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

	log.Logger.Debugw("waiting for sample interval")
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		log.Logger.Debugw("waited for sample interval")
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
