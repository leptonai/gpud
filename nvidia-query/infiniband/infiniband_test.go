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

func TestSupportsInfinibandProduct(t *testing.T) {
	tests := []struct {
		name        string
		productName string
		want        bool
	}{
		{
			// e.g.,
			// "gpu_1x_h100_sxm5" in Lambda Labs
			// "gpu_2x_h100_sxm5" in Lambda Labs
			// "gpu_8x_h100_sxm5" in Lambda Labs
			// H100s in Paperspace
			name:        "H100 supports Infiniband",
			productName: "NVIDIA H100 80GB HBM3",
			want:        true,
		},
		{
			// e.g.,
			// "gpu_1x_a100_sxm4" in Lambda Labs
			name:        "A100 40GB supports Infiniband",
			productName: "NVIDIA A100-SXM4-40GB",
			want:        true,
		},
		{
			// e.g.,
			// "gpu_8x_a100_80gb_sxm4" in Lambda Labs
			name:        "A100 80GB supports Infiniband",
			productName: "NVIDIA A100-SXM4-80GB",
			want:        true,
		},
		{
			// e.g.,
			// "gpu_1x_a10" in Lambda Labs
			name:        "A10 does not support Infiniband",
			productName: "NVIDIA A10",
			want:        false,
		},
		{
			name:        "RTX 4090 does not support Infiniband",
			productName: "NVIDIA GeForce RTX 4090",
			want:        false,
		},
		{
			name:        "TITAN V does not support Infiniband",
			productName: "NVIDIA TITAN V",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsInfinibandProduct(tt.productName); got != tt.want {
				t.Errorf("SupportsInfinibandProduct(%q) = %v, want %v", tt.productName, got, tt.want)
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
		{
			// e.g.,
			// "gpu_1x_h100_sxm5" in Lambda Labs
			// "gpu_2x_h100_sxm5" in Lambda Labs
			// "gpu_8x_h100_sxm5" in Lambda Labs
			// H100s in Paperspace
			name:        "H100 supports Infiniband",
			productName: "NVIDIA H100 80GB HBM3",
			want:        400,
		},
		{
			// e.g.,
			// "gpu_1x_a100_sxm4" in Lambda Labs
			name:        "A100 40GB supports Infiniband",
			productName: "NVIDIA A100-SXM4-40GB",
			want:        200,
		},
		{
			// e.g.,
			// "gpu_8x_a100_80gb_sxm4" in Lambda Labs
			name:        "A100 80GB supports Infiniband",
			productName: "NVIDIA A100-SXM4-80GB",
			want:        200,
		},
		{
			// e.g.,
			// "gpu_1x_a10" in Lambda Labs
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsInfinibandPortRate(tt.productName); got != tt.want {
				t.Errorf("SupportsInfinibandPortRate(%q) = %v, want %v", tt.productName, got, tt.want)
			}
		})
	}
}
