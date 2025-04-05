package clockspeed

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// Mock implementation of nvml.InstanceV2
type mockNVMLInstance struct {
	devices map[string]device.Device
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockNVMLInstance) ProductName() string {
	return "NVIDIA Test GPU"
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return true
}

func (m *mockNVMLInstance) Library() lib.Library {
	return nil
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

// Helper function to create a mock NVML instance
func createMockNVMLInstance() *mockNVMLInstance {
	return &mockNVMLInstance{
		devices: make(map[string]device.Device),
	}
}

// TestData_GetReason tests the getReason method of Data
func TestData_GetReason(t *testing.T) {
	// Test nil data
	var nilData *Data
	reason := nilData.getReason()
	assert.Equal(t, "no clock speed data", reason)

	// Test data with error
	testError := errors.New("test error")
	errData := &Data{
		err: testError,
	}

	reason = errData.getReason()
	assert.Contains(t, reason, "failed to get clock speed data")

	// Test successful data
	successData := &Data{
		ClockSpeeds: []nvidianvml.ClockSpeed{
			{UUID: "test-uuid", GraphicsMHz: 1000, MemoryMHz: 2000},
		},
	}

	reason = successData.getReason()
	assert.Contains(t, reason, "found 1 GPU(s)")
}

// TestData_GetHealth tests the getHealth method of Data
func TestData_GetHealth(t *testing.T) {
	// Test nil data
	var nilData *Data
	health, healthy := nilData.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	// Test data with error
	testError := errors.New("test error")
	errData := &Data{
		err: testError,
	}

	health, healthy = errData.getHealth()
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)

	// Test successful data
	successData := &Data{
		ClockSpeeds: []nvidianvml.ClockSpeed{
			{UUID: "test-uuid", GraphicsMHz: 1000, MemoryMHz: 2000},
		},
	}

	health, healthy = successData.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)
}

// TestData_GetError tests the getError method of Data
func TestData_GetError(t *testing.T) {
	// Test nil data
	var nilData *Data
	errStr := nilData.getError()
	assert.Empty(t, errStr)

	// Test data with error
	testError := errors.New("test error")
	errData := &Data{
		err: testError,
	}

	errStr = errData.getError()
	assert.Equal(t, testError.Error(), errStr)

	// Test successful data
	successData := &Data{
		ClockSpeeds: []nvidianvml.ClockSpeed{
			{UUID: "test-uuid", GraphicsMHz: 1000, MemoryMHz: 2000},
		},
	}

	errStr = successData.getError()
	assert.Empty(t, errStr)
}

// TestData_GetStates tests the getStates method of Data
func TestData_GetStates(t *testing.T) {
	// Test successful data
	successData := &Data{
		ClockSpeeds: []nvidianvml.ClockSpeed{
			{UUID: "test-uuid", GraphicsMHz: 1000, MemoryMHz: 2000},
		},
	}

	states, err := successData.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)

	// Verify ExtraInfo contains JSON data
	dataJSON, ok := states[0].ExtraInfo["data"]
	assert.True(t, ok)

	var parsedData Data
	err = json.Unmarshal([]byte(dataJSON), &parsedData)
	assert.NoError(t, err)
	assert.Equal(t, successData.ClockSpeeds, parsedData.ClockSpeeds)
}

// TestComponent_Events tests the Events method
func TestComponent_Events(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx: ctx,
	}

	events, err := c.Events(ctx, time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)
}

// TestComponent_Close tests the Close method
func TestComponent_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,
	}

	err := c.Close()
	assert.NoError(t, err)
}

// TestComponent_Name tests the Name method
func TestComponent_Name(t *testing.T) {
	c := &component{}
	assert.Equal(t, Name, c.Name())
}

// TestNew tests the New function
func TestNew(t *testing.T) {
	ctx := context.Background()
	nvmlInstance := createMockNVMLInstance()

	comp := New(ctx, nvmlInstance)

	c, ok := comp.(*component)
	require.True(t, ok)
	assert.NotNil(t, c.nvmlInstanceV2)
	assert.NotNil(t, c.getClockSpeedFunc)
	assert.NotNil(t, c.ctx)
	assert.NotNil(t, c.cancel)
}

// TestComponent_Start tests the Start method
func TestComponent_Start(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nvmlInstance := createMockNVMLInstance()

	c := &component{
		ctx:            ctx,
		cancel:         cancel,
		nvmlInstanceV2: nvmlInstance,
		getClockSpeedFunc: func(uuid string, dev device.Device) (nvidianvml.ClockSpeed, error) {
			return nvidianvml.ClockSpeed{}, nil
		},
	}

	err := c.Start()
	assert.NoError(t, err)

	// Allow the goroutine time to initialize
	time.Sleep(100 * time.Millisecond)
}

// TestComponent_States tests the States method
func TestComponent_States(t *testing.T) {
	ctx := context.Background()

	// Test when lastData is nil
	c := &component{
		ctx: ctx,
	}

	states, err := c.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	// Test with valid data
	clockSpeeds := []nvidianvml.ClockSpeed{
		{UUID: "test-uuid", GraphicsMHz: 1000, MemoryMHz: 2000},
	}

	c.lastData = &Data{
		ClockSpeeds: clockSpeeds,
	}

	states, err = c.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	dataJSON, ok := states[0].ExtraInfo["data"]
	assert.True(t, ok)

	var parsedData Data
	err = json.Unmarshal([]byte(dataJSON), &parsedData)
	assert.NoError(t, err)
	assert.Equal(t, clockSpeeds, parsedData.ClockSpeeds)
}

// TestComponent_CheckOnce tests the CheckOnce method
func TestComponent_CheckOnce(t *testing.T) {
	ctx := context.Background()

	// Create mock device
	mockNvmlDevice := &mock.Device{}
	mockDevice := testutil.NewMockDevice(mockNvmlDevice, "test-arch", "test-brand", "1.0", "0000:00:00.0")

	// Create mock NVML instance with devices
	nvmlInstance := &mockNVMLInstance{
		devices: map[string]device.Device{
			"test-uuid": mockDevice,
		},
	}

	// Test successful case
	c := &component{
		ctx:            ctx,
		nvmlInstanceV2: nvmlInstance,
		getClockSpeedFunc: func(uuid string, dev device.Device) (nvidianvml.ClockSpeed, error) {
			return nvidianvml.ClockSpeed{
				UUID:        uuid,
				GraphicsMHz: 1000,
				MemoryMHz:   2000,
			}, nil
		},
	}

	c.CheckOnce()

	// Verify that lastData was updated
	require.NotNil(t, c.lastData)
	assert.Len(t, c.lastData.ClockSpeeds, 1)
	assert.Equal(t, "test-uuid", c.lastData.ClockSpeeds[0].UUID)
	assert.Equal(t, uint32(1000), c.lastData.ClockSpeeds[0].GraphicsMHz)
	assert.Equal(t, uint32(2000), c.lastData.ClockSpeeds[0].MemoryMHz)
	assert.Nil(t, c.lastData.err)

	// Test error case
	testErr := errors.New("test error")
	c = &component{
		ctx:            ctx,
		nvmlInstanceV2: nvmlInstance,
		getClockSpeedFunc: func(uuid string, dev device.Device) (nvidianvml.ClockSpeed, error) {
			return nvidianvml.ClockSpeed{}, testErr
		},
	}

	c.CheckOnce()

	// Verify that lastData contains the error
	require.NotNil(t, c.lastData)
	assert.Len(t, c.lastData.ClockSpeeds, 0)
	assert.Equal(t, testErr, c.lastData.err)
}
