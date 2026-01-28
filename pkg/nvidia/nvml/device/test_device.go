package device

import (
	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
)

// testDevice wraps a Device with configurable failure injection for testing.
// It can simulate GPU failures (lost, requires reset) and fabric health issues.
type testDevice struct {
	Device

	// Error injection flags
	gpuLost          bool
	gpuRequiresReset bool

	// Fabric health injection
	fabricHealthUnhealthy bool
}

var _ Device = &testDevice{}

// Helper to determine which error to return
func (d *testDevice) getErrorReturn() nvml.Return {
	if d.gpuLost {
		return nvml.ERROR_GPU_IS_LOST
	}
	if d.gpuRequiresReset {
		return nvml.ERROR_RESET_REQUIRED
	}
	return nvml.SUCCESS
}

// GetFabricState handles both error injection and fabric health injection
func (d *testDevice) GetFabricState() (FabricState, error) {
	// Check for GPU errors first (these take precedence)
	if d.gpuLost {
		return FabricState{}, nvmlerrors.ErrGPULost
	}
	if d.gpuRequiresReset {
		return FabricState{}, nvmlerrors.ErrGPURequiresReset
	}

	// Get real fabric state from underlying device
	state, err := d.Device.GetFabricState()
	if err != nil {
		return state, err
	}

	// Inject unhealthy fabric state if configured
	if d.fabricHealthUnhealthy {
		state.HealthSummary = nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY
		// Set health mask to indicate degraded bandwidth
		state.HealthMask = uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE) << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW
	}

	return state, nil
}

// Override NVML methods to return configured error
// These are the most commonly used methods in the codebase
var _ device.Device = &testDevice{}

func (d *testDevice) GetUtilizationRates() (nvml.Utilization, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nvml.Utilization{}, err
	}
	return d.Device.GetUtilizationRates()
}

func (d *testDevice) GetPowerUsage() (uint32, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetPowerUsage()
}

func (d *testDevice) GetEnforcedPowerLimit() (uint32, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetEnforcedPowerLimit()
}

func (d *testDevice) GetPowerManagementLimit() (uint32, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetPowerManagementLimit()
}

func (d *testDevice) GetEccMode() (nvml.EnableState, nvml.EnableState, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nvml.FEATURE_DISABLED, nvml.FEATURE_DISABLED, err
	}
	return d.Device.GetEccMode()
}

func (d *testDevice) GetComputeRunningProcesses() ([]nvml.ProcessInfo, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nil, err
	}
	return d.Device.GetComputeRunningProcesses()
}

func (d *testDevice) GetProcessUtilization(lastSeenTimestamp uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nil, err
	}
	return d.Device.GetProcessUtilization(lastSeenTimestamp)
}

func (d *testDevice) GpmQueryDeviceSupport() (nvml.GpmSupport, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nvml.GpmSupport{}, err
	}
	return d.Device.GpmQueryDeviceSupport()
}

func (d *testDevice) GpmSampleGet(gpmSample nvml.GpmSample) nvml.Return {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return err
	}
	return d.Device.GpmSampleGet(gpmSample)
}

func (d *testDevice) GetCurrentClocksEventReasons() (uint64, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetCurrentClocksEventReasons()
}

func (d *testDevice) GetSupportedEventTypes() (uint64, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetSupportedEventTypes()
}

func (d *testDevice) GetMemoryInfo() (nvml.Memory, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nvml.Memory{}, err
	}
	return d.Device.GetMemoryInfo()
}

func (d *testDevice) GetMemoryErrorCounter(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, locationType nvml.MemoryLocation) (uint64, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetMemoryErrorCounter(errorType, counterType, locationType)
}

func (d *testDevice) GetTotalEccErrors(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetTotalEccErrors(errorType, counterType)
}

func (d *testDevice) GetRemappedRows() (int, int, bool, bool, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, 0, false, false, err
	}
	return d.Device.GetRemappedRows()
}

func (d *testDevice) GetRowRemapperHistogram() (nvml.RowRemapperHistogramValues, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nvml.RowRemapperHistogramValues{}, err
	}
	return d.Device.GetRowRemapperHistogram()
}

func (d *testDevice) GetTemperature(sensorType nvml.TemperatureSensors) (uint32, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetTemperature(sensorType)
}

func (d *testDevice) GetClock(clockType nvml.ClockType, clockId nvml.ClockId) (uint32, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetClock(clockType, clockId)
}

func (d *testDevice) GetMaxClockInfo(clockType nvml.ClockType) (uint32, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetMaxClockInfo(clockType)
}

func (d *testDevice) GetPersistenceMode() (nvml.EnableState, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nvml.FEATURE_DISABLED, err
	}
	return d.Device.GetPersistenceMode()
}

func (d *testDevice) GetGspFirmwareVersion() (string, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return "", err
	}
	return d.Device.GetGspFirmwareVersion()
}

func (d *testDevice) GetGspFirmwareMode() (bool, bool, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return false, false, err
	}
	return d.Device.GetGspFirmwareMode()
}

func (d *testDevice) GetNvLinkState(link int) (nvml.EnableState, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nvml.FEATURE_DISABLED, err
	}
	return d.Device.GetNvLinkState(link)
}

func (d *testDevice) GetNvLinkErrorCounter(link int, counter nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return 0, err
	}
	return d.Device.GetNvLinkErrorCounter(link, counter)
}

func (d *testDevice) GetFieldValues(values []nvml.FieldValue) nvml.Return {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return err
	}
	return d.Device.GetFieldValues(values)
}

func (d *testDevice) GetUUID() (string, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return "", err
	}
	return d.Device.GetUUID()
}

func (d *testDevice) GetName() (string, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return "", err
	}
	return d.Device.GetName()
}

func (d *testDevice) GetPciInfo() (nvml.PciInfo, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nvml.PciInfo{}, err
	}
	return d.Device.GetPciInfo()
}

func (d *testDevice) GetGpuFabricInfo() (nvml.GpuFabricInfo, nvml.Return) {
	if err := d.getErrorReturn(); err != nvml.SUCCESS {
		return nvml.GpuFabricInfo{}, err
	}
	// Note: V1 API doesn't have health fields to modify, so we just pass through
	return d.Device.GetGpuFabricInfo()
}

func (d *testDevice) GetGpuFabricInfoV() nvml.GpuFabricInfoHandler {
	// We can't intercept the handler's V3() calls due to concrete struct limitation
	// Error injection happens in GetGpuFabricInfo fallback
	return d.Device.GetGpuFabricInfoV()
}
