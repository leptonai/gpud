package disk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLsblkNullFstypeFallbackIntegration tests the scenario where lsblk
// returns null fstypes and ensures devices are properly detected with findmnt fallback
func TestLsblkNullFstypeFallbackIntegration(t *testing.T) {
	// Scenario where lsblk returns null fstypes
	lsblkOutput := `{
   "blockdevices": [
      {
         "name": "/dev/sda",
         "type": "disk",
         "size": 137438953472,
         "rota": true,
         "serial": null,
         "wwn": null,
         "vendor": "Msft    ",
         "model": "Virtual Disk    ",
         "rev": "1.0 ",
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/sda1",
               "type": "part",
               "size": 137322544640,
               "rota": true,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/var/lib/kubelet/pods/b76d2533-919d-4fc7-8274-132b7a7b7bf6/volume-subpaths/nvidia-device-plugin-entrypoint/nvidia-device-plugin/0",
               "fstype": null,
               "fsused": "97563111424",
               "partuuid": null
            }
         ]
      }
   ]
}`

	// Mock the lsblk command to return the problematic output
	originalLsblkExecutor := lsblkCommandExecutor
	lsblkCommandExecutor = func(ctx context.Context, lsblkBin string, flags string) ([]byte, error) {
		return []byte(lsblkOutput), nil
	}
	defer func() { lsblkCommandExecutor = originalLsblkExecutor }()

	// Mock version detection
	originalGetLsblkVersion := getLsblkBinPathAndVersionFunc
	getLsblkBinPathAndVersionFunc = func(ctx context.Context) (string, string, error) {
		return "/usr/bin/lsblk", "lsblk from util-linux 2.37.2", nil
	}
	defer func() { getLsblkBinPathAndVersionFunc = originalGetLsblkVersion }()

	// Mock findmnt to provide the fstype that lsblk couldn't
	originalFindMntExecutor := findMntExecutor
	findMntCallCount := 0
	findMntExecutor = func(ctx context.Context, target string) (*FindMntOutput, error) {
		findMntCallCount++
		// Mock findmnt response for the test scenario
		if target == "/var/lib/kubelet/pods/b76d2533-919d-4fc7-8274-132b7a7b7bf6/volume-subpaths/nvidia-device-plugin-entrypoint/nvidia-device-plugin/0" ||
			target == "/var/lib/kubelet" {
			return &FindMntOutput{
				Target: "/var/lib/kubelet",
				Filesystems: []FoundMnt{
					{
						MountedPoint:         "/var/lib/kubelet",
						Sources:              []string{"/dev/sda1", "/var/lib/kubelet"},
						Fstype:               "ext4",
						SizeHumanized:        "123.9G",
						SizeBytes:            133009735680,
						UsedHumanized:        "90.9G",
						UsedBytes:            97563111424,
						AvailableHumanized:   "33G",
						AvailableBytes:       35433627648,
						UsedPercentHumanized: "73%",
						UsedPercent:          73,
					},
				},
			}, nil
		}
		return nil, nil // Return nil for devices without mount points
	}
	defer func() { findMntExecutor = originalFindMntExecutor }()

	// Test 1: Verify devices are found (addresses "no block device found" warning)
	ctx := context.Background()
	devs, err := GetBlockDevicesWithLsblk(ctx)
	require.NoError(t, err, "Should successfully get block devices")
	assert.Greater(t, len(devs), 0, "Should find at least one block device (fixes 'no block device found' warning)")

	// Test 2: Verify the device is correctly parsed
	assert.Equal(t, 1, len(devs), "Should find exactly 1 top-level device")
	assert.Equal(t, "/dev/sda", devs[0].Name, "Should find /dev/sda")
	assert.Equal(t, "disk", devs[0].Type)

	// Test 3: Verify the partition with mount point gets fstype from findmnt
	assert.Equal(t, 1, len(devs[0].Children), "Should have 1 child partition")
	child := devs[0].Children[0]
	assert.Equal(t, "/dev/sda1", child.Name)
	assert.Equal(t, "part", child.Type)
	assert.Equal(t, "ext4", child.FSType, "FSType should be populated from findmnt fallback")
	assert.Equal(t, uint64(97563111424), child.FSUsed.Uint64, "FSUsed should be preserved from lsblk")

	// Test 4: Verify findmnt was called (proving the fallback mechanism works)
	assert.Greater(t, findMntCallCount, 0, "findmnt should be called at least once for fallback")

	t.Logf("✓ Successfully handled null fstype scenario with findmnt fallback:")
	t.Logf("  - Found %d block device(s)", len(devs))
	t.Logf("  - Device %s has %d partition(s)", devs[0].Name, len(devs[0].Children))
	t.Logf("  - Partition %s fstype: %s (populated via findmnt fallback)", child.Name, child.FSType)
	t.Logf("  - findmnt was called %d time(s)", findMntCallCount)
}

// TestFilteringWithFstypeFromFindmnt verifies that filtering still works correctly
// when fstype comes from findmnt fallback
func TestFilteringWithFstypeFromFindmnt(t *testing.T) {
	lsblkOutput := `{
   "blockdevices": [
      {
         "name": "/dev/sda",
         "type": "disk",
         "children": [
            {"name": "/dev/sda1", "type": "part", "mountpoint": "/boot", "fstype": null},
            {"name": "/dev/sda2", "type": "part", "mountpoint": "/", "fstype": null},
            {"name": "/dev/sda3", "type": "part", "mountpoint": "/data", "fstype": null}
         ]
      }
   ]
}`

	// Mock lsblk
	originalLsblkExecutor := lsblkCommandExecutor
	lsblkCommandExecutor = func(ctx context.Context, lsblkBin string, flags string) ([]byte, error) {
		return []byte(lsblkOutput), nil
	}
	defer func() { lsblkCommandExecutor = originalLsblkExecutor }()

	// Mock version
	originalGetLsblkVersion := getLsblkBinPathAndVersionFunc
	getLsblkBinPathAndVersionFunc = func(ctx context.Context) (string, string, error) {
		return "/usr/bin/lsblk", "lsblk from util-linux 2.37.2", nil
	}
	defer func() { getLsblkBinPathAndVersionFunc = originalGetLsblkVersion }()

	// Mock findmnt to return different fstypes
	originalFindMntExecutor := findMntExecutor
	findMntExecutor = func(ctx context.Context, target string) (*FindMntOutput, error) {
		switch target {
		case "/boot":
			return &FindMntOutput{Filesystems: []FoundMnt{{Fstype: "vfat"}}}, nil
		case "/":
			return &FindMntOutput{Filesystems: []FoundMnt{{Fstype: "ext4"}}}, nil
		case "/data":
			return &FindMntOutput{Filesystems: []FoundMnt{{Fstype: "xfs"}}}, nil
		}
		return nil, nil
	}
	defer func() { findMntExecutor = originalFindMntExecutor }()

	// Test with fstype filter - should only get ext4 filesystems
	ctx := context.Background()
	devs, err := GetBlockDevicesWithLsblk(ctx, WithFstype(func(fstype string) bool {
		return fstype == "ext4"
	}))
	require.NoError(t, err)

	// Should get the parent device with only the ext4 child
	assert.Equal(t, 1, len(devs))
	assert.Equal(t, 1, len(devs[0].Children), "Should only have 1 child with ext4")
	assert.Equal(t, "/dev/sda2", devs[0].Children[0].Name)
	assert.Equal(t, "ext4", devs[0].Children[0].FSType)
}

// TestRealWorldScenarioNoDevicesFound verifies the original issue is fixed
// This simulates the exact scenario where "no block device found" warning was shown
func TestRealWorldScenarioNoDevicesFound(t *testing.T) {
	// Empty result after filtering (original issue)
	emptyFilteredDevs := make(BlockDevices, 0)

	// Before fix: len(devs) == 0 would trigger warning
	// After fix: Even with null fstypes, devices should be found if they have mount points

	// Simulate the check that was failing
	if len(emptyFilteredDevs) == 0 {
		// This scenario would have triggered a warning before the fix
		t.Log("WARNING: This would have triggered 'no block device found from lsblk command'")
	}

	// Now test with our fix
	lsblkWithNullFstype := `{"blockdevices": [{"name": "/dev/sda", "type": "disk", "fstype": null, "mountpoint": "/mnt"}]}`

	// Mock findmnt to provide fstype
	originalFindMntExecutor := findMntExecutor
	findMntExecutor = func(ctx context.Context, target string) (*FindMntOutput, error) {
		if target == "/mnt" {
			return &FindMntOutput{Filesystems: []FoundMnt{{Fstype: "ext4"}}}, nil
		}
		return nil, nil
	}
	defer func() { findMntExecutor = originalFindMntExecutor }()

	ctx := context.Background()
	devs, err := parseLsblkJSON(ctx, []byte(lsblkWithNullFstype))
	require.NoError(t, err)

	// After fix: Should find the device even with null fstype
	assert.Equal(t, 1, len(devs), "Should find device even with null fstype (fixes warning)")
	assert.Equal(t, "ext4", devs[0].FSType, "FSType should come from findmnt")
}
