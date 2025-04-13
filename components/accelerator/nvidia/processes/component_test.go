package processes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// Override the metricRunningProcesses for testing
var _ = prometheus.Register(metricRunningProcesses) // This will fail silently if metrics are already registered

// mockNVMLInstance is a mock implementation of nvml.InstanceV2
type mockNVMLInstance struct {
	devicesFunc func() map[string]device.Device
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	if m.devicesFunc != nil {
		return m.devicesFunc()
	}
	return nil
}

// Override other InstanceV2 methods to return empty values
func (m *mockNVMLInstance) NVMLExists() bool         { return true }
func (m *mockNVMLInstance) Library() nvmllib.Library { return nil }
func (m *mockNVMLInstance) ProductName() string      { return "Test GPU" }
func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}
func (m *mockNVMLInstance) Shutdown() error { return nil }

func createMockDevice(uuid string, runningProcs []nvml.ProcessInfo) device.Device {
	mockDevice := &mock.Device{
		GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
			return runningProcs, nvml.SUCCESS
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
		GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
			return []nvml.ProcessUtilizationSample{
				{
					Pid:       uint32(pid),
					TimeStamp: 123456789,
					SmUtil:    50,
					MemUtil:   30,
					EncUtil:   0,
					DecUtil:   0,
				},
			}, nvml.SUCCESS
		},
	}

	return testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mockInstance := &mockNVMLInstance{}
	comp := New(ctx, mockInstance)

	assert.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

func TestComponentName(t *testing.T) {
	ctx := context.Background()
	mockInstance := &mockNVMLInstance{}
	comp := New(ctx, mockInstance)

	assert.Equal(t, Name, comp.Name())
}

func TestStartAndClose(t *testing.T) {
	ctx := context.Background()
	mockInstance := &mockNVMLInstance{}
	comp := New(ctx, mockInstance)

	// Test Start method
	err := comp.Start()
	assert.NoError(t, err)

	// Test Close method
	err = comp.Close()
	assert.NoError(t, err)
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	mockInstance := &mockNVMLInstance{}
	comp := New(ctx, mockInstance)

	events, err := comp.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestCheckOnceSuccess(t *testing.T) {
	ctx := context.Background()

	// Create mock devices with running processes
	mockDev1 := createMockDevice("gpu-uuid-1", []nvml.ProcessInfo{
		{Pid: 1234, UsedGpuMemory: 100000000},
	})

	mockDevices := map[string]device.Device{
		"gpu-uuid-1": mockDev1,
	}

	mockInstance := &mockNVMLInstance{
		devicesFunc: func() map[string]device.Device {
			return mockDevices
		},
	}

	// Create component with mocked getProcessesFunc
	c := &component{
		ctx:          ctx,
		cancel:       func() {},
		nvmlInstance: mockInstance,
		getProcessesFunc: func(uuid string, dev device.Device) (nvidianvml.Processes, error) {
			return nvidianvml.Processes{
				UUID: uuid,
				RunningProcesses: []nvidianvml.Process{
					{
						PID:                1234,
						GPUUsedMemoryBytes: 100000000,
					},
				},
			}, nil
		},
	}

	// Run CheckOnce
	c.CheckOnce()

	// Verify state is updated correctly
	states, err := c.States(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.True(t, states[0].DeprecatedHealthy)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
}

func TestCheckOnceError(t *testing.T) {
	ctx := context.Background()

	// Create mock devices with no processes
	mockDev1 := createMockDevice("gpu-uuid-1", nil)

	mockDevices := map[string]device.Device{
		"gpu-uuid-1": mockDev1,
	}

	mockInstance := &mockNVMLInstance{
		devicesFunc: func() map[string]device.Device {
			return mockDevices
		},
	}

	// Create component with mocked getProcessesFunc that returns an error
	testErr := errors.New("test error")
	c := &component{
		ctx:          ctx,
		cancel:       func() {},
		nvmlInstance: mockInstance,
		getProcessesFunc: func(uuid string, dev device.Device) (nvidianvml.Processes, error) {
			return nvidianvml.Processes{}, testErr
		},
	}

	// Run CheckOnce
	c.CheckOnce()

	// Verify state is updated correctly
	states, err := c.States(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.False(t, states[0].DeprecatedHealthy)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Error, testErr.Error())
}

func TestStatesWithNilData(t *testing.T) {
	ctx := context.Background()
	mockInstance := &mockNVMLInstance{}
	comp := New(ctx, mockInstance)

	// At this point, lastData should be nil
	states, err := comp.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))

	// Default values for nil data
	assert.Equal(t, Name, states[0].Name)
	assert.True(t, states[0].DeprecatedHealthy)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataGetStates(t *testing.T) {
	// Test healthy data
	healthyData := &Data{
		Processes: []nvidianvml.Processes{
			{
				UUID: "gpu-uuid-1",
				RunningProcesses: []nvidianvml.Process{
					{PID: 1234},
				},
			},
		},
		healthy: true,
		reason:  "all GPUs healthy",
	}

	states, err := healthyData.getStates()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.True(t, states[0].DeprecatedHealthy)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)

	// Test unhealthy data
	testErr := errors.New("test error")
	unhealthyData := &Data{
		err:     testErr,
		healthy: false,
		reason:  "GPU issue detected",
	}

	states, err = unhealthyData.getStates()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.False(t, states[0].DeprecatedHealthy)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Equal(t, testErr.Error(), states[0].Error)
}

func TestDataGetError(t *testing.T) {
	// Test nil data
	var nilData *Data
	assert.Equal(t, "", nilData.getError())

	// Test nil error
	noErrorData := &Data{}
	assert.Equal(t, "", noErrorData.getError())

	// Test with error
	testErr := errors.New("test error")
	errorData := &Data{err: testErr}
	assert.Equal(t, testErr.Error(), errorData.getError())
}
