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
