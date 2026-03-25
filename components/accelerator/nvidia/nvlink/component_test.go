package nvlink

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
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

func (m *mockNVMLInstance) FabricStateSupported() bool {
	return false
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
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

func (m *mockNVMLInstance) InitError() error {
	return nil
}

type mockNVMLInstanceNoFabric struct {
	mockNVMLInstance
}

func (m *mockNVMLInstanceNoFabric) FabricManagerSupported() bool {
	return false
}

func (m *mockNVMLInstanceNoFabric) FabricStateSupported() bool {
	return false
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
	getNVLinkFunc func(uuid string, dev device.Device) (NVLink, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &mockNVMLInstance{
		devicesFunc: devicesFunc,
	}

	return &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:      mockInstance,
		getNVLinkFunc:     getNVLinkFunc,
		getThresholdsFunc: GetDefaultExpectedLinkStates,
	}
}

func mustComponent(t *testing.T, c components.Component) *component {
	t.Helper()

	component, ok := c.(*component)
	require.True(t, ok)
	return component
}

func readGaugeValue(t *testing.T, gauge prometheus.Gauge) float64 {
	t.Helper()

	var metric dto.Metric
	require.NoError(t, gauge.Write(&metric))
	require.NotNil(t, metric.Gauge)
	return metric.Gauge.GetValue()
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
	assert.NotNil(t, tc.getPeerNVLinkP2PStatusFn, "getPeerNVLinkP2PStatusFn should be set")
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

	nvLinkState := NVLinkState{
		Link:           0,
		FeatureEnabled: true,
		ReplayErrors:   5,
		RecoveryErrors: 2,
		CRCErrors:      1,
	}

	nvLink := NVLink{
		UUID:   uuid,
		States: []NVLinkState{nvLinkState, nvLinkState},
	}

	getNVLinkFunc := func(_ string, _ device.Device) (NVLink, error) {
		return nvLink, nil
	}

	componentAny := MockNVLinkComponent(ctx, getDevicesFunc, getNVLinkFunc)
	component, ok := componentAny.(*component)
	require.True(t, ok)
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
	getNVLinkFunc := func(_ string, _ device.Device) (NVLink, error) {
		return NVLink{}, errExpected
	}

	componentAny := MockNVLinkComponent(ctx, getDevicesFunc, getNVLinkFunc)
	component, ok := componentAny.(*component)
	require.True(t, ok)
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

	component := mustComponent(t, MockNVLinkComponent(ctx, getDevicesFunc, nil))
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
	component := mustComponent(t, MockNVLinkComponent(ctx, nil, nil))

	// Set test data
	nvLinkState := NVLinkState{
		Link:           0,
		FeatureEnabled: true,
		ReplayErrors:   0,
		RecoveryErrors: 0,
		CRCErrors:      0,
	}

	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		NVLinks: []NVLink{
			{
				UUID:   "gpu-uuid-123",
				States: []NVLinkState{nvLinkState, nvLinkState},
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
	component := mustComponent(t, MockNVLinkComponent(ctx, nil, nil))

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

func TestStates_WithSuggestedActions(t *testing.T) {
	ctx := context.Background()
	component := mustComponent(t, MockNVLinkComponent(ctx, nil, nil))

	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		ts:     time.Now().UTC(),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "nvlink threshold violated: require >=1 GPUs with all links active; got 0",
		suggestedActions: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
		},
	}
	component.lastMu.Unlock()

	states := component.LastHealthStates()
	require.Len(t, states, 1)

	state := states[0]
	if assert.NotNil(t, state.SuggestedActions) {
		assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, state.SuggestedActions.RepairActions)
	}
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := mustComponent(t, MockNVLinkComponent(ctx, nil, nil))

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
	ctx := t.Context()

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
	component := mustComponent(t, MockNVLinkComponent(ctx, nil, nil))

	err := component.Close()
	assert.NoError(t, err)

	// Check that context is canceled
	select {
	case <-component.ctx.Done():
		// Context is properly canceled
	default:
		require.FailNow(t, "component context was not canceled on Close")
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
				NVLinks: []NVLink{
					{
						UUID:      "gpu-uuid-123",
						BusID:     "0000:01:00.0",
						Supported: true,
						States: []NVLinkState{
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
		{
			name: "with peer nvlink p2p data",
			data: &checkResult{
				NVLinks: []NVLink{
					{
						UUID:      "gpu-uuid-123",
						BusID:     "0000:01:00.0",
						Supported: true,
						States: []NVLinkState{
							{
								Link:           0,
								FeatureEnabled: true,
							},
						},
					},
				},
				PeerNVLinkProbePairCount: 1,
			},
			contains: "NVLINK P2P OK",
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
		ctx: ctx,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
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
		ctx: ctx,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
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
				NVLinks: []NVLink{
					{
						UUID:      "gpu-uuid-123",
						Supported: true,
						States:    []NVLinkState{},
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
				NVLinks: []NVLink{
					{
						UUID:      "gpu-uuid-123",
						Supported: true,
						States:    []NVLinkState{},
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
	nvLinkStates := []NVLinkState{
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

	nvLink := NVLink{
		UUID:      uuid,
		Supported: true,
		States:    nvLinkStates,
	}

	getNVLinkFunc := func(_ string, _ device.Device) (NVLink, error) {
		return nvLink, nil
	}

	// Create a component and run Check
	component := mustComponent(t, MockNVLinkComponent(ctx, getDevicesFunc, getNVLinkFunc))
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

	// Use nvmlerrors.ErrGPULost for the error
	getNVLinkFunc := func(_ string, _ device.Device) (NVLink, error) {
		return NVLink{}, nvmlerrors.ErrGPULost
	}

	component := mustComponent(t, MockNVLinkComponent(ctx, getDevicesFunc, getNVLinkFunc))
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

	// Return a Reset-like error via nvml.Return and mapping in GetNVLink
	getNVLinkFunc := func(_ string, _ device.Device) (NVLink, error) {
		// Use any API that would surface this return in underlying helper; directly return the mapped error here
		// because the nvlink component only checks errors.Is on ErrGPURequiresReset
		return NVLink{}, nvmlerrors.ErrGPURequiresReset
	}

	component := mustComponent(t, MockNVLinkComponent(ctx, getDevicesFunc, getNVLinkFunc))
	result := component.Check()

	// Verify check result carries suggested actions
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")
	require.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.True(t, errors.Is(data.err, nvmlerrors.ErrGPURequiresReset))
	assert.Equal(t, "GPU requires reset", data.reason)
	if assert.NotNil(t, data.suggestedActions) {
		assert.Equal(t, "GPU requires reset", data.suggestedActions.Description)
		assert.Contains(t, data.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	}

	// Verify suggested actions propagates to health state output
	states := component.LastHealthStates()
	require.Len(t, states, 1)
	assert.NotNil(t, states[0].SuggestedActions)
}

func TestCheck_ThresholdViolationInactive(t *testing.T) {
	ctx := context.Background()
	uuid := "gpu-uuid-inactive"
	mockDeviceObj := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "arch", "brand", "cuda", "pci")

	devs := map[string]device.Device{uuid: mockDev}

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	nvLink := NVLink{
		UUID:      uuid,
		Supported: true,
		States: []NVLinkState{
			{FeatureEnabled: false},
		},
	}

	component := mustComponent(t, MockNVLinkComponent(ctx, getDevicesFunc, func(string, device.Device) (NVLink, error) {
		return nvLink, nil
	}))
	component.getThresholdsFunc = func() ExpectedLinkStates {
		return ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 1}
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, []string{uuid}, cr.InactiveNVLinkUUIDs)
	assert.Contains(t, cr.reason, "threshold violated")
	assert.Contains(t, cr.reason, uuid)
}

func TestCheck_ThresholdViolationUnsupported(t *testing.T) {
	ctx := context.Background()
	uuid := "gpu-uuid-unsupported"
	mockDeviceObj := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "arch", "brand", "cuda", "pci")

	devs := map[string]device.Device{uuid: mockDev}

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	nvLink := NVLink{
		UUID:      uuid,
		Supported: false,
	}

	component := mustComponent(t, MockNVLinkComponent(ctx, getDevicesFunc, func(string, device.Device) (NVLink, error) {
		return nvLink, nil
	}))
	component.getThresholdsFunc = func() ExpectedLinkStates {
		return ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 1}
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, []string{uuid}, cr.UnsupportedNVLinkUUIDs)
	assert.Contains(t, cr.reason, "threshold violated")
	assert.Contains(t, cr.reason, uuid)
	assert.Empty(t, cr.InactiveNVLinkUUIDs)
}

func TestCheck_ThresholdSatisfied(t *testing.T) {
	ctx := context.Background()
	uuid := "gpu-uuid-healthy"
	mockDeviceObj := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "arch", "brand", "cuda", "pci")

	devs := map[string]device.Device{uuid: mockDev}

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	nvLink := NVLink{
		UUID:      uuid,
		Supported: true,
		States: []NVLinkState{
			{FeatureEnabled: true},
		},
	}

	component := mustComponent(t, MockNVLinkComponent(ctx, getDevicesFunc, func(string, device.Device) (NVLink, error) {
		return nvLink, nil
	}))
	component.getThresholdsFunc = func() ExpectedLinkStates {
		return ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 1}
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "threshold satisfied")
	assert.Empty(t, cr.InactiveNVLinkUUIDs)
	assert.Empty(t, cr.UnsupportedNVLinkUUIDs)
}

func TestCheck_ImplicitFailureWhenMultiGPUSystemHasNoActiveNVLink(t *testing.T) {
	ctx := context.Background()

	devs := map[string]device.Device{
		"gpu-uuid-0": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-0"),
		"gpu-uuid-1": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-1"),
	}

	component := &component{
		ctx:    ctx,
		cancel: func() {},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: &mockNVMLInstance{
			devicesFunc: func() map[string]device.Device {
				return devs
			},
		},
		getNVLinkFunc: func(uuid string, _ device.Device) (NVLink, error) {
			return NVLink{
				UUID:      uuid,
				Supported: false,
			}, nil
		},
		getThresholdsFunc: GetDefaultExpectedLinkStates,
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.True(t, cr.SystemExpectedNVLink)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Len(t, cr.UnsupportedNVLinkUUIDs, 2)
	assert.Contains(t, cr.reason, "no GPUs report active nvlink links")
}

func TestCheck_ImplicitFailureWhenMultiGPUSystemHasNoPeerNVLinkP2PConnectivity(t *testing.T) {
	ctx := context.Background()

	devs := map[string]device.Device{
		"gpu-uuid-0": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-0"),
		"gpu-uuid-1": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-1"),
	}

	component := &component{
		ctx:    ctx,
		cancel: func() {},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: &mockNVMLInstance{
			devicesFunc: func() map[string]device.Device {
				return devs
			},
		},
		getNVLinkFunc: func(uuid string, _ device.Device) (NVLink, error) {
			return NVLink{
				UUID:      uuid,
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			}, nil
		},
		getPeerNVLinkP2PStatusFn: func(_ device.Device, _ device.Device) (string, error) {
			return p2pStatusNotSupported, nil
		},
		getThresholdsFunc: GetDefaultExpectedLinkStates,
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.True(t, cr.SystemExpectedNVLink)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, 1, cr.PeerNVLinkProbePairCount)
	assert.Equal(t, 1, cr.PeerNVLinkExpectedPairCount)
	assert.Equal(t, 0, cr.PeerNVLinkOKPairCount)
	assert.Equal(t, []string{p2pStatusNotSupported}, cr.PeerNVLinkObservedStatusCodes)
	assert.Empty(t, cr.PeerNVLinkOKGPUUUIDs)
	assert.Contains(t, cr.reason, "no GPU pairs report NVLink P2P connectivity")
	assert.Contains(t, cr.reason, "peer nvlink p2p statuses=NS")
	assert.Nil(t, cr.suggestedActions)
	assert.Contains(t, cr.String(), "NVLINK P2P OK")
}

func TestCheck_PeerNVLinkP2POKKeepsHealthyWithoutThreshold(t *testing.T) {
	ctx := context.Background()

	devs := map[string]device.Device{
		"gpu-uuid-0": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-0"),
		"gpu-uuid-1": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-1"),
	}

	component := &component{
		ctx:    ctx,
		cancel: func() {},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: &mockNVMLInstance{
			devicesFunc: func() map[string]device.Device {
				return devs
			},
		},
		getNVLinkFunc: func(uuid string, _ device.Device) (NVLink, error) {
			return NVLink{
				UUID:      uuid,
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			}, nil
		},
		getPeerNVLinkP2PStatusFn: func(_ device.Device, _ device.Device) (string, error) {
			return p2pStatusOK, nil
		},
		getThresholdsFunc: GetDefaultExpectedLinkStates,
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.True(t, cr.SystemExpectedNVLink)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, 1, cr.PeerNVLinkProbePairCount)
	assert.Equal(t, 1, cr.PeerNVLinkExpectedPairCount)
	assert.Equal(t, 1, cr.PeerNVLinkOKPairCount)
	assert.Equal(t, []string{"gpu-uuid-0", "gpu-uuid-1"}, cr.PeerNVLinkOKGPUUUIDs)
	assert.Equal(t, []string{p2pStatusOK}, cr.PeerNVLinkObservedStatusCodes)
	assert.Equal(t, "all 2 GPU(s) were checked, no nvlink issue found", cr.reason)
}

func TestCheck_PartialPeerNVLinkP2PCoverageDoesNotTriggerImplicitFailure(t *testing.T) {
	ctx := context.Background()

	devs := map[string]device.Device{
		"gpu-uuid-0": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-0"),
		"gpu-uuid-1": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-1"),
		"gpu-uuid-2": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-2"),
	}

	callCount := 0
	component := &component{
		ctx:    ctx,
		cancel: func() {},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: &mockNVMLInstance{
			devicesFunc: func() map[string]device.Device {
				return devs
			},
		},
		getNVLinkFunc: func(uuid string, _ device.Device) (NVLink, error) {
			return NVLink{
				UUID:      uuid,
				Supported: true,
				States: []NVLinkState{
					{FeatureEnabled: true},
				},
			}, nil
		},
		getPeerNVLinkP2PStatusFn: func(_ device.Device, _ device.Device) (string, error) {
			callCount++
			if callCount == 1 {
				return p2pStatusNotSupported, nil
			}
			return "", errors.New("mock p2p probe failure")
		},
		getThresholdsFunc: GetDefaultExpectedLinkStates,
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.True(t, cr.SystemExpectedNVLink)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 3 GPU(s) were checked, no nvlink issue found", cr.reason)
	assert.Equal(t, 1, cr.PeerNVLinkProbePairCount)
	assert.Equal(t, 3, cr.PeerNVLinkExpectedPairCount)
	assert.Equal(t, 0, cr.PeerNVLinkOKPairCount)
	assert.Equal(t, []string{p2pStatusNotSupported}, cr.PeerNVLinkObservedStatusCodes)
	assert.Empty(t, cr.PeerNVLinkOKGPUUUIDs)
	assert.Nil(t, cr.suggestedActions)
}

func TestCheck_MultiGPUWithoutExpectedNVLinkStillSkipsByDefault(t *testing.T) {
	ctx := context.Background()

	devs := map[string]device.Device{
		"gpu-uuid-0": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-0"),
		"gpu-uuid-1": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-1"),
	}

	component := &component{
		ctx:    ctx,
		cancel: func() {},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: &mockNVMLInstanceNoFabric{
			mockNVMLInstance: mockNVMLInstance{
				devicesFunc: func() map[string]device.Device {
					return devs
				},
			},
		},
		getNVLinkFunc: func(uuid string, _ device.Device) (NVLink, error) {
			return NVLink{
				UUID:      uuid,
				Supported: false,
			}, nil
		},
		getThresholdsFunc: GetDefaultExpectedLinkStates,
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.False(t, cr.SystemExpectedNVLink)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 2 GPU(s) were checked, no nvlink issue found", cr.reason)
	assert.Len(t, cr.UnsupportedNVLinkUUIDs, 2)
}

func TestCheck_UnsupportedNVLinkClearsPerGPUMetrics(t *testing.T) {
	ctx := context.Background()
	uuid := "gpu-uuid-metrics-reset"

	devs := map[string]device.Device{
		uuid: testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-0"),
	}

	supportedState := NVLink{
		UUID:      uuid,
		Supported: true,
		States: []NVLinkState{
			{
				FeatureEnabled: true,
				ReplayErrors:   7,
				RecoveryErrors: 3,
				CRCErrors:      2,
			},
		},
	}
	unsupportedState := NVLink{
		UUID:      uuid,
		Supported: false,
	}

	currentState := supportedState
	component := &component{
		ctx:    ctx,
		cancel: func() {},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: &mockNVMLInstance{
			devicesFunc: func() map[string]device.Device {
				return devs
			},
		},
		getNVLinkFunc: func(_ string, _ device.Device) (NVLink, error) {
			return currentState, nil
		},
		getThresholdsFunc: GetDefaultExpectedLinkStates,
	}

	component.Check()
	labels := prometheus.Labels{"uuid": uuid}
	assert.Equal(t, float64(1), readGaugeValue(t, metricSupported.With(labels)))
	assert.Equal(t, float64(1), readGaugeValue(t, metricFeatureEnabled.With(labels)))
	assert.Equal(t, float64(7), readGaugeValue(t, metricReplayErrors.With(labels)))
	assert.Equal(t, float64(3), readGaugeValue(t, metricRecoveryErrors.With(labels)))
	assert.Equal(t, float64(2), readGaugeValue(t, metricCRCErrors.With(labels)))

	currentState = unsupportedState
	component.Check()

	assert.Equal(t, float64(0), readGaugeValue(t, metricSupported.With(labels)))
	assert.Equal(t, float64(0), readGaugeValue(t, metricFeatureEnabled.With(labels)))
	assert.Equal(t, float64(0), readGaugeValue(t, metricReplayErrors.With(labels)))
	assert.Equal(t, float64(0), readGaugeValue(t, metricRecoveryErrors.With(labels)))
	assert.Equal(t, float64(0), readGaugeValue(t, metricCRCErrors.With(labels)))
}

func TestCheck_SingleGPUSystemDoesNotExpectNVLinkByDefault(t *testing.T) {
	ctx := context.Background()

	devs := map[string]device.Device{
		"gpu-uuid-0": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-0"),
	}

	component := &component{
		ctx:    ctx,
		cancel: func() {},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: &mockNVMLInstance{
			devicesFunc: func() map[string]device.Device {
				return devs
			},
		},
		getNVLinkFunc: func(uuid string, _ device.Device) (NVLink, error) {
			return NVLink{
				UUID:      uuid,
				Supported: false,
			}, nil
		},
		getThresholdsFunc: GetDefaultExpectedLinkStates,
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.False(t, cr.SystemExpectedNVLink)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 1 GPU(s) were checked, no nvlink issue found", cr.reason)
	assert.Equal(t, []string{"gpu-uuid-0"}, cr.UnsupportedNVLinkUUIDs)
}

func TestCheck_PartialActiveNVLinkWithoutThresholdRemainsHealthy(t *testing.T) {
	ctx := context.Background()

	devs := map[string]device.Device{
		"gpu-uuid-0": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-0"),
		"gpu-uuid-1": testutil.NewMockDevice(&mock.Device{}, "arch", "brand", "cuda", "pci-1"),
	}

	component := &component{
		ctx:    ctx,
		cancel: func() {},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: &mockNVMLInstance{
			devicesFunc: func() map[string]device.Device {
				return devs
			},
		},
		getNVLinkFunc: func(uuid string, _ device.Device) (NVLink, error) {
			switch uuid {
			case "gpu-uuid-0":
				return NVLink{
					UUID:      uuid,
					Supported: true,
					States: []NVLinkState{
						{FeatureEnabled: true},
					},
				}, nil
			case "gpu-uuid-1":
				return NVLink{
					UUID:      uuid,
					Supported: true,
					States: []NVLinkState{
						{FeatureEnabled: false},
					},
				}, nil
			default:
				return NVLink{}, nil
			}
		},
		getThresholdsFunc: GetDefaultExpectedLinkStates,
	}

	result := component.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.True(t, cr.SystemExpectedNVLink)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 2 GPU(s) were checked, no nvlink issue found", cr.reason)
	assert.Equal(t, []string{"gpu-uuid-0"}, cr.ActiveNVLinkUUIDs)
	assert.Equal(t, []string{"gpu-uuid-1"}, cr.InactiveNVLinkUUIDs)
}
