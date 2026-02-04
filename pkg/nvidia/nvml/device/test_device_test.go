//go:build linux

package device

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
)

// mockNvDevice wraps a mock.Device to satisfy the Device interface for testing testDevice.
type mockNvDevice struct {
	*stubDevice
	busID       string
	uuid        string
	fabricState FabricState
	fabricErr   error
}

func (d *mockNvDevice) PCIBusID() string { return d.busID }
func (d *mockNvDevice) UUID() string     { return d.uuid }
func (d *mockNvDevice) GetFabricState() (FabricState, error) {
	return d.fabricState, d.fabricErr
}

// newMockNvDevice creates a mock nvDevice for testing.
func newMockNvDevice() *mockNvDevice {
	return &mockNvDevice{
		stubDevice: &stubDevice{Device: &mock.Device{}},
		busID:      "0000:00:00.0",
		uuid:       "GPU-12345678-1234-1234-1234-123456789012",
		fabricState: FabricState{
			CliqueID:      1,
			ClusterUUID:   "cluster-uuid",
			State:         nvml.GPU_FABRIC_STATE_COMPLETED,
			Status:        nvml.SUCCESS,
			HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
		},
	}
}

// TestTestDevice_GetErrorReturn tests the getErrorReturn helper method.
func TestTestDevice_GetErrorReturn(t *testing.T) {
	tests := []struct {
		name             string
		gpuLost          bool
		gpuRequiresReset bool
		expected         nvml.Return
	}{
		{
			name:             "no error injection",
			gpuLost:          false,
			gpuRequiresReset: false,
			expected:         nvml.SUCCESS,
		},
		{
			name:             "GPU lost takes precedence",
			gpuLost:          true,
			gpuRequiresReset: true,
			expected:         nvml.ERROR_GPU_IS_LOST,
		},
		{
			name:             "GPU lost only",
			gpuLost:          true,
			gpuRequiresReset: false,
			expected:         nvml.ERROR_GPU_IS_LOST,
		},
		{
			name:             "GPU requires reset only",
			gpuLost:          false,
			gpuRequiresReset: true,
			expected:         nvml.ERROR_RESET_REQUIRED,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			td := &testDevice{
				Device:           newMockNvDevice(),
				gpuLost:          tc.gpuLost,
				gpuRequiresReset: tc.gpuRequiresReset,
			}

			result := td.getErrorReturn()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestTestDevice_GetFabricState tests the GetFabricState method with error injection.
func TestTestDevice_GetFabricState(t *testing.T) {
	t.Run("GPU lost error", func(t *testing.T) {
		td := &testDevice{
			Device:  newMockNvDevice(),
			gpuLost: true,
		}

		state, err := td.GetFabricState()
		assert.Error(t, err)
		assert.Equal(t, nvmlerrors.ErrGPULost, err)
		assert.Equal(t, FabricState{}, state)
	})

	t.Run("GPU requires reset error", func(t *testing.T) {
		td := &testDevice{
			Device:           newMockNvDevice(),
			gpuRequiresReset: true,
		}

		state, err := td.GetFabricState()
		assert.Error(t, err)
		assert.Equal(t, nvmlerrors.ErrGPURequiresReset, err)
		assert.Equal(t, FabricState{}, state)
	})

	t.Run("fabric health unhealthy injection", func(t *testing.T) {
		td := &testDevice{
			Device:                newMockNvDevice(),
			fabricHealthUnhealthy: true,
		}

		state, err := td.GetFabricState()
		require.NoError(t, err)
		assert.Equal(t, uint8(nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY), state.HealthSummary)
		// Health mask should indicate degraded bandwidth
		expectedMask := uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE) << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW
		assert.Equal(t, expectedMask, state.HealthMask)
	})

	t.Run("healthy state passthrough", func(t *testing.T) {
		td := &testDevice{
			Device: newMockNvDevice(),
		}

		state, err := td.GetFabricState()
		require.NoError(t, err)
		assert.Equal(t, uint8(nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY), state.HealthSummary)
		assert.Equal(t, uint32(1), state.CliqueID)
		assert.Equal(t, "cluster-uuid", state.ClusterUUID)
	})

	t.Run("GPU lost takes precedence over fabric unhealthy", func(t *testing.T) {
		td := &testDevice{
			Device:                newMockNvDevice(),
			gpuLost:               true,
			fabricHealthUnhealthy: true,
		}

		state, err := td.GetFabricState()
		assert.Error(t, err)
		assert.Equal(t, nvmlerrors.ErrGPULost, err)
		assert.Equal(t, FabricState{}, state)
	})
}

// TestTestDevice_GetUtilizationRates tests GetUtilizationRates with error injection.
func TestTestDevice_GetUtilizationRates(t *testing.T) {
	t.Run("GPU lost error", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetUtilizationRatesFunc = func() (nvml.Utilization, nvml.Return) {
			return nvml.Utilization{Gpu: 50, Memory: 30}, nvml.SUCCESS
		}

		td := &testDevice{
			Device:  mockDev,
			gpuLost: true,
		}

		util, ret := td.GetUtilizationRates()
		assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
		assert.Equal(t, nvml.Utilization{}, util)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetUtilizationRatesFunc = func() (nvml.Utilization, nvml.Return) {
			return nvml.Utilization{Gpu: 75, Memory: 40}, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		util, ret := td.GetUtilizationRates()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, uint32(75), util.Gpu)
		assert.Equal(t, uint32(40), util.Memory)
	})
}

// TestTestDevice_GetPowerUsage tests GetPowerUsage with error injection.
func TestTestDevice_GetPowerUsage(t *testing.T) {
	t.Run("GPU requires reset error", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetPowerUsageFunc = func() (uint32, nvml.Return) {
			return 250000, nvml.SUCCESS
		}

		td := &testDevice{
			Device:           mockDev,
			gpuRequiresReset: true,
		}

		power, ret := td.GetPowerUsage()
		assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
		assert.Equal(t, uint32(0), power)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetPowerUsageFunc = func() (uint32, nvml.Return) {
			return 300000, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		power, ret := td.GetPowerUsage()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, uint32(300000), power)
	})
}

// TestTestDevice_GetMemoryInfo tests GetMemoryInfo with error injection.
func TestTestDevice_GetMemoryInfo(t *testing.T) {
	t.Run("GPU lost error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:  mockDev,
			gpuLost: true,
		}

		mem, ret := td.GetMemoryInfo()
		assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
		assert.Equal(t, nvml.Memory{}, mem)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetMemoryInfoFunc = func() (nvml.Memory, nvml.Return) {
			return nvml.Memory{Total: 81920000000, Free: 40960000000, Used: 40960000000}, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		mem, ret := td.GetMemoryInfo()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, uint64(81920000000), mem.Total)
	})
}

// TestTestDevice_GetEccMode tests GetEccMode with error injection.
func TestTestDevice_GetEccMode(t *testing.T) {
	t.Run("GPU requires reset error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:           mockDev,
			gpuRequiresReset: true,
		}

		current, pending, ret := td.GetEccMode()
		assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
		assert.Equal(t, nvml.FEATURE_DISABLED, current)
		assert.Equal(t, nvml.FEATURE_DISABLED, pending)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetEccModeFunc = func() (nvml.EnableState, nvml.EnableState, nvml.Return) {
			return nvml.FEATURE_ENABLED, nvml.FEATURE_ENABLED, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		current, pending, ret := td.GetEccMode()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, nvml.FEATURE_ENABLED, current)
		assert.Equal(t, nvml.FEATURE_ENABLED, pending)
	})
}

// TestTestDevice_GetTemperature tests GetTemperature with error injection.
func TestTestDevice_GetTemperature(t *testing.T) {
	t.Run("GPU lost error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:  mockDev,
			gpuLost: true,
		}

		temp, ret := td.GetTemperature(nvml.TEMPERATURE_GPU)
		assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
		assert.Equal(t, uint32(0), temp)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetTemperatureFunc = func(sensorType nvml.TemperatureSensors) (uint32, nvml.Return) {
			return 65, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		temp, ret := td.GetTemperature(nvml.TEMPERATURE_GPU)
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, uint32(65), temp)
	})
}

// TestTestDevice_GetUUID tests GetUUID with error injection.
func TestTestDevice_GetUUID(t *testing.T) {
	t.Run("GPU requires reset error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:           mockDev,
			gpuRequiresReset: true,
		}

		uuid, ret := td.GetUUID()
		assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
		assert.Equal(t, "", uuid)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetUUIDFunc = func() (string, nvml.Return) {
			return "GPU-ABCD-1234", nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		uuid, ret := td.GetUUID()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, "GPU-ABCD-1234", uuid)
	})
}

// TestTestDevice_GetName tests GetName with error injection.
func TestTestDevice_GetName(t *testing.T) {
	t.Run("GPU lost error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:  mockDev,
			gpuLost: true,
		}

		name, ret := td.GetName()
		assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
		assert.Equal(t, "", name)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetNameFunc = func() (string, nvml.Return) {
			return "NVIDIA H100", nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		name, ret := td.GetName()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, "NVIDIA H100", name)
	})
}

// TestTestDevice_GetPciInfo tests GetPciInfo with error injection.
func TestTestDevice_GetPciInfo(t *testing.T) {
	t.Run("GPU requires reset error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:           mockDev,
			gpuRequiresReset: true,
		}

		pci, ret := td.GetPciInfo()
		assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
		assert.Equal(t, nvml.PciInfo{}, pci)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetPciInfoFunc = func() (nvml.PciInfo, nvml.Return) {
			return nvml.PciInfo{Domain: 0, Bus: 1, Device: 0}, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		pci, ret := td.GetPciInfo()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, uint32(1), pci.Bus)
	})
}

// TestTestDevice_GetPersistenceMode tests GetPersistenceMode with error injection.
func TestTestDevice_GetPersistenceMode(t *testing.T) {
	t.Run("GPU lost error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:  mockDev,
			gpuLost: true,
		}

		mode, ret := td.GetPersistenceMode()
		assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
		assert.Equal(t, nvml.FEATURE_DISABLED, mode)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetPersistenceModeFunc = func() (nvml.EnableState, nvml.Return) {
			return nvml.FEATURE_ENABLED, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		mode, ret := td.GetPersistenceMode()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, nvml.FEATURE_ENABLED, mode)
	})
}

// TestTestDevice_GetNvLinkState tests GetNvLinkState with error injection.
func TestTestDevice_GetNvLinkState(t *testing.T) {
	t.Run("GPU lost error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:  mockDev,
			gpuLost: true,
		}

		state, ret := td.GetNvLinkState(0)
		assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
		assert.Equal(t, nvml.FEATURE_DISABLED, state)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetNvLinkStateFunc = func(link int) (nvml.EnableState, nvml.Return) {
			return nvml.FEATURE_ENABLED, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		state, ret := td.GetNvLinkState(0)
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, nvml.FEATURE_ENABLED, state)
	})
}

// TestTestDevice_GetGspFirmwareVersion tests GetGspFirmwareVersion with error injection.
func TestTestDevice_GetGspFirmwareVersion(t *testing.T) {
	t.Run("GPU requires reset error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:           mockDev,
			gpuRequiresReset: true,
		}

		version, ret := td.GetGspFirmwareVersion()
		assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
		assert.Equal(t, "", version)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetGspFirmwareVersionFunc = func() (string, nvml.Return) {
			return "550.54.14", nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		version, ret := td.GetGspFirmwareVersion()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, "550.54.14", version)
	})
}

// TestTestDevice_GetGspFirmwareMode tests GetGspFirmwareMode with error injection.
func TestTestDevice_GetGspFirmwareMode(t *testing.T) {
	t.Run("GPU lost error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:  mockDev,
			gpuLost: true,
		}

		enabled, defaultMode, ret := td.GetGspFirmwareMode()
		assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
		assert.False(t, enabled)
		assert.False(t, defaultMode)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetGspFirmwareModeFunc = func() (bool, bool, nvml.Return) {
			return true, true, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		enabled, defaultMode, ret := td.GetGspFirmwareMode()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.True(t, enabled)
		assert.True(t, defaultMode)
	})
}

// TestTestDevice_GetRemappedRows tests GetRemappedRows with error injection.
func TestTestDevice_GetRemappedRows(t *testing.T) {
	t.Run("GPU lost error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:  mockDev,
			gpuLost: true,
		}

		correctable, uncorrectable, isPending, failureOccurred, ret := td.GetRemappedRows()
		assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
		assert.Equal(t, 0, correctable)
		assert.Equal(t, 0, uncorrectable)
		assert.False(t, isPending)
		assert.False(t, failureOccurred)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetRemappedRowsFunc = func() (int, int, bool, bool, nvml.Return) {
			return 5, 2, true, false, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		correctable, uncorrectable, isPending, failureOccurred, ret := td.GetRemappedRows()
		assert.Equal(t, nvml.SUCCESS, ret)
		assert.Equal(t, 5, correctable)
		assert.Equal(t, 2, uncorrectable)
		assert.True(t, isPending)
		assert.False(t, failureOccurred)
	})
}

// TestTestDevice_GetComputeRunningProcesses tests GetComputeRunningProcesses with error injection.
func TestTestDevice_GetComputeRunningProcesses(t *testing.T) {
	t.Run("GPU requires reset error", func(t *testing.T) {
		mockDev := newMockNvDevice()

		td := &testDevice{
			Device:           mockDev,
			gpuRequiresReset: true,
		}

		procs, ret := td.GetComputeRunningProcesses()
		assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
		assert.Nil(t, procs)
	})

	t.Run("success passthrough", func(t *testing.T) {
		mockDev := newMockNvDevice()
		mockDev.GetComputeRunningProcessesFunc = func() ([]nvml.ProcessInfo, nvml.Return) {
			return []nvml.ProcessInfo{{Pid: 1234, UsedGpuMemory: 1024}}, nvml.SUCCESS
		}

		td := &testDevice{
			Device: mockDev,
		}

		procs, ret := td.GetComputeRunningProcesses()
		assert.Equal(t, nvml.SUCCESS, ret)
		require.Len(t, procs, 1)
		assert.Equal(t, uint32(1234), procs[0].Pid)
	})
}

// TestTestDevice_ErrorPrecedence tests that GPU errors take precedence over fabric health injection.
func TestTestDevice_ErrorPrecedence(t *testing.T) {
	tests := []struct {
		name             string
		gpuLost          bool
		gpuRequiresReset bool
		fabricUnhealthy  bool
		expectedReturn   nvml.Return
	}{
		{
			name:             "GPU lost takes precedence over reset required",
			gpuLost:          true,
			gpuRequiresReset: true,
			fabricUnhealthy:  true,
			expectedReturn:   nvml.ERROR_GPU_IS_LOST,
		},
		{
			name:             "GPU requires reset when no lost",
			gpuLost:          false,
			gpuRequiresReset: true,
			fabricUnhealthy:  true,
			expectedReturn:   nvml.ERROR_RESET_REQUIRED,
		},
		{
			name:             "Success when no errors injected",
			gpuLost:          false,
			gpuRequiresReset: false,
			fabricUnhealthy:  false,
			expectedReturn:   nvml.SUCCESS,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockDev := newMockNvDevice()

			td := &testDevice{
				Device:                mockDev,
				gpuLost:               tc.gpuLost,
				gpuRequiresReset:      tc.gpuRequiresReset,
				fabricHealthUnhealthy: tc.fabricUnhealthy,
			}

			ret := td.getErrorReturn()
			assert.Equal(t, tc.expectedReturn, ret)
		})
	}
}
