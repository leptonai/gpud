package query

import (
	"testing"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
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
