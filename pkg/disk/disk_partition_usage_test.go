package disk

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGetPartitions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	partitions, err := GetPartitions(ctx, WithFstype(DefaultFsTypeFunc))
	if err != nil {
		t.Fatalf("failed to get partitions: %v", err)
	}
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

func TestPartitions_JSON(t *testing.T) {
	parts := Partitions{
		{
			Device:     "/dev/sda1",
			Fstype:     "ext4",
			MountPoint: "/",
			Mounted:    true,
			Usage: &Usage{
				TotalBytes: 1000,
				FreeBytes:  500,
				UsedBytes:  500,
			},
		},
	}

	jsonBytes, err := json.Marshal(parts)
	if err != nil {
		t.Fatalf("failed to marshal partitions to JSON: %v", err)
	}

	var unmarshaledParts Partitions
	if err := json.Unmarshal(jsonBytes, &unmarshaledParts); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(unmarshaledParts) != len(parts) {
		t.Fatalf("unmarshaled partitions length = %d, want %d", len(unmarshaledParts), len(parts))
	}

	if unmarshaledParts[0].Device != parts[0].Device {
		t.Errorf("unmarshaled partition device = %s, want %s", unmarshaledParts[0].Device, parts[0].Device)
	}

	if unmarshaledParts[0].Usage.TotalBytes != parts[0].Usage.TotalBytes {
		t.Errorf("unmarshaled partition total bytes = %d, want %d", unmarshaledParts[0].Usage.TotalBytes, parts[0].Usage.TotalBytes)
	}
}

func TestPartitions_RenderTable(t *testing.T) {
	parts := Partitions{
		{
			Device:     "/dev/sda1",
			Fstype:     "ext4",
			MountPoint: "/",
			Mounted:    true,
			Usage: &Usage{
				TotalBytes: 1000,
				FreeBytes:  500,
				UsedBytes:  500,
			},
		},
		{
			Device:     "/dev/sda2",
			Fstype:     "xfs",
			MountPoint: "/home",
			Mounted:    false,
			Usage:      nil,
		},
	}

	var buf bytes.Buffer
	parts.RenderTable(&buf)

	output := buf.String()
	t.Logf("Table output: %s", output)

	// Check that the table contains our device values
	if !strings.Contains(output, "/dev/sda1") {
		t.Errorf("table is missing device: %s", output)
	}
	if !strings.Contains(output, "ext4") {
		t.Errorf("table is missing fstype: %s", output)
	}
	if !strings.Contains(output, "/home") {
		t.Errorf("table is missing mount point: %s", output)
	}
}

func TestOpApplyOpts(t *testing.T) {
	tests := []struct {
		name             string
		opts             []OpOption
		wantFstypeMatch  bool
		wantDevTypeMatch bool
	}{
		{
			name:             "no options",
			opts:             nil,
			wantFstypeMatch:  true, // Default should match everything
			wantDevTypeMatch: true, // Default should match everything
		},
		{
			name: "with fstype matcher",
			opts: []OpOption{
				WithFstype(func(fs string) bool { return fs == "ext4" }),
			},
			wantFstypeMatch:  false, // Our test will use "xfs"
			wantDevTypeMatch: true,  // Default should match everything
		},
		{
			name: "with device type matcher",
			opts: []OpOption{
				WithDeviceType(func(dt string) bool { return dt == "disk" }),
			},
			wantFstypeMatch:  true,  // Default should match everything
			wantDevTypeMatch: false, // Our test will use "part"
		},
		{
			name: "with both matchers",
			opts: []OpOption{
				WithFstype(func(fs string) bool { return fs == "ext4" }),
				WithDeviceType(func(dt string) bool { return dt == "disk" }),
			},
			wantFstypeMatch:  false, // Our test will use "xfs"
			wantDevTypeMatch: false, // Our test will use "part"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			if err := op.applyOpts(tt.opts); err != nil {
				t.Fatalf("applyOpts() error = %v", err)
			}

			// Test fstype matcher
			if got := op.matchFuncFstype("xfs"); got != tt.wantFstypeMatch {
				t.Errorf("matchFuncFstype(xfs) = %v, want %v", got, tt.wantFstypeMatch)
			}

			// Test device type matcher
			if got := op.matchFuncDeviceType("part"); got != tt.wantDevTypeMatch {
				t.Errorf("matchFuncDeviceType(part) = %v, want %v", got, tt.wantDevTypeMatch)
			}
		})
	}
}

func TestDefaultMatchFuncDeviceType(t *testing.T) {
	tests := []struct {
		deviceType string
		want       bool
	}{
		{"disk", true},
		{"part", false},
		{"loop", false},
		{"rom", false},
	}

	for _, tt := range tests {
		t.Run(tt.deviceType, func(t *testing.T) {
			if got := DefaultMatchFuncDeviceType(tt.deviceType); got != tt.want {
				t.Errorf("DefaultMatchFuncDeviceType(%s) = %v, want %v", tt.deviceType, got, tt.want)
			}
		})
	}
}

func TestGetPartitionsWithSkipUsage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get partitions with skipUsage set to true
	partitions, err := GetPartitions(ctx, WithSkipUsage(), WithFstype(DefaultFsTypeFunc))
	if err != nil {
		t.Fatalf("failed to get partitions with skipUsage: %v", err)
	}

	// Verify that no partition has Usage information when skipUsage is true
	for i, p := range partitions {
		if p.Mounted && p.Usage != nil {
			t.Errorf("mounted partition %d (%s at %s) has Usage info when skipUsage is true",
				i, p.Device, p.MountPoint)
		}
	}

	// Now get partitions without skipUsage for comparison
	partitionsWithUsage, err := GetPartitions(ctx, WithFstype(DefaultFsTypeFunc))
	if err != nil {
		t.Fatalf("failed to get partitions without skipUsage: %v", err)
	}

	// Verify that mounted partitions have Usage information when skipUsage is false
	var mountedWithUsageCount int
	for _, p := range partitionsWithUsage {
		if p.Mounted && p.Usage != nil {
			mountedWithUsageCount++
		}
	}

	// Log results
	t.Logf("found %d partitions with skipUsage=true", len(partitions))
	t.Logf("found %d partitions with Usage info when skipUsage=false", mountedWithUsageCount)

	// Test isn't meaningful if we don't have mounted partitions with usage info
	if mountedWithUsageCount == 0 {
		t.Log("no mounted partitions with usage info detected, test may not be reliable")
	}
}
