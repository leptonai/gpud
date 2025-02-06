package query

import (
	"reflect"
	"testing"
)

func TestSupportedMemoryMgmtCapsByGPUProduct(t *testing.T) {
	tests := []struct {
		name           string
		gpuProductName string
		expected       MemoryErrorManagementCapabilities
	}{
		{
			name:           "NVIDIA H100 80GB HBM3",
			gpuProductName: "NVIDIA H100 80GB HBM3",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "NVIDIA GeForce RTX 4090",
			gpuProductName: "NVIDIA GeForce RTX 4090",
			expected:       MemoryErrorManagementCapabilities{},
		},
		{
			name:           "NVIDIA A10",
			gpuProductName: "NVIDIA A10",
			expected: MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		},
		{
			name:           "NVIDIA A100",
			gpuProductName: "NVIDIA A100",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "Lowercase input",
			gpuProductName: "nvidia h100 80gb hbm3",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "NVIDIA B100",
			gpuProductName: "NVIDIA B100",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "NVIDIA B200",
			gpuProductName: "NVIDIA B200",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "Mixed case input",
			gpuProductName: "NvIdIa A100 PCIe",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "Empty string",
			gpuProductName: "",
			expected:       MemoryErrorManagementCapabilities{},
		},
		{
			name:           "NVIDIA T4",
			gpuProductName: "NVIDIA T4",
			expected:       MemoryErrorManagementCapabilities{},
		},
		{
			name:           "NVIDIA V100",
			gpuProductName: "NVIDIA V100",
			expected:       MemoryErrorManagementCapabilities{},
		},
		{
			name:           "NVIDIA A10G",
			gpuProductName: "NVIDIA A10G",
			expected: MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		},
		{
			name:           "GPU with SXM suffix",
			gpuProductName: "NVIDIA A100-SXM",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "GPU with PCIe suffix",
			gpuProductName: "NVIDIA A100 PCIe",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "GPU with memory size suffix",
			gpuProductName: "NVIDIA A100 80GB",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "Special characters in name",
			gpuProductName: "NVIDIA-A100_80GB",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
		{
			name:           "Non-NVIDIA prefix",
			gpuProductName: "Some A100 GPU",
			expected: MemoryErrorManagementCapabilities{
				ErrorContainment:     true,
				DynamicPageOfflining: true,
				RowRemapping:         true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SupportedMemoryMgmtCapsByGPUProduct(tt.gpuProductName)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("GetGPUMemoryErrorManagement(%q) = %v, want %v", tt.gpuProductName, result, tt.expected)
			}
		})
	}
}
