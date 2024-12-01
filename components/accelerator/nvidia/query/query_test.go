package query

import (
	"testing"

	"github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
)

func TestOutput_GPUProductName(t *testing.T) {
	tests := []struct {
		name     string
		output   *Output
		expected string
	}{
		{
			name:     "nil output",
			output:   nil,
			expected: "",
		},
		{
			name:     "empty output",
			output:   &Output{},
			expected: "",
		},
		{
			name: "NVML name available",
			output: &Output{
				NVML: &nvml.Output{
					DeviceInfos: []*nvml.DeviceInfo{
						{Name: "NVIDIA A100"},
					},
				},
			},
			expected: "NVIDIA A100",
		},
		{
			name: "SMI name available when NVML empty",
			output: &Output{
				NVML: &nvml.Output{
					DeviceInfos: []*nvml.DeviceInfo{
						{Name: ""},
					},
				},
				SMI: &SMIOutput{
					GPUs: []NvidiaSMIGPU{
						{ProductName: "NVIDIA A100"},
					},
				},
			},
			expected: "NVIDIA A100",
		},
		{
			name: "SMI name available when NVML nil",
			output: &Output{
				SMI: &SMIOutput{
					GPUs: []NvidiaSMIGPU{
						{ProductName: "NVIDIA A100"},
					},
				},
			},
			expected: "NVIDIA A100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.output.GPUProductName()
			if got != tt.expected {
				t.Errorf("GPUProductName() = %v, want %v", got, tt.expected)
			}
		})
	}
}
