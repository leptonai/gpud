package gpm

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

func (m *MockNvmlInstance) FabricManagerSupported() bool {
	return true
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

func (m *MockNvmlInstance) Library() lib.Library {
	return nil
}

func (m *MockNvmlInstance) Shutdown() error {
	return nil
}

// CustomMockNvmlInstance implements the nvml.InstanceV2 interface with customizable NVMLExists behavior
type CustomMockNvmlInstance struct {
	devs       map[string]device.Device
	nvmlExists bool
}

func (m *CustomMockNvmlInstance) Devices() map[string]device.Device {
	return m.devs
}

func (m *CustomMockNvmlInstance) FabricManagerSupported() bool {
	return true
}

func (m *CustomMockNvmlInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *CustomMockNvmlInstance) ProductName() string {
	return "NVIDIA Test GPU"
}

func (m *CustomMockNvmlInstance) Architecture() string {
	return ""
}

func (m *CustomMockNvmlInstance) Brand() string {
	return ""
}

func (m *CustomMockNvmlInstance) DriverVersion() string {
	return ""
}

func (m *CustomMockNvmlInstance) DriverMajor() int {
	return 0
}

func (m *CustomMockNvmlInstance) CUDAVersion() string {
	return ""
}

func (m *CustomMockNvmlInstance) NVMLExists() bool {
	return m.nvmlExists
}

func (m *CustomMockNvmlInstance) Library() lib.Library {
	return nil
}

func (m *CustomMockNvmlInstance) Shutdown() error {
	return nil
}

// MockGPMComponent creates a component with mocked functions for testing
func MockGPMComponent(
	ctx context.Context,
	devicesFunc func() map[string]device.Device,
	getGPMSupportedFunc func(dev device.Device) (bool, error),
	getGPMMetricsFunc func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInstance := &MockNvmlInstance{
		devicesFunc: devicesFunc,
	}

	return &component{
		ctx:                 cctx,
		cancel:              cancel,
		nvmlInstance:        mockInstance,
		getGPMSupportedFunc: getGPMSupportedFunc,
		getGPMMetricsFunc:   getGPMMetricsFunc,
	}
}

func TestNew(t *testing.T) {
	ctx := context.Background()

	// Create a mock GPUdInstance
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
	assert.NotNil(t, tc.getGPMSupportedFunc, "getGPMSupportedFunc should be set")
	assert.NotNil(t, tc.getGPMMetricsFunc, "getGPMMetricsFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockGPMComponent(ctx, nil, nil, nil)
	assert.Equal(t, Name, c.Name(), "Component name should match")
}

func TestCheck_GPMNotSupported(t *testing.T) {
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

	getGPMSupportedFunc := func(dev device.Device) (bool, error) {
		return false, nil
	}

	getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
		return nil, nil
	}

	component := MockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
	component.Check()

	// Verify data
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.False(t, lastCheckResult.GPMSupported, "GPM should not be supported")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Equal(t, "GPM not supported", lastCheckResult.reason)
}

func TestCheck_GPMSupported(t *testing.T) {
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

	getGPMSupportedFunc := func(dev device.Device) (bool, error) {
		return true, nil
	}

	expectedMetrics := map[nvml.GpmMetricId]float64{
		nvml.GPM_METRIC_SM_OCCUPANCY:     75.5,
		nvml.GPM_METRIC_INTEGER_UTIL:     30.2,
		nvml.GPM_METRIC_ANY_TENSOR_UTIL:  80.1,
		nvml.GPM_METRIC_DFMA_TENSOR_UTIL: 40.3,
	}

	getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
		return expectedMetrics, nil
	}

	component := MockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
	component.Check()

	// Verify data
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Len(t, lastCheckResult.GPMMetrics, 1)
	assert.Equal(t, uuid, lastCheckResult.GPMMetrics[0].UUID)
	assert.Equal(t, expectedMetrics, lastCheckResult.GPMMetrics[0].Metrics)
	assert.Equal(t, metav1.Duration{Duration: sampleDuration}, lastCheckResult.GPMMetrics[0].SampleDuration)
	assert.Equal(t, "all 1 GPU(s) were checked, no GPM issue found", lastCheckResult.reason)
}

func TestCheck_GPMSupportError(t *testing.T) {
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

	errExpected := errors.New("GPM support check failed")
	getGPMSupportedFunc := func(dev device.Device) (bool, error) {
		return false, errExpected
	}

	getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
		return nil, nil
	}

	component := MockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
	component.Check()

	// Verify error handling
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, lastCheckResult.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastCheckResult.err)
	assert.Equal(t, "error getting GPM supported for device gpu-uuid-123", lastCheckResult.reason)
}

func TestCheck_GPMMetricsError(t *testing.T) {
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

	getGPMSupportedFunc := func(dev device.Device) (bool, error) {
		return true, nil
	}

	errExpected := errors.New("GPM metrics collection failed")
	getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
		return nil, errExpected
	}

	component := MockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
	component.Check()

	// Verify error handling
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, lastCheckResult.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastCheckResult.err)
	assert.Equal(t, "error getting GPM metrics for device gpu-uuid-123", lastCheckResult.reason)
}

func TestCheck_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := MockGPMComponent(ctx, getDevicesFunc, nil, nil).(*component)
	component.Check()

	// Verify handling of no devices
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no GPM issue found", lastCheckResult.reason)
	assert.Empty(t, lastCheckResult.GPMMetrics)
}

func TestStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockGPMComponent(ctx, nil, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		GPMSupported: true,
		GPMMetrics: []nvidianvml.GPMMetrics{
			{
				UUID: "gpu-uuid-123",
				Metrics: map[nvml.GpmMetricId]float64{
					nvml.GPM_METRIC_SM_OCCUPANCY: 80.0,
				},
				SampleDuration: metav1.Duration{Duration: sampleDuration},
				Time:           metav1.Time{Time: time.Now().UTC()},
			},
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "all 1 GPU(s) were checked, no GPM issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "all 1 GPU(s) were checked, no GPM issue found", state.Reason)
	assert.Contains(t, state.DeprecatedExtraInfo["data"], "gpu-uuid-123")
}

func TestStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockGPMComponent(ctx, nil, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		err:    errors.New("test GPM error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting GPM metrics for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting GPM metrics for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test GPM error", state.Error)
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockGPMComponent(ctx, nil, nil, nil).(*component)

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
	component := MockGPMComponent(ctx, nil, nil, nil)

	events, err := component.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a channel to detect when Check is called
	checkCalled := make(chan bool, 1)

	getDevicesFunc := func() map[string]device.Device {
		// Signal that the function was called
		select {
		case checkCalled <- true:
		default:
			// Channel is full, which is fine
		}
		return map[string]device.Device{}
	}

	component := MockGPMComponent(ctx, getDevicesFunc, nil, nil)

	// Start should be non-blocking
	err := component.Start()
	assert.NoError(t, err)

	// Wait for Check to be called
	select {
	case <-checkCalled:
		// Success - Check was called
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Check was not called within expected time")
	}
}

func TestClose(t *testing.T) {
	ctx := context.Background()
	component := MockGPMComponent(ctx, nil, nil, nil).(*component)

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

func TestCheck_MultipleDevices(t *testing.T) {
	ctx := context.Background()

	uuid1 := "gpu-uuid-123"
	uuid2 := "gpu-uuid-456"

	mockDeviceObj1 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid1, nvml.SUCCESS
		},
	}
	mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "test-arch-1", "test-brand-1", "test-cuda-1", "test-pci-1")

	mockDeviceObj2 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid2, nvml.SUCCESS
		},
	}
	mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "test-arch-2", "test-brand-2", "test-cuda-2", "test-pci-2")

	devs := map[string]device.Device{
		uuid1: mockDev1,
		uuid2: mockDev2,
	}

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	getGPMSupportedFunc := func(dev device.Device) (bool, error) {
		return true, nil
	}

	metrics1 := map[nvml.GpmMetricId]float64{
		nvml.GPM_METRIC_SM_OCCUPANCY: 75.5,
		nvml.GPM_METRIC_INTEGER_UTIL: 30.2,
	}

	metrics2 := map[nvml.GpmMetricId]float64{
		nvml.GPM_METRIC_SM_OCCUPANCY:    60.0,
		nvml.GPM_METRIC_ANY_TENSOR_UTIL: 45.3,
	}

	getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
		uuid, _ := dev.GetUUID()
		if uuid == uuid1 {
			return metrics1, nil
		}
		return metrics2, nil
	}

	component := MockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
	component.Check()

	// Verify data
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Len(t, lastCheckResult.GPMMetrics, 2, "should have metrics for 2 GPUs")

	// Verify metrics for each GPU
	metricsByUUID := make(map[string]map[nvml.GpmMetricId]float64)
	for _, metric := range lastCheckResult.GPMMetrics {
		metricsByUUID[metric.UUID] = metric.Metrics
	}

	assert.Equal(t, metrics1, metricsByUUID[uuid1], "metrics for first GPU should match")
	assert.Equal(t, metrics2, metricsByUUID[uuid2], "metrics for second GPU should match")
	assert.Equal(t, "all 2 GPU(s) were checked, no GPM issue found", lastCheckResult.reason)
}

func TestCheck_MixedGPMSupport(t *testing.T) {
	ctx := context.Background()

	uuid1 := "gpu-uuid-123"
	uuid2 := "gpu-uuid-456"

	mockDeviceObj1 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid1, nvml.SUCCESS
		},
	}
	mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "test-arch-1", "test-brand-1", "test-cuda-1", "test-pci-1")

	mockDeviceObj2 := &mock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid2, nvml.SUCCESS
		},
	}
	mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "test-arch-2", "test-brand-2", "test-cuda-2", "test-pci-2")

	devs := map[string]device.Device{
		uuid1: mockDev1,
		uuid2: mockDev2,
	}

	getDevicesFunc := func() map[string]device.Device {
		return devs
	}

	getGPMSupportedFunc := func(dev device.Device) (bool, error) {
		uuid, _ := dev.GetUUID()
		// Only first GPU supports GPM
		return uuid == uuid1, nil
	}

	metrics := map[nvml.GpmMetricId]float64{
		nvml.GPM_METRIC_SM_OCCUPANCY: 75.5,
	}

	getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
		return metrics, nil
	}

	component := MockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
	component.Check()

	// Verify data
	component.lastMu.RLock()
	lastCheckResult := component.lastCheckResult
	component.lastMu.RUnlock()

	require.NotNil(t, lastCheckResult, "lastCheckResult should not be nil")
	assert.False(t, lastCheckResult.GPMSupported, "GPM should not be supported overall")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health, "data should be marked healthy")
	assert.Equal(t, "GPM not supported", lastCheckResult.reason)
	// We don't have metrics because once we find a device that doesn't support GPM,
	// we stop and report that GPM is not supported overall
	assert.Empty(t, lastCheckResult.GPMMetrics)
}

func TestCheck_NVMLInstanceNil(t *testing.T) {
	ctx := context.Background()

	component := MockGPMComponent(ctx, nil, nil, nil).(*component)
	component.nvmlInstance = nil

	result := component.Check()

	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthState())
	assert.Equal(t, "NVIDIA NVML instance is nil", result.Summary())
}

func TestCheck_NVMLNotLoaded(t *testing.T) {
	ctx := context.Background()

	// Create a custom mock instance with NVMLExists returning false
	mockInstance := &CustomMockNvmlInstance{
		devs:       map[string]device.Device{},
		nvmlExists: false,
	}

	component := MockGPMComponent(ctx, nil, nil, nil).(*component)
	component.nvmlInstance = mockInstance

	result := component.Check()

	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthState())
	assert.Equal(t, "NVIDIA NVML is not loaded", result.Summary())
}

func TestData_String(t *testing.T) {
	tests := []struct {
		name        string
		data        *checkResult
		shouldMatch bool
		contains    string
	}{
		{
			name:        "nil data",
			data:        nil,
			shouldMatch: true,
			contains:    "",
		},
		{
			name: "empty metrics",
			data: &checkResult{
				GPMMetrics: []nvidianvml.GPMMetrics{},
			},
			shouldMatch: true,
			contains:    "no data",
		},
		{
			name: "with metrics",
			data: &checkResult{
				GPMMetrics: []nvidianvml.GPMMetrics{
					{
						UUID: "gpu-uuid-123",
						Metrics: map[nvml.GpmMetricId]float64{
							nvml.GPM_METRIC_SM_OCCUPANCY: 75.5,
						},
					},
				},
			},
			shouldMatch: false,
			contains:    "GPU UUID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.data.String()
			if tt.shouldMatch {
				assert.Equal(t, tt.contains, str)
			} else {
				assert.Contains(t, str, tt.contains)
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
			got := tt.data.HealthState()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCheck_UpdateFrequency(t *testing.T) {
	// This test verifies that the component's Check method runs at the expected frequency
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	checkCount := 0
	countMu := sync.Mutex{}

	getDevicesFunc := func() map[string]device.Device {
		countMu.Lock()
		checkCount++
		countMu.Unlock()
		return map[string]device.Device{}
	}

	// Create a custom Start function to use a faster ticker for testing
	originalStart := func(c *component) error {
		go func() {
			ticker := time.NewTicker(50 * time.Millisecond) // Use shorter interval for testing
			defer ticker.Stop()

			for {
				_ = c.Check()

				select {
				case <-c.ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
		return nil
	}

	component := MockGPMComponent(ctx, getDevicesFunc, nil, nil).(*component)

	// Start the component with our custom start function
	err := originalStart(component)
	assert.NoError(t, err)

	// Wait for the context to be done
	<-ctx.Done()

	// We should have at least 2 checks (initial + at least one from ticker)
	countMu.Lock()
	count := checkCount
	countMu.Unlock()

	assert.GreaterOrEqual(t, count, 2, "Check should have been called at least twice")
}

func TestData_GetLastHealthStates_JSON(t *testing.T) {
	// This test verifies that the getLastHealthStates method properly formats the data as JSON
	now := time.Now().UTC()
	metricTime := metav1.Time{Time: now}

	uuid := "gpu-test-123"
	metrics := map[nvml.GpmMetricId]float64{
		nvml.GPM_METRIC_SM_OCCUPANCY:     75.5,
		nvml.GPM_METRIC_INTEGER_UTIL:     30.2,
		nvml.GPM_METRIC_ANY_TENSOR_UTIL:  80.1,
		nvml.GPM_METRIC_DFMA_TENSOR_UTIL: 40.3,
	}

	data := &checkResult{
		GPMSupported: true,
		GPMMetrics: []nvidianvml.GPMMetrics{
			{
				UUID:           uuid,
				Metrics:        metrics,
				SampleDuration: metav1.Duration{Duration: sampleDuration},
				Time:           metricTime,
			},
		},
		ts:     now,
		health: apiv1.HealthStateTypeHealthy,
		reason: "test health state json",
	}

	states := data.getLastHealthStates()

	// Verify the basic health state properties
	require.Len(t, states, 1)
	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "test health state json", state.Reason)

	// Verify the JSON data in the extra info
	require.Contains(t, state.DeprecatedExtraInfo, "data")
	require.Contains(t, state.DeprecatedExtraInfo, "encoding")
	assert.Equal(t, "json", state.DeprecatedExtraInfo["encoding"])

	// Parse the JSON data to verify its structure
	var parsedData map[string]interface{}
	err := json.Unmarshal([]byte(state.DeprecatedExtraInfo["data"]), &parsedData)
	require.NoError(t, err, "Data should be valid JSON")

	// Verify the GPM metrics data
	assert.True(t, parsedData["gpm_supported"].(bool))

	gpmMetrics, ok := parsedData["gpm_metrics"].([]interface{})
	require.True(t, ok, "gpm_metrics should be an array")
	require.Len(t, gpmMetrics, 1)

	metrics0, ok := gpmMetrics[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, uuid, metrics0["uuid"])

	// Check that sample_duration exists and contains appropriate data
	require.Contains(t, metrics0, "sample_duration")
	sampleDurationStr, ok := metrics0["sample_duration"].(string)
	require.True(t, ok, "sample_duration should be a string")
	assert.Contains(t, sampleDurationStr, "s", "sample_duration should contain time unit")
}
