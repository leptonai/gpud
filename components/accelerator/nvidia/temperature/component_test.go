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
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
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

func (m *mockNVMLInstance) FabricStateSupported() bool {
	return false
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func (m *mockNVMLInstance) InitError() error {
	return nil
}

// MockTemperatureComponent creates a component with mocked functions for testing
func MockTemperatureComponent(
	ctx context.Context,
	nvmlInstance nvidianvml.Instance,
	getTemperatureFunc func(uuid string, dev device.Device) (Temperature, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
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

func TestNew_NilGPUdInstance(t *testing.T) {
	c, err := New(nil)

	assert.Error(t, err)
	assert.Nil(t, c)
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

func TestCheck_HBMTemperatureExceedingThreshold(t *testing.T) {
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

	temperature := Temperature{
		UUID:                           uuid,
		CurrentCelsiusGPUCore:          75,
		CurrentCelsiusHBM:              110,
		HBMTemperatureSupported:        true,
		ThresholdCelsiusSlowdownMargin: 20,
		MarginTemperatureSupported:     true,
		ThresholdCelsiusShutdown:       120,
		ThresholdCelsiusSlowdown:       95,
		ThresholdCelsiusMemMax:         100,
		ThresholdCelsiusGPUMax:         100,
		UsedPercentShutdown:            "62.50",
		UsedPercentSlowdown:            "78.95",
		UsedPercentMemMax:              "110.00",
		UsedPercentGPUMax:              "75.00",
	}

	getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
		return temperature, nil
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
	result := component.Check()

	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeDegraded, data.health)
	assert.Contains(t, data.reason, "HBM temperature anomalies detected")
	assert.Contains(t, data.reason, "HBM temperature is 110 °C exceeding the threshold 100 °C")
}

func TestCheck_CurrentTemperatureExceedingThreshold(t *testing.T) {
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

	temperature := Temperature{
		UUID:                           uuid,
		CurrentCelsiusGPUCore:          110,
		CurrentCelsiusHBM:              80,
		HBMTemperatureSupported:        true,
		ThresholdCelsiusSlowdownMargin: 20,
		MarginTemperatureSupported:     true,
		ThresholdCelsiusShutdown:       120,
		ThresholdCelsiusSlowdown:       95,
		ThresholdCelsiusMemMax:         100,
		ThresholdCelsiusGPUMax:         100,
		UsedPercentShutdown:            "62.50",
		UsedPercentSlowdown:            "78.95",
		UsedPercentMemMax:              "71.43",
		UsedPercentGPUMax:              "75.00",
	}

	getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
		return temperature, nil
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
	result := component.Check()

	data, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	require.NotNil(t, data, "data should not be nil")
	assert.Equal(t, apiv1.HealthStateTypeDegraded, data.health)
	assert.Contains(t, data.reason, "GPU temperature anomalies detected")
	assert.Contains(t, data.reason, "current temperature is 110 °C exceeding the threshold 100 °C")
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

	temperature := Temperature{
		UUID:                           uuid,
		CurrentCelsiusGPUCore:          75, // 75°C
		CurrentCelsiusHBM:              75,
		HBMTemperatureSupported:        true,
		ThresholdCelsiusSlowdownMargin: 20,
		MarginTemperatureSupported:     true,
		ThresholdCelsiusShutdown:       120,     // 120°C
		ThresholdCelsiusSlowdown:       95,      // 95°C
		ThresholdCelsiusMemMax:         105,     // 105°C
		ThresholdCelsiusGPUMax:         100,     // 100°C
		UsedPercentShutdown:            "62.50", // 75/120 = 62.5%
		UsedPercentSlowdown:            "78.95", // 75/95 = 78.95%
		UsedPercentMemMax:              "71.43", // 75/105 = 71.43%
		UsedPercentGPUMax:              "75.00", // 75/100 = 75%
	}

	getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
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
	getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
		return Temperature{}, errExpected
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
	invalidTemperature := Temperature{
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

	getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
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
		Temperatures: []Temperature{
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

	getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
		callCount.Add(1)
		return Temperature{}, nil
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

func TestCheck_MarginTemperatureThreshold(t *testing.T) {
	original := GetDefaultThresholds()
	defer SetDefaultMarginThreshold(original)

	SetDefaultMarginThreshold(Thresholds{CelsiusSlowdownMargin: 10})

	tests := []struct {
		name                 string
		marginCelsius        int32
		expectHealthy        apiv1.HealthStateType
		expectReasonContains string
	}{
		{
			name:                 "Above threshold",
			marginCelsius:        15,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Equal to threshold",
			marginCelsius:        10,
			expectHealthy:        apiv1.HealthStateTypeDegraded,
			expectReasonContains: "margin threshold exceeded",
		},
		{
			name:                 "Below threshold",
			marginCelsius:        5,
			expectHealthy:        apiv1.HealthStateTypeDegraded,
			expectReasonContains: "margin threshold exceeded",
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

			temperature := Temperature{
				UUID:                           uuid,
				CurrentCelsiusGPUCore:          80,
				CurrentCelsiusHBM:              80,
				HBMTemperatureSupported:        true,
				ThresholdCelsiusSlowdownMargin: tt.marginCelsius,
				MarginTemperatureSupported:     true,
				ThresholdCelsiusShutdown:       120,
				ThresholdCelsiusSlowdown:       95,
				ThresholdCelsiusMemMax:         100,
				ThresholdCelsiusGPUMax:         100,
				UsedPercentShutdown:            "66.67", // 80/120 = 66.67%
				UsedPercentSlowdown:            "84.21", // 80/95 = 84.21%
				UsedPercentMemMax:              "80.00", // 80/100 = 80.00%
				UsedPercentGPUMax:              "80.00", // 80/100 = 80.00%
			}

			getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
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

func TestCheck_MarginTemperatureFallbackToHBM(t *testing.T) {
	tests := []struct {
		name                 string
		hbmTemp              uint32
		memMaxThreshold      uint32
		expectHealthy        apiv1.HealthStateType
		expectReasonContains string
	}{
		{
			name:                 "Below threshold",
			hbmTemp:              90,
			memMaxThreshold:      100,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Equal to threshold",
			hbmTemp:              100,
			memMaxThreshold:      100,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Above threshold",
			hbmTemp:              110,
			memMaxThreshold:      100,
			expectHealthy:        apiv1.HealthStateTypeDegraded,
			expectReasonContains: "temperature is",
		},
		{
			name:                 "Threshold is zero (disabled)",
			hbmTemp:              110,
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

			temperature := Temperature{
				UUID:                           uuid,
				CurrentCelsiusGPUCore:          75,
				CurrentCelsiusHBM:              tt.hbmTemp,
				HBMTemperatureSupported:        true,
				ThresholdCelsiusSlowdownMargin: 0,
				MarginTemperatureSupported:     false,
				ThresholdCelsiusShutdown:       120,
				ThresholdCelsiusSlowdown:       95,
				ThresholdCelsiusMemMax:         tt.memMaxThreshold,
				ThresholdCelsiusGPUMax:         100,
				UsedPercentShutdown:            "58.33",
				UsedPercentSlowdown:            "73.68",
				UsedPercentMemMax:              "0.0",
				UsedPercentGPUMax:              "70.00",
			}

			getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
				return temperature, nil
			}

			component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
			result := component.Check()

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

	// Use nvmlerrors.ErrGPULost for the error
	getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
		return Temperature{}, nvmlerrors.ErrGPULost
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
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

// mockNVMLInstanceWithInitError supports returning a custom init error
type mockNVMLInstanceWithInitError struct {
	devices   map[string]device.Device
	exists    bool
	prodName  string
	initError error
}

func (m *mockNVMLInstanceWithInitError) NVMLExists() bool {
	return m.exists
}

func (m *mockNVMLInstanceWithInitError) Library() nvml_lib.Library {
	return nil
}

func (m *mockNVMLInstanceWithInitError) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockNVMLInstanceWithInitError) ProductName() string {
	return m.prodName
}

func (m *mockNVMLInstanceWithInitError) Architecture() string {
	return ""
}

func (m *mockNVMLInstanceWithInitError) Brand() string {
	return ""
}

func (m *mockNVMLInstanceWithInitError) DriverVersion() string {
	return ""
}

func (m *mockNVMLInstanceWithInitError) DriverMajor() int {
	return 0
}

func (m *mockNVMLInstanceWithInitError) CUDAVersion() string {
	return ""
}

func (m *mockNVMLInstanceWithInitError) FabricManagerSupported() bool {
	return true
}

func (m *mockNVMLInstanceWithInitError) FabricStateSupported() bool {
	return false
}

func (m *mockNVMLInstanceWithInitError) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstanceWithInitError) Shutdown() error {
	return nil
}

func (m *mockNVMLInstanceWithInitError) InitError() error {
	return m.initError
}

func TestCheckResult_ComponentName(t *testing.T) {
	t.Run("non-nil checkResult", func(t *testing.T) {
		cr := &checkResult{
			health: apiv1.HealthStateTypeHealthy,
			reason: "all good",
		}
		assert.Equal(t, Name, cr.ComponentName())
	})

	t.Run("empty checkResult", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
	})
}

func TestCheckResult_String(t *testing.T) {
	t.Run("nil checkResult", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.String())
	})

	t.Run("empty temperatures", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "no data", cr.String())
	})

	t.Run("with temperatures HBM and margin supported", func(t *testing.T) {
		cr := &checkResult{
			Temperatures: []Temperature{
				{
					UUID:                           "gpu-uuid-1",
					BusID:                          "0000:0f:00.0",
					CurrentCelsiusGPUCore:          75,
					CurrentCelsiusHBM:              80,
					HBMTemperatureSupported:        true,
					ThresholdCelsiusSlowdownMargin: 15,
					MarginTemperatureSupported:     true,
					ThresholdCelsiusMemMax:         100,
					UsedPercentMemMax:              "80.00",
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-uuid-1")
		assert.Contains(t, result, "0000:0f:00.0")
		assert.Contains(t, result, "75")
		assert.Contains(t, result, "80")
		assert.Contains(t, result, "15")
	})

	t.Run("with temperatures HBM not supported", func(t *testing.T) {
		cr := &checkResult{
			Temperatures: []Temperature{
				{
					UUID:                       "gpu-uuid-2",
					BusID:                      "0000:10:00.0",
					CurrentCelsiusGPUCore:      65,
					HBMTemperatureSupported:    false,
					MarginTemperatureSupported: false,
					ThresholdCelsiusMemMax:     100,
					UsedPercentMemMax:          "0.0",
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-uuid-2")
		assert.Contains(t, result, "n/a")
	})

	t.Run("with multiple temperatures", func(t *testing.T) {
		cr := &checkResult{
			Temperatures: []Temperature{
				{
					UUID:                           "gpu-uuid-a",
					BusID:                          "0000:0a:00.0",
					CurrentCelsiusGPUCore:          70,
					HBMTemperatureSupported:        true,
					CurrentCelsiusHBM:              85,
					MarginTemperatureSupported:     true,
					ThresholdCelsiusSlowdownMargin: 10,
					ThresholdCelsiusMemMax:         100,
					UsedPercentMemMax:              "85.00",
				},
				{
					UUID:                       "gpu-uuid-b",
					BusID:                      "0000:0b:00.0",
					CurrentCelsiusGPUCore:      72,
					HBMTemperatureSupported:    false,
					MarginTemperatureSupported: false,
					ThresholdCelsiusMemMax:     100,
					UsedPercentMemMax:          "0.0",
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-uuid-a")
		assert.Contains(t, result, "gpu-uuid-b")
	})
}

func TestCheckResult_Summary(t *testing.T) {
	t.Run("with reason", func(t *testing.T) {
		cr := &checkResult{reason: "test reason"}
		assert.Equal(t, "test reason", cr.Summary())
	})

	t.Run("empty reason", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "", cr.Summary())
	})
}

func TestCheckResult_HealthStateType(t *testing.T) {
	t.Run("nil checkResult", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	})

	t.Run("healthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeHealthy}
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	t.Run("unhealthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeUnhealthy}
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	})

	t.Run("degraded", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeDegraded}
		assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.HealthStateType())
	})
}

func TestCheck_NilNVMLInstance(t *testing.T) {
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	comp := &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: nil,
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", cr.reason)
}

func TestCheck_NVMLNotExists(t *testing.T) {
	ctx := context.Background()

	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   false,
		prodName: "Test GPU",
	}

	comp := MockTemperatureComponent(ctx, mockNVML, nil).(*component)
	result := comp.Check()

	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", cr.reason)
}

func TestCheck_InitError(t *testing.T) {
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	initErr := errors.New("error getting device handle for index '0': Unknown Error")
	mockInst := &mockNVMLInstanceWithInitError{
		devices:   map[string]device.Device{},
		exists:    true,
		prodName:  "Test GPU",
		initError: initErr,
	}

	comp := &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: mockInst,
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "NVML initialization error")
	assert.NotNil(t, cr.suggestedActions)
	assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
}

func TestCheck_EmptyProductName(t *testing.T) {
	ctx := context.Background()

	mockNVML := &mockNVMLInstance{
		devices:  map[string]device.Device{},
		exists:   true,
		prodName: "",
	}

	comp := MockTemperatureComponent(ctx, mockNVML, nil).(*component)
	result := comp.Check()

	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "missing product name")
}

func TestCheckResult_HealthStates_WithSuggestedActions(t *testing.T) {
	cr := &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "GPU lost",
		err:    nvmlerrors.ErrGPULost,
		suggestedActions: &apiv1.SuggestedActions{
			Description: "GPU lost",
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.NotNil(t, states[0].SuggestedActions)
	assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	assert.Equal(t, "GPU lost", states[0].Error)
}

func TestCheckResult_HealthStates_WithExtraInfo(t *testing.T) {
	cr := &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
		Temperatures: []Temperature{
			{
				UUID:                     "gpu-uuid-123",
				CurrentCelsiusGPUCore:    75,
				ThresholdCelsiusShutdown: 120,
			},
		},
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.NotEmpty(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo["data"], "gpu-uuid-123")
}

func TestCheckResult_HealthStates_NoExtraInfo(t *testing.T) {
	cr := &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.Empty(t, states[0].ExtraInfo)
}

// TestCheck_MarginTemperatureThresholdEdgeCases tests edge cases for margin temperature threshold:
// - When threshold is 0 (disabled), no alert should be triggered
// - When GPU reports margin of 0 or negative (unreliable data from old GPUs or H100), no alert should be triggered
func TestCheck_MarginTemperatureThresholdEdgeCases(t *testing.T) {
	tests := []struct {
		name                 string
		thresholdMargin      int32 // marginThreshold.CelsiusSlowdownMargin
		gpuMargin            int32 // temp.ThresholdCelsiusSlowdownMargin
		expectHealthy        apiv1.HealthStateType
		expectReasonContains string
	}{
		{
			name:                 "Threshold is 0 (disabled) - no alert even with low margin",
			thresholdMargin:      0,
			gpuMargin:            5,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Threshold is 0 (disabled) - no alert with 0 margin",
			thresholdMargin:      0,
			gpuMargin:            0,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Threshold is 0 (disabled) - no alert with negative margin",
			thresholdMargin:      0,
			gpuMargin:            -5,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "GPU reports margin 0 (old GPU/H100 unreliable) - no alert even with positive threshold",
			thresholdMargin:      10,
			gpuMargin:            0,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "GPU reports negative margin (unreliable data) - no alert even with positive threshold",
			thresholdMargin:      10,
			gpuMargin:            -1,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "GPU reports negative margin -5 (unreliable data) - no alert even with positive threshold",
			thresholdMargin:      10,
			gpuMargin:            -5,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Valid margin above threshold - healthy",
			thresholdMargin:      10,
			gpuMargin:            15,
			expectHealthy:        apiv1.HealthStateTypeHealthy,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Valid margin equal to threshold - degraded",
			thresholdMargin:      10,
			gpuMargin:            10,
			expectHealthy:        apiv1.HealthStateTypeDegraded,
			expectReasonContains: "margin threshold exceeded",
		},
		{
			name:                 "Valid margin below threshold - degraded",
			thresholdMargin:      10,
			gpuMargin:            5,
			expectHealthy:        apiv1.HealthStateTypeDegraded,
			expectReasonContains: "margin threshold exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore default thresholds
			original := GetDefaultThresholds()
			defer SetDefaultMarginThreshold(original)

			SetDefaultMarginThreshold(Thresholds{CelsiusSlowdownMargin: tt.thresholdMargin})

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

			temperature := Temperature{
				UUID:                           uuid,
				CurrentCelsiusGPUCore:          80,
				CurrentCelsiusHBM:              80,
				HBMTemperatureSupported:        true,
				ThresholdCelsiusSlowdownMargin: tt.gpuMargin,
				MarginTemperatureSupported:     true,
				ThresholdCelsiusShutdown:       120,
				ThresholdCelsiusSlowdown:       95,
				ThresholdCelsiusMemMax:         100,
				ThresholdCelsiusGPUMax:         100,
				UsedPercentShutdown:            "66.67",
				UsedPercentSlowdown:            "84.21",
				UsedPercentMemMax:              "80.00",
				UsedPercentGPUMax:              "80.00",
			}

			getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
				return temperature, nil
			}

			component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
			result := component.Check()

			data, ok := result.(*checkResult)
			require.True(t, ok, "result should be of type *checkResult")

			require.NotNil(t, data, "data should not be nil")
			assert.Equal(t, tt.expectHealthy, data.health, "health state mismatch for case: %s", tt.name)
			assert.Contains(t, data.reason, tt.expectReasonContains, "reason should contain expected text for case: %s", tt.name)
		})
	}
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

	mockNVML := &mockNVMLInstance{
		devices:  devs,
		exists:   true,
		prodName: "Test GPU",
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

	// Return a Reset-like error
	getTemperatureFunc := func(uuid string, dev device.Device) (Temperature, error) {
		return Temperature{}, nvmlerrors.ErrGPURequiresReset
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
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
	assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
}
