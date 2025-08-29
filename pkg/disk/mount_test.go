package disk

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func Test_findMntTargetDevice(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatalf("failed to open testdata/mountinfo: %v", err)
	}
	defer f.Close()

	buf := bufio.NewScanner(f)

	mountPoint, fsType, err := findMntTargetDevice(buf, "/var/lib/kubelet")
	if err != nil {
		t.Fatalf("failed to find mount point: %v", err)
	}
	if mountPoint != "/dev/mapper/vgroot-lvroot" {
		t.Fatalf("expected mount point: %s, got: %s", "/dev/mapper/vgroot-lvroot", mountPoint)
	}
	if fsType != "ext4" {
		t.Fatalf("expected fsType ext4, got: %s", fsType)
	}
}

func Test_findFsTypeAndDeviceByMinorNumber1(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatalf("failed to open testdata/mountinfo: %v", err)
	}
	defer f.Close()

	buf := bufio.NewScanner(f)

	fsType, dev, err := findFsTypeAndDeviceByMinorNumber(buf, 81)
	if err != nil {
		t.Fatalf("failed to find mount point: %v", err)
	}
	if fsType != "fuse.testfs" {
		t.Fatalf("expected fsType: %s, got: %s", "fuse.testfs", fsType)
	}
	if dev != "TestFS:test-lepton-ai-us-east-dev" {
		t.Fatalf("expected dev: %s, got: %s", "TestFS:test-lepton-ai-us-east-dev", dev)
	}
}

func Test_findFsTypeAndDeviceByMinorNumber2(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatalf("failed to open testdata/mountinfo: %v", err)
	}
	defer f.Close()

	buf := bufio.NewScanner(f)

	fsType, dev, err := findFsTypeAndDeviceByMinorNumber(buf, 550)
	if err != nil {
		t.Fatalf("failed to find mount point: %v", err)
	}
	if fsType != "fuse.testfs" {
		t.Fatalf("expected fsType: %s, got: %s", "fuse.testfs", fsType)
	}
	if dev != "TestFS:ws-test-us-east-training" {
		t.Fatalf("expected dev: %s, got: %s", "TestFS:ws-test-us-east-training", dev)
	}
}

// Test for FindMntTargetDevice public function
func TestFindMntTargetDevice(t *testing.T) {
	// This test will use the actual /proc/self/mountinfo if it exists
	// We'll skip the test if not running on Linux
	if _, err := os.Stat("/proc/self/mountinfo"); os.IsNotExist(err) {
		t.Skip("Skipping test: /proc/self/mountinfo not available")
	}

	// Test with a common mount point that should exist on most Linux systems
	dev, fsType, err := FindMntTargetDevice("/")
	if err != nil {
		t.Fatalf("FindMntTargetDevice failed: %v", err)
	}

	// Root should have some device and filesystem type
	if dev == "" && fsType == "" {
		t.Log("No device found for root mount point (may be expected in some environments)")
	}
}

// Test for FindFsTypeAndDeviceByMinorNumber public function
func TestFindFsTypeAndDeviceByMinorNumber(t *testing.T) {
	// This test will use the actual /proc/self/mountinfo if it exists
	// We'll skip the test if not running on Linux
	if _, err := os.Stat("/proc/self/mountinfo"); os.IsNotExist(err) {
		t.Skip("Skipping test: /proc/self/mountinfo not available")
	}

	// Test with minor number that likely doesn't exist
	fsType, dev, err := FindFsTypeAndDeviceByMinorNumber(999999)
	if err != nil {
		t.Fatalf("FindFsTypeAndDeviceByMinorNumber failed: %v", err)
	}

	// Should return empty strings for non-existent minor number
	if fsType != "" || dev != "" {
		t.Errorf("Expected empty strings for non-existent minor number, got fsType=%s, dev=%s", fsType, dev)
	}
}

// Test edge cases for findMntTargetDevice
func Test_findMntTargetDevice_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		mountinfoData  string
		targetDir      string
		expectedDev    string
		expectedFsType string
		expectError    bool
	}{
		{
			name:           "empty mountinfo",
			mountinfoData:  "",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "malformed line - too few fields",
			mountinfoData:  "1 2 3 4 5",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "no matching mount point",
			mountinfoData:  "1 2 3 4 /other 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "overlay filesystem should be skipped",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 - overlay overlay rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "tmpfs should be skipped",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 - tmpfs tmpfs rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "shm should be skipped",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 - shm /dev/shm rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "valid mount point match",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			targetDir:      "/test",
			expectedDev:    "/dev/sda1",
			expectedFsType: "ext4",
			expectError:    false,
		},
		{
			name:           "prefix match for subdirectory",
			mountinfoData:  "1 2 3 4 /test/subdir 6 7 8 9 10 11 - xfs /dev/sdb1 rw",
			targetDir:      "/test",
			expectedDev:    "/dev/sdb1",
			expectedFsType: "xfs",
			expectError:    false,
		},
		{
			name:           "line without separator",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 ext4 /dev/sda1 rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.mountinfoData))
			dev, fsType, err := findMntTargetDevice(scanner, tt.targetDir)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if dev != tt.expectedDev {
				t.Errorf("Expected dev=%s, got=%s", tt.expectedDev, dev)
			}
			if fsType != tt.expectedFsType {
				t.Errorf("Expected fsType=%s, got=%s", tt.expectedFsType, fsType)
			}
		})
	}
}

// Test edge cases for findFsTypeAndDeviceByMinorNumber
func Test_findFsTypeAndDeviceByMinorNumber_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		mountinfoData  string
		minor          int
		expectedFsType string
		expectedDev    string
		expectError    bool
	}{
		{
			name:           "empty mountinfo",
			mountinfoData:  "",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "malformed line - too few fields",
			mountinfoData:  "1 2",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "no device number field",
			mountinfoData:  "1 2 : 4 5 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "matching minor number",
			mountinfoData:  "1 2 8:10 4 5 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			minor:          10,
			expectedFsType: "ext4",
			expectedDev:    "/dev/sda1",
			expectError:    false,
		},
		{
			name:           "non-matching minor number",
			mountinfoData:  "1 2 8:20 4 5 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "line without separator",
			mountinfoData:  "1 2 8:10 4 5 6 7 8 9 10 11 ext4 /dev/sda1 rw",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "separator but too few fields after",
			mountinfoData:  "1 2 8:10 4 5 6 7 8 9 10 11 - ext4",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.mountinfoData))
			fsType, dev, err := findFsTypeAndDeviceByMinorNumber(scanner, tt.minor)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if fsType != tt.expectedFsType {
				t.Errorf("Expected fsType=%s, got=%s", tt.expectedFsType, fsType)
			}
			if dev != tt.expectedDev {
				t.Errorf("Expected dev=%s, got=%s", tt.expectedDev, dev)
			}
		})
	}
}
