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

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// mockInstanceV2 implements the nvidianvml.InstanceV2 interface for testing
type mockInstanceV2 struct {
	devices    map[string]device.Device
	nvmlExists bool
}

func (m *mockInstanceV2) NVMLExists() bool {
	return m.nvmlExists
}

func (m *mockInstanceV2) Library() nvmllib.Library {
	return nil
}

func (m *mockInstanceV2) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockInstanceV2) ProductName() string {
	return "test-product"
}

func (m *mockInstanceV2) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockInstanceV2) Shutdown() error {
	return nil
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

	states := successData.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)

	// Verify ExtraInfo contains JSON data
	dataJSON, ok := states[0].DeprecatedExtraInfo["data"]
	assert.True(t, ok)

	var parsedData Data
	err := json.Unmarshal([]byte(dataJSON), &parsedData)
	assert.NoError(t, err)
	assert.Equal(t, successData.ClockSpeeds, parsedData.ClockSpeeds)
}

// TestData_String tests the String method of Data
func TestData_String(t *testing.T) {
	// Test nil data
	var nilData *Data
	str := nilData.String()
	assert.Empty(t, str)

	// Test empty data
	emptyData := &Data{}
	str = emptyData.String()
	assert.Equal(t, "no data", str)

	// Test with clock speeds data
	dataWithClockSpeeds := &Data{
		ClockSpeeds: []nvidianvml.ClockSpeed{
			{
				UUID:                   "test-uuid-1",
				GraphicsMHz:            1000,
				MemoryMHz:              2000,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			},
			{
				UUID:                   "test-uuid-2",
				GraphicsMHz:            1500,
				MemoryMHz:              3000,
				ClockGraphicsSupported: false,
				ClockMemorySupported:   false,
			},
		},
	}

	str = dataWithClockSpeeds.String()
	assert.Contains(t, str, "test-uuid-1")
	assert.Contains(t, str, "1000 MHz")
	assert.Contains(t, str, "2000 MHz")
	assert.Contains(t, str, "test-uuid-2")
	assert.Contains(t, str, "1500 MHz")
	assert.Contains(t, str, "3000 MHz")
}

// TestData_Summary tests the Summary method of Data
func TestData_Summary(t *testing.T) {
	// Test nil data
	var nilData *Data
	summary := nilData.Summary()
	assert.Empty(t, summary)

	// Test with reason
	dataWithReason := &Data{
		reason: "test reason",
	}
	summary = dataWithReason.Summary()
	assert.Equal(t, "test reason", summary)
}

// TestData_HealthState tests the HealthState method of Data
func TestData_HealthState(t *testing.T) {
	// Test nil data
	var nilData *Data
	health := nilData.HealthState()
	assert.Empty(t, health)

	// Test with health state
	dataWithHealth := &Data{
		health: apiv1.HealthStateTypeHealthy,
	}
	health = dataWithHealth.HealthState()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, health)
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
	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}
	mockInstance := &mockInstanceV2{
		devices:    mockDevices,
		nvmlExists: true,
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockInstance,
	}
	comp, err := New(gpudInstance)
	require.NoError(t, err)

	c, ok := comp.(*component)
	require.True(t, ok)
	assert.Equal(t, mockInstance, c.nvmlInstance)
	assert.NotNil(t, c.getClockSpeedFunc)
	assert.NotNil(t, c.ctx)
	assert.NotNil(t, c.cancel)
}

// TestComponent_Start tests the Start method
func TestComponent_Start(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}
	mockInstance := &mockInstanceV2{
		devices:    mockDevices,
		nvmlExists: true,
	}

	c := &component{
		ctx:          ctx,
		cancel:       cancel,
		nvmlInstance: mockInstance,
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

	states := c.LastHealthStates()
	assert.Len(t, states, 1)

	// Test with valid data
	clockSpeeds := []nvidianvml.ClockSpeed{
		{UUID: "test-uuid", GraphicsMHz: 1000, MemoryMHz: 2000},
	}

	c.lastData = &Data{
		ClockSpeeds: clockSpeeds,
	}

	states = c.LastHealthStates()
	assert.Len(t, states, 1)

	dataJSON, ok := states[0].DeprecatedExtraInfo["data"]
	assert.True(t, ok)

	var parsedData Data
	err := json.Unmarshal([]byte(dataJSON), &parsedData)
	assert.NoError(t, err)
	assert.Equal(t, clockSpeeds, parsedData.ClockSpeeds)
}

// TestComponent_CheckOnce tests the CheckOnce method
func TestComponent_CheckOnce(t *testing.T) {
	ctx := context.Background()

	// Create mock device
	mockNvmlDevice := &mock.Device{}
	mockDevice := testutil.NewMockDevice(mockNvmlDevice, "test-arch", "test-brand", "1.0", "0000:00:00.0")

	mockDevices := map[string]device.Device{
		"test-uuid": mockDevice,
	}

	// Test successful case
	c := &component{
		ctx: ctx,
		nvmlInstance: &mockInstanceV2{
			devices:    mockDevices,
			nvmlExists: true,
		},
		getClockSpeedFunc: func(uuid string, dev device.Device) (nvidianvml.ClockSpeed, error) {
			return nvidianvml.ClockSpeed{
				UUID:        uuid,
				GraphicsMHz: 1000,
				MemoryMHz:   2000,
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*Data)
	require.True(t, ok)

	// Verify that lastData was updated
	require.NotNil(t, c.lastData)
	assert.Len(t, c.lastData.ClockSpeeds, 1)
	assert.Equal(t, "test-uuid", c.lastData.ClockSpeeds[0].UUID)
	assert.Equal(t, uint32(1000), c.lastData.ClockSpeeds[0].GraphicsMHz)
	assert.Equal(t, uint32(2000), c.lastData.ClockSpeeds[0].MemoryMHz)
	assert.Nil(t, c.lastData.err)
	assert.Equal(t, data, c.lastData)

	// Test error case
	testErr := errors.New("test error")
	c = &component{
		ctx: ctx,
		nvmlInstance: &mockInstanceV2{
			devices:    mockDevices,
			nvmlExists: true,
		},
		getClockSpeedFunc: func(uuid string, dev device.Device) (nvidianvml.ClockSpeed, error) {
			return nvidianvml.ClockSpeed{}, testErr
		},
	}

	result = c.Check()
	data, ok = result.(*Data)
	require.True(t, ok)

	// Verify that lastData contains the error
	require.NotNil(t, c.lastData)
	assert.Len(t, c.lastData.ClockSpeeds, 0)
	assert.Equal(t, testErr, c.lastData.err)
	assert.Equal(t, data, c.lastData)
}

// TestComponent_Check_NilNVML tests the Check method with nil NVML instance
func TestComponent_Check_NilNVML(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx:          ctx,
		nvmlInstance: nil,
	}

	result := c.Check()
	data, ok := result.(*Data)
	require.True(t, ok)

	// Verify health state when NVML is nil
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)
	assert.Nil(t, data.err)
}

// TestComponent_Check_NVMLNotLoaded tests the Check method when NVML is not loaded
func TestComponent_Check_NVMLNotLoaded(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx: ctx,
		nvmlInstance: &mockInstanceV2{
			nvmlExists: false,
		},
	}

	result := c.Check()
	data, ok := result.(*Data)
	require.True(t, ok)

	// Verify health state when NVML is not loaded
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML is not loaded", data.reason)
	assert.Nil(t, data.err)
}

// TestComponent_Check_MultipleDevices tests the Check method with multiple devices
func TestComponent_Check_MultipleDevices(t *testing.T) {
	ctx := context.Background()

	// Create mock devices
	mockNvmlDevice1 := &mock.Device{}
	mockDevice1 := testutil.NewMockDevice(mockNvmlDevice1, "test-arch-1", "test-brand-1", "1.0", "0000:00:00.0")

	mockNvmlDevice2 := &mock.Device{}
	mockDevice2 := testutil.NewMockDevice(mockNvmlDevice2, "test-arch-2", "test-brand-2", "1.0", "0000:00:01.0")

	mockDevices := map[string]device.Device{
		"test-uuid-1": mockDevice1,
		"test-uuid-2": mockDevice2,
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockInstanceV2{
			devices:    mockDevices,
			nvmlExists: true,
		},
		getClockSpeedFunc: func(uuid string, dev device.Device) (nvidianvml.ClockSpeed, error) {
			return nvidianvml.ClockSpeed{
				UUID:                   uuid,
				GraphicsMHz:            1000,
				MemoryMHz:              2000,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*Data)
	require.True(t, ok)

	// Verify that lastData was updated with both devices
	require.NotNil(t, c.lastData)
	assert.Len(t, c.lastData.ClockSpeeds, 2)
	assert.Contains(t, c.lastData.reason, "2 GPU(s) were checked")
	assert.Nil(t, c.lastData.err)
	assert.Equal(t, data, c.lastData)
}
