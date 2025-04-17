package utilization

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

// mockInstanceV2 implements the nvidianvml.InstanceV2 interface for testing
type mockInstanceV2 struct {
	devices map[string]device.Device
}

func (m *mockInstanceV2) NVMLExists() bool {
	return true
}

func (m *mockInstanceV2) Library() nvml_lib.Library {
	return nil
}

func (m *mockInstanceV2) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockInstanceV2) ProductName() string {
	return "Test GPU"
}

func (m *mockInstanceV2) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockInstanceV2) Shutdown() error {
	return nil
}

// MockUtilizationComponent creates a component with mocked functions for testing
func MockUtilizationComponent(
	ctx context.Context,
	getDevicesFunc func() map[string]device.Device,
	getUtilizationFunc func(uuid string, dev device.Device) (nvidianvml.Utilization, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &mockInstanceV2{
		devices: make(map[string]device.Device),
	}

	if getDevicesFunc != nil {
		mockInstance.devices = getDevicesFunc()
	}

	comp := &component{
		ctx:                cctx,
		cancel:             cancel,
		nvmlInstance:       mockInstance,
		getUtilizationFunc: getUtilizationFunc,
	}

	if getUtilizationFunc == nil {
		comp.getUtilizationFunc = nvidianvml.GetUtilization
	}

	return comp
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockNvmlInstance := &mockInstanceV2{
		devices: map[string]device.Device{},
	}

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
	assert.NotNil(t, tc.getUtilizationFunc, "getUtilizationFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockUtilizationComponent(ctx, nil, nil)
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

	utilization := nvidianvml.Utilization{
		UUID:              uuid,
		GPUUsedPercent:    85, // 85% GPU utilization
		MemoryUsedPercent: 70, // 70% Memory utilization
		Supported:         true,
	}

	getUtilizationFunc := func(uuid string, dev device.Device) (nvidianvml.Utilization, error) {
		return utilization, nil
	}

	component := MockUtilizationComponent(ctx, getDevicesFunc, getUtilizationFunc).(*component)
	result := component.Check()

	// Verify the data was collected
	data, ok := result.(*Data)
	require.True(t, ok, "result should be of type *Data")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.StateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no utilization issue found", data.reason)
	assert.Len(t, data.Utilizations, 1)
	assert.Equal(t, utilization, data.Utilizations[0])
}

func TestCheck_UtilizationError(t *testing.T) {
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

	errExpected := errors.New("utilization error")
	getUtilizationFunc := func(uuid string, dev device.Device) (nvidianvml.Utilization, error) {
		return nvidianvml.Utilization{}, errExpected
	}

	component := MockUtilizationComponent(ctx, getDevicesFunc, getUtilizationFunc).(*component)
	result := component.Check()

	// Verify error handling
	data, ok := result.(*Data)
	require.True(t, ok, "result should be of type *Data")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.StateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, data.err)
	assert.Equal(t, "error getting utilization for device gpu-uuid-123", data.reason)
}

func TestCheck_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := MockUtilizationComponent(ctx, getDevicesFunc, nil).(*component)
	result := component.Check()

	// Verify handling of no devices
	data, ok := result.(*Data)
	require.True(t, ok, "result should be of type *Data")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.StateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no utilization issue found", data.reason)
	assert.Empty(t, data.Utilizations)
}

func TestLastHealthStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockUtilizationComponent(ctx, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastData = &Data{
		Utilizations: []nvidianvml.Utilization{
			{
				UUID:              "gpu-uuid-123",
				GPUUsedPercent:    85,
				MemoryUsedPercent: 70,
				Supported:         true,
			},
		},
		health: apiv1.StateTypeHealthy,
		reason: "checked 1 devices for utilization",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeHealthy, state.Health)
	assert.Equal(t, "checked 1 devices for utilization", state.Reason)
	assert.Contains(t, state.DeprecatedExtraInfo["data"], "gpu-uuid-123")
}

func TestLastHealthStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockUtilizationComponent(ctx, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastData = &Data{
		err:    errors.New("test utilization error"),
		health: apiv1.StateTypeUnhealthy,
		reason: "error getting utilization for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting utilization for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test utilization error", state.Error)
}

func TestLastHealthStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockUtilizationComponent(ctx, nil, nil).(*component)

	// Don't set any data

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeHealthy, state.Health)
	assert.Equal(t, "no data yet", state.Reason)
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	component := MockUtilizationComponent(ctx, nil, nil)

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

	component := MockUtilizationComponent(ctx, getDevicesFunc, nil)

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
	component := MockUtilizationComponent(ctx, nil, nil).(*component)

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
				health: apiv1.StateTypeHealthy,
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
