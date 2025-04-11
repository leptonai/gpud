package info

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

// MockNVMLInstanceV2 implements nvml.InstanceV2 for testing
type MockNVMLInstanceV2 struct {
	mock.Mock
}

func (m *MockNVMLInstanceV2) NVMLExists() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockNVMLInstanceV2) Library() nvml_lib.Library {
	args := m.Called()
	return args.Get(0).(nvml_lib.Library)
}

func (m *MockNVMLInstanceV2) Devices() map[string]device.Device {
	args := m.Called()
	return args.Get(0).(map[string]device.Device)
}

func (m *MockNVMLInstanceV2) ProductName() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockNVMLInstanceV2) GetMemoryErrorManagementCapabilities() nvml.MemoryErrorManagementCapabilities {
	args := m.Called()
	return args.Get(0).(nvml.MemoryErrorManagementCapabilities)
}

func (m *MockNVMLInstanceV2) Shutdown() error {
	args := m.Called()
	return args.Error(0)
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	comp := New(ctx, mockInstance)

	assert.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

func TestComponent_Start(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	comp := New(ctx, mockInstance)

	// Execute start
	err := comp.Start()
	assert.NoError(t, err)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

func TestComponent_States_NoData(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	c := New(ctx, mockInstance).(*component)
	c.lastData = nil

	states, err := c.States(context.Background())

	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestComponent_States_WithData(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	c := New(ctx, mockInstance).(*component)
	c.lastData = &Data{
		healthy: true,
		reason:  "all GPUs were checked",
		Driver: Driver{
			Version: "123.45",
		},
		CUDA: CUDA{
			Version: "11.2",
		},
	}

	states, err := c.States(context.Background())

	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "all GPUs were checked", states[0].Reason)
	assert.NotNil(t, states[0].ExtraInfo)
}

func TestComponent_States_Unhealthy(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	c := New(ctx, mockInstance).(*component)
	c.lastData = &Data{
		healthy: false,
		reason:  "error occurred",
		err:     errors.New("something went wrong"),
	}

	states, err := c.States(context.Background())

	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Equal(t, "error occurred", states[0].Reason)
	assert.Equal(t, "something went wrong", states[0].Error)
}

func TestComponent_Events(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	c := New(ctx, mockInstance)

	events, err := c.Events(context.Background(), time.Now())

	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestCheckOnce_Success(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)
	mockInstance.On("Devices").Return(make(map[string]device.Device))

	c := New(ctx, mockInstance).(*component)

	// Mock the functions
	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "12.7", nil
	}

	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	// Call the function
	c.CheckOnce()

	// Verify the results
	assert.NotNil(t, c.lastData)
	assert.True(t, c.lastData.healthy)
	assert.Equal(t, "530.82.01", c.lastData.Driver.Version)
	assert.Equal(t, "12.7", c.lastData.CUDA.Version)
	assert.Equal(t, 1, c.lastData.GPU.DeviceCount)
	assert.Equal(t, 0, c.lastData.GPU.Attached) // No devices in our mock
}

func TestCheckOnce_DriverVersionError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	c := New(ctx, mockInstance).(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "", errors.New("driver error")
	}

	c.CheckOnce()

	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, "error getting driver version: driver error", c.lastData.reason)
	assert.Error(t, c.lastData.err)
}

func TestCheckOnce_EmptyDriverVersion(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	c := New(ctx, mockInstance).(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "", nil
	}

	c.CheckOnce()

	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, "driver version is empty", c.lastData.reason)
	assert.Error(t, c.lastData.err)
}

func TestCheckOnce_CUDAVersionError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	c := New(ctx, mockInstance).(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "", errors.New("cuda error")
	}

	c.CheckOnce()

	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, "error getting CUDA version: cuda error", c.lastData.reason)
	assert.Error(t, c.lastData.err)
}

func TestCheckOnce_EmptyCUDAVersion(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	c := New(ctx, mockInstance).(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "", nil
	}

	c.CheckOnce()

	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, "CUDA version is empty", c.lastData.reason)
	assert.Error(t, c.lastData.err)
}

func TestCheckOnce_DeviceCountError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	c := New(ctx, mockInstance).(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "12.7", nil
	}

	c.getDeviceCountFunc = func() (int, error) {
		return 0, errors.New("device count error")
	}

	c.CheckOnce()

	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, "error getting device count: device count error", c.lastData.reason)
	assert.Error(t, c.lastData.err)
}

func TestData_GetStates(t *testing.T) {
	// Test with nil data
	var nilData *Data
	states, err := nilData.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with healthy data
	healthyData := &Data{
		healthy: true,
		reason:  "all good",
	}
	states, err = healthyData.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)

	// Test with unhealthy data
	unhealthyData := &Data{
		healthy: false,
		reason:  "problems found",
		err:     errors.New("test error"),
	}
	states, err = unhealthyData.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Equal(t, "test error", states[0].Error)
}

func TestData_GetError(t *testing.T) {
	// Test with nil data
	var nilData *Data
	assert.Equal(t, "", nilData.getError())

	// Test with nil error
	noErrorData := &Data{
		err: nil,
	}
	assert.Equal(t, "", noErrorData.getError())

	// Test with error
	withErrorData := &Data{
		err: errors.New("test error"),
	}
	assert.Equal(t, "test error", withErrorData.getError())
}
