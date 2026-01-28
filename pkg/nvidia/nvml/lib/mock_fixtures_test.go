package lib

import (
	"os"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllSuccessInterface_Init tests that Init returns SUCCESS.
func TestAllSuccessInterface_Init(t *testing.T) {
	ret := allSuccessInterface.Init()
	assert.Equal(t, nvml.SUCCESS, ret)
}

// TestAllSuccessInterface_Shutdown tests that Shutdown returns SUCCESS.
func TestAllSuccessInterface_Shutdown(t *testing.T) {
	ret := allSuccessInterface.Shutdown()
	assert.Equal(t, nvml.SUCCESS, ret)
}

// TestAllSuccessInterface_SystemGetDriverVersion tests driver version retrieval.
func TestAllSuccessInterface_SystemGetDriverVersion(t *testing.T) {
	version, ret := allSuccessInterface.SystemGetDriverVersion()
	assert.Equal(t, nvml.SUCCESS, ret)
	assert.Equal(t, "535.161.08", version)
}

// TestAllSuccessInterface_SystemGetCudaDriverVersion tests CUDA version retrieval.
func TestAllSuccessInterface_SystemGetCudaDriverVersion(t *testing.T) {
	version, ret := allSuccessInterface.SystemGetCudaDriverVersion_v2()
	assert.Equal(t, nvml.SUCCESS, ret)
	assert.Equal(t, 12000, version) // CUDA 12.0
}

// TestAllSuccessInterface_DeviceGetCount tests device count retrieval.
func TestAllSuccessInterface_DeviceGetCount(t *testing.T) {
	count, ret := allSuccessInterface.DeviceGetCount()
	assert.Equal(t, nvml.SUCCESS, ret)
	assert.Equal(t, 1, count)
}

// TestAllSuccessInterface_DeviceGetHandleByIndex tests getting device handle.
func TestAllSuccessInterface_DeviceGetHandleByIndex(t *testing.T) {
	device, ret := allSuccessInterface.DeviceGetHandleByIndex(0)
	assert.Equal(t, nvml.SUCCESS, ret)
	require.NotNil(t, device)

	// Test device methods
	t.Run("GetName", func(t *testing.T) {
		name, r := device.GetName()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, "mock", name)
	})

	t.Run("GetUUID", func(t *testing.T) {
		uuid, r := device.GetUUID()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, "mock", uuid)
	})

	t.Run("GetMinorNumber", func(t *testing.T) {
		minor, r := device.GetMinorNumber()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, 1, minor)
	})

	t.Run("GetPciInfo", func(t *testing.T) {
		pci, r := device.GetPciInfo()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.PciInfo{}, pci)
	})

	t.Run("GetNumGpuCores", func(t *testing.T) {
		cores, r := device.GetNumGpuCores()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, 1, cores)
	})

	t.Run("GetSupportedEventTypes", func(t *testing.T) {
		types, r := device.GetSupportedEventTypes()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint64(1), types)
	})

	t.Run("GetArchitecture", func(t *testing.T) {
		arch, r := device.GetArchitecture()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.DeviceArchitecture(7), arch) // AMPERE
	})

	t.Run("GetCudaComputeCapability", func(t *testing.T) {
		major, minor, r := device.GetCudaComputeCapability()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, 8, major)
		assert.Equal(t, 0, minor)
	})

	t.Run("GetBrand", func(t *testing.T) {
		brand, r := device.GetBrand()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.BRAND_GEFORCE_RTX, brand)
	})

	t.Run("GpmQueryDeviceSupport", func(t *testing.T) {
		support, r := device.GpmQueryDeviceSupport()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.GpmSupport{}, support)
	})

	t.Run("GetGspFirmwareMode", func(t *testing.T) {
		enabled, defaultMode, r := device.GetGspFirmwareMode()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.False(t, enabled)
		assert.False(t, defaultMode)
	})

	t.Run("GetPersistenceMode", func(t *testing.T) {
		mode, r := device.GetPersistenceMode()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.EnableState(1), mode)
	})

	t.Run("GetCurrentClocksEventReasons", func(t *testing.T) {
		reasons, r := device.GetCurrentClocksEventReasons()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint64(1), reasons)
	})

	t.Run("GetClockInfo", func(t *testing.T) {
		clock, r := device.GetClockInfo(nvml.CLOCK_SM)
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint32(1), clock)
	})

	t.Run("GetMemoryInfo_v2", func(t *testing.T) {
		mem, r := device.GetMemoryInfo_v2()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.Memory_v2{}, mem)
	})

	t.Run("GetNvLinkState", func(t *testing.T) {
		state, r := device.GetNvLinkState(0)
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.EnableState(0), state)
	})

	t.Run("GetNvLinkErrorCounter", func(t *testing.T) {
		count, r := device.GetNvLinkErrorCounter(0, nvml.NVLINK_ERROR_DL_REPLAY)
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint64(0), count)
	})

	t.Run("GetFieldValues", func(t *testing.T) {
		fields := []nvml.FieldValue{}
		r := device.GetFieldValues(fields)
		assert.Equal(t, nvml.SUCCESS, r)
	})

	t.Run("GetPowerUsage", func(t *testing.T) {
		power, r := device.GetPowerUsage()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint32(1), power)
	})

	t.Run("GetEnforcedPowerLimit", func(t *testing.T) {
		limit, r := device.GetEnforcedPowerLimit()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint32(1), limit)
	})

	t.Run("GetPowerManagementLimit", func(t *testing.T) {
		limit, r := device.GetPowerManagementLimit()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint32(1), limit)
	})

	t.Run("GetTemperature", func(t *testing.T) {
		temp, r := device.GetTemperature(nvml.TEMPERATURE_GPU)
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint32(1), temp)
	})

	t.Run("GetTemperatureThreshold", func(t *testing.T) {
		threshold, r := device.GetTemperatureThreshold(nvml.TEMPERATURE_THRESHOLD_GPU_MAX)
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint32(1), threshold)
	})

	t.Run("GetUtilizationRates", func(t *testing.T) {
		util, r := device.GetUtilizationRates()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.Utilization{}, util)
	})

	t.Run("GetComputeRunningProcesses", func(t *testing.T) {
		procs, r := device.GetComputeRunningProcesses()
		assert.Equal(t, nvml.SUCCESS, r)
		require.Len(t, procs, 1)
		assert.Equal(t, uint32(999), procs[0].Pid)
	})

	t.Run("GetEccMode", func(t *testing.T) {
		current, pending, r := device.GetEccMode()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.EnableState(1), current)
		assert.Equal(t, nvml.EnableState(1), pending)
	})

	t.Run("GetTotalEccErrors", func(t *testing.T) {
		errors, r := device.GetTotalEccErrors(nvml.MEMORY_ERROR_TYPE_CORRECTED, nvml.VOLATILE_ECC)
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint64(1), errors)
	})

	t.Run("GetMemoryErrorCounter", func(t *testing.T) {
		count, r := device.GetMemoryErrorCounter(nvml.MEMORY_ERROR_TYPE_CORRECTED, nvml.VOLATILE_ECC, nvml.MEMORY_LOCATION_L1_CACHE)
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint64(0), count)
	})

	t.Run("GetProcessUtilization", func(t *testing.T) {
		samples, r := device.GetProcessUtilization(0)
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Empty(t, samples)
	})

	t.Run("GetRemappedRows", func(t *testing.T) {
		corr, uncorr, pending, failure, r := device.GetRemappedRows()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, 0, corr)
		assert.Equal(t, 0, uncorr)
		assert.False(t, pending)
		assert.False(t, failure)
	})

	t.Run("GetSerial", func(t *testing.T) {
		serial, r := device.GetSerial()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, "MOCK-GPU-SERIAL", serial)
	})

	t.Run("GetBoardId", func(t *testing.T) {
		id, r := device.GetBoardId()
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, uint32(123), id)
	})

	t.Run("RegisterEvents", func(t *testing.T) {
		eventSet, _ := allSuccessInterface.EventSetCreate()
		r := device.RegisterEvents(1, eventSet)
		assert.Equal(t, nvml.SUCCESS, r)
	})
}

// TestAllSuccessInterface_EventSetCreate tests event set creation.
func TestAllSuccessInterface_EventSetCreate(t *testing.T) {
	eventSet, ret := allSuccessInterface.EventSetCreate()
	assert.Equal(t, nvml.SUCCESS, ret)
	require.NotNil(t, eventSet)

	t.Run("Wait", func(t *testing.T) {
		data, r := eventSet.Wait(1000)
		assert.Equal(t, nvml.SUCCESS, r)
		assert.Equal(t, nvml.EventData{}, data)
	})

	t.Run("Free", func(t *testing.T) {
		r := eventSet.Free()
		assert.Equal(t, nvml.SUCCESS, r)
	})
}

// TestHasNvmlPropertyExtractor tests the property extractor mock.
func TestHasNvmlPropertyExtractor(t *testing.T) {
	t.Run("HasDXCore", func(t *testing.T) {
		has, reason := hasNvmlPropertyExtractor.HasDXCore()
		assert.False(t, has)
		assert.Empty(t, reason)
	})

	t.Run("HasNvml", func(t *testing.T) {
		has, reason := hasNvmlPropertyExtractor.HasNvml()
		assert.True(t, has)
		assert.Empty(t, reason)
	})

	t.Run("HasTegraFiles", func(t *testing.T) {
		has, reason := hasNvmlPropertyExtractor.HasTegraFiles()
		assert.False(t, has)
		assert.Empty(t, reason)
	})

	t.Run("UsesOnlyNVGPUModule", func(t *testing.T) {
		uses, reason := hasNvmlPropertyExtractor.UsesOnlyNVGPUModule()
		assert.False(t, uses)
		assert.Empty(t, reason)
	})
}

// TestNew_WithMockAllSuccessEnv tests the New function with GPUD_NVML_MOCK_ALL_SUCCESS.
func TestNew_WithMockAllSuccessEnv(t *testing.T) {
	// Set the environment variable
	require.NoError(t, os.Setenv(EnvMockAllSuccess, "true"))
	defer func() { _ = os.Unsetenv(EnvMockAllSuccess) }()

	lib, err := New()
	require.NoError(t, err)
	require.NotNil(t, lib)

	// Verify it uses the mock interface
	nvmlLib := lib.NVML()
	assert.NotNil(t, nvmlLib)

	// Test that the mock interface is being used by checking driver version
	version, ret := nvmlLib.SystemGetDriverVersion()
	assert.Equal(t, nvml.SUCCESS, ret)
	assert.Equal(t, "535.161.08", version)

	// Clean up
	_ = lib.Shutdown()
}

// TestNew_WithRemappedRowsInjection tests the New function with remapped rows injection.
func TestNew_WithRemappedRowsInjection(t *testing.T) {
	// Set both environment variables - mock must be enabled for this to work
	require.NoError(t, os.Setenv(EnvMockAllSuccess, "true"))
	require.NoError(t, os.Setenv(EnvInjectRemapedRowsPending, "true"))
	defer func() { _ = os.Unsetenv(EnvMockAllSuccess) }()
	defer func() { _ = os.Unsetenv(EnvInjectRemapedRowsPending) }()

	lib, err := New()
	require.NoError(t, err)
	require.NotNil(t, lib)

	// Clean up
	_ = lib.Shutdown()
}

// TestNew_WithClockEventsInjection tests the New function with clock events injection.
func TestNew_WithClockEventsInjection(t *testing.T) {
	// Set both environment variables - mock must be enabled for this to work
	require.NoError(t, os.Setenv(EnvMockAllSuccess, "true"))
	require.NoError(t, os.Setenv(EnvInjectClockEventsHwSlowdown, "true"))
	defer func() { _ = os.Unsetenv(EnvMockAllSuccess) }()
	defer func() { _ = os.Unsetenv(EnvInjectClockEventsHwSlowdown) }()

	lib, err := New()
	require.NoError(t, err)
	require.NotNil(t, lib)

	// Clean up
	_ = lib.Shutdown()
}

// TestNew_WithAllInjections tests the New function with all injections enabled.
func TestNew_WithAllInjections(t *testing.T) {
	require.NoError(t, os.Setenv(EnvMockAllSuccess, "true"))
	require.NoError(t, os.Setenv(EnvInjectRemapedRowsPending, "true"))
	require.NoError(t, os.Setenv(EnvInjectClockEventsHwSlowdown, "true"))
	defer func() { _ = os.Unsetenv(EnvMockAllSuccess) }()
	defer func() { _ = os.Unsetenv(EnvInjectRemapedRowsPending) }()
	defer func() { _ = os.Unsetenv(EnvInjectClockEventsHwSlowdown) }()

	lib, err := New()
	require.NoError(t, err)
	require.NotNil(t, lib)

	// Verify the mock interface is being used
	version, ret := lib.NVML().SystemGetDriverVersion()
	assert.Equal(t, nvml.SUCCESS, ret)
	assert.Equal(t, "535.161.08", version)

	// Clean up
	_ = lib.Shutdown()
}

// TestEnvConstants tests that the environment variable constants are defined.
func TestEnvConstants(t *testing.T) {
	assert.Equal(t, "GPUD_NVML_MOCK_ALL_SUCCESS", EnvMockAllSuccess)
	assert.Equal(t, "GPUD_NVML_INJECT_REMAPPED_ROWS_PENDING", EnvInjectRemapedRowsPending)
	assert.Equal(t, "GPUD_NVML_INJECT_CLOCK_EVENTS_HW_SLOWDOWN", EnvInjectClockEventsHwSlowdown)
}

// TestClockEventReasonConstants tests clock event reason constants.
func TestClockEventReasonConstants(t *testing.T) {
	// These are hex values for NVML clock event reasons
	// ref: https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html
	assert.Equal(t, uint64(0x0000000000000008), reasonHWSlowdown)
	assert.Equal(t, uint64(0x0000000000000020), reasonSwThermalSlowdown)
	assert.Equal(t, uint64(0x0000000000000040), reasonHWSlowdownThermal)
	assert.Equal(t, uint64(0x0000000000000080), reasonHWSlowdownPowerBrake)
}
