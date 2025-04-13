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

// MockNvmlInstance implements the nvidianvml.InstanceV2 interface for testing
type MockNvmlInstance struct {
	DevicesFunc func() map[string]device.Device
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

func (m *MockNvmlInstance) NVMLExists() bool {
	return true
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
	nvmlInstance nvidianvml.InstanceV2,
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
	c := New(ctx, mockNvmlInstance)

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
	component.CheckOnce()

	// Verify the data was collected
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no memory issue found", lastData.reason)
	assert.Len(t, lastData.Memories, 1)
	assert.Equal(t, memory, lastData.Memories[0])
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
	}

	errExpected := errors.New("memory error")
	getMemoryFunc := func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{}, errExpected
	}

	component := MockMemoryComponent(ctx, mockNvmlInstance, getMemoryFunc).(*component)
	component.CheckOnce()

	// Verify error handling
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.healthy, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastData.err)
	assert.Equal(t, "error getting memory for device gpu-uuid-123", lastData.reason)
}

func TestCheckOnce_NoDevices(t *testing.T) {
	ctx := context.Background()

	mockNvmlInstance := &MockNvmlInstance{
		DevicesFunc: func() map[string]device.Device {
			return map[string]device.Device{} // Empty map
		},
	}

	component := MockMemoryComponent(ctx, mockNvmlInstance, nil).(*component)
	component.CheckOnce()

	// Verify handling of no devices
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no memory issue found", lastData.reason)
	assert.Empty(t, lastData.Memories)
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
	component.CheckOnce()

	// Verify error handling for GetUsedPercent failure
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.healthy, "data should be marked unhealthy")
	assert.NotNil(t, lastData.err)
	assert.Equal(t, "error getting used percent for device gpu-uuid-123", lastData.reason)
}

func TestStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockMemoryComponent(ctx, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastData = &Data{
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
		healthy: true,
		reason:  "all 1 GPU(s) were checked, no memory issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeHealthy, state.Health)
	assert.True(t, state.DeprecatedHealthy)
	assert.Equal(t, "all 1 GPU(s) were checked, no memory issue found", state.Reason)
	assert.Contains(t, state.DeprecatedExtraInfo["data"], "gpu-uuid-123")
}

func TestStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockMemoryComponent(ctx, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastData = &Data{
		err:     errors.New("test memory error"),
		healthy: false,
		reason:  "error getting memory for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, state.Health)
	assert.False(t, state.DeprecatedHealthy)
	assert.Equal(t, "error getting memory for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test memory error", state.Error)
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockMemoryComponent(ctx, nil, nil).(*component)

	// Don't set any data

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeHealthy, state.Health)
	assert.True(t, state.DeprecatedHealthy)
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
	}

	component := MockMemoryComponent(ctx, mockNvmlInstance, nil)

	// Start should be non-blocking
	err := component.Start()
	assert.NoError(t, err)

	// Give the goroutine time to execute CheckOnce at least once
	time.Sleep(100 * time.Millisecond)

	// Verify CheckOnce was called
	assert.GreaterOrEqual(t, callCount.Load(), int32(1), "CheckOnce should have been called at least once")
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
		data     *Data
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "with error",
			data: &Data{
				err: errors.New("test error"),
			},
			expected: "test error",
		},
		{
			name: "no error",
			data: &Data{
				healthy: true,
				reason:  "all good",
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
