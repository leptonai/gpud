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
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

// Ensure we have access to the New function - it should be in the same package
var _ = New

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

func createMockGPUdInstance(ctx context.Context, nvmlInstance nvml.InstanceV2) *components.GPUdInstance {
	return &components.GPUdInstance{
		RootCtx:                    ctx,
		LibraryAndAlternativeNames: make(map[string][]string),
		LibrarySearchDirs:          []string{},
		KernelModulesToCheck:       []string{},
		NVMLInstance:               nvmlInstance,
		NVIDIAToolOverwrites:       common.ToolOverwrites{},
		Annotations:                make(map[string]string),
		DBRO:                       nil,
		EventStore:                 nil,
		RebootEventStore:           nil,
		MountPoints:                []string{},
		MountTargets:               []string{},
	}
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)

	assert.NoError(t, err)
	assert.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

func TestComponent_Start(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	// Execute start
	err = comp.Start()
	assert.NoError(t, err)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

func TestComponent_States_NoData(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)
	c.lastData = nil

	states := c.LastHealthStates()

	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestComponent_States_WithData(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)
	c.lastData = &Data{
		Driver: Driver{
			Version: "123.45",
		},
		CUDA: CUDA{
			Version: "11.2",
		},
		health: apiv1.StateTypeHealthy,
		reason: "all GPUs were checked",
	}

	states := c.LastHealthStates()

	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "all GPUs were checked", states[0].Reason)
	assert.NotNil(t, states[0].DeprecatedExtraInfo)
}

func TestComponent_States_Unhealthy(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)
	c.lastData = &Data{
		health: apiv1.StateTypeUnhealthy,
		reason: "error occurred",
		err:    errors.New("something went wrong"),
	}

	states := c.LastHealthStates()

	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "error occurred", states[0].Reason)
	assert.Equal(t, "something went wrong", states[0].Error)
}

func TestComponent_Events(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	events, err := comp.Events(context.Background(), time.Now())

	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestCheckOnce_Success(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)
	mockInstance.On("Devices").Return(make(map[string]device.Device))

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

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
	result := c.Check()
	d := result.(*Data)

	// Verify the results
	assert.NotNil(t, d)
	assert.Equal(t, apiv1.StateTypeHealthy, d.health)
	assert.Equal(t, "530.82.01", d.Driver.Version)
	assert.Equal(t, "12.7", d.CUDA.Version)
	assert.Equal(t, 1, d.GPU.DeviceCount)
	assert.Equal(t, 0, d.GPU.Attached) // No devices in our mock
}

func TestCheckOnce_DriverVersionError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "", errors.New("driver error")
	}

	result := c.Check()
	d := result.(*Data)

	assert.NotNil(t, d)
	assert.Equal(t, apiv1.StateTypeUnhealthy, d.health)
	assert.Equal(t, "error getting driver version: driver error", d.reason)
	assert.Error(t, d.err)
}

func TestCheckOnce_EmptyDriverVersion(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "", nil
	}

	result := c.Check()
	d := result.(*Data)

	assert.NotNil(t, d)
	assert.Equal(t, apiv1.StateTypeUnhealthy, d.health)
	assert.Equal(t, "driver version is empty", d.reason)
	assert.Error(t, d.err)
}

func TestCheckOnce_CUDAVersionError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "", errors.New("cuda error")
	}

	result := c.Check()
	d := result.(*Data)

	assert.NotNil(t, d)
	assert.Equal(t, apiv1.StateTypeUnhealthy, d.health)
	assert.Equal(t, "error getting CUDA version: cuda error", d.reason)
	assert.Error(t, d.err)
}

func TestCheckOnce_EmptyCUDAVersion(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "", nil
	}

	result := c.Check()
	d := result.(*Data)

	assert.NotNil(t, d)
	assert.Equal(t, apiv1.StateTypeUnhealthy, d.health)
	assert.Equal(t, "CUDA version is empty", d.reason)
	assert.Error(t, d.err)
}

func TestCheckOnce_DeviceCountError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "12.7", nil
	}

	c.getDeviceCountFunc = func() (int, error) {
		return 0, errors.New("device count error")
	}

	result := c.Check()
	d := result.(*Data)

	assert.NotNil(t, d)
	assert.Equal(t, apiv1.StateTypeUnhealthy, d.health)
	assert.Equal(t, "error getting device count: device count error", d.reason)
	assert.Error(t, d.err)
}

func TestData_GetHealthStates(t *testing.T) {
	// Test with nil data
	var nilData *Data
	states := nilData.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with healthy data
	healthyData := &Data{
		health: apiv1.StateTypeHealthy,
		reason: "all good",
	}
	states = healthyData.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)

	// Test with unhealthy data
	unhealthyData := &Data{
		health: apiv1.StateTypeUnhealthy,
		reason: "problems found",
		err:    errors.New("test error"),
	}
	states = unhealthyData.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
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
