package nvml

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// createECCErrorsDevice creates a mock device for ECC errors testing
func createECCErrorsDevice(
	uuid string,
	totalECCCorrected uint64,
	totalECCUncorrected uint64,
	totalECCRet nvml.Return,
	memoryErrorRet nvml.Return,
) device.Device {
	mockDevice := &mock.Device{
		GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
			if totalECCRet != nvml.SUCCESS {
				return 0, totalECCRet
			}
			if errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
				return totalECCCorrected, nvml.SUCCESS
			}
			return totalECCUncorrected, nvml.SUCCESS
		},
		GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
			if memoryErrorRet != nvml.SUCCESS {
				return 0, memoryErrorRet
			}
			if errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
				return 1, nvml.SUCCESS
			}
			return 2, nvml.SUCCESS
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}

	return testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")
}

func TestGetECCErrors(t *testing.T) {
	tests := []struct {
		name                  string
		uuid                  string
		eccModeEnabled        bool
		totalECCCorrected     uint64
		totalECCUncorrected   uint64
		totalECCRet           nvml.Return
		memoryErrorRet        nvml.Return
		expectedECCErrors     ECCErrors
		expectError           bool
		expectedErrorContains string
	}{
		{
			name:                "successful case with ECC mode enabled",
			uuid:                "test-uuid",
			eccModeEnabled:      true,
			totalECCCorrected:   5,
			totalECCUncorrected: 2,
			totalECCRet:         nvml.SUCCESS,
			memoryErrorRet:      nvml.SUCCESS,
			expectedECCErrors: ECCErrors{
				UUID: "test-uuid",
				Aggregate: AllECCErrorCounts{
					Total: ECCErrorCounts{
						Corrected:   5,
						Uncorrected: 2,
					},
					L1Cache: ECCErrorCounts{
						Corrected:   1,
						Uncorrected: 2,
					},
					// ... other memory locations will have similar values due to mock
				},
				Volatile: AllECCErrorCounts{
					Total: ECCErrorCounts{
						Corrected:   5,
						Uncorrected: 2,
					},
					L1Cache: ECCErrorCounts{
						Corrected:   1,
						Uncorrected: 2,
					},
					// ... other memory locations will have similar values due to mock
				},
				Supported: true,
			},
			expectError: false,
		},
		{
			name:                "ECC mode disabled",
			uuid:                "test-uuid",
			eccModeEnabled:      false,
			totalECCCorrected:   5,
			totalECCUncorrected: 2,
			totalECCRet:         nvml.SUCCESS,
			memoryErrorRet:      nvml.SUCCESS,
			expectedECCErrors: ECCErrors{
				UUID: "test-uuid",
				Aggregate: AllECCErrorCounts{
					Total: ECCErrorCounts{
						Corrected:   5,
						Uncorrected: 2,
					},
				},
				Volatile: AllECCErrorCounts{
					Total: ECCErrorCounts{
						Corrected:   5,
						Uncorrected: 2,
					},
				},
				Supported: true,
			},
			expectError: false,
		},
		{
			name:           "not supported error",
			uuid:           "test-uuid",
			eccModeEnabled: true,
			totalECCRet:    nvml.ERROR_NOT_SUPPORTED,
			expectedECCErrors: ECCErrors{
				UUID:      "test-uuid",
				Supported: false,
			},
			expectError: false,
		},
		{
			name:                  "total ECC error",
			uuid:                  "test-uuid",
			eccModeEnabled:        true,
			totalECCRet:           nvml.ERROR_UNKNOWN,
			expectError:           true,
			expectedErrorContains: "failed to get total ecc errors",
		},
		{
			name:                  "memory error counter error",
			uuid:                  "test-uuid",
			eccModeEnabled:        true,
			totalECCRet:           nvml.SUCCESS,
			memoryErrorRet:        nvml.ERROR_UNKNOWN,
			expectError:           true,
			expectedErrorContains: "failed to get",
		},
		{
			name:                  "GPU lost error on total ECC",
			uuid:                  "test-uuid",
			eccModeEnabled:        true,
			totalECCRet:           nvml.ERROR_GPU_IS_LOST,
			expectError:           true,
			expectedErrorContains: "gpu lost",
		},
		{
			name:                  "GPU lost error on memory error counter",
			uuid:                  "test-uuid",
			eccModeEnabled:        true,
			totalECCRet:           nvml.SUCCESS,
			memoryErrorRet:        nvml.ERROR_GPU_IS_LOST,
			expectError:           true,
			expectedErrorContains: "gpu lost",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := createECCErrorsDevice(
				tc.uuid,
				tc.totalECCCorrected,
				tc.totalECCUncorrected,
				tc.totalECCRet,
				tc.memoryErrorRet,
			)

			result, err := GetECCErrors(tc.uuid, mockDevice, tc.eccModeEnabled)

			if tc.expectError {
				assert.Error(t, err)
				if tc.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorContains)
				}
				if tc.totalECCRet == nvml.ERROR_GPU_IS_LOST || tc.memoryErrorRet == nvml.ERROR_GPU_IS_LOST {
					assert.True(t, errors.Is(err, ErrGPULost), "Expected GPU lost error")
				}
			} else {
				assert.NoError(t, err)
				if tc.totalECCRet == nvml.SUCCESS {
					// Check total counts
					assert.Equal(t, tc.expectedECCErrors.Aggregate.Total, result.Aggregate.Total)
					assert.Equal(t, tc.expectedECCErrors.Volatile.Total, result.Volatile.Total)

					if tc.eccModeEnabled && tc.memoryErrorRet == nvml.SUCCESS {
						// Check L1 Cache as an example of memory location counts
						assert.Equal(t, tc.expectedECCErrors.Aggregate.L1Cache, result.Aggregate.L1Cache)
						assert.Equal(t, tc.expectedECCErrors.Volatile.L1Cache, result.Volatile.L1Cache)
					}
				}
				assert.Equal(t, tc.expectedECCErrors.Supported, result.Supported)
				assert.Equal(t, tc.expectedECCErrors.UUID, result.UUID)
			}
		})
	}
}

func TestECCErrors_JSON(t *testing.T) {
	eccErrors := ECCErrors{
		UUID: "test-uuid",
		Aggregate: AllECCErrorCounts{
			Total: ECCErrorCounts{
				Corrected:   5,
				Uncorrected: 2,
			},
		},
		Volatile: AllECCErrorCounts{
			Total: ECCErrorCounts{
				Corrected:   3,
				Uncorrected: 1,
			},
		},
		Supported: true,
	}

	jsonData, err := json.Marshal(eccErrors)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), `"uuid":"test-uuid"`)
	assert.Contains(t, string(jsonData), `"supported":true`)
	assert.Contains(t, string(jsonData), `"corrected":5`)
	assert.Contains(t, string(jsonData), `"uncorrected":2`)
}

func TestAllECCErrorCounts_FindUncorrectedErrs(t *testing.T) {
	tests := []struct {
		name     string
		counts   AllECCErrorCounts
		expected []string
	}{
		{
			name:     "No uncorrected errors",
			counts:   AllECCErrorCounts{},
			expected: nil,
		},
		{
			name: "Only total uncorrected errors",
			counts: AllECCErrorCounts{
				Total: ECCErrorCounts{Uncorrected: 5},
			},
			expected: []string{"total uncorrected 5 errors"},
		},
		{
			name: "Multiple uncorrected errors",
			counts: AllECCErrorCounts{
				Total:           ECCErrorCounts{Uncorrected: 10},
				L1Cache:         ECCErrorCounts{Uncorrected: 2},
				GPUDeviceMemory: ECCErrorCounts{Uncorrected: 3},
				GPURegisterFile: ECCErrorCounts{Uncorrected: 1},
			},
			expected: []string{
				"total uncorrected 10 errors",
				"L1 Cache uncorrected 2 errors",
				"GPU device memory uncorrected 3 errors",
				"GPU register file uncorrected 1 errors",
			},
		},
		{
			name: "All types of uncorrected errors",
			counts: AllECCErrorCounts{
				Total:            ECCErrorCounts{Uncorrected: 1},
				L1Cache:          ECCErrorCounts{Uncorrected: 1},
				L2Cache:          ECCErrorCounts{Uncorrected: 1},
				DRAM:             ECCErrorCounts{Uncorrected: 1},
				SRAM:             ECCErrorCounts{Uncorrected: 1},
				GPUDeviceMemory:  ECCErrorCounts{Uncorrected: 1},
				GPUTextureMemory: ECCErrorCounts{Uncorrected: 1},
				SharedMemory:     ECCErrorCounts{Uncorrected: 1},
				GPURegisterFile:  ECCErrorCounts{Uncorrected: 1},
			},
			expected: []string{
				"total uncorrected 1 errors",
				"L1 Cache uncorrected 1 errors",
				"L2 Cache uncorrected 1 errors",
				"DRAM uncorrected 1 errors",
				"SRAM uncorrected 1 errors",
				"GPU device memory uncorrected 1 errors",
				"GPU texture memory uncorrected 1 errors",
				"shared memory uncorrected 1 errors",
				"GPU register file uncorrected 1 errors",
			},
		},
		{
			name: "Ignore corrected errors",
			counts: AllECCErrorCounts{
				Total:   ECCErrorCounts{Corrected: 5},
				L1Cache: ECCErrorCounts{Corrected: 3, Uncorrected: 1},
			},
			expected: []string{"L1 Cache uncorrected 1 errors"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.counts.FindUncorrectedErrs()
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("FindUncorrectedErrs() = %v, want %v", result, tt.expected)
			}
		})
	}
}
