package nvml

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	assert.True(t, errors.Is(err, ErrGPULost), "Expected GPU lost error")
}

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
