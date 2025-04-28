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

// mockNVMLInstance implements the nvml.InstanceV2 interface for testing
type mockNVMLInstance struct {
	devicesFunc func() map[string]device.Device
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	if m.devicesFunc != nil {
		return m.devicesFunc()
	}
	return nil
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return true
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{
		ErrorContainment:     true,
		DynamicPageOfflining: true,
		RowRemapping:         true,
	}
}

func (m *mockNVMLInstance) ProductName() string {
	return "NVIDIA Test GPU"
}

func (m *mockNVMLInstance) Architecture() string {
	return "test-architecture"
}

func (m *mockNVMLInstance) Brand() string {
	return "test-brand"
}

func (m *mockNVMLInstance) DriverVersion() string {
	return "test-driver-version"
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 1
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return "test-cuda-version"
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return true
}

func (m *mockNVMLInstance) Library() lib.Library {
	return nil
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

// MockNvmlInstanceNVMLNotLoaded is a special mock implementation that always returns false for NVMLExists
type MockNvmlInstanceNVMLNotLoaded struct {
	*mockNVMLInstance
}

func (m *MockNvmlInstanceNVMLNotLoaded) NVMLExists() bool {
	return false
}

// MockECCComponent creates a component with mocked functions for testing
func MockECCComponent(
	ctx context.Context,
	devicesFunc func() map[string]device.Device,
	getECCModeEnabledFunc func(uuid string, dev device.Device) (nvidianvml.ECCMode, error),
	getECCErrorsFunc func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (nvidianvml.ECCErrors, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &mockNVMLInstance{
		devicesFunc: devicesFunc,
	}

	return &component{
		ctx:                   cctx,
		cancel:                cancel,
		nvmlInstance:          mockInstance,
		getECCModeEnabledFunc: getECCModeEnabledFunc,
		getECCErrorsFunc:      getECCErrorsFunc,
	}
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockInstance := &mockNVMLInstance{
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
	assert.NotNil(t, tc.getECCModeEnabledFunc, "getECCModeEnabledFunc should be set")
	assert.NotNil(t, tc.getECCErrorsFunc, "getECCErrorsFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockECCComponent(ctx, nil, nil, nil)
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
	result := component.Check()

	// Verify the data was collected
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no ECC issue found", data.reason)
	assert.Len(t, data.ECCModes, 1)
	assert.Len(t, data.ECCErrors, 1)
	assert.Equal(t, eccMode, data.ECCModes[0])
	assert.Equal(t, eccErrors, data.ECCErrors[0])
}

func TestCheck_ECCModeError(t *testing.T) {
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
	result := component.Check()

	// Verify error handling
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, data.err)
	assert.Equal(t, "error getting ECC mode for device gpu-uuid-123", data.reason)
}

func TestCheck_ECCErrorsError(t *testing.T) {
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
	result := component.Check()

	// Verify error handling
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, data.err)
	assert.Equal(t, "error getting ECC errors for device gpu-uuid-123", data.reason)
}

func TestCheck_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := MockECCComponent(ctx, getDevicesFunc, nil, nil).(*component)
	result := component.Check()

	// Verify handling of no devices
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no ECC issue found", data.reason)
	assert.Empty(t, data.ECCModes)
	assert.Empty(t, data.ECCErrors)
}

func TestLastHealthStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockECCComponent(ctx, nil, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
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
		health: apiv1.HealthStateTypeHealthy,
		reason: "all 1 GPU(s) were checked, no ECC issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "all 1 GPU(s) were checked, no ECC issue found", state.Reason)
	assert.Contains(t, state.ExtraInfo["data"], "gpu-uuid-123")
}

func TestLastHealthStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockECCComponent(ctx, nil, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		err:    errors.New("test ECC error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting ECC mode for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting ECC mode for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test ECC error", state.Error)
}

func TestLastHealthStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockECCComponent(ctx, nil, nil, nil).(*component)

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

	// Give the goroutine time to execute Check at least once
	time.Sleep(100 * time.Millisecond)

	// Verify Check was called
	assert.GreaterOrEqual(t, callCount.Load(), int32(1), "Check should have been called at least once")
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
		contains []string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name:     "empty data",
			data:     &checkResult{},
			expected: "no data",
		},
		{
			name: "with data",
			data: &checkResult{
				ECCModes: []nvidianvml.ECCMode{
					{
						UUID:           "gpu-uuid-123",
						EnabledCurrent: true,
						EnabledPending: false,
						Supported:      true,
					},
				},
				ECCErrors: []nvidianvml.ECCErrors{
					{
						UUID: "gpu-uuid-123",
						Aggregate: nvidianvml.AllECCErrorCounts{
							Total: nvidianvml.ECCErrorCounts{
								Corrected:   10,
								Uncorrected: 5,
							},
						},
						Volatile: nvidianvml.AllECCErrorCounts{
							Total: nvidianvml.ECCErrorCounts{
								Corrected:   3,
								Uncorrected: 1,
							},
						},
					},
				},
			},
			contains: []string{
				"GPU UUID",
				"ENABLED CURRENT",
				"ENABLED PENDING",
				"SUPPORTED",
				"gpu-uuid-123",
				"AGGREGATE TOTAL CORRECTED",
				"AGGREGATE TOTAL UNCORRECTED",
				"VOLATILE TOTAL CORRECTED",
				"VOLATILE TOTAL UNCORRECTED",
				"10", "5", "3", "1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.String()
			if tt.expected != "" {
				assert.Equal(t, tt.expected, got)
			}
			for _, str := range tt.contains {
				assert.Contains(t, got, str)
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
				reason: "test summary reason",
			},
			expected: "test summary reason",
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
			name: "healthy state",
			data: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
			},
			expected: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy state",
			data: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expected: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.HealthStateType()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCheck_NilNvmlInstance(t *testing.T) {
	ctx := context.Background()

	// Create component with nil NVML instance
	cctx, cancel := context.WithCancel(ctx)
	component := &component{
		ctx:                   cctx,
		cancel:                cancel,
		nvmlInstance:          nil,
		getECCModeEnabledFunc: nil,
		getECCErrorsFunc:      nil,
	}

	result := component.Check()

	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)
}

func TestCheck_NvmlNotLoaded(t *testing.T) {
	ctx := context.Background()

	// Create base mock instance
	baseMock := &mockNVMLInstance{
		devicesFunc: func() map[string]device.Device { return nil },
	}

	// Wrap it with our special mock that overrides NVMLExists
	mockInstance := &MockNvmlInstanceNVMLNotLoaded{
		mockNVMLInstance: baseMock,
	}

	cctx, cancel := context.WithCancel(ctx)
	component := &component{
		ctx:                   cctx,
		cancel:                cancel,
		nvmlInstance:          mockInstance,
		getECCModeEnabledFunc: nil,
		getECCErrorsFunc:      nil,
	}

	result := component.Check()

	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", data.reason)
}

func TestCheck_MultipleDevices(t *testing.T) {
	ctx := context.Background()

	uuid1 := "gpu-uuid-123"
	uuid2 := "gpu-uuid-456"

	mockDeviceObj1 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid1, nvml.SUCCESS
		},
	}
	mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "test-arch1", "test-brand1", "test-cuda1", "test-pci1")

	mockDeviceObj2 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid2, nvml.SUCCESS
		},
	}
	mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "test-arch2", "test-brand2", "test-cuda2", "test-pci2")

	devs := map[string]device.Device{
		uuid1: mockDev1,
		uuid2: mockDev2,
	}

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	getECCModeEnabledFunc := func(uuid string, dev device.Device) (nvidianvml.ECCMode, error) {
		return nvidianvml.ECCMode{
			UUID:           uuid,
			EnabledCurrent: true,
			EnabledPending: true,
			Supported:      true,
		}, nil
	}

	getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (nvidianvml.ECCErrors, error) {
		return nvidianvml.ECCErrors{
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
		}, nil
	}

	component := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)
	result := component.Check()

	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "all 2 GPU(s) were checked, no ECC issue found", data.reason)
	assert.Len(t, data.ECCModes, 2)
	assert.Len(t, data.ECCErrors, 2)

	// Check that data for both GPUs was collected
	uuids := []string{data.ECCModes[0].UUID, data.ECCModes[1].UUID}
	assert.Contains(t, uuids, uuid1)
	assert.Contains(t, uuids, uuid2)
}
