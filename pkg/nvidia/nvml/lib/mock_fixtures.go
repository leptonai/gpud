package lib

import (
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

// allSuccessInterface provides a pre-configured mock NVML interface where all
// methods return success. Used for the GPUD_NVML_MOCK_ALL_SUCCESS environment
// variable to enable debugging without real NVIDIA hardware.
var allSuccessInterface = &nvmlmock.Interface{
	InitFunc: func() nvml.Return {
		return nvml.SUCCESS
	},

	SystemGetDriverVersionFunc: func() (string, nvml.Return) {
		return "535.161.08", nvml.SUCCESS
	},

	SystemGetCudaDriverVersion_v2Func: func() (int, nvml.Return) {
		return 12000, nvml.SUCCESS // CUDA 12.0 version
	},

	DeviceGetCountFunc: func() (int, nvml.Return) {
		return 1, nvml.SUCCESS
	},

	ShutdownFunc: func() nvml.Return {
		return nvml.SUCCESS
	},

	DeviceGetHandleByIndexFunc: func(n int) (nvml.Device, nvml.Return) {
		return &nvmlmock.Device{
			GetNameFunc: func() (string, nvml.Return) {
				return "mock", nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "mock", nvml.SUCCESS
			},
			GetMinorNumberFunc: func() (int, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetPciInfoFunc: func() (nvml.PciInfo, nvml.Return) {
				return nvml.PciInfo{}, nvml.SUCCESS
			},
			GetNumGpuCoresFunc: func() (int, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetSupportedEventTypesFunc: func() (uint64, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
				return 7, nvml.SUCCESS // NVML_DEVICE_ARCH_AMPERE (7)
			},
			GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
				return 8, 0, nvml.SUCCESS // CUDA 12.0 version
			},
			GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
				return nvml.BRAND_GEFORCE_RTX, nvml.SUCCESS // GeForce RTX
			},
			RegisterEventsFunc: func(v uint64, eventSet nvml.EventSet) nvml.Return {
				return nvml.SUCCESS
			},
			GpmQueryDeviceSupportFunc: func() (nvml.GpmSupport, nvml.Return) {
				return nvml.GpmSupport{}, nvml.SUCCESS
			},
			GetGspFirmwareModeFunc: func() (bool, bool, nvml.Return) {
				return false, false, nvml.SUCCESS
			},
			GetPersistenceModeFunc: func() (nvml.EnableState, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetMemoryInfo_v2Func: func() (nvml.Memory_v2, nvml.Return) {
				return nvml.Memory_v2{}, nvml.SUCCESS
			},
			GetNvLinkStateFunc: func(n int) (nvml.EnableState, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetNvLinkErrorCounterFunc: func(n int, nvLinkErrorCounter nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetFieldValuesFunc: func(fieldValues []nvml.FieldValue) nvml.Return {
				return nvml.SUCCESS
			},
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetTemperatureFunc: func(temperatureSensors nvml.TemperatureSensors) (uint32, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetTemperatureThresholdFunc: func(temperatureThresholds nvml.TemperatureThresholds) (uint32, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
				return nvml.Utilization{}, nvml.SUCCESS
			},
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{{Pid: 999}}, nvml.SUCCESS
			},
			GetEccModeFunc: func() (nvml.EnableState, nvml.EnableState, nvml.Return) {
				return 1, 1, nvml.SUCCESS
			},
			GetTotalEccErrorsFunc: func(memoryErrorType nvml.MemoryErrorType, eccCounterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(memoryErrorType nvml.MemoryErrorType, eccCounterType nvml.EccCounterType, memoryLocation nvml.MemoryLocation) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
			GetRemappedRowsFunc: func() (int, int, bool, bool, nvml.Return) {
				return 0, 0, false, false, nvml.SUCCESS
			},
			GetSerialFunc: func() (string, nvml.Return) {
				return "MOCK-GPU-SERIAL", nvml.SUCCESS
			},
			GetBoardIdFunc: func() (uint32, nvml.Return) {
				return 123, nvml.SUCCESS
			},
		}, nvml.SUCCESS
	},

	EventSetCreateFunc: func() (nvml.EventSet, nvml.Return) {
		return &nvmlmock.EventSet{
			WaitFunc: func(v uint32) (nvml.EventData, nvml.Return) {
				return nvml.EventData{}, nvml.SUCCESS
			},
			FreeFunc: func() nvml.Return {
				return nvml.SUCCESS
			},
		}, nvml.SUCCESS
	},
}

// hasNvmlPropertyExtractor provides a mock property extractor that reports
// NVML as available. Used with allSuccessInterface for the
// GPUD_NVML_MOCK_ALL_SUCCESS environment variable.
var hasNvmlPropertyExtractor = &nvinfo.PropertyExtractorMock{
	HasDXCoreFunc: func() (bool, string) {
		return false, ""
	},
	HasNvmlFunc: func() (bool, string) {
		return true, ""
	},
	HasTegraFilesFunc: func() (bool, string) {
		return false, ""
	},
	UsesOnlyNVGPUModuleFunc: func() (bool, string) {
		return false, ""
	},
}
