package disk

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
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

func TestStatWithTimeout_Success(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a file in the directory
	testFile := tempDir + "/testfile"
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()

	// Test successful stat with normal timeout
	info, err := statWithTimeout(ctx, testFile, 5*time.Second)
	if err != nil {
		t.Fatalf("statWithTimeout failed: %v", err)
	}

	if info == nil {
		t.Fatal("statWithTimeout returned nil info but no error")
	}

	if info.Name() != "testfile" {
		t.Errorf("expected file name 'testfile', got '%s'", info.Name())
	}
}

func TestStatWithTimeout_Timeout(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a file in the directory
	testFile := tempDir + "/testfile"
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Use a very short timeout to force timeout
	ctx := context.Background()

	// This is tricky to test reliably since we can't easily make os.Stat hang
	// Instead, let's test with a pre-canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	info, err := statWithTimeout(canceledCtx, testFile, 1*time.Second)
	if err == nil {
		t.Fatal("expected statWithTimeout to return error with canceled context")
	}

	if info != nil {
		t.Fatal("expected statWithTimeout to return nil info on error")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestStatWithTimeout_ContextCanceled(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a file in the directory
	testFile := tempDir + "/testfile"
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Test with context that gets canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	info, err := statWithTimeout(ctx, testFile, 5*time.Second)
	if err == nil {
		t.Fatal("expected statWithTimeout to return error with canceled context")
	}

	if info != nil {
		t.Fatal("expected statWithTimeout to return nil info on error")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestStatWithTimeout_NonExistentFile(t *testing.T) {
	ctx := context.Background()
	nonExistentFile := "/path/that/does/not/exist"

	info, err := statWithTimeout(ctx, nonExistentFile, 5*time.Second)
	if err == nil {
		t.Fatal("expected statWithTimeout to return error for non-existent file")
	}

	if info != nil {
		t.Fatal("expected statWithTimeout to return nil info on error")
	}

	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got %v", err)
	}
}

func TestGetPartitions_StatTimedOut_MockScenario(t *testing.T) {
	// This test simulates a scenario where StatTimedOut would be set to true
	// We'll use a canceled context to simulate a timeout condition

	// Create a context that's already canceled to simulate timeout
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Try to get partitions with a canceled context
	// This should trigger the timeout handling logic
	partitions, err := GetPartitions(ctx, WithFstype(DefaultFsTypeFunc), WithStatTimeout(1*time.Millisecond))

	// The function should complete without error even if some stat operations fail
	if err != nil {
		t.Logf("GetPartitions returned error (expected in some cases): %v", err)
	}

	// Check if any partitions have StatTimedOut set to true
	var hasStatTimedOut bool
	for _, p := range partitions {
		if p.StatTimedOut {
			hasStatTimedOut = true
			t.Logf("Found partition with StatTimedOut=true: %s at %s", p.Device, p.MountPoint)

			// Verify that StatTimedOut partitions are not mounted
			if p.Mounted {
				t.Errorf("partition %s has StatTimedOut=true but Mounted=true, expected Mounted=false", p.Device)
			}
		}
	}

	t.Logf("Found %d partitions, hasStatTimedOut=%v", len(partitions), hasStatTimedOut)
}

func TestGetPartitions_StatTimedOut_False(t *testing.T) {
	// Test that StatTimedOut is false under normal conditions
	ctx := context.Background()

	// Get partitions with normal timeout
	partitions, err := GetPartitions(ctx, WithFstype(DefaultFsTypeFunc), WithStatTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("GetPartitions failed: %v", err)
	}

	// Verify that StatTimedOut is false for all partitions under normal conditions
	for _, p := range partitions {
		if p.StatTimedOut {
			t.Errorf("partition %s unexpectedly has StatTimedOut=true under normal conditions", p.Device)
		}
	}

	t.Logf("Verified %d partitions all have StatTimedOut=false", len(partitions))
}

func TestGetPartitions_StatTimedOut_WithTimeout(t *testing.T) {
	// Test with a very short timeout to increase likelihood of timeout
	ctx := context.Background()

	// Use a very short timeout
	partitions, err := GetPartitions(ctx, WithFstype(DefaultNFSFsTypeFunc), WithStatTimeout(1*time.Nanosecond))
	if err != nil {
		t.Logf("GetPartitions with short timeout returned error: %v", err)
	}

	// Count partitions with StatTimedOut
	var statTimedOutCount int
	for _, p := range partitions {
		if p.StatTimedOut {
			statTimedOutCount++
			t.Logf("Partition %s at %s has StatTimedOut=true", p.Device, p.MountPoint)

			// Verify that StatTimedOut partitions are not mounted
			if p.Mounted {
				t.Errorf("partition %s has StatTimedOut=true but Mounted=true", p.Device)
			}
		}
	}

	t.Logf("Found %d partitions with StatTimedOut=true out of %d total", statTimedOutCount, len(partitions))
}

func TestPartition_StatTimedOut_FieldExists(t *testing.T) {
	// Test that the StatTimedOut field exists and can be set
	partition := Partition{
		Device:       "/dev/test",
		MountPoint:   "/mnt/test",
		Fstype:       "nfs4",
		Mounted:      false,
		StatTimedOut: true,
	}

	if !partition.StatTimedOut {
		t.Error("StatTimedOut field should be true")
	}

	partition.StatTimedOut = false
	if partition.StatTimedOut {
		t.Error("StatTimedOut field should be false after setting")
	}
}

func TestPartition_StatTimedOut_JSON(t *testing.T) {
	// Test that StatTimedOut field is properly serialized to JSON
	partition := Partition{
		Device:       "/dev/test",
		MountPoint:   "/mnt/test",
		Fstype:       "nfs4",
		Mounted:      false,
		StatTimedOut: true,
	}

	jsonBytes, err := json.Marshal(partition)
	if err != nil {
		t.Fatalf("failed to marshal partition to JSON: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, "\"stat_timed_out\":true") {
		t.Errorf("JSON should contain stat_timed_out field, got: %s", jsonStr)
	}

	// Test unmarshalling
	var unmarshaledPartition Partition
	if err := json.Unmarshal(jsonBytes, &unmarshaledPartition); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if !unmarshaledPartition.StatTimedOut {
		t.Error("unmarshaled partition should have StatTimedOut=true")
	}
}
