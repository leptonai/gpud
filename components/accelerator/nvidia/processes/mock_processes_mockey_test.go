//go:build linux

package processes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// customMockNVMLInstanceProcesses with customizable NVMLExists and ProductName
type customMockNVMLInstanceProcesses struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
}

func (m *customMockNVMLInstanceProcesses) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstanceProcesses) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstanceProcesses) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstanceProcesses) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *customMockNVMLInstanceProcesses) ProductName() string   { return m.productName }
func (m *customMockNVMLInstanceProcesses) Architecture() string  { return "" }
func (m *customMockNVMLInstanceProcesses) Brand() string         { return "" }
func (m *customMockNVMLInstanceProcesses) DriverVersion() string { return "" }
func (m *customMockNVMLInstanceProcesses) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstanceProcesses) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstanceProcesses) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstanceProcesses) Library() lib.Library  { return nil }
func (m *customMockNVMLInstanceProcesses) Shutdown() error       { return nil }
func (m *customMockNVMLInstanceProcesses) InitError() error      { return nil }

// mockNVMLInstanceWithInitErrorProcesses returns an init error
type mockNVMLInstanceWithInitErrorProcesses struct {
	devs      map[string]device.Device
	initError error
}

func (m *mockNVMLInstanceWithInitErrorProcesses) Devices() map[string]device.Device { return m.devs }
func (m *mockNVMLInstanceWithInitErrorProcesses) FabricManagerSupported() bool      { return true }
func (m *mockNVMLInstanceWithInitErrorProcesses) FabricStateSupported() bool        { return false }
func (m *mockNVMLInstanceWithInitErrorProcesses) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNVMLInstanceWithInitErrorProcesses) ProductName() string   { return "NVIDIA H100" }
func (m *mockNVMLInstanceWithInitErrorProcesses) Architecture() string  { return "" }
func (m *mockNVMLInstanceWithInitErrorProcesses) Brand() string         { return "" }
func (m *mockNVMLInstanceWithInitErrorProcesses) DriverVersion() string { return "" }
func (m *mockNVMLInstanceWithInitErrorProcesses) DriverMajor() int      { return 0 }
func (m *mockNVMLInstanceWithInitErrorProcesses) CUDAVersion() string   { return "" }
func (m *mockNVMLInstanceWithInitErrorProcesses) NVMLExists() bool      { return true }
func (m *mockNVMLInstanceWithInitErrorProcesses) Library() lib.Library  { return nil }
func (m *mockNVMLInstanceWithInitErrorProcesses) Shutdown() error       { return nil }
func (m *mockNVMLInstanceWithInitErrorProcesses) InitError() error      { return m.initError }

// createMockProcessesComponent creates a component with mock functions for testing
func createMockProcessesComponent(
	ctx context.Context,
	getDevicesFunc func() map[string]device.Device,
	getProcessesFunc func(uuid string, dev device.Device) (Processes, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)

	mockInst := &customMockNVMLInstanceProcesses{
		devs:        getDevicesFunc(),
		nvmlExists:  true,
		productName: "NVIDIA H100",
	}

	// Override Devices to return our mock devices
	mockInstWrapper := &mockNVMLInstanceWrapper{
		getDevicesFunc: getDevicesFunc,
		nvmlExists:     true,
		productName:    "NVIDIA H100",
	}
	_ = mockInst // keep for reference

	return &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:     mockInstWrapper,
		getProcessesFunc: getProcessesFunc,
	}
}

// mockNVMLInstanceWrapper wraps device functions for testing
type mockNVMLInstanceWrapper struct {
	getDevicesFunc func() map[string]device.Device
	nvmlExists     bool
	productName    string
}

func (m *mockNVMLInstanceWrapper) Devices() map[string]device.Device { return m.getDevicesFunc() }
func (m *mockNVMLInstanceWrapper) FabricManagerSupported() bool      { return true }
func (m *mockNVMLInstanceWrapper) FabricStateSupported() bool        { return false }
func (m *mockNVMLInstanceWrapper) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNVMLInstanceWrapper) ProductName() string   { return m.productName }
func (m *mockNVMLInstanceWrapper) Architecture() string  { return "" }
func (m *mockNVMLInstanceWrapper) Brand() string         { return "" }
func (m *mockNVMLInstanceWrapper) DriverVersion() string { return "" }
func (m *mockNVMLInstanceWrapper) DriverMajor() int      { return 0 }
func (m *mockNVMLInstanceWrapper) CUDAVersion() string   { return "" }
func (m *mockNVMLInstanceWrapper) NVMLExists() bool      { return m.nvmlExists }
func (m *mockNVMLInstanceWrapper) Library() lib.Library  { return nil }
func (m *mockNVMLInstanceWrapper) Shutdown() error       { return nil }
func (m *mockNVMLInstanceWrapper) InitError() error      { return nil }

// mockProcessInspector implements processInspector for testing without mockey.
type mockProcessInspector struct {
	cmdlineSlice  []string
	cmdlineErr    error
	createTimeMS  int64
	createTimeErr error
	status        []string
	statusErr     error
	environ       []string
	environErr    error
}

func (m *mockProcessInspector) CmdlineSlice() ([]string, error) { return m.cmdlineSlice, m.cmdlineErr }
func (m *mockProcessInspector) CreateTime() (int64, error)      { return m.createTimeMS, m.createTimeErr }
func (m *mockProcessInspector) Status() ([]string, error)       { return m.status, m.statusErr }
func (m *mockProcessInspector) Environ() ([]string, error)      { return m.environ, m.environErr }

// TestNew_WithMockey tests the New function using mockey for isolation
func TestNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation", t, func() {
		ctx := context.Background()
		mockInstance := &mockNVMLInstance{
			devicesFunc: func() map[string]device.Device { return nil },
		}

		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockInstance,
		}

		c, err := New(gpudInstance)

		assert.NoError(t, err)
		assert.NotNil(t, c)
		assert.Equal(t, Name, c.Name())

		tc, ok := c.(*component)
		require.True(t, ok)
		assert.NotNil(t, tc.getProcessesFunc)
	})
}

// TestComponent_IsSupported_WithMockey tests IsSupported method with various conditions
func TestComponent_IsSupported_WithMockey(t *testing.T) {
	testCases := []struct {
		name         string
		setupNilNVML bool
		nvmlExists   bool
		productName  string
		expected     bool
	}{
		{
			name:         "nil NVML instance returns false",
			setupNilNVML: true,
			expected:     false,
		},
		{
			name:        "NVML not loaded returns false",
			nvmlExists:  false,
			productName: "NVIDIA H100",
			expected:    false,
		},
		{
			name:        "no product name returns false",
			nvmlExists:  true,
			productName: "",
			expected:    false,
		},
		{
			name:        "NVML loaded with product name returns true",
			nvmlExists:  true,
			productName: "NVIDIA H100",
			expected:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				ctx := context.Background()
				cctx, cancel := context.WithCancel(ctx)
				defer cancel()

				var comp *component
				if tc.setupNilNVML {
					comp = &component{
						ctx:          cctx,
						cancel:       cancel,
						nvmlInstance: nil,
					}
				} else {
					customMock := &customMockNVMLInstanceProcesses{
						devs:        map[string]device.Device{},
						nvmlExists:  tc.nvmlExists,
						productName: tc.productName,
					}
					comp = &component{
						ctx:          cctx,
						cancel:       cancel,
						nvmlInstance: customMock,
					}
				}

				result := comp.IsSupported()
				assert.Equal(t, tc.expected, result)
			})
		})
	}
}

// TestCheck_InitError_WithMockey tests Check when NVML has an initialization error
func TestCheck_InitError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML init error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &mockNVMLInstanceWithInitErrorProcesses{
			devs:      map[string]device.Device{},
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
	})
}

// TestCheck_MissingProductName_WithMockey tests Check when product name is empty
func TestCheck_MissingProductName_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with missing product name", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceProcesses{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "",
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

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "missing product name")
	})
}

// TestGetProcesses_NotSupported_ComputeRunningProcesses tests when GetComputeRunningProcesses is not supported
func TestGetProcesses_NotSupported_ComputeRunningProcesses(t *testing.T) {
	mockey.PatchConvey("GetProcesses with compute running processes not supported", t, func() {
		testUUID := "GPU-NOT-SUPPORTED"

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return nil, nvml.ERROR_NOT_SUPPORTED
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetProcesses(testUUID, dev)

		assert.NoError(t, err)
		assert.Equal(t, testUUID, result.UUID)
		assert.False(t, result.GetComputeRunningProcessesSupported)
	})
}

// TestGetProcesses_GPURequiresReset_ComputeRunningProcesses tests when GetComputeRunningProcesses returns reset required
func TestGetProcesses_GPURequiresReset_ComputeRunningProcesses(t *testing.T) {
	mockey.PatchConvey("GetProcesses with compute running processes reset required", t, func() {
		testUUID := "GPU-RESET"

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return nil, nvml.ERROR_RESET_REQUIRED
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetProcesses(testUUID, dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetProcesses_NotSupported_ProcessUtilization tests when GetProcessUtilization is not supported
func TestGetProcesses_NotSupported_ProcessUtilization(t *testing.T) {
	mockey.PatchConvey("GetProcesses with process utilization not supported", t, func() {
		testUUID := "GPU-UTIL-NOT-SUPPORTED"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return nil, nvml.ERROR_NOT_SUPPORTED
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.NoError(t, err)
		assert.Equal(t, testUUID, result.UUID)
		assert.False(t, result.GetProcessUtilizationSupported)
	})
}

// TestGetProcesses_NotFound_ProcessUtilization tests when GetProcessUtilization returns NOT_FOUND
func TestGetProcesses_NotFound_ProcessUtilization(t *testing.T) {
	mockey.PatchConvey("GetProcesses with process utilization not found", t, func() {
		testUUID := "GPU-UTIL-NOT-FOUND"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return nil, nvml.ERROR_NOT_FOUND
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		// Should skip the process and return success
		assert.NoError(t, err)
		assert.Equal(t, testUUID, result.UUID)
		assert.Empty(t, result.RunningProcesses)
	})
}

// TestGetProcesses_GPULost_ProcessUtilization tests GPU lost error in GetProcessUtilization
func TestGetProcesses_GPULost_ProcessUtilization(t *testing.T) {
	mockey.PatchConvey("GetProcesses with GPU lost in process utilization", t, func() {
		testUUID := "GPU-LOST-UTIL"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return nil, nvml.ERROR_GPU_IS_LOST
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetProcesses_GPURequiresReset_ProcessUtilization tests GPU requires reset error in GetProcessUtilization
func TestGetProcesses_GPURequiresReset_ProcessUtilization(t *testing.T) {
	mockey.PatchConvey("GetProcesses with GPU requires reset in process utilization", t, func() {
		testUUID := "GPU-RESET-UTIL"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return nil, nvml.ERROR_RESET_REQUIRED
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetProcesses_UnknownError_ProcessUtilization tests unknown error in GetProcessUtilization
func TestGetProcesses_UnknownError_ProcessUtilization(t *testing.T) {
	mockey.PatchConvey("GetProcesses with unknown error in process utilization", t, func() {
		testUUID := "GPU-UNKNOWN-UTIL"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return nil, nvml.ERROR_UNKNOWN
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get process")
		assert.Contains(t, err.Error(), "utilization")
	})
}

// TestGetProcesses_StatusError tests error in getting process status
func TestGetProcesses_StatusError(t *testing.T) {
	mockey.PatchConvey("GetProcesses with status error", t, func() {
		testUUID := "GPU-STATUS-ERROR"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
				statusErr:    errors.New("failed to get status"),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{
					{Pid: 1234, MemUtil: 50},
				}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get process")
		assert.Contains(t, err.Error(), "status")
	})
}

// TestGetProcesses_EnvironError tests error in getting process environment
func TestGetProcesses_EnvironError(t *testing.T) {
	mockey.PatchConvey("GetProcesses with environ error", t, func() {
		testUUID := "GPU-ENVIRON-ERROR"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
				status:       []string{"S"},
				environErr:   errors.New("failed to get environ"),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{
					{Pid: 1234, MemUtil: 50},
				}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get process")
		assert.Contains(t, err.Error(), "environ")
	})
}

// TestGetProcesses_ZombieProcess tests detection of zombie processes
func TestGetProcesses_ZombieProcess(t *testing.T) {
	mockey.PatchConvey("GetProcesses with zombie process", t, func() {
		testUUID := "GPU-ZOMBIE"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"zombie-cmd"},
				createTimeMS: time.Now().UnixMilli(),
				status:       []string{process.Zombie},
				environ:      []string{},
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{
					{Pid: 1234, MemUtil: 50},
				}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.NoError(t, err)
		require.Len(t, result.RunningProcesses, 1)
		assert.True(t, result.RunningProcesses[0].ZombieStatus)
	})
}

// TestGetProcesses_BadCUDAEnvVars tests detection of bad CUDA environment variables
func TestGetProcesses_BadCUDAEnvVars(t *testing.T) {
	mockey.PatchConvey("GetProcesses with bad CUDA env vars", t, func() {
		testUUID := "GPU-BAD-ENV"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
				status:       []string{"S"},
				environ: []string{
					"CUDA_AUTO_BOOST=1",
					"CUDA_PROFILE=1",
					"NORMAL_VAR=value",
				},
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{
					{Pid: 1234, MemUtil: 50},
				}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.NoError(t, err)
		require.Len(t, result.RunningProcesses, 1)
		require.NotNil(t, result.RunningProcesses[0].BadEnvVarsForCUDA)
		assert.Equal(t, "1", result.RunningProcesses[0].BadEnvVarsForCUDA["CUDA_AUTO_BOOST"])
		assert.Equal(t, "1", result.RunningProcesses[0].BadEnvVarsForCUDA["CUDA_PROFILE"])
		_, exists := result.RunningProcesses[0].BadEnvVarsForCUDA["NORMAL_VAR"]
		assert.False(t, exists)
	})
}

// TestGetProcesses_ProcessNotRunning tests when process is not running
func TestGetProcesses_ProcessNotRunning(t *testing.T) {
	mockey.PatchConvey("GetProcesses with process not running", t, func() {
		testUUID := "GPU-PROC-NOT-RUNNING"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return nil, process.ErrorProcessNotRunning
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.NoError(t, err)
		assert.Empty(t, result.RunningProcesses)
	})
}

// TestGetProcesses_CmdlineSliceError tests error in CmdlineSlice
func TestGetProcesses_CmdlineSliceError(t *testing.T) {
	mockey.PatchConvey("GetProcesses with cmdline slice error", t, func() {
		testUUID := "GPU-CMDLINE-ERROR"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineErr: errors.New("permission denied"),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get process")
		assert.Contains(t, err.Error(), "args")
	})
}

// TestGetProcesses_CreateTimeError tests error in CreateTime
func TestGetProcesses_CreateTimeError(t *testing.T) {
	mockey.PatchConvey("GetProcesses with create time error", t, func() {
		testUUID := "GPU-CREATETIME-ERROR"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice:  []string{"test-cmd"},
				createTimeErr: errors.New("permission denied"),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get process")
		assert.Contains(t, err.Error(), "create time")
	})
}

// TestGetProcesses_UnknownError_ComputeRunningProcesses tests unknown error in GetComputeRunningProcesses
func TestGetProcesses_UnknownError_ComputeRunningProcesses(t *testing.T) {
	mockey.PatchConvey("GetProcesses with unknown error in compute running processes", t, func() {
		testUUID := "GPU-UNKNOWN-COMPUTE"

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return nil, nvml.ERROR_UNKNOWN
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetProcesses(testUUID, dev)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get device compute processes")
	})
}

// TestGetProcesses_SuccessfulCollection tests successful process collection
func TestGetProcesses_SuccessfulCollection(t *testing.T) {
	mockey.PatchConvey("GetProcesses successful collection", t, func() {
		testUUID := "GPU-SUCCESS"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"python", "train.py"},
				createTimeMS: time.Now().UnixMilli(),
				status:       []string{"S", "R"},
				environ:      []string{"PATH=/usr/bin", "HOME=/home/user"},
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024 * 1024 * 100}, // 100MB
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{
					{Pid: 1234, MemUtil: 75, TimeStamp: 123456789},
				}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.NoError(t, err)
		assert.Equal(t, testUUID, result.UUID)
		assert.True(t, result.GetComputeRunningProcessesSupported)
		assert.True(t, result.GetProcessUtilizationSupported)
		require.Len(t, result.RunningProcesses, 1)

		proc := result.RunningProcesses[0]
		assert.Equal(t, uint32(1234), proc.PID)
		assert.Equal(t, []string{"python", "train.py"}, proc.CmdArgs)
		assert.Equal(t, uint32(75), proc.GPUUsedPercent)
		assert.Equal(t, uint64(1024*1024*100), proc.GPUUsedMemoryBytes)
		assert.False(t, proc.ZombieStatus)
		assert.Nil(t, proc.BadEnvVarsForCUDA)
	})
}

// TestCheck_ConcurrentAccess_WithMockey tests concurrent access to Check and LastHealthStates
func TestCheck_ConcurrentAccess_WithMockey(t *testing.T) {
	mockey.PatchConvey("Concurrent Check and LastHealthStates access", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-concurrent"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		getProcessesFunc := func(uuid string, dev device.Device) (Processes, error) {
			return Processes{
				UUID:                                uuid,
				GetComputeRunningProcessesSupported: true,
				GetProcessUtilizationSupported:      true,
			}, nil
		}

		comp := createMockProcessesComponent(ctx, getDevicesFunc, getProcessesFunc).(*component)

		// Run concurrent access
		done := make(chan bool, 10)
		for i := 0; i < 5; i++ {
			go func() {
				comp.Check()
				done <- true
			}()
			go func() {
				_ = comp.LastHealthStates()
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify final state is consistent
		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
	})
}

// TestCheckResult_Methods_WithMockey tests all checkResult methods
func TestCheckResult_Methods_WithMockey(t *testing.T) {
	t.Run("ComponentName", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
	})

	t.Run("HealthStates with suggested actions", func(t *testing.T) {
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
	})
}

// TestCheck_NilNVMLInstance_WithMockey tests Check with nil NVML instance
func TestCheck_NilNVMLInstance_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with nil NVML instance", t, func() {
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
		assert.Contains(t, cr.reason, "NVIDIA NVML instance is nil")
	})
}

// TestCheck_NVMLNotExists_WithMockey tests Check when NVML library is not loaded
func TestCheck_NVMLNotExists_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML not exists", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceProcesses{
			devs:        map[string]device.Device{},
			nvmlExists:  false,
			productName: "NVIDIA H100",
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

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "NVIDIA NVML library is not loaded")
	})
}

// TestCheckResult_String_WithMockey tests the String method of checkResult
func TestCheckResult_String_WithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.String())
	})

	t.Run("empty processes", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "no data", cr.String())
	})

	t.Run("with processes", func(t *testing.T) {
		cr := &checkResult{
			Processes: []Processes{
				{
					UUID: "gpu-1",
					RunningProcesses: []Process{
						{PID: 1234},
						{PID: 5678},
					},
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-1")
		assert.Contains(t, result, "2") // 2 processes
	})
}

// TestCheckResult_Summary_WithMockey tests the Summary method
func TestCheckResult_Summary_WithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.Summary())
	})

	t.Run("with reason", func(t *testing.T) {
		cr := &checkResult{reason: "test reason"}
		assert.Equal(t, "test reason", cr.Summary())
	})
}

// TestCheckResult_HealthStateType_WithMockey tests the HealthStateType method
func TestCheckResult_HealthStateType_WithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	})

	t.Run("healthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeHealthy}
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})
}

// TestCheckResult_HealthStates_NilResult_WithMockey tests HealthStates with nil checkResult
func TestCheckResult_HealthStates_NilResult_WithMockey(t *testing.T) {
	var cr *checkResult
	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

// TestCheckResult_HealthStates_WithExtraInfo_WithMockey tests HealthStates with Processes
func TestCheckResult_HealthStates_WithExtraInfo_WithMockey(t *testing.T) {
	cr := &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
		Processes: []Processes{
			{
				UUID: "gpu-1",
				RunningProcesses: []Process{
					{PID: 1234},
				},
			},
		},
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.NotEmpty(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo["data"], "gpu-1")
}

// TestCheck_MultipleGPUs_WithMockey tests Check with multiple GPUs
func TestCheck_MultipleGPUs_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with multiple GPUs", t, func() {
		ctx := context.Background()

		uuid1 := "gpu-uuid-1"
		uuid2 := "gpu-uuid-2"

		mockDeviceObj1 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid1, nvml.SUCCESS
			},
		}
		mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "test-arch", "test-brand", "test-cuda", "test-pci-1")

		mockDeviceObj2 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid2, nvml.SUCCESS
			},
		}
		mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "test-arch", "test-brand", "test-cuda", "test-pci-2")

		devs := map[string]device.Device{
			uuid1: mockDev1,
			uuid2: mockDev2,
		}

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		getProcessesFunc := func(uuid string, dev device.Device) (Processes, error) {
			return Processes{
				UUID: uuid,
				RunningProcesses: []Process{
					{PID: 1234},
				},
				GetComputeRunningProcessesSupported: true,
				GetProcessUtilizationSupported:      true,
			}, nil
		}

		comp := createMockProcessesComponent(ctx, getDevicesFunc, getProcessesFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "2 GPU(s) were checked")
		assert.Len(t, cr.Processes, 2)
	})
}

// TestLastHealthStates_SuggestedActionsPropagate_WithMockey tests suggested actions propagation
func TestLastHealthStates_SuggestedActionsPropagate_WithMockey(t *testing.T) {
	mockey.PatchConvey("Suggested actions propagate to health states", t, func() {
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

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		// Simulate GPU requires reset error
		getProcessesFunc := func(uuid string, dev device.Device) (Processes, error) {
			return Processes{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockProcessesComponent(ctx, getDevicesFunc, getProcessesFunc).(*component)
		comp.Check()

		states := comp.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestGetProcesses_MultipleUtilizationSamples tests sorting of utilization samples
func TestGetProcesses_MultipleUtilizationSamples(t *testing.T) {
	mockey.PatchConvey("GetProcesses with multiple utilization samples", t, func() {
		testUUID := "GPU-MULTI-SAMPLES"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
				status:       []string{"S"},
				environ:      []string{},
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				// Return multiple samples with different timestamps
				// The one with highest timestamp should be used
				return []nvml.ProcessUtilizationSample{
					{Pid: 1234, MemUtil: 30, TimeStamp: 100}, // Older
					{Pid: 1234, MemUtil: 50, TimeStamp: 300}, // Newest - should be used
					{Pid: 1234, MemUtil: 40, TimeStamp: 200}, // Middle
				}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.NoError(t, err)
		require.Len(t, result.RunningProcesses, 1)
		// Should use the sample with highest timestamp (300), which has MemUtil 50
		assert.Equal(t, uint32(50), result.RunningProcesses[0].GPUUsedPercent)
	})
}

// TestGetProcesses_EmptyUtilizationSamples tests when no utilization samples are returned
func TestGetProcesses_EmptyUtilizationSamples(t *testing.T) {
	mockey.PatchConvey("GetProcesses with empty utilization samples", t, func() {
		testUUID := "GPU-EMPTY-SAMPLES"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
				status:       []string{"S"},
				environ:      []string{},
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		assert.NoError(t, err)
		require.Len(t, result.RunningProcesses, 1)
		// Should have 0 utilization when no samples
		assert.Equal(t, uint32(0), result.RunningProcesses[0].GPUUsedPercent)
	})
}

// TestGetProcesses_StatusNoSuchFileOrDirectory tests Status with no such file error (should skip)
func TestGetProcesses_StatusNoSuchFileOrDirectory(t *testing.T) {
	mockey.PatchConvey("GetProcesses with status no such file error", t, func() {
		testUUID := "GPU-STATUS-NOSUCH"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
				statusErr:    errors.New("no such file or directory"),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{
					{Pid: 1234, MemUtil: 50},
				}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		// Should skip the process
		assert.NoError(t, err)
		assert.Empty(t, result.RunningProcesses)
	})
}

// TestGetProcesses_EnvironNoSuchFileOrDirectory tests Environ with no such file error (should skip)
func TestGetProcesses_EnvironNoSuchFileOrDirectory(t *testing.T) {
	mockey.PatchConvey("GetProcesses with environ no such file error", t, func() {
		testUUID := "GPU-ENVIRON-NOSUCH"

		newProcessFunc := func(pid int32) (processInspector, error) {
			return &mockProcessInspector{
				cmdlineSlice: []string{"test-cmd"},
				createTimeMS: time.Now().UnixMilli(),
				status:       []string{"S"},
				environErr:   errors.New("no such file or directory"),
			}, nil
		}

		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{
					{Pid: 1234, MemUtil: 50},
				}, nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := getProcesses(testUUID, dev, newProcessFunc)

		// Should skip the process
		assert.NoError(t, err)
		assert.Empty(t, result.RunningProcesses)
	})
}

// TestGetProcesses_NewProcessError_WithMockey tests error when process.NewProcess fails
func TestGetProcesses_NewProcessError_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetProcesses with NewProcess error", t, func() {
		mockDevice := &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{{Pid: 1234, UsedGpuMemory: 1024}}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{{Pid: 1234, MemUtil: 10, TimeStamp: 1}}, nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		mockey.Mock(process.NewProcess).To(func(pid int32) (*process.Process, error) {
			return nil, errors.New("new process failed")
		}).Build()

		_, err := GetProcesses("uuid", dev)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get process")
	})
}
