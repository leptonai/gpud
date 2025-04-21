package memory

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// MockNvmlInstance implements the nvidianvml.Instance interface for testing
type MockNvmlInstance struct {
	DevicesFunc func() map[string]device.Device
	nvmlExists  bool
}

func (m *MockNvmlInstance) Devices() map[string]device.Device {
	if m.DevicesFunc != nil {
		return m.DevicesFunc()
	}
	return nil
}

func (m *MockNvmlInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *MockNvmlInstance) ProductName() string {
	return "NVIDIA Test GPU"
}

func (m *MockNvmlInstance) Architecture() string {
	return ""
}

func (m *MockNvmlInstance) Brand() string {
	return ""
}

func (m *MockNvmlInstance) DriverVersion() string {
	return ""
}

func (m *MockNvmlInstance) DriverMajor() int {
	return 0
}

func (m *MockNvmlInstance) CUDAVersion() string {
	return ""
}

func (m *MockNvmlInstance) NVMLExists() bool {
	return m.nvmlExists
}

func (m *MockNvmlInstance) Library() nvml_lib.Library {
	return nil
}

func (m *MockNvmlInstance) Shutdown() error {
	return nil
}

// MockMemoryComponent creates a component with mocked functions for testing
func MockMemoryComponent(
	ctx context.Context,
	nvmlInstance nvidianvml.Instance,
	getMemoryFunc func(uuid string, dev device.Device) (nvidianvml.Memory, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	return &component{
		ctx:           cctx,
		cancel:        cancel,
		nvmlInstance:  nvmlInstance,
		getMemoryFunc: getMemoryFunc,
	}
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockNvmlInstance := &MockNvmlInstance{}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockNvmlInstance,
	}

	c, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, c, "New should return a non-nil component")
	assert.Equal(t, Name, c.Name(), "Component name should match")

	// Type assertion to access internal fields
	tc, ok := c.(*component)
	require.True(t, ok, "Component should be of type *component")

	assert.NotNil(t, tc.ctx, "Context should be set")
	assert.NotNil(t, tc.cancel, "Cancel function should be set")
	assert.NotNil(t, tc.nvmlInstance, "nvmlInstance should be set")
	assert.NotNil(t, tc.getMemoryFunc, "getMemoryFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockMemoryComponent(ctx, nil, nil)
	assert.Equal(t, Name, c.Name(), "Component name should match")
}

func TestCheckOnce_Success(t *testing.T) {
	ctx := context.Background()

	uuid := "gpu-uuid-123"
	mockDeviceObj := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

	devs := map[string]device.Device{
		uuid: mockDev,
	}

	mockNvmlInstance := &MockNvmlInstance{
		DevicesFunc: func() map[string]device.Device {
			return devs
		},
		nvmlExists: true,
	}

	memory := nvidianvml.Memory{
		UUID:          uuid,
		TotalBytes:    16 * 1024 * 1024 * 1024, // 16 GB
		ReservedBytes: 1 * 1024 * 1024 * 1024,  // 1 GB
		UsedBytes:     8 * 1024 * 1024 * 1024,  // 8 GB
		FreeBytes:     7 * 1024 * 1024 * 1024,  // 7 GB

		// These humanized values are required
		TotalHumanized:    "16 GB",
		ReservedHumanized: "1 GB",
		UsedHumanized:     "8 GB",
		FreeHumanized:     "7 GB",

		// Important: This is required for GetUsedPercent to work
		UsedPercent: "50.00",

		Supported: true,
	}

	getMemoryFunc := func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return memory, nil
	}

	component := MockMemoryComponent(ctx, mockNvmlInstance, getMemoryFunc).(*component)
	result := component.Check()

	// Verify the data was collected
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no memory issue found", data.reason)
	assert.Len(t, data.Memories, 1)
	assert.Equal(t, memory, data.Memories[0])
}

func TestCheckOnce_MemoryError(t *testing.T) {
	ctx := context.Background()

	uuid := "gpu-uuid-123"
	mockDeviceObj := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

	devs := map[string]device.Device{
		uuid: mockDev,
	}

	mockNvmlInstance := &MockNvmlInstance{
		DevicesFunc: func() map[string]device.Device {
			return devs
		},
		nvmlExists: true,
	}

	errExpected := errors.New("memory error")
	getMemoryFunc := func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{}, errExpected
	}

	component := MockMemoryComponent(ctx, mockNvmlInstance, getMemoryFunc).(*component)
	result := component.Check()

	// Verify error handling
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, data.err)
	assert.Equal(t, "error getting memory for device gpu-uuid-123", data.reason)
}

func TestCheckOnce_NoDevices(t *testing.T) {
	ctx := context.Background()

	mockNvmlInstance := &MockNvmlInstance{
		DevicesFunc: func() map[string]device.Device {
			return map[string]device.Device{} // Empty map
		},
		nvmlExists: true,
	}

	component := MockMemoryComponent(ctx, mockNvmlInstance, nil).(*component)
	result := component.Check()

	// Verify handling of no devices
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no memory issue found", data.reason)
	assert.Empty(t, data.Memories)
}

func TestCheckOnce_GetUsedPercentError(t *testing.T) {
	ctx := context.Background()

	uuid := "gpu-uuid-123"
	mockDeviceObj := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

	devs := map[string]device.Device{
		uuid: mockDev,
	}

	mockNvmlInstance := &MockNvmlInstance{
		DevicesFunc: func() map[string]device.Device {
			return devs
		},
		nvmlExists: true,
	}

	// Create malformed memory data that will cause GetUsedPercent to fail
	invalidMemory := nvidianvml.Memory{
		UUID:           uuid,
		TotalBytes:     16 * 1024 * 1024 * 1024,
		UsedBytes:      8 * 1024 * 1024 * 1024,
		FreeBytes:      7 * 1024 * 1024 * 1024,
		TotalHumanized: "16 GB",
		UsedHumanized:  "8 GB",
		FreeHumanized:  "7 GB",
		UsedPercent:    "invalid", // Invalid format will cause ParseFloat to fail
	}

	getMemoryFunc := func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return invalidMemory, nil
	}

	component := MockMemoryComponent(ctx, mockNvmlInstance, getMemoryFunc).(*component)
	result := component.Check()

	// Verify error handling for GetUsedPercent failure
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.NotNil(t, data.err)
	assert.Equal(t, "error getting used percent for device gpu-uuid-123", data.reason)
}

func TestStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockMemoryComponent(ctx, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		Memories: []nvidianvml.Memory{
			{
				UUID:              "gpu-uuid-123",
				TotalBytes:        16 * 1024 * 1024 * 1024,
				ReservedBytes:     1 * 1024 * 1024 * 1024,
				UsedBytes:         8 * 1024 * 1024 * 1024,
				FreeBytes:         7 * 1024 * 1024 * 1024,
				TotalHumanized:    "16 GB",
				ReservedHumanized: "1 GB",
				UsedHumanized:     "8 GB",
				FreeHumanized:     "7 GB",
				UsedPercent:       "50.00",
			},
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "all 1 GPU(s) were checked, no memory issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "all 1 GPU(s) were checked, no memory issue found", state.Reason)
	assert.Contains(t, state.DeprecatedExtraInfo["data"], "gpu-uuid-123")
}

func TestStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockMemoryComponent(ctx, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		err:    errors.New("test memory error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting memory for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting memory for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test memory error", state.Error)
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockMemoryComponent(ctx, nil, nil).(*component)

	// Don't set any data

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "no data yet", state.Reason)
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	component := MockMemoryComponent(ctx, nil, nil)

	events, err := component.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock functions that count calls
	callCount := &atomic.Int32{}
	mockNvmlInstance := &MockNvmlInstance{
		DevicesFunc: func() map[string]device.Device {
			callCount.Add(1)
			return map[string]device.Device{}
		},
		nvmlExists: true,
	}

	component := MockMemoryComponent(ctx, mockNvmlInstance, nil)

	// Start should be non-blocking
	err := component.Start()
	assert.NoError(t, err)

	// Give the goroutine time to execute Check at least once
	time.Sleep(100 * time.Millisecond)

	// Verify Check was called
	assert.GreaterOrEqual(t, callCount.Load(), int32(1), "Check should have been called at least once")
}

func TestClose(t *testing.T) {
	ctx := context.Background()
	component := MockMemoryComponent(ctx, nil, nil).(*component)

	err := component.Close()
	assert.NoError(t, err)

	// Check that context is canceled
	select {
	case <-component.ctx.Done():
		// Context is properly canceled
	default:
		t.Fatal("component context was not canceled on Close")
	}
}

func TestData_GetError(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "with error",
			data: &checkResult{
				err: errors.New("test error"),
			},
			expected: "test error",
		},
		{
			name: "no error",
			data: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
				reason: "all good",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.getError()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestData_String(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		expected string
		contains []string // For partial matching of table output
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "empty memories",
			data: &checkResult{
				Memories: []nvidianvml.Memory{},
			},
			expected: "no data",
		},
		{
			name: "with memory data",
			data: &checkResult{
				Memories: []nvidianvml.Memory{
					{
						UUID:              "gpu-uuid-123",
						TotalBytes:        16 * 1024 * 1024 * 1024,
						ReservedBytes:     1 * 1024 * 1024 * 1024,
						UsedBytes:         8 * 1024 * 1024 * 1024,
						FreeBytes:         7 * 1024 * 1024 * 1024,
						TotalHumanized:    "16 GB",
						ReservedHumanized: "1 GB",
						UsedHumanized:     "8 GB",
						FreeHumanized:     "7 GB",
						UsedPercent:       "50.00",
					},
				},
			},
			contains: []string{
				"GPU UUID",
				"TOTAL",
				"RESERVED",
				"USED",
				"FREE",
				"USED %",
				"gpu-uuid-123",
				"16 GB",
				"1 GB",
				"8 GB",
				"7 GB",
				"50.00",
			},
		},
		{
			name: "multiple memory entries",
			data: &checkResult{
				Memories: []nvidianvml.Memory{
					{
						UUID:              "gpu-uuid-123",
						TotalBytes:        16 * 1024 * 1024 * 1024,
						ReservedBytes:     1 * 1024 * 1024 * 1024,
						UsedBytes:         8 * 1024 * 1024 * 1024,
						FreeBytes:         7 * 1024 * 1024 * 1024,
						TotalHumanized:    "16 GB",
						ReservedHumanized: "1 GB",
						UsedHumanized:     "8 GB",
						FreeHumanized:     "7 GB",
						UsedPercent:       "50.00",
					},
					{
						UUID:              "gpu-uuid-456",
						TotalBytes:        32 * 1024 * 1024 * 1024,
						ReservedBytes:     2 * 1024 * 1024 * 1024,
						UsedBytes:         20 * 1024 * 1024 * 1024,
						FreeBytes:         10 * 1024 * 1024 * 1024,
						TotalHumanized:    "32 GB",
						ReservedHumanized: "2 GB",
						UsedHumanized:     "20 GB",
						FreeHumanized:     "10 GB",
						UsedPercent:       "62.50",
					},
				},
			},
			contains: []string{
				"gpu-uuid-123",
				"16 GB",
				"gpu-uuid-456",
				"32 GB",
				"20 GB",
				"62.50",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.String()

			if tt.expected != "" {
				assert.Equal(t, tt.expected, got)
			}

			for _, s := range tt.contains {
				assert.Contains(t, got, s)
			}
		})
	}
}

func TestData_Summary(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "with reason",
			data: &checkResult{
				reason: "test reason",
			},
			expected: "test reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.Summary()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestData_HealthState(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		expected apiv1.HealthStateType
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "healthy",
			data: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
			},
			expected: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy",
			data: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expected: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.HealthState()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCheckOnce_NilNvmlInstance(t *testing.T) {
	ctx := context.Background()

	// Create a component with nil nvmlInstance
	component := MockMemoryComponent(ctx, nil, nil).(*component)
	result := component.Check()

	// Verify data when nvmlInstance is nil
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)
}

func TestCheckOnce_NvmlNotExists(t *testing.T) {
	ctx := context.Background()

	// Create a mock NVML instance where NVMLExists returns false
	mockNvmlInstance := &MockNvmlInstance{
		DevicesFunc: func() map[string]device.Device {
			return map[string]device.Device{}
		},
		nvmlExists: false,
	}

	component := MockMemoryComponent(ctx, mockNvmlInstance, nil).(*component)
	result := component.Check()

	// Verify data when NVML doesn't exist
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "NVIDIA NVML is not loaded", data.reason)
}
