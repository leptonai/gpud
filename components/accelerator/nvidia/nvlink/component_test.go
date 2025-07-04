package nvlink

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
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
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
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) ProductName() string {
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

func (m *mockNVMLInstance) NVMLExists() bool {
	return true
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

// mockNVMLInstanceNVMLNotExists is a special mock for the case where NVMLExists returns false
type mockNVMLInstanceNVMLNotExists struct {
	mockNVMLInstance
}

func (m *mockNVMLInstanceNVMLNotExists) NVMLExists() bool {
	return false
}

// MockNVLinkComponent creates a component with mocked functions for testing
func MockNVLinkComponent(
	ctx context.Context,
	devicesFunc func() map[string]device.Device,
	getNVLinkFunc func(uuid string, dev device.Device) (nvidianvml.NVLink, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &mockNVMLInstance{
		devicesFunc: devicesFunc,
	}

	return &component{
		ctx:           cctx,
		cancel:        cancel,
		nvmlInstance:  mockInstance,
		getNVLinkFunc: getNVLinkFunc,
	}
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockInstance := &mockNVMLInstance{
		devicesFunc: func() map[string]device.Device { return nil },
	}

	// Create a GPUdInstance for the New function
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockInstance,
	}

	c, err := New(gpudInstance)

	assert.NotNil(t, c, "New should return a non-nil component")
	assert.NoError(t, err, "New should not return an error")
	assert.Equal(t, Name, c.Name(), "Component name should match")

	// Type assertion to access internal fields
	tc, ok := c.(*component)
	require.True(t, ok, "Component should be of type *component")

	assert.NotNil(t, tc.ctx, "Context should be set")
	assert.NotNil(t, tc.cancel, "Cancel function should be set")
	assert.NotNil(t, tc.nvmlInstance, "nvmlInstance should be set")
	assert.NotNil(t, tc.getNVLinkFunc, "getNVLinkFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockNVLinkComponent(ctx, nil, nil)
	assert.Equal(t, Name, c.Name(), "Component name should match")
}

func TestTags(t *testing.T) {
	ctx := context.Background()
	c := MockNVLinkComponent(ctx, nil, nil)

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

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	nvLinkState := nvidianvml.NVLinkState{
		Link:           0,
		FeatureEnabled: true,
		ReplayErrors:   5,
		RecoveryErrors: 2,
		CRCErrors:      1,
	}

	nvLink := nvidianvml.NVLink{
		UUID:   uuid,
		States: []nvidianvml.NVLinkState{nvLinkState, nvLinkState},
	}

	getNVLinkFunc := func(uuid string, dev device.Device) (nvidianvml.NVLink, error) {
		return nvLink, nil
	}

	component := MockNVLinkComponent(ctx, getDevicesFunc, getNVLinkFunc).(*component)
	_ = component.Check()

	// Verify the data was collected
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Equal(t, "all 1 GPU(s) were checked, no nvlink issue found", lastCheckResult.reason)
	assert.Len(t, lastCheckResult.NVLinks, 1)
	assert.Equal(t, nvLink, lastCheckResult.NVLinks[0])
}

func TestCheckOnce_NVLinkError(t *testing.T) {
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

	errExpected := errors.New("NVLink error")
	getNVLinkFunc := func(uuid string, dev device.Device) (nvidianvml.NVLink, error) {
		return nvidianvml.NVLink{}, errExpected
	}

	component := MockNVLinkComponent(ctx, getDevicesFunc, getNVLinkFunc).(*component)
	_ = component.Check()

	// Verify error handling
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, lastCheckResult.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastCheckResult.err)
	assert.Equal(t, "error getting nvlink", lastCheckResult.reason)
}

func TestCheckOnce_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := MockNVLinkComponent(ctx, getDevicesFunc, nil).(*component)
	_ = component.Check()

	// Verify handling of no devices
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no nvlink issue found", lastCheckResult.reason)
	assert.Empty(t, lastCheckResult.NVLinks)
}

func TestStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockNVLinkComponent(ctx, nil, nil).(*component)

	// Set test data
	nvLinkState := nvidianvml.NVLinkState{
		Link:           0,
		FeatureEnabled: true,
		ReplayErrors:   0,
		RecoveryErrors: 0,
		CRCErrors:      0,
	}

	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		NVLinks: []nvidianvml.NVLink{
			{
				UUID:   "gpu-uuid-123",
				States: []nvidianvml.NVLinkState{nvLinkState, nvLinkState},
			},
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "all 1 GPU(s) were checked, no nvlink issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "all 1 GPU(s) were checked, no nvlink issue found", state.Reason)
	assert.Contains(t, state.ExtraInfo["data"], "gpu-uuid-123")
}

func TestStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockNVLinkComponent(ctx, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		err:    errors.New("test NVLink error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting nvlink",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting nvlink", state.Reason)
	assert.Equal(t, "test NVLink error", state.Error)
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockNVLinkComponent(ctx, nil, nil).(*component)

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
	component := MockNVLinkComponent(ctx, nil, nil)

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

	component := MockNVLinkComponent(ctx, getDevicesFunc, nil)

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
	component := MockNVLinkComponent(ctx, nil, nil).(*component)

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
		contains string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name:     "empty nvlinks",
			data:     &checkResult{},
			expected: "no data",
		},
		{
			name: "with nvlink data",
			data: &checkResult{
				NVLinks: []nvidianvml.NVLink{
					{
						UUID:      "gpu-uuid-123",
						BusID:     "0000:01:00.0",
						Supported: true,
						States: []nvidianvml.NVLinkState{
							{
								Link:           0,
								FeatureEnabled: true,
							},
						},
					},
				},
			},
			contains: "gpu-uuid-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.String()
			if tt.expected != "" {
				assert.Equal(t, tt.expected, got)
			}
			if tt.contains != "" {
				assert.Contains(t, got, tt.contains)
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
		{
			name:     "empty reason",
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

func TestCheckOnce_NilNVMLInstance(t *testing.T) {
	ctx := context.Background()

	// Create component with nil nvmlInstance
	component := &component{
		ctx:          ctx,
		nvmlInstance: nil,
	}

	result := component.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)
}

func TestCheckOnce_NVMLNotLoaded(t *testing.T) {
	ctx := context.Background()

	// Use specialized mock instance where NVMLExists returns false
	mockInstance := &mockNVMLInstanceNVMLNotExists{
		mockNVMLInstance: mockNVMLInstance{
			devicesFunc: func() map[string]device.Device { return nil },
		},
	}

	component := &component{
		ctx:          ctx,
		nvmlInstance: mockInstance,
	}

	result := component.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", data.reason)
}

func TestData_getLastHealthStates(t *testing.T) {
	tests := []struct {
		name           string
		data           *checkResult
		expectedHealth apiv1.HealthStateType
		expectedReason string
		expectedError  string
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "no data yet",
		},
		{
			name: "healthy data",
			data: &checkResult{
				NVLinks: []nvidianvml.NVLink{
					{
						UUID:      "gpu-uuid-123",
						Supported: true,
						States:    []nvidianvml.NVLinkState{},
					},
				},
				health: apiv1.HealthStateTypeHealthy,
				reason: "all good",
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "all good",
		},
		{
			name: "unhealthy data with error",
			data: &checkResult{
				NVLinks: []nvidianvml.NVLink{
					{
						UUID:      "gpu-uuid-123",
						Supported: true,
						States:    []nvidianvml.NVLinkState{},
					},
				},
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "something wrong",
				err:    errors.New("test error"),
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedReason: "something wrong",
			expectedError:  "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := tt.data.HealthStates()
			require.Len(t, states, 1)

			state := states[0]
			assert.Equal(t, Name, state.Name)
			assert.Equal(t, tt.expectedHealth, state.Health)
			assert.Equal(t, tt.expectedReason, state.Reason)
			assert.Equal(t, tt.expectedError, state.Error)

			// Check that extraInfo is properly populated for non-nil data
			if tt.data != nil {
				assert.NotEmpty(t, state.ExtraInfo["data"])
			}
		})
	}
}

func TestCheck_MetricsGeneration(t *testing.T) {
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

	// Create NVLink data with specific error counts to check metric values
	nvLinkStates := []nvidianvml.NVLinkState{
		{
			Link:           0,
			FeatureEnabled: true,
			ReplayErrors:   5,
			RecoveryErrors: 3,
			CRCErrors:      2,
		},
		{
			Link:           1,
			FeatureEnabled: false,
			ReplayErrors:   1,
			RecoveryErrors: 2,
			CRCErrors:      3,
		},
	}

	nvLink := nvidianvml.NVLink{
		UUID:      uuid,
		Supported: true,
		States:    nvLinkStates,
	}

	getNVLinkFunc := func(uuid string, dev device.Device) (nvidianvml.NVLink, error) {
		return nvLink, nil
	}

	// Create a component and run Check
	component := MockNVLinkComponent(ctx, getDevicesFunc, getNVLinkFunc).(*component)
	_ = component.Check()

	// Verify the data was collected
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Len(t, lastCheckResult.NVLinks, 1)

	// The actual metrics are set using prometheus counters which we can't directly test here
	// without additional mocking, but we can at least ensure the right structure is in place
	// and the feature enabled state is correctly determined
	assert.False(t, lastCheckResult.NVLinks[0].States.AllFeatureEnabled(),
		"AllFeatureEnabled should be false since not all links have FeatureEnabled=true")
	assert.Equal(t, uint64(6), lastCheckResult.NVLinks[0].States.TotalReplayErrors(),
		"TotalReplayErrors should match the sum")
	assert.Equal(t, uint64(5), lastCheckResult.NVLinks[0].States.TotalRecoveryErrors(),
		"TotalRecoveryErrors should match the sum")
	assert.Equal(t, uint64(5), lastCheckResult.NVLinks[0].States.TotalCRCErrors(),
		"TotalCRCErrors should match the sum")
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
	getNVLinkFunc := func(uuid string, dev device.Device) (nvidianvml.NVLink, error) {
		return nvidianvml.NVLink{}, nvidianvml.ErrGPULost
	}

	component := MockNVLinkComponent(ctx, getDevicesFunc, getNVLinkFunc).(*component)
	result := component.Check()

	// Verify error handling for GPU lost case
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.True(t, errors.Is(data.err, nvidianvml.ErrGPULost), "error should be nvidianvml.ErrGPULost")
	assert.Equal(t, "error getting nvlink", data.reason,
		"reason should have '(GPU is lost)' suffix")
}
