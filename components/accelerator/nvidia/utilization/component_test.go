package utilization

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
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// mockInstance implements the nvidianvml.Instance interface for testing
type mockInstance struct {
	devices map[string]device.Device
}

func (m *mockInstance) NVMLExists() bool {
	return true
}

func (m *mockInstance) Library() nvml_lib.Library {
	return nil
}

func (m *mockInstance) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockInstance) ProductName() string {
	return "Test GPU"
}

func (m *mockInstance) Architecture() string {
	return "Test Architecture"
}

func (m *mockInstance) Brand() string {
	return "Test Brand"
}

func (m *mockInstance) DriverVersion() string {
	return ""
}

func (m *mockInstance) DriverMajor() int {
	return 0
}

func (m *mockInstance) CUDAVersion() string {
	return ""
}

func (m *mockInstance) FabricManagerSupported() bool {
	return true
}

func (m *mockInstance) FabricStateSupported() bool {
	return false
}

func (m *mockInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}

func (m *mockInstance) Shutdown() error {
	return nil
}

// MockUtilizationComponent creates a component with mocked functions for testing
func MockUtilizationComponent(
	ctx context.Context,
	getDevicesFunc func() map[string]device.Device,
	getUtilizationFunc func(uuid string, dev device.Device) (Utilization, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &mockInstance{
		devices: make(map[string]device.Device),
	}

	if getDevicesFunc != nil {
		mockInstance.devices = getDevicesFunc()
	}

	comp := &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:       mockInstance,
		getUtilizationFunc: getUtilizationFunc,
	}

	if getUtilizationFunc == nil {
		comp.getUtilizationFunc = GetUtilization
	}

	return comp
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockNVMLInstance := &mockInstance{
		devices: map[string]device.Device{},
	}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockNVMLInstance,
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

func TestTags(t *testing.T) {
	ctx := context.Background()
	c := MockUtilizationComponent(ctx, nil, nil)

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

	utilization := Utilization{
		UUID:              uuid,
		GPUUsedPercent:    85, // 85% GPU utilization
		MemoryUsedPercent: 70, // 70% Memory utilization
		Supported:         true,
	}

	getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
		return utilization, nil
	}

	component := MockUtilizationComponent(ctx, getDevicesFunc, getUtilizationFunc).(*component)
	result := component.Check()

	// Verify the data was collected
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
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
	getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
		return Utilization{}, errExpected
	}

	component := MockUtilizationComponent(ctx, getDevicesFunc, getUtilizationFunc).(*component)
	result := component.Check()

	// Verify error handling
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, data.err)
	assert.Equal(t, "error getting utilization", data.reason)
}

func TestCheck_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := MockUtilizationComponent(ctx, getDevicesFunc, nil).(*component)
	result := component.Check()

	// Verify handling of no devices
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no utilization issue found", data.reason)
	assert.Empty(t, data.Utilizations)
}

func TestLastHealthStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockUtilizationComponent(ctx, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		Utilizations: []Utilization{
			{
				UUID:              "gpu-uuid-123",
				GPUUsedPercent:    85,
				MemoryUsedPercent: 70,
				Supported:         true,
			},
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "checked 1 devices for utilization",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "checked 1 devices for utilization", state.Reason)
	assert.Contains(t, state.ExtraInfo["data"], "gpu-uuid-123")
}

func TestLastHealthStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockUtilizationComponent(ctx, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		err:    errors.New("test utilization error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting utilization",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting utilization", state.Reason)
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
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
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
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name:     "empty utilizations",
			data:     &checkResult{},
			expected: "no data",
		},
		{
			name: "with utilization data",
			data: &checkResult{
				Utilizations: []Utilization{
					{
						UUID:              "gpu-uuid-123",
						GPUUsedPercent:    75,
						MemoryUsedPercent: 60,
						Supported:         true,
					},
				},
			},
			expected: "", // We just check that it's not empty as the actual output depends on tablewriter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.String()

			if tt.expected == "" && tt.data != nil && len(tt.data.Utilizations) > 0 {
				// For the case with utilization data, just verify it's not empty
				assert.NotEmpty(t, result)
				assert.Contains(t, result, "GPU")
				assert.Contains(t, result, "USED %")
				assert.Contains(t, result, "MEMORY UTILIZATION")
			} else {
				assert.Equal(t, tt.expected, result)
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
		{
			name:     "empty data",
			data:     &checkResult{},
			expected: "",
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
			got := tt.data.HealthStateType()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCheck_NilNVMLInstance(t *testing.T) {
	ctx := context.Background()

	// Create component with nil NVML instance
	cctx, cancel := context.WithCancel(ctx)
	comp := &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: nil, // Explicitly nil
	}

	result := comp.Check()

	// Verify data
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)
	assert.Empty(t, data.Utilizations)
}

func TestCheck_NVMLNotExists(t *testing.T) {
	ctx := context.Background()

	// Create a custom mock instance with NVMLExists returning false
	mockInstance := &mockInstanceV2WithNVMLExists{
		mockInstance: &mockInstance{
			devices: map[string]device.Device{},
		},
		nvmlExists: false,
	}

	cctx, cancel := context.WithCancel(ctx)
	comp := &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: mockInstance,
	}

	result := comp.Check()

	// Verify data
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", data.reason)
	assert.Empty(t, data.Utilizations)
}

// mockInstanceV2WithNVMLExists is a mock that allows setting NVMLExists result
type mockInstanceV2WithNVMLExists struct {
	*mockInstance
	nvmlExists bool
}

func (m *mockInstanceV2WithNVMLExists) NVMLExists() bool {
	return m.nvmlExists
}

func TestCheck_MultipleGPUs(t *testing.T) {
	ctx := context.Background()

	// Create multiple mock devices
	uuid1 := "gpu-uuid-123"
	uuid2 := "gpu-uuid-456"

	mockDeviceObj1 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid1, nvml.SUCCESS
		},
	}
	mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "test-arch", "test-brand", "test-cuda", "test-pci")

	mockDeviceObj2 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid2, nvml.SUCCESS
		},
	}
	mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "test-arch", "test-brand", "test-cuda", "test-pci")

	devs := map[string]device.Device{
		uuid1: mockDev1,
		uuid2: mockDev2,
	}

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
		// Return different utilization values based on UUID
		if uuid == uuid1 {
			return Utilization{
				UUID:              uuid1,
				GPUUsedPercent:    75,
				MemoryUsedPercent: 60,
				Supported:         true,
			}, nil
		} else {
			return Utilization{
				UUID:              uuid2,
				GPUUsedPercent:    90,
				MemoryUsedPercent: 85,
				Supported:         true,
			}, nil
		}
	}

	component := MockUtilizationComponent(ctx, getDevicesFunc, getUtilizationFunc).(*component)
	result := component.Check()

	// Verify the data was collected for both GPUs
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "all 2 GPU(s) were checked, no utilization issue found", data.reason)
	assert.Len(t, data.Utilizations, 2)

	// Verify we have utilization data for both GPUs
	uuids := []string{}
	for _, util := range data.Utilizations {
		uuids = append(uuids, util.UUID)
	}
	assert.Contains(t, uuids, uuid1)
	assert.Contains(t, uuids, uuid2)

	// Check the values for each GPU
	for _, util := range data.Utilizations {
		switch util.UUID {
		case uuid1:
			assert.Equal(t, uint32(75), util.GPUUsedPercent)
			assert.Equal(t, uint32(60), util.MemoryUsedPercent)
		case uuid2:
			assert.Equal(t, uint32(90), util.GPUUsedPercent)
			assert.Equal(t, uint32(85), util.MemoryUsedPercent)
		}
	}
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

	// Use nvmlerrors.ErrGPULost for the error
	getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
		return Utilization{}, nvmlerrors.ErrGPULost
	}

	component := MockUtilizationComponent(ctx, getDevicesFunc, getUtilizationFunc).(*component)
	result := component.Check()

	// Verify error handling for GPU lost case
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.True(t, errors.Is(data.err, nvmlerrors.ErrGPULost), "error should be nvmlerrors.ErrGPULost")
	assert.Equal(t, nvmlerrors.ErrGPULost.Error(), data.reason)

	// Verify suggested actions for GPU lost case
	if assert.NotNil(t, data.suggestedActions) {
		assert.Equal(t, nvmlerrors.ErrGPULost.Error(), data.suggestedActions.Description)
		assert.Contains(t, data.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	}

	// Verify suggested actions propagates to health state output
	states := component.LastHealthStates()
	require.Len(t, states, 1)
	assert.NotNil(t, states[0].SuggestedActions)
	assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
}

func TestCheck_GPURequiresResetSuggestedActions(t *testing.T) {
	ctx := context.Background()

	uuid := "gpu-uuid-123"
	mockDeviceObj := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) { return uuid, nvml.SUCCESS },
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

	devs := map[string]device.Device{
		uuid: mockDev,
	}

	getDevicesFunc := func() map[string]device.Device { return devs }

	// Simulate NVML returning a code whose string is "GPU requires reset"
	originalErrorString := nvml.ErrorString
	nvml.ErrorString = func(ret nvml.Return) string {
		if ret == nvml.Return(5555) {
			return "GPU requires reset"
		}
		return originalErrorString(ret)
	}
	defer func() { nvml.ErrorString = originalErrorString }()

	// Return a Reset-like error via nvml.Return and mapping in GetUtilization
	getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
		// Use any API that would surface this return in underlying helper; directly return the mapped error here
		// because the utilization component only checks errors.Is on ErrGPURequiresReset
		return Utilization{}, nvmlerrors.ErrGPURequiresReset
	}

	component := MockUtilizationComponent(ctx, getDevicesFunc, getUtilizationFunc).(*component)
	result := component.Check()

	// Verify check result carries suggested actions
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")
	require.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.True(t, errors.Is(data.err, nvmlerrors.ErrGPURequiresReset))
	assert.Equal(t, nvmlerrors.ErrGPURequiresReset.Error(), data.reason)
	if assert.NotNil(t, data.suggestedActions) {
		assert.Equal(t, nvmlerrors.ErrGPURequiresReset.Error(), data.suggestedActions.Description)
		assert.Contains(t, data.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	}

	// Verify suggested actions propagates to health state output
	states := component.LastHealthStates()
	require.Len(t, states, 1)
	assert.NotNil(t, states[0].SuggestedActions)
	assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
}
