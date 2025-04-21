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

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// MockNvmlInstance implements the nvidianvml.Instance interface for testing
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

	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockInstance,
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
	assert.NotNil(t, tc.getGSPFirmwareModeFunc, "getGSPFirmwareModeFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockGSPFirmwareModeComponent(ctx, nil, nil)
	assert.Equal(t, Name, c.Name(), "Component name should match")
}

func TestCheck_Success(t *testing.T) {
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
	result := component.Check()

	// Verify the data was collected
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no GSP firmware mode issue found", lastCheckResult.reason)
	assert.Len(t, lastCheckResult.GSPFirmwareModes, 1)
	assert.Equal(t, gspMode, lastCheckResult.GSPFirmwareModes[0])

	// Also check the returned result
	cr, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
}

func TestCheck_Error(t *testing.T) {
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
	component.Check()

	// Verify error handling
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, lastCheckResult.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastCheckResult.err)
	assert.Equal(t, "error getting GSP firmware mode for device gpu-uuid-123", lastCheckResult.reason)
}

func TestCheck_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := MockGSPFirmwareModeComponent(ctx, getDevicesFunc, nil).(*component)
	component.Check()

	// Verify handling of no devices
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no GSP firmware mode issue found", lastCheckResult.reason)
	assert.Empty(t, lastCheckResult.GSPFirmwareModes)
}

func TestLastHealthStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockGSPFirmwareModeComponent(ctx, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		GSPFirmwareModes: []nvidianvml.GSPFirmwareMode{
			{
				UUID:      "gpu-uuid-123",
				Enabled:   true,
				Supported: true,
			},
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "all 1 GPU(s) were checked, no GSP firmware mode issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "all 1 GPU(s) were checked, no GSP firmware mode issue found", state.Reason)
	assert.Contains(t, state.DeprecatedExtraInfo["data"], "gpu-uuid-123")
}

func TestLastHealthStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockGSPFirmwareModeComponent(ctx, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		err:    errors.New("test GSP firmware mode error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting GSP firmware mode for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting GSP firmware mode for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test GSP firmware mode error", state.Error)
}

func TestLastHealthStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockGSPFirmwareModeComponent(ctx, nil, nil).(*component)

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

	// Give the goroutine time to execute Check at least once
	time.Sleep(100 * time.Millisecond)

	// Verify Check was called
	assert.GreaterOrEqual(t, callCount.Load(), int32(1), "Check should have been called at least once")
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
