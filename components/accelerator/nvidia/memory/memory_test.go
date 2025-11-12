package memory

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetMemory(t *testing.T) {
	testCases := []struct {
		name            string
		memoryV2        nvml.Memory_v2
		memoryV2Ret     nvml.Return
		memory          nvml.Memory
		memoryRet       nvml.Return
		expectedMemory  Memory
		expectError     bool
		expectedErrType error
	}{
		{
			name: "successful v2 API",
			memoryV2: nvml.Memory_v2{
				Total:    4096 * 1024 * 1024,
				Free:     2048 * 1024 * 1024,
				Used:     1024 * 1024 * 1024,
				Reserved: 1024 * 1024 * 1024,
			},
			memoryV2Ret: nvml.SUCCESS,
			memory:      nvml.Memory{},
			memoryRet:   nvml.SUCCESS,
			expectedMemory: Memory{
				UUID:              "test-uuid",
				BusID:             "test-pci",
				TotalBytes:        4096 * 1024 * 1024,
				TotalHumanized:    "4.0 GiB",
				FreeBytes:         2048 * 1024 * 1024,
				FreeHumanized:     "2.0 GiB",
				UsedBytes:         1024 * 1024 * 1024,
				UsedHumanized:     "1.0 GiB",
				ReservedBytes:     1024 * 1024 * 1024,
				ReservedHumanized: "1.0 GiB",
				UsedPercent:       "25.00",
				Supported:         true,
			},
			expectError:     false,
			expectedErrType: nil,
		},
		{
			name:        "fallback to v1 API",
			memoryV2:    nvml.Memory_v2{},
			memoryV2Ret: nvml.ERROR_NOT_SUPPORTED,
			memory: nvml.Memory{
				Total: 4096 * 1024 * 1024,
				Free:  2048 * 1024 * 1024,
				Used:  2048 * 1024 * 1024,
			},
			memoryRet: nvml.SUCCESS,
			expectedMemory: Memory{
				UUID:              "test-uuid",
				BusID:             "test-pci",
				TotalBytes:        4096 * 1024 * 1024,
				TotalHumanized:    "4.0 GiB",
				FreeBytes:         2048 * 1024 * 1024,
				FreeHumanized:     "2.0 GiB",
				UsedBytes:         2048 * 1024 * 1024,
				UsedHumanized:     "2.0 GiB",
				ReservedBytes:     0,
				ReservedHumanized: "0 B",
				UsedPercent:       "50.00",
				Supported:         true,
			},
			expectError:     false,
			expectedErrType: nil,
		},
		{
			name:        "not supported",
			memoryV2:    nvml.Memory_v2{},
			memoryV2Ret: nvml.ERROR_NOT_SUPPORTED,
			memory:      nvml.Memory{},
			memoryRet:   nvml.ERROR_NOT_SUPPORTED,
			expectedMemory: Memory{
				UUID:              "test-uuid",
				BusID:             "test-pci",
				TotalBytes:        0,
				TotalHumanized:    "",
				FreeBytes:         0,
				FreeHumanized:     "",
				UsedBytes:         0,
				UsedHumanized:     "",
				ReservedBytes:     0,
				ReservedHumanized: "",
				UsedPercent:       "",
				Supported:         false,
			},
			expectError:     false,
			expectedErrType: nil,
		},
		{
			name:            "both APIs fail",
			memoryV2:        nvml.Memory_v2{},
			memoryV2Ret:     nvml.ERROR_UNKNOWN,
			memory:          nvml.Memory{},
			memoryRet:       nvml.ERROR_UNKNOWN,
			expectError:     true,
			expectedErrType: nil,
		},
		{
			name:            "GPU lost error in v1 API",
			memoryV2:        nvml.Memory_v2{},
			memoryV2Ret:     nvml.ERROR_NOT_SUPPORTED, // v2 not supported, fall back to v1
			memory:          nvml.Memory{},
			memoryRet:       nvml.ERROR_GPU_IS_LOST, // v1 reports GPU is lost
			expectError:     true,
			expectedErrType: nvmlerrors.ErrGPULost,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := testutil.CreateMemoryDevice(
				"test-uuid",
				tc.memoryV2, tc.memoryV2Ret,
				tc.memory, tc.memoryRet,
			)

			mem, err := GetMemory("test-uuid", mockDevice)

			if tc.expectError {
				assert.Error(t, err)
				if tc.expectedErrType != nil {
					assert.True(t, errors.Is(err, tc.expectedErrType), "Expected error type %v but got %v", tc.expectedErrType, err)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedMemory, mem)
			}
		})
	}
}

// TestGetMemoryWithDirectGPULostError tests the direct handling of GPU lost error
func TestGetMemoryWithDirectGPULostError(t *testing.T) {
	// Create a mock device that simulates a GPU lost error
	mockDevice := &mock.Device{
		GetMemoryInfoFunc: func() (nvml.Memory, nvml.Return) {
			return nvml.Memory{}, nvml.ERROR_GPU_IS_LOST
		},
		GetMemoryInfo_v2Func: func() (nvml.Memory_v2, nvml.Return) {
			return nvml.Memory_v2{}, nvml.ERROR_NOT_SUPPORTED
		},
	}

	// Wrap with testutil.MockDevice
	dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

	// Call the function
	_, err := GetMemory("GPU-LOST", dev)

	// Check that we get a GPU lost error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
}

func TestMemoryGetUsedPercent(t *testing.T) {
	testCases := []struct {
		name          string
		memory        Memory
		expectedValue float64
		expectedError bool
	}{
		{
			name: "valid percent",
			memory: Memory{
				UsedPercent: "25.50",
			},
			expectedValue: 25.50,
			expectedError: false,
		},
		{
			name: "invalid percent",
			memory: Memory{
				UsedPercent: "invalid",
			},
			expectedError: true,
		},
		{
			name: "empty percent",
			memory: Memory{
				UsedPercent: "",
			},
			expectedValue: 0.0,
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			value, err := tc.memory.GetUsedPercent()

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedValue, value)
			}
		})
	}
}
