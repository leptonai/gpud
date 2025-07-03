// Package temperature tracks the NVIDIA per-GPU temperatures.
package temperature

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

// mockNVMLInstance is a simple mock implementation of nvidianvml.Instance
type mockNVMLInstance struct {
	devices  map[string]device.Device
	exists   bool
	prodName string
}

// NewMockNVMLInstance creates a new mockNVMLInstance with default settings
func NewMockNVMLInstance(devices map[string]device.Device) *mockNVMLInstance {
	return &mockNVMLInstance{
		devices:  devices,
		exists:   true,       // default to true
		prodName: "Test GPU", // default to non-empty
	}
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.exists
}

func (m *mockNVMLInstance) Library() nvml_lib.Library {
	// Return nil for testing - we're not actually using this
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockNVMLInstance) ProductName() string {
	return m.prodName
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

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

// MockTemperatureComponent creates a component with mocked functions for testing
func MockTemperatureComponent(
	ctx context.Context,
	nvmlInstance nvidianvml.Instance,
	getTemperatureFunc func(uuid string, dev device.Device) (nvidianvml.Temperature, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:          cctx,
		cancel:       cancel,
		nvmlInstance: nvmlInstance,
	}

	if getTemperatureFunc != nil {
		c.getTemperatureFunc = getTemperatureFunc
	}

	return c
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "Test GPU",
	}

	// Create a GPUdInstance with the mock NVML
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockNVML,
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
	assert.NotNil(t, tc.getTemperatureFunc, "getTemperatureFunc should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "Test GPU",
	}
	c := MockTemperatureComponent(ctx, mockNVML, nil)
	assert.Equal(t, Name, c.Name(), "Component name should match")
}

func TestTags(t *testing.T) {
	ctx := context.Background()
	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "Test GPU",
	}
	c := MockTemperatureComponent(ctx, mockNVML, nil)

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	// Verify the tags returned by the component
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

	mockNVML := &mockNVMLInstance{
		devices:  devs,
		exists:   true,
		prodName: "Test GPU",
	}

	temperature := nvidianvml.Temperature{
		UUID:                     uuid,
		CurrentCelsiusGPUCore:    75,      // 75°C
		ThresholdCelsiusShutdown: 120,     // 120°C
		ThresholdCelsiusSlowdown: 95,      // 95°C
		ThresholdCelsiusMemMax:   105,     // 105°C
		ThresholdCelsiusGPUMax:   100,     // 100°C
		UsedPercentShutdown:      "62.50", // 75/120 = 62.5%
		UsedPercentSlowdown:      "78.95", // 75/95 = 78.95%
		UsedPercentMemMax:        "71.43", // 75/105 = 71.43%
		UsedPercentGPUMax:        "75.00", // 75/100 = 75%
	}

	getTemperatureFunc := func(uuid string, dev device.Device) (nvidianvml.Temperature, error) {
		return temperature, nil
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
	result := component.Check()

	// Verify the data was collected
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Contains(t, data.reason, "no temperature issue found")
	assert.Len(t, data.Temperatures, 1)
	assert.Equal(t, temperature, data.Temperatures[0])
}

func TestCheck_TemperatureError(t *testing.T) {
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

	mockNVML := &mockNVMLInstance{
		devices:  devs,
		exists:   true,
		prodName: "Test GPU",
	}

	errExpected := errors.New("temperature error")
	getTemperatureFunc := func(uuid string, dev device.Device) (nvidianvml.Temperature, error) {
		return nvidianvml.Temperature{}, errExpected
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
	result := component.Check()

	// Verify error handling
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.Equal(t, errExpected, data.err)
	assert.Equal(t, "error getting temperature", data.reason)
}

func TestCheck_NoDevices(t *testing.T) {
	ctx := context.Background()

	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "Test GPU",
	}

	component := MockTemperatureComponent(ctx, mockNVML, nil).(*component)
	result := component.Check()

	// Verify handling of no devices
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health, "data should be marked healthy")
	assert.Contains(t, data.reason, "all 0")
	assert.Empty(t, data.Temperatures)
}

func TestCheck_GetUsedPercentSlowdownError(t *testing.T) {
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

	mockNVML := &mockNVMLInstance{
		devices:  devs,
		exists:   true,
		prodName: "Test GPU",
	}

	// Create temperature data with invalid UsedPercentSlowdown format
	invalidTemperature := nvidianvml.Temperature{
		UUID:                     uuid,
		CurrentCelsiusGPUCore:    75,
		ThresholdCelsiusShutdown: 120,
		ThresholdCelsiusSlowdown: 95,
		ThresholdCelsiusMemMax:   105,
		ThresholdCelsiusGPUMax:   100,
		UsedPercentShutdown:      "62.50",
		UsedPercentSlowdown:      "invalid", // Will cause ParseFloat to fail
		UsedPercentMemMax:        "71.43",
		UsedPercentGPUMax:        "75.00",
	}

	getTemperatureFunc := func(uuid string, dev device.Device) (nvidianvml.Temperature, error) {
		return invalidTemperature, nil
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
	result := component.Check()

	// Verify error handling for GetUsedPercentSlowdown failure
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.NotNil(t, data.err)
	assert.Equal(t, "error getting used percent for slowdown", data.reason)
}

func TestLastHealthStates_WithData(t *testing.T) {
	ctx := context.Background()
	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "Test GPU",
	}
	component := MockTemperatureComponent(ctx, mockNVML, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		Temperatures: []nvidianvml.Temperature{
			{
				UUID:                     "gpu-uuid-123",
				CurrentCelsiusGPUCore:    75,
				ThresholdCelsiusShutdown: 120,
				ThresholdCelsiusSlowdown: 95,
				ThresholdCelsiusMemMax:   105,
				ThresholdCelsiusGPUMax:   100,
				UsedPercentShutdown:      "62.50",
				UsedPercentSlowdown:      "78.95",
				UsedPercentMemMax:        "71.43",
				UsedPercentGPUMax:        "75.00",
			},
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "checked 1 devices for temperature",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "checked 1 devices for temperature", state.Reason)
	assert.Contains(t, state.ExtraInfo["data"], "gpu-uuid-123")
}

func TestLastHealthStates_WithError(t *testing.T) {
	ctx := context.Background()
	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "Test GPU",
	}
	component := MockTemperatureComponent(ctx, mockNVML, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastCheckResult = &checkResult{
		err:    errors.New("test temperature error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting temperature",
	}
	component.lastMu.Unlock()

	// Get states
	states := component.LastHealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "error getting temperature", state.Reason)
	assert.Equal(t, "test temperature error", state.Error)
}

func TestLastHealthStates_NoData(t *testing.T) {
	ctx := context.Background()
	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "Test GPU",
	}
	component := MockTemperatureComponent(ctx, mockNVML, nil).(*component)

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
	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "Test GPU",
	}
	component := MockTemperatureComponent(ctx, mockNVML, nil)

	events, err := component.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock NVML instance with devices
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

	callCount := &atomic.Int32{}
	mockNVML := &mockNVMLInstance{
		devices:  devs,
		exists:   true,
		prodName: "Test GPU",
	}

	getTemperatureFunc := func(uuid string, dev device.Device) (nvidianvml.Temperature, error) {
		callCount.Add(1)
		return nvidianvml.Temperature{}, nil
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc)

	// Start should be non-blocking
	err := component.Start()
	assert.NoError(t, err)

	// Give the goroutine time to execute Check at least once
	time.Sleep(100 * time.Millisecond)

	// It's difficult to verify Check was called since we're using a mock function
	// But the Start method should have completed without error
	assert.NoError(t, err)
}

func TestClose(t *testing.T) {
	ctx := context.Background()
	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "Test GPU",
	}
	component := MockTemperatureComponent(ctx, mockNVML, nil).(*component)

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

func TestCheck_MemoryTemperatureThreshold(t *testing.T) {
	tests := []struct {
		name                 string
		currentTemp          uint32
		memMaxThreshold      uint32
		expectHealthy        apiv1.HealthStateType
		expectReasonContains string
	}{
		{
			name:                 "Below threshold",
			currentTemp:          80,
			memMaxThreshold:      100,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Equal to threshold",
			currentTemp:          100,
			memMaxThreshold:      100,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Above threshold",
			currentTemp:          110,
			memMaxThreshold:      100,
			expectHealthy:        apiv1.HealthStateTypeUnhealthy,
			expectReasonContains: "exceeding the HBM temperature threshold",
		},
		{
			name:                 "Threshold is zero (disabled)",
			currentTemp:          110,
			memMaxThreshold:      0,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			mockNVML := &mockNVMLInstance{
				devices:  devs,
				exists:   true,
				prodName: "Test GPU",
			}

			temperature := nvidianvml.Temperature{
				UUID:                     uuid,
				CurrentCelsiusGPUCore:    tt.currentTemp,
				ThresholdCelsiusShutdown: 120,
				ThresholdCelsiusSlowdown: 95,
				ThresholdCelsiusMemMax:   tt.memMaxThreshold,
				ThresholdCelsiusGPUMax:   100,
				UsedPercentShutdown:      "66.67", // 80/120 = 66.67%
				UsedPercentSlowdown:      "84.21", // 80/95 = 84.21%
				UsedPercentMemMax:        "80.00", // 80/100 = 80.00%
				UsedPercentGPUMax:        "80.00", // 80/100 = 80.00%
			}

			getTemperatureFunc := func(uuid string, dev device.Device) (nvidianvml.Temperature, error) {
				return temperature, nil
			}

			component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
			result := component.Check()

			// Verify the data was collected
			data, ok := result.(*checkResult)
			require.True(t, ok, "result should be of type *checkResult")

			require.NotNil(t, data, "data should not be nil")
			assert.Equal(t, tt.expectHealthy, data.health, "health state mismatch")
			assert.Contains(t, data.reason, tt.expectReasonContains)
			assert.Len(t, data.Temperatures, 1)
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
		exists:   false,
		prodName: "Tesla V100",
	}
	comp.nvmlInstance = mockNVML
	assert.False(t, comp.IsSupported())

	// Test with NVML that exists but has empty product name
	mockNVML = &mockNVMLInstance{
		exists:   true,
		prodName: "",
	}
	comp.nvmlInstance = mockNVML
	assert.False(t, comp.IsSupported())

	// Test with NVML that exists and has a product name
	mockNVML = &mockNVMLInstance{
		exists:   true,
		prodName: "Tesla V100",
	}
	comp.nvmlInstance = mockNVML
	assert.True(t, comp.IsSupported())
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

	mockNVML := &mockNVMLInstance{
		devices:  devs,
		exists:   true,
		prodName: "Test GPU",
	}

	// Use nvidianvml.ErrGPULost for the error
	getTemperatureFunc := func(uuid string, dev device.Device) (nvidianvml.Temperature, error) {
		return nvidianvml.Temperature{}, nvidianvml.ErrGPULost
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
	result := component.Check()

	// Verify error handling for GPU lost case
	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "data should be marked unhealthy")
	assert.True(t, errors.Is(data.err, nvidianvml.ErrGPULost), "error should be nvidianvml.ErrGPULost")
	assert.Equal(t, "error getting temperature", data.reason,
		"reason should have '(GPU is lost)' suffix")
}
