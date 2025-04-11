package gspfirmwaremode

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

	components "github.com/leptonai/gpud/api/v1"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// MockNvmlInstance implements the nvml.InstanceV2 interface for testing
type MockNvmlInstance struct {
	devicesFunc func() map[string]device.Device
}

func (m *MockNvmlInstance) Devices() map[string]device.Device {
	if m.devicesFunc != nil {
		return m.devicesFunc()
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

// MockGSPFirmwareModeComponent creates a component with mocked functions for testing
func MockGSPFirmwareModeComponent(
	ctx context.Context,
	devicesFunc func() map[string]device.Device,
	getGSPFirmwareModeFunc func(uuid string, dev device.Device) (nvidianvml.GSPFirmwareMode, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &MockNvmlInstance{
		devicesFunc: devicesFunc,
	}

	return &component{
		ctx:                    cctx,
		cancel:                 cancel,
		nvmlInstance:           mockInstance,
		getGSPFirmwareModeFunc: getGSPFirmwareModeFunc,
	}
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockInstance := &MockNvmlInstance{
		devicesFunc: func() map[string]device.Device { return nil },
	}
	c := New(ctx, mockInstance)

	assert.NotNil(t, c, "New should return a non-nil component")
	assert.Equal(t, Name, c.Name(), "Component name should match")

	// Type assertion to access internal fields
	tc, ok := c.(*component)
	require.True(t, ok, "Component should be of type *component")

	assert.NotNil(t, tc.ctx, "Context should be set")
	assert.NotNil(t, tc.cancel, "Cancel function should be set")
	assert.NotNil(t, tc.nvmlInstance, "nvmlInstance should be set")
	assert.NotNil(t, tc.getGSPFirmwareModeFunc, "getGSPFirmwareModeFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockGSPFirmwareModeComponent(ctx, nil, nil)
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

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	gspMode := nvidianvml.GSPFirmwareMode{
		UUID:      uuid,
		Enabled:   true,
		Supported: true,
	}

	getGSPFirmwareModeFunc := func(uuid string, dev device.Device) (nvidianvml.GSPFirmwareMode, error) {
		return gspMode, nil
	}

	component := MockGSPFirmwareModeComponent(ctx, getDevicesFunc, getGSPFirmwareModeFunc).(*component)
	component.CheckOnce()

	// Verify the data was collected
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no GSP firmware mode issue found", lastData.reason)
	assert.Len(t, lastData.GSPFirmwareModes, 1)
	assert.Equal(t, gspMode, lastData.GSPFirmwareModes[0])
}

func TestCheckOnce_Error(t *testing.T) {
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

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	errExpected := errors.New("GSP firmware mode error")
	getGSPFirmwareModeFunc := func(uuid string, dev device.Device) (nvidianvml.GSPFirmwareMode, error) {
		return nvidianvml.GSPFirmwareMode{}, errExpected
	}

	component := MockGSPFirmwareModeComponent(ctx, getDevicesFunc, getGSPFirmwareModeFunc).(*component)
	component.CheckOnce()

	// Verify error handling
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.healthy, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastData.err)
	assert.Equal(t, "error getting GSP firmware mode for device gpu-uuid-123", lastData.reason)
}

func TestCheckOnce_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := MockGSPFirmwareModeComponent(ctx, getDevicesFunc, nil).(*component)
	component.CheckOnce()

	// Verify handling of no devices
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no GSP firmware mode issue found", lastData.reason)
	assert.Empty(t, lastData.GSPFirmwareModes)
}

func TestStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockGSPFirmwareModeComponent(ctx, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastData = &Data{
		GSPFirmwareModes: []nvidianvml.GSPFirmwareMode{
			{
				UUID:      "gpu-uuid-123",
				Enabled:   true,
				Supported: true,
			},
		},
		healthy: true,
		reason:  "all 1 GPU(s) were checked, no GSP firmware mode issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, components.StateHealthy, state.Health)
	assert.True(t, state.Healthy)
	assert.Equal(t, "all 1 GPU(s) were checked, no GSP firmware mode issue found", state.Reason)
	assert.Contains(t, state.ExtraInfo["data"], "gpu-uuid-123")
}

func TestStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockGSPFirmwareModeComponent(ctx, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastData = &Data{
		err:     errors.New("test GSP firmware mode error"),
		healthy: false,
		reason:  "error getting GSP firmware mode for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, components.StateUnhealthy, state.Health)
	assert.False(t, state.Healthy)
	assert.Equal(t, "error getting GSP firmware mode for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test GSP firmware mode error", state.Error)
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockGSPFirmwareModeComponent(ctx, nil, nil).(*component)

	// Don't set any data

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, components.StateHealthy, state.Health)
	assert.True(t, state.Healthy)
	assert.Equal(t, "no data yet", state.Reason)
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	component := MockGSPFirmwareModeComponent(ctx, nil, nil)

	events, err := component.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock functions that count calls
	callCount := &atomic.Int32{}
	getDevicesFunc := func() map[string]device.Device {
		callCount.Add(1)
		return map[string]device.Device{}
	}

	component := MockGSPFirmwareModeComponent(ctx, getDevicesFunc, nil)

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
	component := MockGSPFirmwareModeComponent(ctx, nil, nil).(*component)

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
