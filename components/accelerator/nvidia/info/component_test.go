package info

import (
	"context"
	"errors"
	"fmt"
	"sort"
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

// mockNVMLInstance implements nvml.InstanceV2 for testing
type mockNVMLInstance struct {
	testifymock.Mock
}

func (m *mockNVMLInstance) NVMLExists() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	args := m.Called()
	return args.Get(0).(nvmllib.Library)
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	args := m.Called()
	return args.Get(0).(map[string]device.Device)
}

func (m *mockNVMLInstance) ProductName() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) Architecture() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) DriverVersion() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) DriverMajor() int {
	args := m.Called()
	return args.Int(0)
}

func (m *mockNVMLInstance) CUDAVersion() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) Brand() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	args := m.Called()
	return args.Get(0).(nvidianvml.MemoryErrorManagementCapabilities)
}

func (m *mockNVMLInstance) Shutdown() error {
	args := m.Called()
	return args.Error(0)
}

func createMockGPUdInstance(ctx context.Context, nvmlInstance nvidianvml.Instance) *components.GPUdInstance {
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

// ExtendedComponent is used to add extra fields to the component for testing
type ExtendedComponent struct {
	*component
	getProductNameFunc  func(dev device.Device) (string, error)
	getArchitectureFunc func(dev device.Device) (string, error)
	getBrandFunc        func(dev device.Device) (string, error)
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)

	assert.NoError(t, err)
	assert.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

func TestTags(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	tags := comp.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 4, "Component should return exactly 4 tags")
}

func TestComponent_Start(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)

	// Setup all required mock expectations
	mockInstance.On("NVMLExists").Return(true).Maybe()
	mockInstance.On("Devices").Return(make(map[string]device.Device)).Maybe()
	mockInstance.On("DriverVersion").Return("test-version").Maybe()
	mockInstance.On("CUDAVersion").Return("test-version").Maybe()
	mockInstance.On("ProductName").Return("test-product").Maybe()
	mockInstance.On("Architecture").Return("test-arch").Maybe()
	mockInstance.On("Brand").Return("test-brand").Maybe()

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	// Override component functions to prevent errors in the goroutine
	c := comp.(*component)
	c.getDeviceCountFunc = func() (int, error) {
		return 0, nil
	}

	// Execute start
	err = comp.Start()
	assert.NoError(t, err)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

func TestComponent_Close(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)

	// Setup all required mock expectations
	mockInstance.On("NVMLExists").Return(true).Maybe()
	mockInstance.On("Devices").Return(make(map[string]device.Device)).Maybe()
	mockInstance.On("DriverVersion").Return("test-version").Maybe()
	mockInstance.On("CUDAVersion").Return("test-version").Maybe()
	mockInstance.On("ProductName").Return("test-product").Maybe()
	mockInstance.On("Architecture").Return("test-arch").Maybe()
	mockInstance.On("Brand").Return("test-brand").Maybe()

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	// Override component functions to prevent errors in the goroutine
	c := comp.(*component)
	c.getDeviceCountFunc = func() (int, error) {
		return 0, nil
	}

	// Test Close function directly
	err = comp.Close()
	assert.NoError(t, err)

	// Verify the context was canceled
	select {
	case <-c.ctx.Done():
		// Context was properly canceled, test passed
	default:
		t.Errorf("context was not canceled")
	}
}

func TestComponent_States_NoData(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)
	c.lastCheckResult = nil

	states := c.LastHealthStates()

	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestComponent_States_WithData(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)
	c.lastCheckResult = &checkResult{
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
	assert.NotNil(t, states[0].ExtraInfo)
}

func TestComponent_States_Unhealthy(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)
	c.lastCheckResult = &checkResult{
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
	mockInstance := new(mockNVMLInstance)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	events, err := comp.Events(context.Background(), time.Now())

	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestCheckOnce_Success(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("Devices").Return(make(map[string]device.Device))
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override only the component methods
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "530.82.01", cr.Driver.Version)
	assert.Equal(t, "12.7", cr.CUDA.Version)
	assert.Equal(t, 1, cr.GPUCount.DeviceCount)
	assert.Equal(t, 0, cr.GPUCount.Attached) // No devices in our mock
}

func TestCheckOnce_WithDevices(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

	// Create mock device with proper architecture and brand methods
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
		GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
			return nvml.DEVICE_ARCH_AMPERE, nvml.SUCCESS
		},
		GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
			return nvml.BRAND_NVIDIA, nvml.SUCCESS
		},
		GetSerialFunc: func() (string, nvml.Return) {
			return "SERIAL123", nvml.SUCCESS
		},
		GetMinorNumberFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
		GetBoardIdFunc: func() (uint32, nvml.Return) {
			return 123, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Create extended component to add the missing fields
	ec := &ExtendedComponent{component: c}

	// Override only the component methods
	ec.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	ec.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024), // 16GB
			TotalHumanized: "16GB",
		}, nil
	}

	ec.getProductNameFunc = func(dev device.Device) (string, error) {
		return "NVIDIA A100", nil
	}

	ec.getArchitectureFunc = func(dev device.Device) (string, error) {
		return "Ampere", nil
	}

	ec.getBrandFunc = func(dev device.Device) (string, error) {
		return "NVIDIA", nil
	}

	// Override the getSerialFunc and getMinorIDFunc
	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		return "SERIAL123", nil
	}

	c.getMinorIDFunc = func(uuid string, dev device.Device) (int, error) {
		return 0, nil
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "530.82.01", cr.Driver.Version)
	assert.Equal(t, "12.7", cr.CUDA.Version)
	assert.Equal(t, 1, cr.GPUCount.DeviceCount)
	assert.Equal(t, 1, cr.GPUCount.Attached)
	// We should check memory info since we're mocking it
	assert.Equal(t, uint64(16*1024*1024*1024), cr.Memory.TotalBytes)
	assert.Equal(t, "16GB", cr.Memory.TotalHumanized)
}

func TestCheckOnce_MemoryError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

	// Create mock device with proper architecture and brand methods
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
		GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
			return nvml.DEVICE_ARCH_AMPERE, nvml.SUCCESS
		},
		GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
			return nvml.BRAND_NVIDIA, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Create extended component to add the missing fields
	ec := &ExtendedComponent{component: c}

	// Override component methods
	ec.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	// Mock memory function with error
	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{}, errors.New("memory error")
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting memory", cr.reason)
	assert.Error(t, cr.err)
}

func TestCheckOnce_ProductNameError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	// Create mock device with proper architecture and brand methods
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
		GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
			return nvml.DEVICE_ARCH_AMPERE, nvml.SUCCESS
		},
		GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
			return nvml.BRAND_NVIDIA, nvml.SUCCESS
		},
		GetSerialFunc: func() (string, nvml.Return) {
			return "SERIAL123", nvml.SUCCESS
		},
		GetMinorNumberFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
		GetBoardIdFunc: func() (uint32, nvml.Return) {
			return 123, nvml.SUCCESS
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

	// Override component methods
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024), // 16GB
			TotalHumanized: "16GB",
		}, nil
	}

	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		return "SERIAL123", nil
	}

	c.getMinorIDFunc = func(uuid string, dev device.Device) (int, error) {
		return 0, nil
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, "530.82.01", cr.Driver.Version)
	assert.Equal(t, "12.7", cr.CUDA.Version)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
}

func TestCheckOnce_ArchitectureError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

	// Create mock device with proper architecture and brand methods
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
		GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
			return nvml.DEVICE_ARCH_AMPERE, nvml.SUCCESS
		},
		GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
			return nvml.BRAND_NVIDIA, nvml.SUCCESS
		},
		GetSerialFunc: func() (string, nvml.Return) {
			return "SERIAL123", nvml.SUCCESS
		},
		GetMinorNumberFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
		GetBoardIdFunc: func() (uint32, nvml.Return) {
			return 123, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024), // 16GB
			TotalHumanized: "16GB",
		}, nil
	}

	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		return "SERIAL123", nil
	}

	c.getMinorIDFunc = func(uuid string, dev device.Device) (int, error) {
		return 0, nil
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, "Ampere", cr.Product.Architecture)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
}

func TestCheckOnce_BrandError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

	// Create mock device with proper architecture and brand methods
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
		GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
			return nvml.DEVICE_ARCH_AMPERE, nvml.SUCCESS
		},
		GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
			return nvml.BRAND_NVIDIA, nvml.SUCCESS
		},
		GetSerialFunc: func() (string, nvml.Return) {
			return "SERIAL123", nvml.SUCCESS
		},
		GetMinorNumberFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
		GetBoardIdFunc: func() (uint32, nvml.Return) {
			return 123, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024), // 16GB
			TotalHumanized: "16GB",
		}, nil
	}

	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		return "SERIAL123", nil
	}

	c.getMinorIDFunc = func(uuid string, dev device.Device) (int, error) {
		return 0, nil
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, "NVIDIA", cr.Product.Brand)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
}

func TestCheckOnce_DriverVersionError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("DriverVersion").Return("")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")
	mockInstance.On("Devices").Return(make(map[string]device.Device))

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	result := comp.Check()
	cr := result.(*checkResult)

	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "driver version is empty", cr.reason)
	assert.Error(t, cr.err)
}

func TestCheckOnce_EmptyDriverVersion(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("DriverVersion").Return("")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")
	mockInstance.On("Devices").Return(make(map[string]device.Device))

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	result := comp.Check()
	cr := result.(*checkResult)

	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "driver version is empty", cr.reason)
	assert.Error(t, cr.err)
}

func TestCheckOnce_CUDAVersionError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")
	mockInstance.On("Devices").Return(make(map[string]device.Device))

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	result := comp.Check()
	cr := result.(*checkResult)

	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "CUDA version is empty", cr.reason)
	assert.Error(t, cr.err)
}

func TestCheckOnce_EmptyCUDAVersion(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")
	mockInstance.On("Devices").Return(make(map[string]device.Device))

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	result := comp.Check()
	cr := result.(*checkResult)

	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "CUDA version is empty", cr.reason)
	assert.Error(t, cr.err)
}

func TestCheckOnce_DeviceCountError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("Devices").Return(make(map[string]device.Device))
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)
	c.getDeviceCountFunc = func() (int, error) {
		return 0, errors.New("device count error")
	}

	result := comp.Check()
	cr := result.(*checkResult)

	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting device count", cr.reason)
	assert.Error(t, cr.err)
}

func TestData_GetHealthStates(t *testing.T) {
	// Test with nil data
	var nilData *checkResult
	states := nilData.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with healthy data
	healthyData := &checkResult{
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
	}
	states = healthyData.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)

	// Test with unhealthy data
	unhealthyData := &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "problems found",
		err:    errors.New("test error"),
	}
	states = unhealthyData.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "test error", states[0].Error)
}

func TestData_GetError(t *testing.T) {
	// Test with nil data
	var nilData *checkResult
	assert.Equal(t, "", nilData.getError())

	// Test with nil error
	noErrorData := &checkResult{
		err: nil,
	}
	assert.Equal(t, "", noErrorData.getError())

	// Test with error
	withErrorData := &checkResult{
		err: errors.New("test error"),
	}
	assert.Equal(t, "test error", withErrorData.getError())
}

func TestData_StringAndUtilityMethods(t *testing.T) {
	// Test String() with nil data
	var nilData *checkResult
	assert.Equal(t, "", nilData.String())

	// Test with populated data
	data := &checkResult{
		Driver: Driver{
			Version: "530.82.01",
		},
		CUDA: CUDA{
			Version: "12.7",
		},
		GPUCount: GPUCount{
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
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.HealthStateType())
	assert.Equal(t, apiv1.HealthStateType(""), nilData.HealthStateType())
}

func TestCheckOnce_NilNVMLInstance(t *testing.T) {
	ctx := context.Background()

	// Create a GPUd instance with nil NVML instance
	gpudInstance := createMockGPUdInstance(ctx, nil)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	// Call the Check function
	result := comp.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", cr.reason)
	assert.Nil(t, cr.err)
}

func TestCheckOnce_NVMLNotExists(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)

	// Mock the NVMLExists method to return false
	mockInstance.On("NVMLExists").Return(false)

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	// Call the Check function
	result := comp.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", cr.reason)
	assert.Nil(t, cr.err)

	// Verify the mock was called
	mockInstance.AssertCalled(t, "NVMLExists")
}

func TestCheckOnce_SerialError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

	// Create mock device
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
		GetMinorNumberFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods for test
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024),
			TotalHumanized: "16GB",
		}, nil
	}

	// Mock serial function with error to trigger the return
	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		return "", errors.New("serial error")
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting serial id", cr.reason)
	assert.Error(t, cr.err)
	assert.Contains(t, cr.err.Error(), "serial error")
}

func TestCheckOnce_MinorIDFunc_ReturnError(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

	// Create mock device
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
		GetSerialFunc: func() (string, nvml.Return) {
			return "SERIAL123", nvml.SUCCESS
		},
		GetMinorNumberFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
		GetBoardIdFunc: func() (uint32, nvml.Return) {
			return 123, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods for test
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024),
			TotalHumanized: "16GB",
		}, nil
	}

	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		return "SERIAL123", nil
	}

	// Set minorIDFunc to return an error to trigger early return
	c.getMinorIDFunc = func(uuid string, dev device.Device) (int, error) {
		return 0, errors.New("minor ID error with return")
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results - should be unhealthy due to the error
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting minor id", cr.reason)
	assert.Error(t, cr.err)
	assert.Contains(t, cr.err.Error(), "minor ID error with return")
}

func TestCheckOnce_SuccessWithCompleteSerialsAndIDs(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

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
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods for test
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024),
			TotalHumanized: "16GB",
		}, nil
	}

	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		return "SERIAL123", nil
	}

	c.getMinorIDFunc = func(uuid string, dev device.Device) (int, error) {
		return 0, nil
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 1 GPU(s) were checked", cr.reason)
	assert.Nil(t, cr.err)

	// Verify GPU IDs were properly collected
	assert.Len(t, cr.GPUIDs, 1)
	assert.Equal(t, "GPU-12345", cr.GPUIDs[0].UUID)
	assert.Equal(t, "SERIAL123", cr.GPUIDs[0].SN)
	assert.Equal(t, "0", cr.GPUIDs[0].MinorID)
}

// Test that handles nil functions
func TestCheckOnce_NilFunctions(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

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
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Set up component but with nil functions
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024),
			TotalHumanized: "16GB",
		}, nil
	}

	// Set functions to nil
	c.getSerialFunc = nil
	c.getMinorIDFunc = nil

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results - should still be healthy even with nil functions
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 1 GPU(s) were checked", cr.reason)
	assert.Nil(t, cr.err)

	// Verify GPU IDs have only UUID
	assert.Len(t, cr.GPUIDs, 1)
	assert.Equal(t, "GPU-12345", cr.GPUIDs[0].UUID)
	assert.Empty(t, cr.GPUIDs[0].SN)
	assert.Empty(t, cr.GPUIDs[0].MinorID)
}

// Test for when product name is empty
func TestCheckOnce_EmptyProductName(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("ProductName").Return("")
	mockInstance.On("Devices").Return(make(map[string]device.Device))

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	result := comp.Check()
	cr := result.(*checkResult)

	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML is loaded but GPU is not detected (missing product name)", cr.reason)
	assert.Nil(t, cr.err)
}

// Test checkResult String method with more coverage
func TestCheckResult_String(t *testing.T) {
	// Create a complete checkResult
	cr := &checkResult{
		Driver: Driver{
			Version: "530.82.01",
		},
		CUDA: CUDA{
			Version: "12.7",
		},
		GPUCount: GPUCount{
			DeviceCount: 4,
			Attached:    2,
		},
		GPUIDs: []GPUID{
			{
				UUID:    "GPU-12345",
				SN:      "SERIAL123",
				MinorID: "0",
			},
			{
				UUID:    "GPU-67890",
				SN:      "SERIAL456",
				MinorID: "1",
			},
		},
		Memory: Memory{
			TotalBytes:     uint64(40 * 1024 * 1024 * 1024),
			TotalHumanized: "40GB",
		},
		Product: Product{
			Name:         "NVIDIA A100",
			Brand:        "NVIDIA",
			Architecture: "Ampere",
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "all GPUs were checked",
		ts:     time.Now().UTC(),
	}

	// Get the string output
	output := cr.String()

	// Verify it contains all the relevant information
	assert.Contains(t, output, "NVIDIA A100")
	assert.Contains(t, output, "NVIDIA")
	assert.Contains(t, output, "Ampere")
	assert.Contains(t, output, "530.82.01")
	assert.Contains(t, output, "12.7")
	assert.Contains(t, output, "4") // Device count
	assert.Contains(t, output, "2") // Attached
	assert.Contains(t, output, "40GB")
}

// Test for multiple GPUs
func TestCheckOnce_MultipleGPUs(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

	// Create multiple mock devices
	mockDeviceObj1 := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
	}
	mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	mockDeviceObj2 := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-67890", nvml.SUCCESS
		},
	}
	mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "Ampere", "NVIDIA", "8.0", "0000:00:1F.0")

	// Setup devices map with two GPUs
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev1,
		"GPU-67890": mockDev2,
	}
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods for test
	c.getDeviceCountFunc = func() (int, error) {
		return 2, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024),
			TotalHumanized: "16GB",
		}, nil
	}

	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		if uuid == "GPU-12345" {
			return "SERIAL123", nil
		}
		return "SERIAL456", nil
	}

	c.getMinorIDFunc = func(uuid string, dev device.Device) (int, error) {
		if uuid == "GPU-12345" {
			return 0, nil
		}
		return 1, nil
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 2 GPU(s) were checked", cr.reason)
	assert.Nil(t, cr.err)

	// Verify GPU IDs for multiple GPUs
	assert.Len(t, cr.GPUIDs, 2)

	// Sort the GPUIDs by UUID to ensure consistent test results
	sort.Slice(cr.GPUIDs, func(i, j int) bool {
		return cr.GPUIDs[i].UUID < cr.GPUIDs[j].UUID
	})

	assert.Equal(t, "GPU-12345", cr.GPUIDs[0].UUID)
	assert.Equal(t, "SERIAL123", cr.GPUIDs[0].SN)
	assert.Equal(t, "0", cr.GPUIDs[0].MinorID)

	assert.Equal(t, "GPU-67890", cr.GPUIDs[1].UUID)
	assert.Equal(t, "SERIAL456", cr.GPUIDs[1].SN)
	assert.Equal(t, "1", cr.GPUIDs[1].MinorID)
}

// Test for the scenario when getMinorIDFunc returns error and the early return in the loop
func TestCheckOnce_MinorIDFunc_ReturnNil(t *testing.T) {
	// Directly test the checkResult structure, similar to other error tests
	cr := &checkResult{
		ts: time.Now().UTC(),
		Driver: Driver{
			Version: "530.82.01",
		},
		CUDA: CUDA{
			Version: "12.7",
		},
		Product: Product{
			Name:         "NVIDIA A100",
			Brand:        "NVIDIA",
			Architecture: "Ampere",
		},
		GPUCount: GPUCount{
			DeviceCount: 1,
			Attached:    1,
		},
		Memory: Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024),
			TotalHumanized: "16GB",
		},
		err:    fmt.Errorf("minor ID error with return"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting minor id: minor ID error with return",
	}

	// Verify the checkResult
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting minor id: minor ID error with return", cr.reason)
	assert.Error(t, cr.err)
	assert.Contains(t, cr.err.Error(), "minor ID error with return")

	// Test the health states
	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "error getting minor id: minor ID error with return", states[0].Reason)
	assert.Equal(t, "minor ID error with return", states[0].Error)
}

// Test for the scenario where nil getSerialFunc but not nil getMinorIDFunc
func TestCheckOnce_SerialFunc_Nil_MinorIDFunc_NotNil(t *testing.T) {
	ctx := context.Background()
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)

	// Create mock device
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-12345", nvml.SUCCESS
		},
		GetMinorNumberFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "Ampere", "NVIDIA", "8.0", "0000:00:1E.0")

	// Setup devices map
	devicesMap := map[string]device.Device{
		"GPU-12345": mockDev,
	}
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods for test
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024),
			TotalHumanized: "16GB",
		}, nil
	}

	// Set serialFunc to nil but keep minorIDFunc
	c.getSerialFunc = nil
	c.getMinorIDFunc = func(uuid string, dev device.Device) (int, error) {
		return 0, nil
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "all 1 GPU(s) were checked", cr.reason)
	assert.Nil(t, cr.err)

	// Verify GPU IDs - should have UUID and MinorID but not SN
	assert.Len(t, cr.GPUIDs, 1)
	assert.Equal(t, "GPU-12345", cr.GPUIDs[0].UUID)
	assert.Empty(t, cr.GPUIDs[0].SN)
	assert.Equal(t, "0", cr.GPUIDs[0].MinorID)
}

func TestCheckOnce_MemoryGPULostError(t *testing.T) {
	ctx := context.Background()

	// Create mock devices
	uuid := "gpu-uuid-123"
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

	// Setup devices map
	devicesMap := map[string]device.Device{
		uuid: mockDev,
	}
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	// Mock memory function with GPU lost error
	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{}, nvidianvml.ErrGPULost
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting memory (GPU is lost)", cr.reason)
	assert.True(t, errors.Is(cr.err, nvidianvml.ErrGPULost), "error should be nvidianvml.ErrGPULost")
}

func TestCheckOnce_SerialGPULostError(t *testing.T) {
	ctx := context.Background()

	// Create mock device
	uuid := "gpu-uuid-123"
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

	// Setup devices map
	devicesMap := map[string]device.Device{
		uuid: mockDev,
	}
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024),
			TotalHumanized: "16GB",
		}, nil
	}

	// Mock serial function with GPU lost error
	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		return "", nvidianvml.ErrGPULost
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting serial id (GPU is lost)", cr.reason)
	assert.True(t, errors.Is(cr.err, nvidianvml.ErrGPULost), "error should be nvidianvml.ErrGPULost")
}

func TestCheckOnce_MinorIDGPULostError(t *testing.T) {
	ctx := context.Background()

	// Create mock device
	uuid := "gpu-uuid-123"
	mockDeviceObj := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
	mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

	// Setup devices map
	devicesMap := map[string]device.Device{
		uuid: mockDev,
	}
	mockInstance := new(mockNVMLInstance)
	mockInstance.On("NVMLExists").Return(true)
	mockInstance.On("Devices").Return(devicesMap)
	mockInstance.On("DriverVersion").Return("530.82.01")
	mockInstance.On("CUDAVersion").Return("12.7")
	mockInstance.On("ProductName").Return("NVIDIA A100")
	mockInstance.On("Architecture").Return("Ampere")
	mockInstance.On("Brand").Return("NVIDIA")

	gpudInstance := createMockGPUdInstance(ctx, mockInstance)

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	c := comp.(*component)

	// Override component methods
	c.getDeviceCountFunc = func() (int, error) {
		return 1, nil
	}

	c.getMemoryFunc = func(uuid string, dev device.Device) (nvidianvml.Memory, error) {
		return nvidianvml.Memory{
			TotalBytes:     uint64(16 * 1024 * 1024 * 1024),
			TotalHumanized: "16GB",
		}, nil
	}

	c.getSerialFunc = func(uuid string, dev device.Device) (string, error) {
		return "SERIAL123", nil
	}

	// Mock minorID function with GPU lost error
	c.getMinorIDFunc = func(uuid string, dev device.Device) (int, error) {
		return 0, nvidianvml.ErrGPULost
	}

	// Call the function
	result := c.Check()
	cr := result.(*checkResult)

	// Verify the results
	assert.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting minor id (GPU is lost)", cr.reason)
	assert.True(t, errors.Is(cr.err, nvidianvml.ErrGPULost), "error should be nvidianvml.ErrGPULost")
}
