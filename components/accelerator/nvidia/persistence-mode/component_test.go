package persistencemode

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// mockNVMLInstance implements the nvml.InstanceV2 interface for testing
type mockNVMLInstance struct {
	devicesFunc      func() map[string]device.Device
	nvmlExists       bool
	emptyProductName bool
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	if m.devicesFunc != nil {
		return m.devicesFunc()
	}
	return nil
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{
		ErrorContainment:     true,
		DynamicPageOfflining: true,
		RowRemapping:         true,
	}
}

func (m *mockNVMLInstance) ProductName() string {
	if m.emptyProductName {
		return ""
	}
	return "NVIDIA Test GPU"
}

func (m *mockNVMLInstance) Architecture() string {
	return ""
}

func (m *mockNVMLInstance) Brand() string {
	return ""
}

func (m *mockNVMLInstance) DriverVersion() string {
	return ""
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 0
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return ""
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return true
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.nvmlExists
}

func (m *mockNVMLInstance) Library() lib.Library {
	return nil
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

// mockComponent creates a component with mocked functions for testing
func mockComponent(
	ctx context.Context,
	devicesFunc func() map[string]device.Device,
	getPersistenceModeFunc func(uuid string, dev device.Device) (nvidianvml.PersistenceMode, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &mockNVMLInstance{
		devicesFunc: devicesFunc,
		nvmlExists:  true,
	}

	return &component{
		ctx:                    cctx,
		cancel:                 cancel,
		nvmlInstance:           mockInstance,
		getPersistenceModeFunc: getPersistenceModeFunc,
	}
}

// MockPersistenceModeComponentWithNVMLExists creates a component with control over NVMLExists
func MockPersistenceModeComponentWithNVMLExists(
	ctx context.Context,
	devicesFunc func() map[string]device.Device,
	getPersistenceModeFunc func(uuid string, dev device.Device) (nvidianvml.PersistenceMode, error),
	nvmlExists bool,
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &mockNVMLInstance{
		devicesFunc: devicesFunc,
		nvmlExists:  nvmlExists,
	}

	return &component{
		ctx:                    cctx,
		cancel:                 cancel,
		nvmlInstance:           mockInstance,
		getPersistenceModeFunc: getPersistenceModeFunc,
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
	assert.NotNil(t, tc.getPersistenceModeFunc, "getPersistenceModeFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := mockComponent(ctx, nil, nil)
	assert.Equal(t, Name, c.Name(), "Component name should match")
}

func TestTags(t *testing.T) {
	ctx := context.Background()
	c := mockComponent(ctx, nil, nil)

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 4, "Component should return exactly 4 tags")
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

	persistenceMode := nvidianvml.PersistenceMode{
		UUID:    uuid,
		Enabled: true,
	}

	getPersistenceModeFunc := func(uuid string, dev device.Device) (nvidianvml.PersistenceMode, error) {
		return persistenceMode, nil
	}

	component := mockComponent(ctx, getDevicesFunc, getPersistenceModeFunc).(*component)
	result := component.Check()

	// Verify the data was collected
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no persistence mode issue found", data.reason)
	assert.Len(t, data.PersistenceModes, 1)
	assert.Equal(t, persistenceMode, data.PersistenceModes[0])
}

func TestCheck_PersistenceModeError(t *testing.T) {
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

	errExpected := errors.New("persistence mode error")
	getPersistenceModeFunc := func(uuid string, dev device.Device) (nvidianvml.PersistenceMode, error) {
		return nvidianvml.PersistenceMode{}, errExpected
	}

	component := mockComponent(ctx, getDevicesFunc, getPersistenceModeFunc).(*component)
	result := component.Check()

	// Verify error handling
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, data.err)
	assert.Equal(t, "error getting persistence mode", data.reason)
}

func TestCheck_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := mockComponent(ctx, getDevicesFunc, nil).(*component)
	result := component.Check()

	// Verify handling of no devices
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no persistence mode issue found", data.reason)
	assert.Empty(t, data.PersistenceModes)
}

func TestCheck_MultipleDevices(t *testing.T) {
	ctx := context.Background()

	uuid1 := "gpu-uuid-123"
	uuid2 := "gpu-uuid-456"

	mockDevice1 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid1, nvml.SUCCESS
		},
	}
	mockDev1 := testutil.NewMockDevice(mockDevice1, "test-arch", "test-brand", "test-cuda", "test-pci")

	mockDevice2 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid2, nvml.SUCCESS
		},
	}
	mockDev2 := testutil.NewMockDevice(mockDevice2, "test-arch", "test-brand", "test-cuda", "test-pci")

	devs := map[string]device.Device{
		uuid1: mockDev1,
		uuid2: mockDev2,
	}

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	getPersistenceModeFunc := func(uuid string, dev device.Device) (nvidianvml.PersistenceMode, error) {
		return nvidianvml.PersistenceMode{
			UUID:      uuid,
			Enabled:   uuid == uuid1, // First device has persistence mode enabled
			Supported: true,
		}, nil
	}

	component := mockComponent(ctx, getDevicesFunc, getPersistenceModeFunc).(*component)
	result := component.Check()

	// Verify the data was collected for both devices
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	// This should be unhealthy since one device has persistence mode supported but not enabled
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.Equal(t, "gpu-uuid-456 persistence mode supported but not enabled", data.reason)
	assert.Len(t, data.PersistenceModes, 2)

	// Verify each device's persistence mode status
	var device1Mode, device2Mode nvidianvml.PersistenceMode
	for _, mode := range data.PersistenceModes {
		if mode.UUID == uuid1 {
			device1Mode = mode
		} else if mode.UUID == uuid2 {
			device2Mode = mode
		}
	}

	assert.Equal(t, uuid1, device1Mode.UUID)
	assert.True(t, device1Mode.Enabled)
	assert.True(t, device1Mode.Supported)

	assert.Equal(t, uuid2, device2Mode.UUID)
	assert.False(t, device2Mode.Enabled)
	assert.True(t, device2Mode.Supported)
}

func TestCheck_NilNVMLInstance(t *testing.T) {
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)

	component := &component{
		ctx:          cctx,
		cancel:       cancel,
		nvmlInstance: nil, // Explicitly set to nil
	}

	result := component.Check()

	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)
}

func TestCheck_NVMLNotExists(t *testing.T) {
	ctx := context.Background()

	component := MockPersistenceModeComponentWithNVMLExists(
		ctx,
		nil,
		nil,
		false, // NVML doesn't exist
	).(*component)

	result := component.Check()

	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", data.reason)
}

func TestLastHealthStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := mockComponent(ctx, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		PersistenceModes: []nvidianvml.PersistenceMode{
			{
				UUID:    "gpu-uuid-123",
				Enabled: true,
			},
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "all 1 GPU(s) were checked, no persistence mode issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "all 1 GPU(s) were checked, no persistence mode issue found", state.Reason)
	assert.Contains(t, state.ExtraInfo["data"], "gpu-uuid-123")
}

func TestLastHealthStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := mockComponent(ctx, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		err:    errors.New("test persistence mode error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting persistence mode",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting persistence mode", state.Reason)
	assert.Equal(t, "test persistence mode error", state.Error)
}

func TestLastHealthStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := mockComponent(ctx, nil, nil).(*component)

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
	component := mockComponent(ctx, nil, nil)

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

	component := mockComponent(ctx, getDevicesFunc, nil)

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
	component := mockComponent(ctx, nil, nil).(*component)

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
		contains []string
	}{
		{
			name:     "nil data",
			data:     nil,
			contains: []string{""},
		},
		{
			name:     "empty data",
			data:     &checkResult{},
			contains: []string{"no data"},
		},
		{
			name: "with persistence modes",
			data: &checkResult{
				PersistenceModes: []nvidianvml.PersistenceMode{
					{
						UUID:      "gpu-uuid-123",
						BusID:     "0000:01:00.0",
						Enabled:   true,
						Supported: true,
					},
					{
						UUID:      "gpu-uuid-456",
						BusID:     "0000:02:00.0",
						Enabled:   false,
						Supported: true,
					},
				},
			},
			contains: []string{
				"GPU UUID", "GPU BUS ID", "PERSISTENCE MODE ENABLED", "PERSISTENCE MODE SUPPORTED",
				"gpu-uuid-123", "0000:01:00.0", "true", "true",
				"gpu-uuid-456", "0000:02:00.0", "false", "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.String()
			for _, substr := range tt.contains {
				assert.Contains(t, result, substr)
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

func TestIsSupported(t *testing.T) {
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Test with nil NVML instance
	comp := &component{
		ctx:          cctx,
		cancel:       cancel,
		nvmlInstance: nil, // Explicitly nil
	}
	assert.False(t, comp.IsSupported())

	// Test with NVML instance that doesn't exist
	mockNVML := &mockNVMLInstance{
		nvmlExists: false,
	}
	comp.nvmlInstance = mockNVML
	assert.False(t, comp.IsSupported())

	// Test with NVML that exists but has empty product name
	mockNVML = &mockNVMLInstance{
		nvmlExists:       true,
		emptyProductName: true,
	}
	comp.nvmlInstance = mockNVML
	assert.False(t, comp.IsSupported())

	// Test with NVML that exists and has a product name
	mockNVML = &mockNVMLInstance{
		nvmlExists:       true,
		emptyProductName: false,
	}
	comp.nvmlInstance = mockNVML
	assert.True(t, comp.IsSupported())
}

func TestPersistenceModeCheck(t *testing.T) {
	tests := []struct {
		name              string
		persistenceModes  []nvidianvml.PersistenceMode
		expectedHealth    apiv1.HealthStateType
		expectedReasonCmp string
	}{
		{
			name: "all GPUs have persistence mode supported and enabled",
			persistenceModes: []nvidianvml.PersistenceMode{
				{UUID: "GPU-1", Supported: true, Enabled: true},
				{UUID: "GPU-2", Supported: true, Enabled: true},
			},
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReasonCmp: "all 2 GPU(s) were checked, no persistence mode issue found",
		},
		{
			name: "some GPUs have persistence mode supported but not enabled",
			persistenceModes: []nvidianvml.PersistenceMode{
				{UUID: "GPU-1", Supported: true, Enabled: true},
				{UUID: "GPU-2", Supported: true, Enabled: false},
			},
			expectedHealth:    apiv1.HealthStateTypeUnhealthy,
			expectedReasonCmp: "GPU-2 persistence mode supported but not enabled",
		},
		{
			name: "all GPUs have persistence mode supported but not enabled",
			persistenceModes: []nvidianvml.PersistenceMode{
				{UUID: "GPU-1", Supported: true, Enabled: false},
				{UUID: "GPU-2", Supported: true, Enabled: false},
			},
			expectedHealth:    apiv1.HealthStateTypeUnhealthy,
			expectedReasonCmp: "all 2 GPU(s) disabled persistence mode",
		},
		{
			name: "GPU has persistence mode not supported",
			persistenceModes: []nvidianvml.PersistenceMode{
				{UUID: "GPU-1", Supported: false, Enabled: false},
			},
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReasonCmp: "all 1 GPU(s) were checked, no persistence mode issue found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create device map for test
			devs := make(map[string]device.Device)
			for _, pm := range tt.persistenceModes {
				mockDeviceObj := &mock.Device{
					GetUUIDFunc: func() (string, nvml.Return) {
						return pm.UUID, nvml.SUCCESS
					},
				}
				devs[pm.UUID] = testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")
			}

			// Mock NVML instance
			mockNVML := &mockNVMLInstance{
				devicesFunc: func() map[string]device.Device {
					return devs
				},
				nvmlExists: true, // Set this to true to pass the NVML existence check
			}

			// Create component with mocked functions
			c := &component{
				nvmlInstance: mockNVML,
				getPersistenceModeFunc: func(uuid string, dev device.Device) (nvidianvml.PersistenceMode, error) {
					for _, pm := range tt.persistenceModes {
						if pm.UUID == uuid {
							return pm, nil
						}
					}
					return nvidianvml.PersistenceMode{}, nil
				},
			}

			result := c.Check().(*checkResult)

			// Verify the result
			assert.Equal(t, tt.expectedHealth, result.health)
			assert.Equal(t, tt.expectedReasonCmp, result.reason)
		})
	}
}

func TestComponentName(t *testing.T) {
	result := &checkResult{}
	assert.Equal(t, Name, result.ComponentName())
}

func TestCheck_EmptyProductName(t *testing.T) {
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mockNVML := &mockNVMLInstance{
		nvmlExists:       true,
		emptyProductName: true,
	}

	component := &component{
		ctx:          cctx,
		cancel:       cancel,
		nvmlInstance: mockNVML,
	}

	result := component.Check()

	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML is loaded but GPU is not detected (missing product name)", data.reason)
}

func TestCheck_GPULostError(t *testing.T) {
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

	// Use nvidianvml.ErrGPULost for the error
	getPersistenceModeFunc := func(uuid string, dev device.Device) (nvidianvml.PersistenceMode, error) {
		return nvidianvml.PersistenceMode{}, nvidianvml.ErrGPULost
	}

	component := mockComponent(ctx, getDevicesFunc, getPersistenceModeFunc).(*component)
	result := component.Check()

	// Verify error handling for GPU lost case
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.True(t, errors.Is(data.err, nvidianvml.ErrGPULost), "error should be nvidianvml.ErrGPULost")
	assert.Equal(t, "error getting persistence mode", data.reason,
		"reason should have '(GPU is lost)' suffix")
}
