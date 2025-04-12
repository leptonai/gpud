package gpm

import (
	"context"
	"errors"
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

func (m *MockNvmlInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
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
	assert.NotNil(t, tc.nvmlInstance, "nvmlInstance should be set")
	assert.NotNil(t, tc.getGPMSupportedFunc, "getGPMSupportedFunc should be set")
	assert.NotNil(t, tc.getGPMMetricsFunc, "getGPMMetricsFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := MockGPMComponent(ctx, nil, nil, nil)
	assert.Equal(t, Name, c.Name(), "Component name should match")
}

func TestCheckOnce_GPMNotSupported(t *testing.T) {
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
	component.CheckOnce()

	// Verify data
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.GPMSupported, "GPM should not be supported")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Equal(t, "GPM not supported", lastData.reason)
}

func TestCheckOnce_GPMSupported(t *testing.T) {
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
	component.CheckOnce()

	// Verify data
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Len(t, lastData.GPMMetrics, 1)
	assert.Equal(t, uuid, lastData.GPMMetrics[0].UUID)
	assert.Equal(t, expectedMetrics, lastData.GPMMetrics[0].Metrics)
	assert.Equal(t, metav1.Duration{Duration: sampleDuration}, lastData.GPMMetrics[0].SampleDuration)
	assert.Equal(t, "all 1 GPU(s) were checked, no GPM issue found", lastData.reason)
}

func TestCheckOnce_GPMSupportError(t *testing.T) {
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
	component.CheckOnce()

	// Verify error handling
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.healthy, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastData.err)
	assert.Equal(t, "error getting GPM supported for device gpu-uuid-123", lastData.reason)
}

func TestCheckOnce_GPMMetricsError(t *testing.T) {
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
	component.CheckOnce()

	// Verify error handling
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.healthy, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastData.err)
	assert.Equal(t, "error getting GPM metrics for device gpu-uuid-123", lastData.reason)
}

func TestCheckOnce_NoDevices(t *testing.T) {
	ctx := context.Background()

	getDevicesFunc := func() map[string]device.Device {
		return map[string]device.Device{} // Empty map
	}

	component := MockGPMComponent(ctx, getDevicesFunc, nil, nil).(*component)
	component.CheckOnce()

	// Verify handling of no devices
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Equal(t, "all 0 GPU(s) were checked, no GPM issue found", lastData.reason)
	assert.Empty(t, lastData.GPMMetrics)
}

func TestStates_WithData(t *testing.T) {
	ctx := context.Background()
	component := MockGPMComponent(ctx, nil, nil, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastData = &Data{
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
		healthy: true,
		reason:  "all 1 GPU(s) were checked, no GPM issue found",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateHealthy, state.Health)
	assert.True(t, state.Healthy)
	assert.Equal(t, "all 1 GPU(s) were checked, no GPM issue found", state.Reason)
	assert.Contains(t, state.ExtraInfo["data"], "gpu-uuid-123")
}

func TestStates_WithError(t *testing.T) {
	ctx := context.Background()
	component := MockGPMComponent(ctx, nil, nil, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastData = &Data{
		err:     errors.New("test GPM error"),
		healthy: false,
		reason:  "error getting GPM metrics for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateUnhealthy, state.Health)
	assert.False(t, state.Healthy)
	assert.Equal(t, "error getting GPM metrics for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test GPM error", state.Error)
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	component := MockGPMComponent(ctx, nil, nil, nil).(*component)

	// Don't set any data

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateHealthy, state.Health)
	assert.True(t, state.Healthy)
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

	// Use a channel to detect when CheckOnce is called
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

	// Wait for CheckOnce to be called
	select {
	case <-checkCalled:
		// Success - CheckOnce was called
	case <-time.After(200 * time.Millisecond):
		t.Fatal("CheckOnce was not called within expected time")
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
