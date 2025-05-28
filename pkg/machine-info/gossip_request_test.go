package machineinfo

import (
	"errors"
	"runtime"
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

// mockNVMLInstance implements the nvidianvml.Instance interface for testing
type mockNVMLInstance struct{}

func (m *mockNVMLInstance) NVMLExists() bool                  { return false }
func (m *mockNVMLInstance) Library() nvmllib.Library          { return nil }
func (m *mockNVMLInstance) Devices() map[string]device.Device { return nil }
func (m *mockNVMLInstance) ProductName() string               { return "Test GPU" }
func (m *mockNVMLInstance) Architecture() string              { return "test-arch" }
func (m *mockNVMLInstance) Brand() string                     { return "Test Brand" }
func (m *mockNVMLInstance) DriverVersion() string             { return "123.45" }
func (m *mockNVMLInstance) DriverMajor() int                  { return 123 }
func (m *mockNVMLInstance) CUDAVersion() string               { return "11.7" }
func (m *mockNVMLInstance) FabricManagerSupported() bool      { return false }
func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}
func (m *mockNVMLInstance) Shutdown() error { return nil }

// TestCreateGossipRequest tests the gossip request creation
func TestCreateGossipRequest(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Test only runs on Linux or macOS")
	}

	// Skip if NVML is not available
	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		t.Skip("NVML not available, skipping test")
	}
	defer func() {
		if err := nvmlInstance.Shutdown(); err != nil {
			log.Logger.Warnw("failed to shutdown nvml instance", "error", err)
		}
	}()

	// Test with valid parameters
	machineID := "test-machine-id"
	req, err := CreateGossipRequest(machineID, nvmlInstance)
	if err != nil {
		t.Skipf("Could not create gossip request: %v", err)
	}

	// Validate request fields
	assert.Equal(t, machineID, req.MachineID)
	assert.NotNil(t, req.MachineInfo)
	assert.NotEmpty(t, req.MachineInfo.Hostname)
	assert.NotNil(t, req.MachineInfo.CPUInfo)
}

// TestCreateGossipRequestMocked tests the createGossipRequest function with mocked dependencies
func TestCreateGossipRequestMocked(t *testing.T) {
	// Setup
	machineID := "test-machine-id"
	nvmlInstance := &mockNVMLInstance{}

	// Test cases for the private function
	tests := []struct {
		name               string
		getMachineInfoFunc func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error)
		wantError          bool
		expectedErrorMsg   string
	}{
		{
			name: "successful request creation",
			getMachineInfoFunc: func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return &apiv1.MachineInfo{
					Hostname: "test-host",
					CPUInfo: &apiv1.MachineCPUInfo{
						Type: "test-cpu",
					},
				}, nil
			},
			wantError: false,
		},
		{
			name: "getMachineInfo returns error",
			getMachineInfoFunc: func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				return nil, errors.New("machine info error")
			},
			wantError:        true,
			expectedErrorMsg: "failed to get machine info: machine info error",
		},
	}

	// Run all test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := createGossipRequest(machineID, nvmlInstance, tc.getMachineInfoFunc)

			if tc.wantError {
				assert.Error(t, err)
				assert.Nil(t, req)
				assert.Contains(t, err.Error(), tc.expectedErrorMsg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, req)
				assert.Equal(t, machineID, req.MachineID)
				assert.NotNil(t, req.MachineInfo)
				assert.Equal(t, "test-host", req.MachineInfo.Hostname)
				assert.Equal(t, "test-cpu", req.MachineInfo.CPUInfo.Type)
			}
		})
	}
}
