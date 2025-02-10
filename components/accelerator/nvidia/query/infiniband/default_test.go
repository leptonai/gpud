package infiniband

import "testing"

func TestSupportsInfinibandPortRate(t *testing.T) {
	tests := []struct {
		name           string
		gpuProductName string
		want           ExpectedPortStates
	}{
		{
			name:           "A100 GPU",
			gpuProductName: "NVIDIA A100-SXM4-80GB",
			want: ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			},
		},
		{
			name:           "H100 GPU",
			gpuProductName: "NVIDIA H100-SXM5-80GB",
			want: ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			},
		},
		{
			name:           "H200 GPU",
			gpuProductName: "NVIDIA H200",
			want: ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			},
		},
		{
			name:           "B200 GPU",
			gpuProductName: "NVIDIA B200",
			want: ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			},
		},
		{
			name:           "Unknown GPU",
			gpuProductName: "NVIDIA T4",
			want: ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			},
		},
		{
			name:           "Case insensitive A100",
			gpuProductName: "nvidia a100",
			want: ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SupportsInfinibandPortRate(tt.gpuProductName)
			if got != tt.want {
				t.Errorf("SupportsInfinibandPortRate(%q) = %+v, want %+v", tt.gpuProductName, got, tt.want)
			}
		})
	}
}
