package nvml

import (
	"encoding/json"
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
