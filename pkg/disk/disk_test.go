package disk

import (
	"context"
	"os"
	"testing"
)

func TestGetPartitions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	partitions, err := GetPartitions(ctx, WithFstype(DefaultMatchFuncFstype))
	if err != nil {
		t.Fatalf("failed to get partitions: %v", err)
	}
	yb, err := partitions.YAML()
	if err != nil {
		t.Fatalf("failed to marshal partitions to yaml: %v", err)
	}
	t.Logf("partitions:\n%s\n", string(yb))

	partitions.RenderTable(os.Stdout)
}

func TestPartitions_TotalBytes(t *testing.T) {
	tests := []struct {
		name  string
		parts Partitions
		want  uint64
	}{
		{
			name:  "empty partitions",
			parts: Partitions{},
			want:  0,
		},
		{
			name: "single mounted partition",
			parts: Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Mounted:    true,
					Usage: &Usage{
						TotalBytes: 1000,
					},
				},
			},
			want: 1000,
		},
		{
			name: "multiple mounted partitions",
			parts: Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Mounted:    true,
					Usage: &Usage{
						TotalBytes: 1000,
					},
				},
				{
					Device:     "/dev/sda2",
					MountPoint: "/home",
					Mounted:    true,
					Usage: &Usage{
						TotalBytes: 2000,
					},
				},
			},
			want: 3000,
		},
		{
			name: "skip unmounted partition",
			parts: Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Mounted:    true,
					Usage: &Usage{
						TotalBytes: 1000,
					},
				},
				{
					Device:     "/dev/sda2",
					MountPoint: "/home",
					Mounted:    false,
					Usage: &Usage{
						TotalBytes: 2000,
					},
				},
			},
			want: 1000,
		},
		{
			name: "skip nil usage",
			parts: Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Mounted:    true,
					Usage: &Usage{
						TotalBytes: 1000,
					},
				},
				{
					Device:     "/dev/sda2",
					MountPoint: "/home",
					Mounted:    true,
					Usage:      nil,
				},
			},
			want: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.parts.GetMountedTotalBytes(); got != tt.want {
				t.Errorf("Partitions.TotalBytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUsage_GetUsedPercent(t *testing.T) {
	tests := []struct {
		name    string
		usage   Usage
		want    float64
		wantErr bool
	}{
		{
			name: "valid percent",
			usage: Usage{
				UsedPercent: "75.50",
			},
			want:    75.50,
			wantErr: false,
		},
		{
			name: "invalid percent",
			usage: Usage{
				UsedPercent: "invalid",
			},
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.usage.GetUsedPercent()
			if (err != nil) != tt.wantErr {
				t.Errorf("Usage.GetUsedPercent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Usage.GetUsedPercent() = %v, want %v", got, tt.want)
			}
		})
	}
}
