package infiniband

import (
	"os"
	"testing"
)

func TestCountInfinibandClassBySubDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dirPath string
		dirs    []string
		want    int
	}{
		{
			name:    "multiple infiniband devices",
			dirPath: t.TempDir(),
			dirs: []string{
				"mlx5_0", "mlx5_1", "mlx5_2", "mlx5_3",
				"mlx5_4", "mlx5_5", "mlx5_6", "mlx5_7",
				"mlx5_8", "mlx5_9", "mlx5_10", "mlx5_11",
			},
			want: 12,
		},
		{
			name:    "single infiniband device",
			dirPath: t.TempDir(),
			dirs:    []string{"mlx5_0"},
			want:    1,
		},
		{
			name:    "no infiniband devices",
			dirPath: t.TempDir(),
			dirs:    []string{},
			want:    0,
		},
		{
			name:    "non-existent directory",
			dirPath: "/non/existent/path",
			dirs:    []string{},
			want:    0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create test directories if path exists
			if tt.dirPath != "/non/existent/path" {
				for _, d := range tt.dirs {
					if err := os.Mkdir(tt.dirPath+"/"+d, 0755); err != nil {
						t.Fatalf("Failed to create test directory: %v", err)
					}
				}
			}

			got := CountInfinibandClassBySubDir(tt.dirPath)
			if got != tt.want {
				t.Errorf("countInfinibandClass() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSupportsInfinibandPortRate(t *testing.T) {
	tests := []struct {
		name        string
		productName string
		want        int
	}{
		// A100 tests
		{
			name:        "A100 40GB supports Infiniband at 200Gb/s",
			productName: "NVIDIA A100-SXM4-40GB",
			want:        200,
		},
		{
			name:        "A100 80GB supports Infiniband at 200Gb/s",
			productName: "NVIDIA A100-SXM4-80GB",
			want:        200,
		},
		{
			name:        "A100 lowercase supports Infiniband at 200Gb/s",
			productName: "nvidia a100",
			want:        200,
		},

		// H100 tests
		{
			name:        "H100 80GB supports Infiniband at 400Gb/s",
			productName: "NVIDIA H100 80GB HBM3",
			want:        400,
		},
		{
			name:        "H100 PCIe supports Infiniband at 400Gb/s",
			productName: "NVIDIA H100 PCIe",
			want:        400,
		},
		{
			name:        "H100 lowercase supports Infiniband at 400Gb/s",
			productName: "nvidia h100",
			want:        400,
		},

		// B100 tests
		{
			name:        "B100 supports Infiniband at 400Gb/s",
			productName: "NVIDIA B100",
			want:        400,
		},
		{
			name:        "B100 lowercase supports Infiniband at 400Gb/s",
			productName: "nvidia b100",
			want:        400,
		},

		// H200 tests
		{
			name:        "H200 supports Infiniband at 400Gb/s",
			productName: "NVIDIA H200",
			want:        400,
		},
		{
			name:        "H200 lowercase supports Infiniband at 400Gb/s",
			productName: "nvidia h200",
			want:        400,
		},

		// B200 tests
		{
			name:        "B200 supports Infiniband at 400Gb/s",
			productName: "NVIDIA B200",
			want:        400,
		},
		{
			name:        "B200 lowercase supports Infiniband at 400Gb/s",
			productName: "nvidia b200",
			want:        400,
		},

		// Non-supported GPUs
		{
			name:        "A10 does not support Infiniband",
			productName: "NVIDIA A10",
			want:        0,
		},
		{
			name:        "RTX 4090 does not support Infiniband",
			productName: "NVIDIA GeForce RTX 4090",
			want:        0,
		},
		{
			name:        "TITAN V does not support Infiniband",
			productName: "NVIDIA TITAN V",
			want:        0,
		},

		// Edge cases
		{
			name:        "Empty string returns 0",
			productName: "",
			want:        0,
		},
		{
			name:        "Mixed case name is handled correctly",
			productName: "NvIdIa H100 GPU",
			want:        400,
		},
		{
			name:        "Product name with spaces and special characters",
			productName: "NVIDIA-H100-GPU (Rev. 1.0)",
			want:        400,
		},
		{
			name:        "Product name with numbers in wrong place",
			productName: "NVIDIA 100H GPU",
			want:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsInfinibandPortRate(tt.productName); got != tt.want {
				t.Errorf("SupportsInfinibandPortRate(%q) = %v, want %v", tt.productName, got, tt.want)
			}
		})
	}
}
