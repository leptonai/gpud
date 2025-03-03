package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetMemory(t *testing.T) {
	testCases := []struct {
		name           string
		memoryV2       nvml.Memory_v2
		memoryV2Ret    nvml.Return
		memory         nvml.Memory
		memoryRet      nvml.Return
		expectedMemory Memory
		expectError    bool
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
				TotalBytes:        4096 * 1024 * 1024,
				TotalHumanized:    "4.3 GB",
				FreeBytes:         2048 * 1024 * 1024,
				FreeHumanized:     "2.1 GB",
				UsedBytes:         1024 * 1024 * 1024,
				UsedHumanized:     "1.1 GB",
				ReservedBytes:     1024 * 1024 * 1024,
				ReservedHumanized: "1.1 GB",
				UsedPercent:       "25.00",
				Supported:         true,
			},
			expectError: false,
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
				TotalBytes:        4096 * 1024 * 1024,
				TotalHumanized:    "4.3 GB",
				FreeBytes:         2048 * 1024 * 1024,
				FreeHumanized:     "2.1 GB",
				UsedBytes:         2048 * 1024 * 1024,
				UsedHumanized:     "2.1 GB",
				ReservedBytes:     0,
				ReservedHumanized: "0 B",
				UsedPercent:       "50.00",
				Supported:         true,
			},
			expectError: false,
		},
		{
			name:        "not supported",
			memoryV2:    nvml.Memory_v2{},
			memoryV2Ret: nvml.ERROR_NOT_SUPPORTED,
			memory:      nvml.Memory{},
			memoryRet:   nvml.ERROR_NOT_SUPPORTED,
			expectedMemory: Memory{
				UUID:              "test-uuid",
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
			expectError: false,
		},
		{
			name:        "both APIs fail",
			memoryV2:    nvml.Memory_v2{},
			memoryV2Ret: nvml.ERROR_UNKNOWN,
			memory:      nvml.Memory{},
			memoryRet:   nvml.ERROR_UNKNOWN,
			expectError: true,
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
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedMemory, mem)
			}
		})
	}
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
