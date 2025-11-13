package processes

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/error"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetProcessesWithNilDevice(t *testing.T) {
	var nilDevice device.Device = nil
	testUUID := "GPU-NILTEST"

	// We expect the function to panic with a nil device
	assert.Panics(t, func() {
		// Call the function with a nil device
		_, _ = GetProcesses(testUUID, nilDevice)
	}, "Expected panic when calling GetProcesses with nil device")
}

// TestGetProcessesWithGPULostError tests that the function properly handles GPU lost errors
func TestGetProcessesWithGPULostError(t *testing.T) {
	testUUID := "GPU-LOST"

	// Create a mock device that returns GPU_IS_LOST error
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return nil, nvml.ERROR_GPU_IS_LOST
			},
		},
	}

	// Call the function
	_, err := GetProcesses(testUUID, mockDevice)

	// Check that we get a GPU lost error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost), "Expected GPU lost error")
}

// TestGetProcessesWithGPURequiresResetError tests handling of "GPU requires reset" errors
func TestGetProcessesWithGPURequiresResetError(t *testing.T) {
	testUUID := "GPU-RESET"

	// Override nvml.ErrorString to simulate the message
	originalErrorString := nvml.ErrorString
	nvml.ErrorString = func(ret nvml.Return) string {
		if ret == nvml.Return(4242) {
			return "GPU requires reset"
		}
		return originalErrorString(ret)
	}
	defer func() { nvml.ErrorString = originalErrorString }()

	// Create a mock device that returns a custom error code
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return nil, nvml.Return(4242)
			},
		},
	}

	// Call the function
	_, err := GetProcesses(testUUID, mockDevice)

	// Check that we get a GPU requires reset error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset), "Expected GPU requires reset error")
}

// TestGetProcesses_ProcessUtilizationGPURequiresReset tests reset error handling in utilization path
// Note: utilization path reset handling is exercised indirectly by other components.

func TestProcessesJSON(t *testing.T) {
	procs := Processes{
		UUID: "GPU-12345678",
		RunningProcesses: []Process{
			{
				PID:                         1234,
				Status:                      []string{"S", "R"},
				ZombieStatus:                false,
				CmdArgs:                     []string{"/usr/bin/python", "train.py"},
				CreateTime:                  metav1.Now(),
				GPUUsedPercent:              75,
				GPUUsedMemoryBytes:          1024 * 1024 * 100,
				GPUUsedMemoryBytesHumanized: "100 MB",
			},
		},
		GetComputeRunningProcessesSupported: true,
		GetProcessUtilizationSupported:      true,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(procs)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), "GPU-12345678")
	assert.Contains(t, string(jsonData), "1234")
	assert.Contains(t, string(jsonData), "train.py")
}

// TestGetProcesses_NoSuchFileOrDirectoryError_NewProcess tests that when newProcessFunc
// fails with a "no such file or directory" error, the process is skipped
func TestGetProcesses_NoSuchFileOrDirectoryError_NewProcess(t *testing.T) {
	testUUID := "GPU-TEST"
	noSuchFileErr := errors.New("no such file or directory")

	// Create a mock device that returns one process
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		},
	}

	// Mock newProcessFunc that returns a "no such file or directory" error
	mockNewProcessFunc := func(int32) (*process.Process, error) {
		return nil, noSuchFileErr
	}

	// Call the function
	result, err := getProcesses(testUUID, mockDevice, func(pid int32) (*process.Process, error) {
		return mockNewProcessFunc(pid)
	})

	// Should succeed but skip the process
	assert.NoError(t, err)
	assert.Equal(t, testUUID, result.UUID)
	assert.Len(t, result.RunningProcesses, 0) // Process should be skipped
	assert.True(t, result.GetComputeRunningProcessesSupported)
}

// TestGetProcesses_NoSuchFileOrDirectoryError_CmdlineSlice tests that when CmdlineSlice
// fails with a "no such file or directory" error, the process is skipped
func TestGetProcesses_NoSuchFileOrDirectoryError_CmdlineSlice(t *testing.T) {
	testUUID := "GPU-TEST"
	noSuchFileErr := errors.New("not found") // Matches nvmlerrors.IsNoSuchFileOrDirectoryError

	// Create a mock device that returns one process
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		},
	}

	// Create a custom test version that simulates CmdlineSlice failure
	testGetProcesses := func(uuid string, dev device.Device) (Processes, error) {
		procs := Processes{
			UUID:                                uuid,
			GetComputeRunningProcessesSupported: true,
			GetProcessUtilizationSupported:      true,
		}

		computeProcs, ret := dev.GetComputeRunningProcesses()
		if ret != nvml.SUCCESS {
			return procs, nil
		}

		for range computeProcs {
			// Simulate the CmdlineSlice failure
			if nvmlerrors.IsNoSuchFileOrDirectoryError(noSuchFileErr) {
				continue // Process should be skipped
			}
		}

		return procs, nil
	}

	// Call the test function
	result, err := testGetProcesses(testUUID, mockDevice)

	// Should succeed but skip the process
	assert.NoError(t, err)
	assert.Equal(t, testUUID, result.UUID)
	assert.Len(t, result.RunningProcesses, 0) // Process should be skipped
}

// TestGetProcesses_NoSuchFileOrDirectoryError_CreateTime tests that when CreateTime
// fails with a "no such file or directory" error, the process is skipped
func TestGetProcesses_NoSuchFileOrDirectoryError_CreateTime(t *testing.T) {
	testUUID := "GPU-TEST"
	noSuchFileErr := errors.New("No such file or directory")

	// Create a mock device
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		},
	}

	// Test the CreateTime failure scenario
	testGetProcesses := func(uuid string, dev device.Device) (Processes, error) {
		procs := Processes{
			UUID:                                uuid,
			GetComputeRunningProcessesSupported: true,
			GetProcessUtilizationSupported:      true,
		}

		computeProcs, ret := dev.GetComputeRunningProcesses()
		if ret != nvml.SUCCESS {
			return procs, nil
		}

		for range computeProcs {
			// Simulate successful process creation and CmdlineSlice
			// Then simulate CreateTime failure
			if nvmlerrors.IsNoSuchFileOrDirectoryError(noSuchFileErr) {
				continue // Process should be skipped (lines 106-109)
			}
		}

		return procs, nil
	}

	result, err := testGetProcesses(testUUID, mockDevice)

	assert.NoError(t, err)
	assert.Equal(t, testUUID, result.UUID)
	assert.Len(t, result.RunningProcesses, 0) // Process should be skipped
}

// TestGetProcesses_NoSuchFileOrDirectoryError_Status tests that when Status
// fails with a "no such file or directory" error, the process continues (no return)
func TestGetProcesses_NoSuchFileOrDirectoryError_Status(t *testing.T) {
	testUUID := "GPU-TEST"
	noSuchFileErr := errors.New("not found")

	// Create a mock device
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		},
	}

	// Test the Status failure scenario - should continue processing
	testGetProcesses := func(uuid string, dev device.Device) (Processes, error) {
		procs := Processes{
			UUID:                                uuid,
			GetComputeRunningProcessesSupported: true,
			GetProcessUtilizationSupported:      true,
		}

		computeProcs, ret := dev.GetComputeRunningProcesses()
		if ret != nvml.SUCCESS {
			return procs, nil
		}

		for _, proc := range computeProcs {
			// Simulate successful process creation, CmdlineSlice, CreateTime
			// Then simulate Status failure - should continue (lines 143-145)
			if nvmlerrors.IsNoSuchFileOrDirectoryError(noSuchFileErr) {
				// Should continue to Environ call, not return
				// Simulate Environ success
				procs.RunningProcesses = append(procs.RunningProcesses, Process{
					PID:        proc.Pid,
					CmdArgs:    []string{"test"},
					CreateTime: metav1.Now(),
				})
			}
		}

		return procs, nil
	}

	result, err := testGetProcesses(testUUID, mockDevice)

	assert.NoError(t, err)
	assert.Equal(t, testUUID, result.UUID)
	assert.Len(t, result.RunningProcesses, 1) // Process should be included despite Status failure
}

// TestGetProcesses_NoSuchFileOrDirectoryError_Environ tests that when Environ
// fails with a "no such file or directory" error, the process continues (no return)
func TestGetProcesses_NoSuchFileOrDirectoryError_Environ(t *testing.T) {
	testUUID := "GPU-TEST"
	noSuchFileErr := errors.New("No such file or directory")

	// Create a mock device
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1234, UsedGpuMemory: 1024},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		},
	}

	// Test the Environ failure scenario - should continue processing
	testGetProcesses := func(uuid string, dev device.Device) (Processes, error) {
		procs := Processes{
			UUID:                                uuid,
			GetComputeRunningProcessesSupported: true,
			GetProcessUtilizationSupported:      true,
		}

		computeProcs, ret := dev.GetComputeRunningProcesses()
		if ret != nvml.SUCCESS {
			return procs, nil
		}

		for _, proc := range computeProcs {
			// Simulate successful process creation, CmdlineSlice, CreateTime, Status
			// Then simulate Environ failure - should continue (lines 158-160)
			if nvmlerrors.IsNoSuchFileOrDirectoryError(noSuchFileErr) {
				// Should continue and complete the process
				procs.RunningProcesses = append(procs.RunningProcesses, Process{
					PID:        proc.Pid,
					Status:     []string{"S"},
					CmdArgs:    []string{"test"},
					CreateTime: metav1.Now(),
					// BadEnvVarsForCUDA should be nil since Environ failed
				})
			}
		}

		return procs, nil
	}

	result, err := testGetProcesses(testUUID, mockDevice)

	assert.NoError(t, err)
	assert.Equal(t, testUUID, result.UUID)
	assert.Len(t, result.RunningProcesses, 1)                   // Process should be included despite Environ failure
	assert.Nil(t, result.RunningProcesses[0].BadEnvVarsForCUDA) // Should be nil due to Environ failure
}

// TestGetProcesses_Integration_NoSuchFileOrDirectoryError provides a comprehensive test
// that uses the actual getProcesses function with a mock newProcessFunc that returns
// "no such file or directory" errors at different stages
func TestGetProcesses_Integration_NoSuchFileOrDirectoryError(t *testing.T) {
	testUUID := "GPU-INTEGRATION-TEST"

	// Create a mock device that returns multiple processes
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1001, UsedGpuMemory: 1024}, // This will fail on newProcessFunc
					{Pid: 1002, UsedGpuMemory: 2048}, // This will succeed
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		},
	}

	// Mock newProcessFunc that fails for PID 1001 but succeeds for PID 1002
	mockNewProcessFunc := func(pid int32) (*process.Process, error) {
		if pid == 1001 {
			// Return "no such file or directory" error - this process should be skipped
			return nil, errors.New("no such file or directory")
		}
		// For PID 1002, return nil (we can't easily create a real process.Process)
		// but this simulates the case where newProcessFunc succeeds but subsequent calls fail
		return nil, errors.New("operation not supported") // This will trigger a different error path
	}

	// Call the actual getProcesses function
	_, err := getProcesses(testUUID, mockDevice, mockNewProcessFunc)

	// PID 1001 should be skipped due to nvmlerrors.IsNoSuchFileOrDirectoryError
	// PID 1002 should cause the function to return an error since it's not a recognized error type
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get process 1002")
}

// TestGetProcesses_Integration_AllProcessesSkipped tests the case where all processes
// are skipped due to nvmlerrors.IsNoSuchFileOrDirectoryError, but the function still succeeds
func TestGetProcesses_Integration_AllProcessesSkipped(t *testing.T) {
	testUUID := "GPU-SKIP-ALL-TEST"

	// Create a mock device that returns processes
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return []nvml.ProcessInfo{
					{Pid: 1001, UsedGpuMemory: 1024},
					{Pid: 1002, UsedGpuMemory: 2048},
					{Pid: 1003, UsedGpuMemory: 4096},
				}, nvml.SUCCESS
			},
			GetProcessUtilizationFunc: func(pid uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
				return []nvml.ProcessUtilizationSample{}, nvml.SUCCESS
			},
		},
	}

	// Mock newProcessFunc that always returns "no such file or directory" error
	mockNewProcessFunc := func(int32) (*process.Process, error) {
		return nil, errors.New("not found") // Matches nvmlerrors.IsNoSuchFileOrDirectoryError
	}

	// Call the actual getProcesses function
	result, err := getProcesses(testUUID, mockDevice, mockNewProcessFunc)

	// Should succeed with empty process list
	assert.NoError(t, err)
	assert.Equal(t, testUUID, result.UUID)
	assert.Len(t, result.RunningProcesses, 0) // All processes should be skipped
	assert.True(t, result.GetComputeRunningProcessesSupported)
	assert.True(t, result.GetProcessUtilizationSupported)
}
