package infiniband

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupportsInfinibandPortRate(t *testing.T) {
	tests := []struct {
		name           string
		gpuProductName string
		wantPorts      int
		wantRate       int
		wantErr        bool
	}{
		{
			name:           "GB200 GPU",
			gpuProductName: "NVIDIA GB200 GPU",
			wantPorts:      8,
			wantRate:       400,
			wantErr:        false,
		},
		{
			name:           "GB200 lowercase",
			gpuProductName: "nvidia gb200 gpu",
			wantPorts:      8,
			wantRate:       400,
			wantErr:        false,
		},
		{
			name:           "B200 GPU",
			gpuProductName: "NVIDIA B200 GPU",
			wantPorts:      8,
			wantRate:       400,
			wantErr:        false,
		},
		{
			name:           "H100 GPU",
			gpuProductName: "NVIDIA H100 PCIe",
			wantPorts:      8,
			wantRate:       400,
			wantErr:        false,
		},
		{
			name:           "H100 SXM GPU",
			gpuProductName: "NVIDIA H100-SXM5-80GB",
			wantPorts:      8,
			wantRate:       400,
			wantErr:        false,
		},
		{
			name:           "H200 GPU",
			gpuProductName: "NVIDIA H200",
			wantPorts:      8,
			wantRate:       400,
			wantErr:        false,
		},
		{
			name:           "A100 GPU",
			gpuProductName: "NVIDIA A100-SXM4-80GB",
			wantPorts:      1,
			wantRate:       200,
			wantErr:        false,
		},
		{
			name:           "Case insensitive A100",
			gpuProductName: "nvidia a100",
			wantPorts:      1,
			wantRate:       200,
			wantErr:        false,
		},
		{
			name:           "unsupported GPU",
			gpuProductName: "NVIDIA K80",
			wantPorts:      0,
			wantRate:       0,
			wantErr:        true,
		},
		{
			name:           "empty product name",
			gpuProductName: "",
			wantPorts:      0,
			wantRate:       0,
			wantErr:        true,
		},
		{
			name:           "Unknown GPU T4",
			gpuProductName: "NVIDIA T4",
			wantPorts:      0,
			wantRate:       0,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SupportsInfinibandPortRate(tt.gpuProductName)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrNoExpectedPortStates)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPorts, got.AtLeastPorts)
			assert.Equal(t, tt.wantRate, got.AtLeastRate)
		})
	}
}
