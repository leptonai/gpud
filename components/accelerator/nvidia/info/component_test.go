package info

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/config/common"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// Ensure we have access to the New function - it should be in the same package
var _ = New

// MockNVMLInstanceV2 implements nvml.InstanceV2 for testing
type MockNVMLInstanceV2 struct {
	testifymock.Mock
}

func (m *MockNVMLInstanceV2) NVMLExists() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockNVMLInstanceV2) Library() nvmllib.Library {
	args := m.Called()
	return args.Get(0).(nvmllib.Library)
}

func (m *MockNVMLInstanceV2) Devices() map[string]device.Device {
	args := m.Called()
	return args.Get(0).(map[string]device.Device)
}

func (m *MockNVMLInstanceV2) ProductName() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockNVMLInstanceV2) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	args := m.Called()
	return args.Get(0).(nvidianvml.MemoryErrorManagementCapabilities)
}

func (m *MockNVMLInstanceV2) Shutdown() error {
	args := m.Called()
	return args.Error(0)
}

func createMockGPUdInstance(ctx context.Context, nvmlInstance nvidianvml.InstanceV2) *components.GPUdInstance {
	return &components.GPUdInstance{
		RootCtx:              ctx,
		KernelModulesToCheck: []string{},
		NVMLInstance:         nvmlInstance,
		NVIDIAToolOverwrites: common.ToolOverwrites{},
		Annotations:          make(map[string]string),
		DBRO:                 nil,
		EventStore:           nil,
		RebootEventStore:     nil,
		MountPoints:          []string{},
		MountTargets:         []string{},
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

func TestComponent_Close(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	// Test Close function directly
	err = comp.Close()
	assert.NoError(t, err)

	// Verify the context was canceled
	c := comp.(*component)
	select {
	case <-c.ctx.Done():
		// Context was properly canceled, test passed
	default:
		t.Errorf("context was not canceled")
	}
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
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
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
		health: apiv1.HealthStateTypeHealthy,
		reason: "all GPUs were checked",
	}

	states := c.LastHealthStates()

	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
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
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error occurred",
		err:    errors.New("something went wrong"),
	}

	states := c.LastHealthStates()

	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
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
	assert.Equal(t, apiv1.HealthStateTypeHealthy, d.health)
	assert.Equal(t, "530.82.01", d.Driver.Version)
	assert.Equal(t, "12.7", d.CUDA.Version)
	assert.Equal(t, 1, d.GPU.DeviceCount)
	assert.Equal(t, 0, d.GPU.Attached) // No devices in our mock
}

func TestCheckOnce_WithDevices(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	// Create mock device
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)

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

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024), // 16GB
			TotalHumanized: "16GB",
		}, nil
	}

	c.getProductNameFunc = func(dev device.Device) (string, error) {
		return "NVIDIA A100", nil
	}

	c.getArchitectureFunc = func(dev device.Device) (string, error) {
		return "Ampere", nil
	}

	c.getBrandFunc = func(dev device.Device) (string, error) {
		return "NVIDIA", nil
	}

	// Call the function
	result := c.Check()
	d := result.(*Data)

	// Verify the results
	assert.NotNil(t, d)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, d.health)
	assert.Equal(t, "530.82.01", d.Driver.Version)
	assert.Equal(t, "12.7", d.CUDA.Version)
	assert.Equal(t, 1, d.GPU.DeviceCount)
	assert.Equal(t, 1, d.GPU.Attached)
	assert.Equal(t, uint64(16*1024*1024*1024), d.Memory.TotalBytes)
	assert.Equal(t, "16GB", d.Memory.TotalHumanized)
	assert.Equal(t, "NVIDIA A100", d.Product.Name)
	assert.Equal(t, "Ampere", d.Product.Architecture)
	assert.Equal(t, "NVIDIA", d.Product.Brand)
}

func TestCheckOnce_MemoryError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	// Create mock device
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Mock the functions with success
	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "12.7", nil
	}

	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	// Mock memory function with error
	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{}, errors.New("memory error")
	}

	// Call the function
	result := c.Check()
	d := result.(*Data)

	// Verify the results
	assert.NotNil(t, d)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.health)
	assert.Equal(t, "error getting memory: memory error", d.reason)
	assert.Error(t, d.err)
}

func TestCheckOnce_ProductNameError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	// Create mock device
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Mock the functions with success
	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "12.7", nil
	}

	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes: uint64(16 * 1024 * 1024 * 1024), // 16GB
		}, nil
	}

	// Mock product name function with error
	c.getProductNameFunc = func(dev device.Device) (string, error) {
		return "", errors.New("product name error")
	}

	// Call the function
	result := c.Check()
	d := result.(*Data)

	// Verify the results
	assert.NotNil(t, d)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.health)
	assert.Equal(t, "error getting product name: product name error", d.reason)
	assert.Error(t, d.err)
}

func TestCheckOnce_ArchitectureError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	// Create mock device
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Mock the functions with success
	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "12.7", nil
	}

	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes: uint64(16 * 1024 * 1024 * 1024), // 16GB
		}, nil
	}

	c.getProductNameFunc = func(dev device.Device) (string, error) {
		return "NVIDIA A100", nil
	}

	// Mock architecture function with error
	c.getArchitectureFunc = func(dev device.Device) (string, error) {
		return "", errors.New("architecture error")
	}

	// Call the function
	result := c.Check()
	d := result.(*Data)

	// Verify the results
	assert.NotNil(t, d)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.health)
	assert.Equal(t, "error getting architecture: architecture error", d.reason)
	assert.Error(t, d.err)
}

func TestCheckOnce_BrandError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(MockNVMLInstanceV2)

	// Create mock device
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Mock the functions with success
	c.getDriverVersionFunc = func() (string, error) {
		return "530.82.01", nil
	}

	c.getCUDAVersionFunc = func() (string, error) {
		return "12.7", nil
	}

	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes: uint64(16 * 1024 * 1024 * 1024), // 16GB
		}, nil
	}

	c.getProductNameFunc = func(dev device.Device) (string, error) {
		return "NVIDIA A100", nil
	}

	c.getArchitectureFunc = func(dev device.Device) (string, error) {
		return "Ampere", nil
	}

	// Mock brand function with error
	c.getBrandFunc = func(dev device.Device) (string, error) {
		return "", errors.New("brand error")
	}

	// Call the function
	result := c.Check()
	d := result.(*Data)

	// Verify the results
	assert.NotNil(t, d)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.health)
	assert.Equal(t, "error getting brand: brand error", d.reason)
	assert.Error(t, d.err)
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
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.health)
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
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.health)
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
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.health)
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
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.health)
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
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.health)
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
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
	}
	states = healthyData.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)

	// Test with unhealthy data
	unhealthyData := &Data{
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "problems found",
		err:    errors.New("test error"),
	}
	states = unhealthyData.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
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

func TestData_StringAndUtilityMethods(t *testing.T) {
	// Test String() with nil data
	var nilData *Data
	assert.Equal(t, "", nilData.String())

	// Test with populated data
	data := &Data{
		Driver: Driver{
			Version: "530.82.01",
		},
		CUDA: CUDA{
			Version: "12.7",
		},
		GPU: GPU{
			DeviceCount: 2,
			Attached:    1,
		},
		Memory: Memory{
			TotalBytes:     16 * 1024 * 1024 * 1024,
			TotalHumanized: "16GB",
		},
		Product: Product{
			Name:         "NVIDIA A100",
			Brand:        "NVIDIA",
			Architecture: "Ampere",
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "all 2 GPU(s) were checked",
	}

	// Test String method
	assert.NotEmpty(t, data.String())

	// Test Summary method
	assert.Equal(t, "all 2 GPU(s) were checked", data.Summary())
	assert.Equal(t, "", nilData.Summary())

	// Test HealthState method
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.HealthState())
	assert.Equal(t, apiv1.HealthStateType(""), nilData.HealthState())
}
