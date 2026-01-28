package device

import (
	"bytes"
	"testing"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
)

// TestFabricStateToString tests the FabricStateToString function.
func TestFabricStateToString(t *testing.T) {
	tests := []struct {
		name     string
		state    uint8
		expected string
	}{
		{
			name:     "not supported",
			state:    nvml.GPU_FABRIC_STATE_NOT_SUPPORTED,
			expected: "Not Supported",
		},
		{
			name:     "not started",
			state:    nvml.GPU_FABRIC_STATE_NOT_STARTED,
			expected: "Not Started",
		},
		{
			name:     "in progress",
			state:    nvml.GPU_FABRIC_STATE_IN_PROGRESS,
			expected: "In Progress",
		},
		{
			name:     "completed",
			state:    nvml.GPU_FABRIC_STATE_COMPLETED,
			expected: "Completed",
		},
		{
			name:     "unknown state",
			state:    99,
			expected: "Unknown(99)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FabricStateToString(tc.state)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestFabricStatusToString tests the FabricStatusToString function.
func TestFabricStatusToString(t *testing.T) {
	tests := []struct {
		name     string
		status   nvml.Return
		expected string
	}{
		{
			name:     "success",
			status:   nvml.SUCCESS,
			expected: "Success",
		},
		{
			name:     "uninitialized error",
			status:   nvml.ERROR_UNINITIALIZED,
			expected: nvml.ERROR_UNINITIALIZED.Error(), // Uses NVML's error string
		},
		{
			name:     "gpu lost error",
			status:   nvml.ERROR_GPU_IS_LOST,
			expected: nvml.ERROR_GPU_IS_LOST.Error(), // Uses NVML's error string
		},
		{
			name:     "reset required error",
			status:   nvml.ERROR_RESET_REQUIRED,
			expected: nvml.ERROR_RESET_REQUIRED.Error(),
		},
		{
			name:     "not supported error",
			status:   nvml.ERROR_NOT_SUPPORTED,
			expected: nvml.ERROR_NOT_SUPPORTED.Error(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FabricStatusToString(tc.status)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestFabricSummaryToString tests the FabricSummaryToString function.
func TestFabricSummaryToString(t *testing.T) {
	tests := []struct {
		name     string
		summary  uint8
		expected string
	}{
		{
			name:     "not supported",
			summary:  nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED,
			expected: "Not Supported",
		},
		{
			name:     "healthy",
			summary:  nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
			expected: "Healthy",
		},
		{
			name:     "unhealthy",
			summary:  nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY,
			expected: "Unhealthy",
		},
		{
			name:     "limited capacity",
			summary:  nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY,
			expected: "Limited Capacity",
		},
		{
			name:     "unknown summary",
			summary:  99,
			expected: "Unknown(99)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FabricSummaryToString(tc.summary)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestFabricState_ToEntry tests the ToEntry method of FabricState.
func TestFabricState_ToEntry(t *testing.T) {
	fs := FabricState{
		CliqueID:      42,
		ClusterUUID:   "test-cluster-uuid",
		State:         nvml.GPU_FABRIC_STATE_COMPLETED,
		Status:        nvml.SUCCESS,
		HealthMask:    0,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
	}

	entry := fs.ToEntry("GPU-12345")

	assert.Equal(t, "GPU-12345", entry.GPUUUID)
	assert.Equal(t, uint32(42), entry.CliqueID)
	assert.Equal(t, "test-cluster-uuid", entry.ClusterUUID)
	assert.Equal(t, "Completed", entry.State)
	assert.Equal(t, "Success", entry.Status)
	assert.Equal(t, "Healthy", entry.Summary)
}

// TestFabricStateEntry_RenderTable tests the RenderTable method.
func TestFabricStateEntry_RenderTable(t *testing.T) {
	tests := []struct {
		name           string
		entry          FabricStateEntry
		expectedFields []string
	}{
		{
			name: "basic entry",
			entry: FabricStateEntry{
				GPUUUID:  "GPU-12345",
				CliqueID: 1,
				State:    "Completed",
				Status:   "Success",
			},
			expectedFields: []string{"GPU-12345", "Completed", "Success"},
		},
		{
			name: "entry with cluster UUID",
			entry: FabricStateEntry{
				GPUUUID:     "GPU-12345",
				CliqueID:    1,
				ClusterUUID: "cluster-uuid-123",
				State:       "Completed",
				Status:      "Success",
			},
			expectedFields: []string{"GPU-12345", "cluster-uuid-123", "Completed", "Success"},
		},
		{
			name: "entry with health summary",
			entry: FabricStateEntry{
				GPUUUID:  "GPU-12345",
				CliqueID: 1,
				State:    "Completed",
				Status:   "Success",
				Summary:  "Healthy",
			},
			expectedFields: []string{"GPU-12345", "Completed", "Success", "Healthy"},
		},
		{
			name: "entry with health details",
			entry: FabricStateEntry{
				GPUUUID:  "GPU-12345",
				CliqueID: 1,
				State:    "Completed",
				Status:   "Success",
				Health: FabricHealthSnapshot{
					Bandwidth:             "Full",
					RouteRecoveryProgress: "False",
					RouteUnhealthy:        "False",
					AccessTimeoutRecovery: "False",
				},
			},
			expectedFields: []string{"GPU-12345", "Completed", "Success", "Full"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			tc.entry.RenderTable(&buf)
			output := buf.String()

			for _, field := range tc.expectedFields {
				assert.Contains(t, output, field)
			}
		})
	}
}

// TestDeviceOptions tests the device options (OpOption functions).
func TestDeviceOptions(t *testing.T) {
	tests := []struct {
		name    string
		options []OpOption
		checkOp func(*testing.T, *Op)
	}{
		{
			name:    "WithGPULost",
			options: []OpOption{WithGPULost()},
			checkOp: func(t *testing.T, op *Op) {
				assert.True(t, op.GPULost)
				assert.False(t, op.GPURequiresReset)
				assert.False(t, op.FabricHealthUnhealthy)
			},
		},
		{
			name:    "WithGPURequiresReset",
			options: []OpOption{WithGPURequiresReset()},
			checkOp: func(t *testing.T, op *Op) {
				assert.False(t, op.GPULost)
				assert.True(t, op.GPURequiresReset)
				assert.False(t, op.FabricHealthUnhealthy)
			},
		},
		{
			name:    "WithFabricHealthUnhealthy",
			options: []OpOption{WithFabricHealthUnhealthy()},
			checkOp: func(t *testing.T, op *Op) {
				assert.False(t, op.GPULost)
				assert.False(t, op.GPURequiresReset)
				assert.True(t, op.FabricHealthUnhealthy)
			},
		},
		{
			name:    "WithDriverMajor",
			options: []OpOption{WithDriverMajor(550)},
			checkOp: func(t *testing.T, op *Op) {
				assert.Equal(t, 550, op.DriverMajor)
			},
		},
		{
			name: "multiple options",
			options: []OpOption{
				WithGPULost(),
				WithDriverMajor(560),
			},
			checkOp: func(t *testing.T, op *Op) {
				assert.True(t, op.GPULost)
				assert.Equal(t, 560, op.DriverMajor)
			},
		},
		{
			name: "all failure injection options",
			options: []OpOption{
				WithGPULost(),
				WithGPURequiresReset(),
				WithFabricHealthUnhealthy(),
				WithDriverMajor(535),
			},
			checkOp: func(t *testing.T, op *Op) {
				assert.True(t, op.GPULost)
				assert.True(t, op.GPURequiresReset)
				assert.True(t, op.FabricHealthUnhealthy)
				assert.Equal(t, 535, op.DriverMajor)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op := &Op{}
			op.applyOpts(tc.options)
			tc.checkOp(t, op)
		})
	}
}

// TestFabricBandwidthStatus tests the fabricBandwidthStatus helper function.
func TestFabricBandwidthStatus(t *testing.T) {
	tests := []struct {
		name     string
		val      uint32
		expected string
	}{
		{
			name:     "not supported",
			val:      nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_NOT_SUPPORTED,
			expected: "Not Supported",
		},
		{
			name:     "degraded",
			val:      nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE,
			expected: "Degraded",
		},
		{
			name:     "full",
			val:      nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_FALSE,
			expected: "Full",
		},
		{
			name:     "unknown value",
			val:      999,
			expected: "Unknown(999)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := fabricBandwidthStatus(tc.val)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestFabricTriStateStatus tests the fabricTriStateStatus helper function.
func TestFabricTriStateStatus(t *testing.T) {
	tests := []struct {
		name         string
		val          uint32
		notSupported uint32
		trueValue    uint32
		falseValue   uint32
		expected     string
	}{
		{
			name:         "not supported",
			val:          0,
			notSupported: 0,
			trueValue:    1,
			falseValue:   2,
			expected:     "Not Supported",
		},
		{
			name:         "true value",
			val:          1,
			notSupported: 0,
			trueValue:    1,
			falseValue:   2,
			expected:     "True",
		},
		{
			name:         "false value",
			val:          2,
			notSupported: 0,
			trueValue:    1,
			falseValue:   2,
			expected:     "False",
		},
		{
			name:         "unknown value",
			val:          99,
			notSupported: 0,
			trueValue:    1,
			falseValue:   2,
			expected:     "Unknown(99)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := fabricTriStateStatus(tc.val, tc.notSupported, tc.trueValue, tc.falseValue)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestMinDriverVersionForV3FabricAPI_Constant tests the constant value.
func TestMinDriverVersionForV3FabricAPI_Constant(t *testing.T) {
	// This is a simple test to verify the constant exists and has the expected value
	assert.Equal(t, 550, MinDriverVersionForV3FabricAPI)
}

// TestFabricHealthSnapshot_ZeroValue tests the zero value of FabricHealthSnapshot.
func TestFabricHealthSnapshot_ZeroValue(t *testing.T) {
	var snapshot FabricHealthSnapshot
	assert.Empty(t, snapshot.Bandwidth)
	assert.Empty(t, snapshot.RouteRecoveryProgress)
	assert.Empty(t, snapshot.RouteUnhealthy)
	assert.Empty(t, snapshot.AccessTimeoutRecovery)
}

// TestFabricState_ZeroValue tests the zero value of FabricState.
func TestFabricState_ZeroValue(t *testing.T) {
	var state FabricState
	assert.Equal(t, uint32(0), state.CliqueID)
	assert.Empty(t, state.ClusterUUID)
	assert.Equal(t, uint8(0), state.State)
	assert.Equal(t, nvml.Return(0), state.Status)
	assert.Equal(t, uint32(0), state.HealthMask)
	assert.Equal(t, uint8(0), state.HealthSummary)
}

// --- Device New() and getter tests ---

// newStubDevice creates a stubDevice with the given mock.Device for testing
func newStubDevice(md *mock.Device) nvlibdevice.Device {
	return &stubDevice{Device: md}
}

// TestNew_BasicDevice tests New() creates a device correctly.
func TestNew_BasicDevice(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
	}

	dev := New(newStubDevice(md), "0000:00:1e.0")
	require.NotNil(t, dev)
	assert.Equal(t, "0000:00:1e.0", dev.PCIBusID())
	assert.Equal(t, "GPU-12345", dev.UUID())
}

// TestNew_WithDriverMajor tests New() with the WithDriverMajor option.
func TestNew_WithDriverMajor(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-99999", nvml.SUCCESS
		},
	}

	dev := New(newStubDevice(md), "0000:01:00.0", WithDriverMajor(550))
	require.NotNil(t, dev)

	// Should be a plain nvDevice (no test flags set)
	nvDev, ok := dev.(*nvDevice)
	require.True(t, ok)
	assert.Equal(t, 550, nvDev.driverMajor)
	assert.Equal(t, "GPU-99999", dev.UUID())
}

// TestNew_WithGPULost tests New() with GPU lost injection creates testDevice.
func TestNew_WithGPULost(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-LOST", nvml.SUCCESS
		},
	}

	dev := New(newStubDevice(md), "0000:02:00.0", WithGPULost())
	require.NotNil(t, dev)

	// Should be a testDevice
	td, ok := dev.(*testDevice)
	require.True(t, ok)
	assert.True(t, td.gpuLost)
}

// TestNew_WithGPURequiresReset tests New() with GPU requires reset injection.
func TestNew_WithGPURequiresReset(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-RESET", nvml.SUCCESS
		},
	}

	dev := New(newStubDevice(md), "0000:03:00.0", WithGPURequiresReset())
	require.NotNil(t, dev)

	td, ok := dev.(*testDevice)
	require.True(t, ok)
	assert.True(t, td.gpuRequiresReset)
}

// TestNew_WithFabricHealthUnhealthy tests New() with fabric health injection.
func TestNew_WithFabricHealthUnhealthy(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-FABRIC", nvml.SUCCESS
		},
	}

	dev := New(newStubDevice(md), "0000:04:00.0", WithFabricHealthUnhealthy())
	require.NotNil(t, dev)

	td, ok := dev.(*testDevice)
	require.True(t, ok)
	assert.True(t, td.fabricHealthUnhealthy)
}

// --- testDevice method coverage tests ---

// createGPULostTestDevice creates a testDevice with gpuLost=true for method testing.
func createGPULostTestDevice() *testDevice {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-TEST", nvml.SUCCESS
		},
	}
	baseDev := &nvDevice{
		Device: newStubDevice(md),
		busID:  "0000:00:1e.0",
		uuid:   "GPU-TEST",
	}
	return &testDevice{
		Device:  baseDev,
		gpuLost: true,
	}
}

// createGPUResetTestDevice creates a testDevice with gpuRequiresReset=true.
func createGPUResetTestDevice() *testDevice {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-TEST-RESET", nvml.SUCCESS
		},
	}
	baseDev := &nvDevice{
		Device: newStubDevice(md),
		busID:  "0000:00:1e.0",
		uuid:   "GPU-TEST-RESET",
	}
	return &testDevice{
		Device:           baseDev,
		gpuRequiresReset: true,
	}
}

func TestTestDevice_GetEnforcedPowerLimit_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetEnforcedPowerLimit()
	assert.Equal(t, uint32(0), val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetEnforcedPowerLimit_GPUReset(t *testing.T) {
	td := createGPUResetTestDevice()
	val, ret := td.GetEnforcedPowerLimit()
	assert.Equal(t, uint32(0), val)
	assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
}

func TestTestDevice_GetPowerManagementLimit_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetPowerManagementLimit()
	assert.Equal(t, uint32(0), val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetPowerManagementLimit_GPUReset(t *testing.T) {
	td := createGPUResetTestDevice()
	val, ret := td.GetPowerManagementLimit()
	assert.Equal(t, uint32(0), val)
	assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
}

func TestTestDevice_GetProcessUtilization_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	samples, ret := td.GetProcessUtilization(0)
	assert.Nil(t, samples)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GpmQueryDeviceSupport_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	support, ret := td.GpmQueryDeviceSupport()
	assert.Equal(t, nvml.GpmSupport{}, support)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GpmSampleGet_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	ret := td.GpmSampleGet(nil)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetCurrentClocksEventReasons_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetCurrentClocksEventReasons()
	assert.Equal(t, uint64(0), val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetSupportedEventTypes_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetSupportedEventTypes()
	assert.Equal(t, uint64(0), val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetMemoryErrorCounter_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetMemoryErrorCounter(nvml.MEMORY_ERROR_TYPE_CORRECTED, nvml.VOLATILE_ECC, nvml.MEMORY_LOCATION_L1_CACHE)
	assert.Equal(t, uint64(0), val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetTotalEccErrors_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetTotalEccErrors(nvml.MEMORY_ERROR_TYPE_CORRECTED, nvml.VOLATILE_ECC)
	assert.Equal(t, uint64(0), val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetRowRemapperHistogram_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetRowRemapperHistogram()
	assert.Equal(t, nvml.RowRemapperHistogramValues{}, val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetClock_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetClock(nvml.CLOCK_SM, nvml.CLOCK_ID_CURRENT)
	assert.Equal(t, uint32(0), val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetMaxClockInfo_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetMaxClockInfo(nvml.CLOCK_SM)
	assert.Equal(t, uint32(0), val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetNvLinkErrorCounter_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	val, ret := td.GetNvLinkErrorCounter(0, nvml.NVLINK_ERROR_DL_REPLAY)
	assert.Equal(t, uint64(0), val)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetFieldValues_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	ret := td.GetFieldValues(nil)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

func TestTestDevice_GetGpuFabricInfo_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	info, ret := td.GetGpuFabricInfo()
	assert.Equal(t, nvml.GpuFabricInfo{}, info)
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, ret)
}

// --- testDevice GetFabricState tests ---

func TestTestDevice_GetFabricState_GPULost(t *testing.T) {
	td := createGPULostTestDevice()
	_, err := td.GetFabricState()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GPU lost")
}

func TestTestDevice_GetFabricState_GPUReset(t *testing.T) {
	td := createGPUResetTestDevice()
	_, err := td.GetFabricState()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GPU requires reset")
}

// --- nvDevice PCIBusID and UUID tests ---

func TestNvDevice_PCIBusID(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-BUSID-TEST", nvml.SUCCESS
		},
	}

	dev := New(newStubDevice(md), "0000:AF:00.0")
	assert.Equal(t, "0000:AF:00.0", dev.PCIBusID())
}

func TestNvDevice_UUID(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-UUID-12345-ABCDE", nvml.SUCCESS
		},
	}

	dev := New(newStubDevice(md), "0000:00:00.0")
	assert.Equal(t, "GPU-UUID-12345-ABCDE", dev.UUID())
}

// --- getErrorReturn logic tests ---

func TestGetErrorReturn_NoFlags(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-OK", nvml.SUCCESS
		},
	}
	baseDev := &nvDevice{
		Device: newStubDevice(md),
		busID:  "0000:00:00.0",
		uuid:   "GPU-OK",
	}
	td := &testDevice{
		Device: baseDev,
	}
	assert.Equal(t, nvml.SUCCESS, td.getErrorReturn())
}

func TestGetErrorReturn_GPULostPrecedence(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-BOTH", nvml.SUCCESS
		},
	}
	baseDev := &nvDevice{
		Device: newStubDevice(md),
		busID:  "0000:00:00.0",
		uuid:   "GPU-BOTH",
	}
	td := &testDevice{
		Device:           baseDev,
		gpuLost:          true,
		gpuRequiresReset: true,
	}
	// gpuLost should take precedence
	assert.Equal(t, nvml.ERROR_GPU_IS_LOST, td.getErrorReturn())
}

// --- extractHealthValue direct tests ---

func TestExtractHealthValue_ZeroMask(t *testing.T) {
	result := extractHealthValue(0, 0, 0xFFFFFFFF)
	assert.Equal(t, uint32(0), result)
}

func TestExtractHealthValue_AllOnes(t *testing.T) {
	result := extractHealthValue(0xFFFFFFFF, 0, 0xFFFFFFFF)
	assert.Equal(t, uint32(0xFFFFFFFF), result)
}

func TestExtractHealthValue_WithShift(t *testing.T) {
	// Place value 0x5 at bit position 4 (mask=0xF shifted by 4)
	mask := uint32(0x5) << 4
	result := extractHealthValue(mask, 4, 0xF)
	assert.Equal(t, uint32(0x5), result)
}

func TestExtractHealthValue_BandwidthBits(t *testing.T) {
	// Test with actual NVML constants for bandwidth
	mask := uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE) << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW
	result := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_DEGRADED_BW)
	assert.Equal(t, uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE), result)
}

// --- ParseHealthMask additional tests ---

func TestParseHealthMask_AllZero(t *testing.T) {
	health := ParseHealthMask(0)
	assert.NotEmpty(t, health.Bandwidth) // Should be "Not Supported" or similar
	assert.NotEmpty(t, health.RouteRecoveryProgress)
	assert.NotEmpty(t, health.RouteUnhealthy)
	assert.NotEmpty(t, health.AccessTimeoutRecovery)
}

// --- GetIssues additional tests ---

func TestGetIssues_CompletelyHealthy(t *testing.T) {
	state := FabricState{
		State:         nvml.GPU_FABRIC_STATE_COMPLETED,
		Status:        nvml.SUCCESS,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
		HealthMask:    0,
	}

	issues := state.GetIssues()
	assert.Empty(t, issues)
}

func TestGetIssues_NotCompletedState(t *testing.T) {
	state := FabricState{
		State:         nvml.GPU_FABRIC_STATE_IN_PROGRESS,
		Status:        nvml.SUCCESS,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
		HealthMask:    0,
	}

	issues := state.GetIssues()
	assert.NotEmpty(t, issues)
	found := false
	for _, issue := range issues {
		if issue == "state=In Progress" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestGetIssues_FailedStatus(t *testing.T) {
	state := FabricState{
		State:         nvml.GPU_FABRIC_STATE_COMPLETED,
		Status:        nvml.ERROR_UNKNOWN,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
		HealthMask:    0,
	}

	issues := state.GetIssues()
	assert.NotEmpty(t, issues)
}

func TestGetIssues_LimitedCapacity(t *testing.T) {
	state := FabricState{
		State:         nvml.GPU_FABRIC_STATE_COMPLETED,
		Status:        nvml.SUCCESS,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY,
		HealthMask:    0,
	}

	issues := state.GetIssues()
	found := false
	for _, issue := range issues {
		if issue == "summary=Limited Capacity" {
			found = true
		}
	}
	assert.True(t, found)
}

// --- New with multiple options ---

func TestNew_AllFlagsSet(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-ALL-FLAGS", nvml.SUCCESS
		},
	}
	dev := New(newStubDevice(md), "0000:00:00.0",
		WithGPULost(),
		WithGPURequiresReset(),
		WithFabricHealthUnhealthy(),
		WithDriverMajor(560),
	)

	// Should be wrapped as testDevice
	td, ok := dev.(*testDevice)
	assert.True(t, ok)
	assert.True(t, td.gpuLost)
	assert.True(t, td.gpuRequiresReset)
	assert.True(t, td.fabricHealthUnhealthy)
}

func TestNew_NoFlags_ReturnsNvDevice(t *testing.T) {
	md := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-NO-FLAGS", nvml.SUCCESS
		},
	}
	dev := New(newStubDevice(md), "0000:00:01.0", WithDriverMajor(555))

	// Should be an nvDevice, not a testDevice
	nd, ok := dev.(*nvDevice)
	assert.True(t, ok)
	assert.Equal(t, "0000:00:01.0", nd.PCIBusID())
	assert.Equal(t, "GPU-NO-FLAGS", nd.UUID())
	assert.Equal(t, 555, nd.driverMajor)
}

// --- Op applyOpts ---

func TestOp_ApplyMultipleOpts(t *testing.T) {
	op := &Op{}
	op.applyOpts([]OpOption{
		WithDriverMajor(550),
		WithGPULost(),
		WithGPURequiresReset(),
		WithFabricHealthUnhealthy(),
	})

	assert.Equal(t, 550, op.DriverMajor)
	assert.True(t, op.GPULost)
	assert.True(t, op.GPURequiresReset)
	assert.True(t, op.FabricHealthUnhealthy)
}

func TestOp_ApplyEmptyOpts(t *testing.T) {
	op := &Op{}
	op.applyOpts(nil)

	assert.Equal(t, 0, op.DriverMajor)
	assert.False(t, op.GPULost)
	assert.False(t, op.GPURequiresReset)
	assert.False(t, op.FabricHealthUnhealthy)
}

// --- RenderTable edge cases ---

func TestFabricStateEntry_RenderTable_MinimalFields(t *testing.T) {
	entry := FabricStateEntry{
		GPUUUID:  "GPU-MINIMAL",
		CliqueID: 0,
		State:    "Completed",
		Status:   "Success",
	}

	var buf bytes.Buffer
	entry.RenderTable(&buf)
	output := buf.String()

	assert.Contains(t, output, "GPU-MINIMAL")
	assert.Contains(t, output, "Completed")
	assert.Contains(t, output, "Success")
	// ClusterUUID and Summary should not appear since they are empty
	assert.NotContains(t, output, "Cluster UUID")
	assert.NotContains(t, output, "Health Summary")
}

func TestFabricStateEntry_RenderTable_AllFields(t *testing.T) {
	entry := FabricStateEntry{
		GPUUUID:     "GPU-FULL",
		CliqueID:    42,
		ClusterUUID: "cluster-abc",
		State:       "Completed",
		Status:      "Success",
		Summary:     "Healthy",
		Health: FabricHealthSnapshot{
			Bandwidth:             "Full",
			RouteRecoveryProgress: "False",
			RouteUnhealthy:        "False",
			AccessTimeoutRecovery: "False",
		},
	}

	var buf bytes.Buffer
	entry.RenderTable(&buf)
	output := buf.String()

	assert.Contains(t, output, "GPU-FULL")
	assert.Contains(t, output, "cluster-abc")
	assert.Contains(t, output, "Healthy")
	assert.Contains(t, output, "Full")
}

// --- FabricState ToEntry ---

func TestFabricState_ToEntry_WithAllFields(t *testing.T) {
	state := FabricState{
		CliqueID:      5,
		ClusterUUID:   "test-cluster",
		State:         nvml.GPU_FABRIC_STATE_COMPLETED,
		Status:        nvml.SUCCESS,
		HealthMask:    0,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
	}

	entry := state.ToEntry("GPU-TEST-UUID")
	assert.Equal(t, "GPU-TEST-UUID", entry.GPUUUID)
	assert.Equal(t, uint32(5), entry.CliqueID)
	assert.Equal(t, "test-cluster", entry.ClusterUUID)
	assert.Equal(t, "Completed", entry.State)
	assert.Equal(t, "Success", entry.Status)
	assert.Equal(t, "Healthy", entry.Summary)
}

func TestNvDevice_GetFabricState_V3Success(t *testing.T) {
	mockey.PatchConvey("GetFabricState uses V3 API on supported drivers", t, func() {
		clusterUUID := [16]uint8{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
		v3Info := nvml.GpuFabricInfo_v3{
			ClusterUuid:   clusterUUID,
			Status:        uint32(nvml.SUCCESS),
			CliqueId:      42,
			State:         nvml.GPU_FABRIC_STATE_COMPLETED,
			HealthMask:    0x1234,
			HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
		}

		mockey.Mock(nvml.GpuFabricInfoHandler.V3).To(func(_ nvml.GpuFabricInfoHandler) (nvml.GpuFabricInfo_v3, nvml.Return) {
			return v3Info, nvml.SUCCESS
		}).Build()

		md := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-V3", nvml.SUCCESS
			},
			GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler {
				return nvml.GpuFabricInfoHandler{}
			},
		}

		dev := &nvDevice{
			Device:      newStubDevice(md),
			busID:       "0000:00:00.0",
			uuid:        "GPU-V3",
			driverMajor: MinDriverVersionForV3FabricAPI,
		}

		state, err := dev.GetFabricState()
		require.NoError(t, err)
		assert.Equal(t, v3Info.CliqueId, state.CliqueID)
		assert.Equal(t, formatClusterUUID(clusterUUID), state.ClusterUUID)
		assert.Equal(t, v3Info.State, state.State)
		assert.Equal(t, nvml.Return(v3Info.Status), state.Status)
		assert.Equal(t, v3Info.HealthMask, state.HealthMask)
		assert.Equal(t, v3Info.HealthSummary, state.HealthSummary)
	})
}

func TestNvDevice_GetFabricState_V3FallbackToV1(t *testing.T) {
	mockey.PatchConvey("GetFabricState falls back to V1 when V3 fails", t, func() {
		mockey.Mock(nvml.GpuFabricInfoHandler.V3).To(func(_ nvml.GpuFabricInfoHandler) (nvml.GpuFabricInfo_v3, nvml.Return) {
			return nvml.GpuFabricInfo_v3{}, nvml.ERROR_UNKNOWN
		}).Build()

		clusterUUID := [16]uint8{0xaa, 0xbb, 0xcc, 0xdd, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb}
		v1Info := nvml.GpuFabricInfo{
			ClusterUuid: clusterUUID,
			Status:      uint32(nvml.SUCCESS),
			CliqueId:    7,
			State:       nvml.GPU_FABRIC_STATE_COMPLETED,
		}

		md := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-V1", nvml.SUCCESS
			},
			GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler {
				return nvml.GpuFabricInfoHandler{}
			},
			GetGpuFabricInfoFunc: func() (nvml.GpuFabricInfo, nvml.Return) {
				return v1Info, nvml.SUCCESS
			},
		}

		dev := &nvDevice{
			Device:      newStubDevice(md),
			busID:       "0000:00:00.0",
			uuid:        "GPU-V1",
			driverMajor: MinDriverVersionForV3FabricAPI,
		}

		state, err := dev.GetFabricState()
		require.NoError(t, err)
		assert.Equal(t, v1Info.CliqueId, state.CliqueID)
		assert.Equal(t, formatClusterUUID(clusterUUID), state.ClusterUUID)
		assert.Equal(t, v1Info.State, state.State)
		assert.Equal(t, nvml.Return(v1Info.Status), state.Status)
		assert.Equal(t, uint32(0), state.HealthMask)
		assert.Equal(t, uint8(nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED), state.HealthSummary)
	})
}

func TestNvDevice_GetFabricState_V1Errors(t *testing.T) {
	mockey.PatchConvey("GetFabricState handles V1 error returns", t, func() {
		tests := []struct {
			name        string
			ret         nvml.Return
			expectErr   error
			expectMatch string
		}{
			{
				name:      "gpu lost",
				ret:       nvml.ERROR_GPU_IS_LOST,
				expectErr: nvmlerrors.ErrGPULost,
			},
			{
				name:      "gpu requires reset",
				ret:       nvml.ERROR_RESET_REQUIRED,
				expectErr: nvmlerrors.ErrGPURequiresReset,
			},
			{
				name:        "not supported",
				ret:         nvml.ERROR_NOT_SUPPORTED,
				expectMatch: "fabric state telemetry not supported",
			},
			{
				name:        "other error",
				ret:         nvml.ERROR_UNKNOWN,
				expectMatch: nvml.ErrorString(nvml.ERROR_UNKNOWN),
			},
		}

		for _, tc := range tests {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				md := &mock.Device{
					GetUUIDFunc: func() (string, nvml.Return) {
						return "GPU-V1-ERR", nvml.SUCCESS
					},
					GetGpuFabricInfoFunc: func() (nvml.GpuFabricInfo, nvml.Return) {
						return nvml.GpuFabricInfo{}, tc.ret
					},
				}

				dev := &nvDevice{
					Device:      newStubDevice(md),
					busID:       "0000:00:00.0",
					uuid:        "GPU-V1-ERR",
					driverMajor: MinDriverVersionForV3FabricAPI - 1,
				}

				_, err := dev.GetFabricState()
				require.Error(t, err)
				if tc.expectErr != nil {
					assert.Equal(t, tc.expectErr, err)
				}
				if tc.expectMatch != "" {
					assert.Contains(t, err.Error(), tc.expectMatch)
				}
			})
		}
	})
}

// --- testDevice additional method coverage ---

func TestTestDevice_GetProcessUtilization_GPUReset(t *testing.T) {
	md := &mock.Device{
		GetProcessUtilizationFunc: func(lastSeenTimestamp uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
			return []nvml.ProcessUtilizationSample{{Pid: 100}}, nvml.SUCCESS
		},
	}
	baseDev := &nvDevice{Device: newStubDevice(md), busID: "0000:00:00.0", uuid: "GPU-PU"}

	td := &testDevice{Device: baseDev, gpuRequiresReset: true}
	samples, ret := td.GetProcessUtilization(0)
	assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
	assert.Nil(t, samples)

	// Success path
	td2 := &testDevice{Device: baseDev}
	samples2, ret2 := td2.GetProcessUtilization(0)
	assert.Equal(t, nvml.SUCCESS, ret2)
	assert.Len(t, samples2, 1)
}

func TestTestDevice_GetTotalEccErrors_GPUReset(t *testing.T) {
	md := &mock.Device{
		GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
			return 42, nvml.SUCCESS
		},
	}
	baseDev := &nvDevice{Device: newStubDevice(md), busID: "0000:00:00.0", uuid: "GPU-ECC"}

	td := &testDevice{Device: baseDev, gpuRequiresReset: true}
	count, ret := td.GetTotalEccErrors(nvml.MEMORY_ERROR_TYPE_CORRECTED, nvml.VOLATILE_ECC)
	assert.Equal(t, nvml.ERROR_RESET_REQUIRED, ret)
	assert.Equal(t, uint64(0), count)

	td2 := &testDevice{Device: baseDev}
	count2, ret2 := td2.GetTotalEccErrors(nvml.MEMORY_ERROR_TYPE_CORRECTED, nvml.VOLATILE_ECC)
	assert.Equal(t, nvml.SUCCESS, ret2)
	assert.Equal(t, uint64(42), count2)
}
