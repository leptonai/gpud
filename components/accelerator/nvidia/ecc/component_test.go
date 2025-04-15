package ecc

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
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
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
	return nvidianvml.MemoryErrorManagementCapabilities{
		ErrorContainment:     true,
		DynamicPageOfflining: true,
		RowRemapping:         true,
	}
}

func (m *MockNvmlInstance) ProductName() string {
	return "NVIDIA Test GPU"
}

func (m *MockNvmlInstance) NVMLExists() bool {
	return true
}

func (m *MockNvmlInstance) Library() lib.Library {
	return nil
}

func (m *MockNvmlInstance) Shutdown() error {
	return nil
}

// MockECCComponent creates a component with mocked functions for testing
func MockECCComponent(
	ctx context.Context,
	devicesFunc func() map[string]device.Device,
	getECCModeEnabledFunc func(uuid string, dev device.Device) (nvidianvml.ECCMode, error),
	getECCErrorsFunc func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (nvidianvml.ECCErrors, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &MockNvmlInstance{
		devicesFunc: devicesFunc,
	}

	return &component{
		ctx:                   cctx,
		cancel:                cancel,
		nvmlInstanceV2:        mockInstance,
		getECCModeEnabledFunc: getECCModeEnabledFunc,
		getECCErrorsFunc:      getECCErrorsFunc,
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
	assert.NotNil(t, tc.nvmlInstanceV2, "nvmlInstanceV2 should be set")
	assert.NotNil(t, tc.getECCModeEnabledFunc, "getECCModeEnabledFunc should be set")
	assert.NotNil(t, tc.getECCErrorsFunc, "getECCErrorsFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockECCComponent(ctx, nil, nil, nil)
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

	eccMode := nvidianvml.ECCMode{
		UUID:           uuid,
		EnabledCurrent: true,
		EnabledPending: true,
		Supported:      true,
	}

	getECCModeEnabledFunc := func(uuid string, dev device.Device) (nvidianvml.ECCMode, error) {
		return eccMode, nil
	}

	eccErrors := nvidianvml.ECCErrors{
		UUID: uuid,
		Aggregate: nvidianvml.AllECCErrorCounts{
			Total: nvidianvml.ECCErrorCounts{
				Corrected:   5,
				Uncorrected: 2,
			},
		},
		Volatile: nvidianvml.AllECCErrorCounts{
			Total: nvidianvml.ECCErrorCounts{
				Corrected:   3,
				Uncorrected: 1,
			},
		},
		Supported: true,
	}

	getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (nvidianvml.ECCErrors, error) {
		return eccErrors, nil
	}

	component := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)
	component.CheckOnce()

	// Verify the data was collected
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no ECC issue found", lastData.reason)
	assert.Len(t, lastData.ECCModes, 1)
	assert.Len(t, lastData.ECCErrors, 1)
	assert.Equal(t, eccMode, lastData.ECCModes[0])
	assert.Equal(t, eccErrors, lastData.ECCErrors[0])
}

func TestCheckOnce_ECCModeError(t *testing.T) {
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

	errExpected := errors.New("ECC mode error")
	getECCModeEnabledFunc := func(uuid string, dev device.Device) (nvidianvml.ECCMode, error) {
		return nvidianvml.ECCMode{}, errExpected
	}

	getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (nvidianvml.ECCErrors, error) {
		return nvidianvml.ECCErrors{}, nil
	}

	component := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)
	component.CheckOnce()

	// Verify error handling
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.healthy, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastData.err)
	assert.Equal(t, "error getting ECC mode for device gpu-uuid-123", lastData.reason)
}

func TestCheckOnce_ECCErrorsError(t *testing.T) {
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

	eccMode := nvidianvml.ECCMode{
		UUID:           uuid,
		EnabledCurrent: true,
		EnabledPending: true,
		Supported:      true,
	}

	getECCModeEnabledFunc := func(uuid string, dev device.Device) (nvidianvml.ECCMode, error) {
		return eccMode, nil
	}

	errExpected := errors.New("ECC errors error")
	getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (nvidianvml.ECCErrors, error) {
		return nvidianvml.ECCErrors{}, errExpected
	}

	component := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)
	component.CheckOnce()

	// Verify error handling
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.healthy, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastData.err)
	assert.Equal(t, "error getting ECC errors for device gpu-uuid-123", lastData.reason)
}

func TestCheckOnce_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := MockECCComponent(ctx, getDevicesFunc, nil, nil).(*component)
	component.CheckOnce()

	// Verify handling of no devices
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no ECC issue found", lastData.reason)
	assert.Empty(t, lastData.ECCModes)
	assert.Empty(t, lastData.ECCErrors)
}

func TestStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockECCComponent(ctx, nil, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastData = &Data{
		ECCModes: []nvidianvml.ECCMode{
			{
				UUID:           "gpu-uuid-123",
				EnabledCurrent: true,
				EnabledPending: true,
				Supported:      true,
			},
		},
		ECCErrors: []nvidianvml.ECCErrors{
			{
				UUID: "gpu-uuid-123",
				Aggregate: nvidianvml.AllECCErrorCounts{
					Total: nvidianvml.ECCErrorCounts{
						Corrected:   5,
						Uncorrected: 2,
					},
				},
				Supported: true,
			},
		},
		healthy: true,
		reason:  "all 1 GPU(s) were checked, no ECC issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.HealthStates(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeHealthy, state.Health)
	assert.Equal(t, "all 1 GPU(s) were checked, no ECC issue found", state.Reason)
	assert.Contains(t, state.DeprecatedExtraInfo["data"], "gpu-uuid-123")
}

func TestStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockECCComponent(ctx, nil, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastData = &Data{
		err:     errors.New("test ECC error"),
		healthy: false,
		reason:  "error getting ECC mode for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.HealthStates(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting ECC mode for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test ECC error", state.Error)
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockECCComponent(ctx, nil, nil, nil).(*component)

	// Don't set any data

	// Get states
	states, err := component.HealthStates(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeHealthy, state.Health)
	assert.Equal(t, "no data yet", state.Reason)
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	component := MockECCComponent(ctx, nil, nil, nil)

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

	component := MockECCComponent(ctx, getDevicesFunc, nil, nil)

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
	component := MockECCComponent(ctx, nil, nil, nil).(*component)

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
