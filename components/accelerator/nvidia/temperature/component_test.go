// Package temperature tracks the NVIDIA per-GPU temperatures.
package temperature

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

	components "github.com/leptonai/gpud/api/v1"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// MockInstanceV2 is a simple mock implementation of nvidianvml.InstanceV2
type MockInstanceV2 struct {
	devices map[string]device.Device
}

func (m *MockInstanceV2) NVMLExists() bool {
	return true
}

func (m *MockInstanceV2) Library() nvml_lib.Library {
	// Return nil for testing - we're not actually using this
	return nil
}

func (m *MockInstanceV2) Devices() map[string]device.Device {
	return m.devices
}

func (m *MockInstanceV2) ProductName() string {
	return "Test GPU"
}

func (m *MockInstanceV2) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *MockInstanceV2) Shutdown() error {
	return nil
}

// MockTemperatureComponent creates a component with mocked functions for testing
func MockTemperatureComponent(
	ctx context.Context,
	nvmlInstance nvidianvml.InstanceV2,
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
	mockNVML := &MockInstanceV2{devices: map[string]device.Device{}}
	c := New(ctx, mockNVML)

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
	mockNVML := &MockInstanceV2{devices: map[string]device.Device{}}
	c := MockTemperatureComponent(ctx, mockNVML, nil)
	assert.Equal(t, Name, c.Name(), "Component name should match")
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

	mockNVML := &MockInstanceV2{devices: devs}

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
	component.CheckOnce()

	// Verify the data was collected
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Contains(t, lastData.reason, "all")
	assert.Len(t, lastData.Temperatures, 1)
	assert.Equal(t, temperature, lastData.Temperatures[0])
}

func TestCheckOnce_TemperatureError(t *testing.T) {
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

	mockNVML := &MockInstanceV2{devices: devs}

	errExpected := errors.New("temperature error")
	getTemperatureFunc := func(uuid string, dev device.Device) (nvidianvml.Temperature, error) {
		return nvidianvml.Temperature{}, errExpected
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc).(*component)
	component.CheckOnce()

	// Verify error handling
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.healthy, "data should be marked unhealthy")
	assert.Equal(t, errExpected, lastData.err)
	assert.Equal(t, "error getting temperature for device gpu-uuid-123", lastData.reason)
}

func TestCheckOnce_NoDevices(t *testing.T) {
	ctx := context.Background()

	mockNVML := &MockInstanceV2{devices: map[string]device.Device{}}

	component := MockTemperatureComponent(ctx, mockNVML, nil).(*component)
	component.CheckOnce()

	// Verify handling of no devices
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.True(t, lastData.healthy, "data should be marked healthy")
	assert.Contains(t, lastData.reason, "all 0")
	assert.Empty(t, lastData.Temperatures)
}

func TestCheckOnce_GetUsedPercentSlowdownError(t *testing.T) {
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

	mockNVML := &MockInstanceV2{devices: devs}

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
	component.CheckOnce()

	// Verify error handling for GetUsedPercentSlowdown failure
	component.lastMu.RLock()
	lastData := component.lastData
	component.lastMu.RUnlock()

	require.NotNil(t, lastData, "lastData should not be nil")
	assert.False(t, lastData.healthy, "data should be marked unhealthy")
	assert.NotNil(t, lastData.err)
	assert.Equal(t, "error getting used percent for slowdown for device gpu-uuid-123", lastData.reason)
}

func TestStates_WithData(t *testing.T) {
	ctx := context.Background()
	mockNVML := &MockInstanceV2{devices: map[string]device.Device{}}
	component := MockTemperatureComponent(ctx, mockNVML, nil).(*component)

	// Set test data
	component.lastMu.Lock()
	component.lastData = &Data{
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
		healthy: true,
		reason:  "checked 1 devices for temperature",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, components.StateHealthy, state.Health)
	assert.True(t, state.Healthy)
	assert.Equal(t, "checked 1 devices for temperature", state.Reason)
	assert.Contains(t, state.ExtraInfo["data"], "gpu-uuid-123")
}

func TestStates_WithError(t *testing.T) {
	ctx := context.Background()
	mockNVML := &MockInstanceV2{devices: map[string]device.Device{}}
	component := MockTemperatureComponent(ctx, mockNVML, nil).(*component)

	// Set test data with error
	component.lastMu.Lock()
	component.lastData = &Data{
		err:     errors.New("test temperature error"),
		healthy: false,
		reason:  "error getting temperature for device gpu-uuid-123",
	}
	component.lastMu.Unlock()

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, components.StateUnhealthy, state.Health)
	assert.False(t, state.Healthy)
	assert.Equal(t, "error getting temperature for device gpu-uuid-123", state.Reason)
	assert.Equal(t, "test temperature error", state.Error)
}

func TestStates_NoData(t *testing.T) {
	ctx := context.Background()
	mockNVML := &MockInstanceV2{devices: map[string]device.Device{}}
	component := MockTemperatureComponent(ctx, mockNVML, nil).(*component)

	// Don't set any data

	// Get states
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, components.StateHealthy, state.Health)
	assert.True(t, state.Healthy)
	assert.Equal(t, "no data yet", state.Reason)
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	mockNVML := &MockInstanceV2{devices: map[string]device.Device{}}
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
	mockNVML := &MockInstanceV2{devices: devs}

	getTemperatureFunc := func(uuid string, dev device.Device) (nvidianvml.Temperature, error) {
		callCount.Add(1)
		return nvidianvml.Temperature{}, nil
	}

	component := MockTemperatureComponent(ctx, mockNVML, getTemperatureFunc)

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
	mockNVML := &MockInstanceV2{devices: map[string]device.Device{}}
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

func TestCheckOnce_MemoryTemperatureThreshold(t *testing.T) {
	tests := []struct {
		name                 string
		currentTemp          uint32
		memMaxThreshold      uint32
		expectHealthy        bool
		expectReasonContains string
	}{
		{
			name:                 "Below threshold",
			currentTemp:          80,
			memMaxThreshold:      100,
			expectHealthy:        true,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Equal to threshold",
			currentTemp:          100,
			memMaxThreshold:      100,
			expectHealthy:        true,
			expectReasonContains: "no temperature issue found",
		},
		{
			name:                 "Above threshold",
			currentTemp:          110,
			memMaxThreshold:      100,
			expectHealthy:        false,
			expectReasonContains: "exceeding the HBM temperature threshold",
		},
		{
			name:                 "Threshold is zero (disabled)",
			currentTemp:          110,
			memMaxThreshold:      0,
			expectHealthy:        true,
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

			mockNVML := &MockInstanceV2{devices: devs}

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
			component.CheckOnce()

			// Verify the data was collected
			component.lastMu.RLock()
			lastData := component.lastData
			component.lastMu.RUnlock()

			require.NotNil(t, lastData, "lastData should not be nil")
			assert.Equal(t, tt.expectHealthy, lastData.healthy, "health state mismatch")
			assert.Contains(t, lastData.reason, tt.expectReasonContains)
			assert.Len(t, lastData.Temperatures, 1)
		})
	}
}
