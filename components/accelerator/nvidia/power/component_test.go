// Package power tracks the NVIDIA per-GPU power usage.
package power

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
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// MockPowerComponent creates a component with mocked functions for testing
func MockPowerComponent(
	ctx context.Context,
	mockNVMLInstance *mockNVMLInstance,
	getPowerFunc func(uuid string, dev device.Device) (nvidianvml.Power, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: mockNVMLInstance,
		getPowerFunc: getPowerFunc,
	}
}

// mockNVMLInstance implements InstanceV2 interface for testing
type mockNVMLInstance struct {
	devices map[string]device.Device
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return true
}

func (m *mockNVMLInstance) Library() nvml_lib.Library {
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockNVMLInstance) ProductName() string {
	return "Test GPU"
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

func (m *mockNVMLInstance) FabricStateSupported() bool {
	return false
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockInstance := &mockNVMLInstance{devices: make(map[string]device.Device)}

	// Create a mock GPUdInstance
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
	assert.NotNil(t, tc.getPowerFunc, "getPowerFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockPowerComponent(ctx, nil, nil)
	assert.Equal(t, Name, c.Name(), "Component name should match")
}

func TestTags(t *testing.T) {
	ctx := context.Background()
	c := MockPowerComponent(ctx, nil, nil)

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

	mockNvml := &mockNVMLInstance{
		devices: devs,
	}

	power := nvidianvml.Power{
		UUID:                             uuid,
		UsageMilliWatts:                  150000,  // 150W
		EnforcedLimitMilliWatts:          250000,  // 250W
		ManagementLimitMilliWatts:        300000,  // 300W
		UsedPercent:                      "60.00", // Important: Must be set for GetUsedPercent
		GetPowerUsageSupported:           true,
		GetPowerLimitSupported:           true,
		GetPowerManagementLimitSupported: true,
	}

	getPowerFunc := func(uuid string, dev device.Device) (nvidianvml.Power, error) {
		return power, nil
	}

	component := MockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
	result := component.Check()

	// Cast the result to *checkResult
	lastCheckResult := result.(*checkResult)

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no power issue found", lastCheckResult.reason)
	assert.Len(t, lastCheckResult.Powers, 1)
	assert.Equal(t, power, lastCheckResult.Powers[0])
}

func TestCheckOnce_PowerError(t *testing.T) {
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

	mockNvml := &mockNVMLInstance{
		devices: devs,
	}

	errExpected := errors.New("power error")
	getPowerFunc := func(uuid string, dev device.Device) (nvidianvml.Power, error) {
		return nvidianvml.Power{}, errExpected
	}

	component := MockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
	result := component.Check()

	// Cast the result to *checkResult
	lastCheckResult := result.(*checkResult)

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, lastCheckResult.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastCheckResult.err)
	assert.Equal(t, "error getting power", lastCheckResult.reason)
}

func TestCheckOnce_NoDevices(t *testing.T) {
	ctx := context.Background()

	mockNvml := &mockNVMLInstance{
		devices: map[string]device.Device{}, // Empty map
	}

	component := MockPowerComponent(ctx, mockNvml, nil).(*component)
	result := component.Check()

	// Cast the result to *checkResult
	lastCheckResult := result.(*checkResult)

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no power issue found", lastCheckResult.reason)
	assert.Empty(t, lastCheckResult.Powers)
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

	mockNvml := &mockNVMLInstance{
		devices: devs,
	}

	// Create power data with invalid UsedPercent format
	invalidPower := nvidianvml.Power{
		UUID:                             uuid,
		UsageMilliWatts:                  150000,
		EnforcedLimitMilliWatts:          250000,
		ManagementLimitMilliWatts:        300000,
		UsedPercent:                      "invalid", // Will cause ParseFloat to fail
		GetPowerUsageSupported:           true,
		GetPowerLimitSupported:           true,
		GetPowerManagementLimitSupported: true,
	}

	getPowerFunc := func(uuid string, dev device.Device) (nvidianvml.Power, error) {
		return invalidPower, nil
	}

	component := MockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
	result := component.Check()

	// Cast the result to *checkResult
	lastCheckResult := result.(*checkResult)

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, lastCheckResult.health, "data should be marked unhealthy")
	assert.NotNil(t, lastCheckResult.err)
	assert.Equal(t, "error getting used percent", lastCheckResult.reason)
}

func TestStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockPowerComponent(ctx, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		Powers: []nvidianvml.Power{
			{
				UUID:                             "gpu-uuid-123",
				UsageMilliWatts:                  150000, // 150W
				EnforcedLimitMilliWatts:          250000, // 250W
				ManagementLimitMilliWatts:        300000, // 300W
				UsedPercent:                      "60.00",
				GetPowerUsageSupported:           true,
				GetPowerLimitSupported:           true,
				GetPowerManagementLimitSupported: true,
			},
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "all 1 GPU(s) were checked, no power issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "all 1 GPU(s) were checked, no power issue found", state.Reason)
	assert.Contains(t, state.ExtraInfo["data"], "gpu-uuid-123")
}

func TestStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockPowerComponent(ctx, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		err:    errors.New("test power error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting power",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting power", state.Reason)
	assert.Equal(t, "test power error", state.Error)
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockPowerComponent(ctx, nil, nil).(*component)

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
	component := MockPowerComponent(ctx, nil, nil)

	events, err := component.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock functions that count calls
	callCount := &atomic.Int32{}
	mockNvml := &mockNVMLInstance{
		devices: map[string]device.Device{
			"gpu-uuid-123": testutil.NewMockDevice(nil, "test-arch", "test-brand", "test-cuda", "test-pci"),
		},
	}

	component := MockPowerComponent(ctx, mockNvml, func(uuid string, dev device.Device) (nvidianvml.Power, error) {
		callCount.Add(1)
		return nvidianvml.Power{}, nil
	})

	// Start should be non-blocking
	err := component.Start()
	assert.NoError(t, err)

	// Give the goroutine time to execute Check at least once
	time.Sleep(time.Second)

	// Verify Check was called
	assert.GreaterOrEqual(t, callCount.Load(), int32(1), "Check should have been called at least once")
}

func TestClose(t *testing.T) {
	ctx := context.Background()
	component := MockPowerComponent(ctx, nil, nil).(*component)

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

	mockNvml := &mockNVMLInstance{
		devices: devs,
	}

	// Use nvidianvml.ErrGPULost for the error
	getPowerFunc := func(uuid string, dev device.Device) (nvidianvml.Power, error) {
		return nvidianvml.Power{}, nvidianvml.ErrGPULost
	}

	component := MockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
	result := component.Check()

	// Verify error handling for GPU lost case
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.True(t, errors.Is(data.err, nvidianvml.ErrGPULost), "error should be nvidianvml.ErrGPULost")
	assert.Equal(t, nvidianvml.ErrGPULost.Error(), data.reason)

	// Verify suggested actions for GPU lost case
	if assert.NotNil(t, data.suggestedActions) {
		assert.Equal(t, nvidianvml.ErrGPULost.Error(), data.suggestedActions.Description)
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

	mockNvml := &mockNVMLInstance{
		devices: devs,
	}

	// Simulate NVML returning a code whose string is "GPU requires reset"
	originalErrorString := nvml.ErrorString
	nvml.ErrorString = func(ret nvml.Return) string {
		if ret == nvml.Return(5555) {
			return "GPU requires reset"
		}
		return originalErrorString(ret)
	}
	defer func() { nvml.ErrorString = originalErrorString }()

	// Return a Reset-like error via nvml.Return and mapping in GetPower
	getPowerFunc := func(uuid string, dev device.Device) (nvidianvml.Power, error) {
		// Use any API that would surface this return in underlying helper; directly return the mapped error here
		// because the power component only checks errors.Is on ErrGPURequiresReset
		return nvidianvml.Power{}, nvidianvml.ErrGPURequiresReset
	}

	component := MockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
	result := component.Check()

	// Verify check result carries suggested actions
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")
	require.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.True(t, errors.Is(data.err, nvidianvml.ErrGPURequiresReset))
	assert.Equal(t, "GPU requires reset", data.reason)
	if assert.NotNil(t, data.suggestedActions) {
		assert.Equal(t, "GPU requires reset", data.suggestedActions.Description)
		assert.Contains(t, data.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	}

	// Verify suggested actions propagates to health state output
	states := component.LastHealthStates()
	require.Len(t, states, 1)
	assert.NotNil(t, states[0].SuggestedActions)
	assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
}
