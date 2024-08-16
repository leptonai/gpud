package nvml

import (
	"reflect"
	"testing"
)

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
