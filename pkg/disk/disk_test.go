package disk

import (
	"context"
	"math"
	"os"
	"strconv"
	"testing"
)

func TestGetPartitions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	partitions, err := GetPartitions(ctx)
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
					Device:      "/dev/sda1",
					MountPoints: []string{"/"},
					Mounted:     true,
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
					Device:      "/dev/sda1",
					MountPoints: []string{"/"},
					Mounted:     true,
					Usage: &Usage{
						TotalBytes: 1000,
					},
				},
				{
					Device:      "/dev/sda2",
					MountPoints: []string{"/home"},
					Mounted:     true,
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
					Device:      "/dev/sda1",
					MountPoints: []string{"/"},
					Mounted:     true,
					Usage: &Usage{
						TotalBytes: 1000,
					},
				},
				{
					Device:      "/dev/sda2",
					MountPoints: []string{"/home"},
					Mounted:     false,
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
					Device:      "/dev/sda1",
					MountPoints: []string{"/"},
					Mounted:     true,
					Usage: &Usage{
						TotalBytes: 1000,
					},
				},
				{
					Device:      "/dev/sda2",
					MountPoints: []string{"/home"},
					Mounted:     true,
					Usage:       nil,
				},
			},
			want: 1000,
		},
		{
			name: "skip aggregated partitions",
			parts: Partitions{
				{
					Device:      "/dev/sda1",
					MountPoints: []string{"/", "/home"},
					Mounted:     true,
					Usage: &Usage{
						TotalBytes: 1000,
					},
				},
				{
					Device:      "/dev/sda2",
					MountPoints: []string{"/var"},
					Mounted:     true,
					Usage: &Usage{
						TotalBytes: 2000,
					},
				},
			},
			want: 2000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.parts.TotalBytes(); got != tt.want {
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

func TestUsage_Add(t *testing.T) {
	tests := []struct {
		name string
		a    Usage
		b    Usage
		want Usage
	}{
		{
			name: "add two usages",
			a: Usage{
				TotalBytes:  1000,
				UsedBytes:   600,
				FreeBytes:   400,
				InodesTotal: 100,
				InodesUsed:  60,
				InodesFree:  40,
			},
			b: Usage{
				TotalBytes:  2000,
				UsedBytes:   1000,
				FreeBytes:   1000,
				InodesTotal: 200,
				InodesUsed:  100,
				InodesFree:  100,
			},
			want: Usage{
				TotalBytes:             3000,
				TotalHumanized:         "3.0 kB",
				UsedBytes:              1600,
				UsedHumanized:          "1.6 kB",
				FreeBytes:              1400,
				FreeHumanized:          "1.4 kB",
				UsedPercent:            "53.33",
				UsedPercentFloat:       53.33333333333333,
				InodesTotal:            300,
				InodesUsed:             160,
				InodesFree:             140,
				InodesUsedPercent:      "53.33",
				InodesUsedPercentFloat: 53.33333333333333,
			},
		},
		{
			name: "add with zero usage",
			a: Usage{
				TotalBytes:  1000,
				UsedBytes:   600,
				FreeBytes:   400,
				InodesTotal: 100,
				InodesUsed:  60,
				InodesFree:  40,
			},
			b: Usage{},
			want: Usage{
				TotalBytes:             1000,
				TotalHumanized:         "1.0 kB",
				UsedBytes:              600,
				UsedHumanized:          "600 B",
				FreeBytes:              400,
				FreeHumanized:          "400 B",
				UsedPercent:            "60.00",
				UsedPercentFloat:       60.0,
				InodesTotal:            100,
				InodesUsed:             60,
				InodesFree:             40,
				InodesUsedPercent:      "60.00",
				InodesUsedPercentFloat: 60.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Add(tt.b)

			// Compare numeric fields directly
			if got.TotalBytes != tt.want.TotalBytes {
				t.Errorf("TotalBytes = %v, want %v", got.TotalBytes, tt.want.TotalBytes)
			}
			if got.UsedBytes != tt.want.UsedBytes {
				t.Errorf("UsedBytes = %v, want %v", got.UsedBytes, tt.want.UsedBytes)
			}
			if got.FreeBytes != tt.want.FreeBytes {
				t.Errorf("FreeBytes = %v, want %v", got.FreeBytes, tt.want.FreeBytes)
			}
			if got.InodesTotal != tt.want.InodesTotal {
				t.Errorf("InodesTotal = %v, want %v", got.InodesTotal, tt.want.InodesTotal)
			}
			if got.InodesUsed != tt.want.InodesUsed {
				t.Errorf("InodesUsed = %v, want %v", got.InodesUsed, tt.want.InodesUsed)
			}
			if got.InodesFree != tt.want.InodesFree {
				t.Errorf("InodesFree = %v, want %v", got.InodesFree, tt.want.InodesFree)
			}

			// Compare percentage strings with some tolerance due to floating point arithmetic
			gotUsedPercent, _ := strconv.ParseFloat(got.UsedPercent, 64)
			wantUsedPercent, _ := strconv.ParseFloat(tt.want.UsedPercent, 64)
			if math.Abs(gotUsedPercent-wantUsedPercent) > 0.01 {
				t.Errorf("UsedPercent = %v, want %v", got.UsedPercent, tt.want.UsedPercent)
			}

			gotInodesUsedPercent, _ := strconv.ParseFloat(got.InodesUsedPercent, 64)
			wantInodesUsedPercent, _ := strconv.ParseFloat(tt.want.InodesUsedPercent, 64)
			if math.Abs(gotInodesUsedPercent-wantInodesUsedPercent) > 0.01 {
				t.Errorf("InodesUsedPercent = %v, want %v", got.InodesUsedPercent, tt.want.InodesUsedPercent)
			}
		})
	}
}
