package disk

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseLsblkWithFindmntFallback tests the complete lsblk parsing with findmnt fallback.
func TestParseLsblkWithFindmntFallback(t *testing.T) {
	lsblkJSON := `{
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
            },{
               "name": "/dev/sda14",
               "type": "part",
               "size": 4194304,
               "rota": true,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": null,
               "fstype": null,
               "fsused": null,
               "partuuid": null
            },{
               "name": "/dev/sda15",
               "type": "part",
               "size": 111149056,
               "rota": true,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": null,
               "fstype": null,
               "fsused": null,
               "partuuid": null
            }
         ]
      },{
         "name": "/dev/sdb",
         "type": "disk",
         "size": 1099511627776,
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
               "name": "/dev/sdb1",
               "type": "part",
               "size": 1099509530624,
               "rota": true,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": null,
               "fstype": null,
               "fsused": null,
               "partuuid": null
            }
         ]
      },{
         "name": "/dev/sr0",
         "type": "rom",
         "size": 1073741312,
         "rota": true,
         "serial": null,
         "wwn": null,
         "vendor": "Msft    ",
         "model": "Virtual DVD-ROM ",
         "rev": "1.0 ",
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null
      },{
         "name": "/dev/nvme0n1",
         "type": "disk",
         "size": 3840755982336,
         "rota": false,
         "serial": "1cf3ab3ce1db00000001",
         "wwn": null,
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk              ",
         "rev": null,
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null
      },{
         "name": "/dev/nvme3n1",
         "type": "disk",
         "size": 3840755982336,
         "rota": false,
         "serial": "1cf3ab3ce1db00000004",
         "wwn": null,
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk              ",
         "rev": null,
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null
      },{
         "name": "/dev/nvme4n1",
         "type": "disk",
         "size": 3840755982336,
         "rota": false,
         "serial": "1cf3ab3ce1db00000005",
         "wwn": null,
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk              ",
         "rev": null,
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null
      },{
         "name": "/dev/nvme2n1",
         "type": "disk",
         "size": 3840755982336,
         "rota": false,
         "serial": "1cf3ab3ce1db00000003",
         "wwn": null,
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk              ",
         "rev": null,
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null
      },{
         "name": "/dev/nvme1n1",
         "type": "disk",
         "size": 3840755982336,
         "rota": false,
         "serial": "1cf3ab3ce1db00000002",
         "wwn": null,
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk              ",
         "rev": null,
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null
      },{
         "name": "/dev/nvme7n1",
         "type": "disk",
         "size": 3840755982336,
         "rota": false,
         "serial": "1cf3ab3ce1db00000008",
         "wwn": null,
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk              ",
         "rev": null,
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null
      },{
         "name": "/dev/nvme5n1",
         "type": "disk",
         "size": 3840755982336,
         "rota": false,
         "serial": "1cf3ab3ce1db00000006",
         "wwn": null,
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk              ",
         "rev": null,
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null
      },{
         "name": "/dev/nvme6n1",
         "type": "disk",
         "size": 3840755982336,
         "rota": false,
         "serial": "1cf3ab3ce1db00000007",
         "wwn": null,
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk              ",
         "rev": null,
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null
      }
   ]
}`

	// Mock findmnt to return ext4 for the specific mount point
	originalFindMntExecutor := findMntExecutor
	findMntCallCount := 0
	findMntExecutor = func(ctx context.Context, target string) (*FindMntOutput, error) {
		findMntCallCount++
		expectedMount := "/var/lib/kubelet/pods/b76d2533-919d-4fc7-8274-132b7a7b7bf6/volume-subpaths/nvidia-device-plugin-entrypoint/nvidia-device-plugin/0"
		if target == expectedMount {
			return &FindMntOutput{
				Target: target,
				Filesystems: []FoundMnt{
					{
						MountedPoint:         target,
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
		// For unmounted devices, return error (simulating findmnt failure)
		return nil, fmt.Errorf("findmnt: can't find %s in /proc/self/mountinfo", target)
	}
	defer func() { findMntExecutor = originalFindMntExecutor }()

	ctx := context.Background()
	devs, err := parseLsblkJSON(ctx, []byte(lsblkJSON))
	require.NoError(t, err)

	// Verify we got all devices
	assert.Equal(t, 11, len(devs), "Should have 11 top-level devices")

	// Find /dev/sda and verify its children
	var sdaDev *BlockDevice
	for i := range devs {
		if devs[i].Name == "/dev/sda" {
			sdaDev = &devs[i]
			break
		}
	}
	require.NotNil(t, sdaDev, "Should find /dev/sda")
	assert.Equal(t, 3, len(sdaDev.Children), "Should have 3 partitions")

	// Verify that /dev/sda1 got its fstype from findmnt
	var sda1 *BlockDevice
	for i := range sdaDev.Children {
		if sdaDev.Children[i].Name == "/dev/sda1" {
			sda1 = &sdaDev.Children[i]
			break
		}
	}
	require.NotNil(t, sda1, "Should find /dev/sda1")
	assert.Equal(t, "ext4", sda1.FSType, "FSType should be populated from findmnt")
	assert.Equal(t, uint64(97563111424), sda1.FSUsed.Uint64, "FSUsed should be preserved")

	// Verify that findmnt was only called once (caching works)
	assert.Equal(t, 1, findMntCallCount, "findmnt should only be called once due to caching")

	// Verify unmounted partitions have no fstype
	var sda14 *BlockDevice
	for i := range sdaDev.Children {
		if sdaDev.Children[i].Name == "/dev/sda14" {
			sda14 = &sdaDev.Children[i]
			break
		}
	}
	require.NotNil(t, sda14, "Should find /dev/sda14")
	assert.Equal(t, "", sda14.FSType, "Unmounted partition should have empty fstype")
}

// TestGetBlockDevicesWithLsblkMocked tests the full GetBlockDevicesWithLsblk function with mocked lsblk command
func TestGetBlockDevicesWithLsblkMocked(t *testing.T) {
	// Same JSON as above but testing the full flow
	lsblkJSON := `{
   "blockdevices": [
      {
         "name": "/dev/sda",
         "type": "disk",
         "size": 137438953472,
         "rota": true,
         "mountpoint": null,
         "fstype": null,
         "children": [
            {
               "name": "/dev/sda1",
               "type": "part",
               "size": 137322544640,
               "mountpoint": "/var/lib/kubelet",
               "fstype": null,
               "fsused": "97563111424"
            }
         ]
      }
   ]
}`

	// Mock lsblk command execution
	originalLsblkExecutor := lsblkCommandExecutor
	lsblkCommandExecutor = func(ctx context.Context, lsblkBin string, flags string) ([]byte, error) {
		// Return our test JSON regardless of the command
		return []byte(lsblkJSON), nil
	}
	defer func() { lsblkCommandExecutor = originalLsblkExecutor }()

	// Mock lsblk version detection
	originalGetLsblkVersion := getLsblkBinPathAndVersionFunc
	getLsblkBinPathAndVersionFunc = func(ctx context.Context) (string, string, error) {
		// Return a version that supports JSON
		return "/usr/bin/lsblk", "lsblk from util-linux 2.37.2", nil
	}
	defer func() { getLsblkBinPathAndVersionFunc = originalGetLsblkVersion }()

	// Mock findmnt
	originalFindMntExecutor := findMntExecutor
	findMntExecutor = func(ctx context.Context, target string) (*FindMntOutput, error) {
		if target == "/var/lib/kubelet" {
			return &FindMntOutput{
				Filesystems: []FoundMnt{
					{Fstype: "ext4"},
				},
			}, nil
		}
		return nil, fmt.Errorf("not found")
	}
	defer func() { findMntExecutor = originalFindMntExecutor }()

	ctx := context.Background()
	devs, err := GetBlockDevicesWithLsblk(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, len(devs), "Should have 1 device")
	assert.Equal(t, "/dev/sda", devs[0].Name)
	assert.Equal(t, 1, len(devs[0].Children))
	assert.Equal(t, "ext4", devs[0].Children[0].FSType, "Child should have fstype from findmnt")
}

// TestFindmntCachingPerformance verifies that findmnt is not called multiple times for the same mount point
func TestFindmntCachingPerformance(t *testing.T) {
	// JSON with multiple devices having the same mount point
	lsblkJSON := `{
   "blockdevices": [
      {
         "name": "/dev/sda",
         "type": "disk",
         "mountpoint": "/mnt/data",
         "fstype": null,
         "children": [
            {"name": "/dev/sda1", "type": "part", "mountpoint": "/mnt/data", "fstype": null},
            {"name": "/dev/sda2", "type": "part", "mountpoint": "/mnt/data", "fstype": null}
         ]
      },
      {
         "name": "/dev/sdb",
         "type": "disk",
         "mountpoint": "/mnt/data",
         "fstype": null
      }
   ]
}`

	callCount := 0
	originalFindMntExecutor := findMntExecutor
	findMntExecutor = func(ctx context.Context, target string) (*FindMntOutput, error) {
		callCount++
		if target == "/mnt/data" {
			return &FindMntOutput{
				Filesystems: []FoundMnt{{Fstype: "xfs"}},
			}, nil
		}
		return nil, fmt.Errorf("not found")
	}
	defer func() { findMntExecutor = originalFindMntExecutor }()

	ctx := context.Background()
	devs, err := parseLsblkJSON(ctx, []byte(lsblkJSON))
	require.NoError(t, err)

	// Verify all devices got the fstype
	assert.Equal(t, "xfs", devs[0].FSType)
	assert.Equal(t, "xfs", devs[0].Children[0].FSType)
	assert.Equal(t, "xfs", devs[0].Children[1].FSType)
	assert.Equal(t, "xfs", devs[1].FSType)

	// Verify findmnt was only called once due to caching
	assert.Equal(t, 1, callCount, "findmnt should only be called once for the same mount point")
}

// TestNullFstypeWithoutMountpoint verifies devices without mount points are handled correctly
func TestNullFstypeWithoutMountpoint(t *testing.T) {
	lsblkJSON := `{
   "blockdevices": [
      {
         "name": "/dev/nvme0n1",
         "type": "disk",
         "size": 3840755982336,
         "mountpoint": null,
         "fstype": null
      }
   ]
}`

	// Mock findmnt - should not be called
	callCount := 0
	originalFindMntExecutor := findMntExecutor
	findMntExecutor = func(ctx context.Context, target string) (*FindMntOutput, error) {
		callCount++
		return nil, fmt.Errorf("should not be called")
	}
	defer func() { findMntExecutor = originalFindMntExecutor }()

	ctx := context.Background()
	devs, err := parseLsblkJSON(ctx, []byte(lsblkJSON))
	require.NoError(t, err)

	assert.Equal(t, 1, len(devs))
	assert.Equal(t, "", devs[0].FSType, "Unmounted device should have empty fstype")
	assert.Equal(t, 0, callCount, "findmnt should not be called for unmounted devices")
}

// TestExistingFstypeNotOverwritten verifies that existing fstype values are not overwritten
func TestExistingFstypeNotOverwritten(t *testing.T) {
	lsblkJSON := `{
   "blockdevices": [
      {
         "name": "/dev/sda1",
         "type": "part",
         "mountpoint": "/boot",
         "fstype": "vfat"
      }
   ]
}`

	// Mock findmnt - should not be called
	callCount := 0
	originalFindMntExecutor := findMntExecutor
	findMntExecutor = func(ctx context.Context, target string) (*FindMntOutput, error) {
		callCount++
		return &FindMntOutput{
			Filesystems: []FoundMnt{{Fstype: "ext4"}}, // Different fstype
		}, nil
	}
	defer func() { findMntExecutor = originalFindMntExecutor }()

	ctx := context.Background()
	devs, err := parseLsblkJSON(ctx, []byte(lsblkJSON))
	require.NoError(t, err)

	assert.Equal(t, 1, len(devs))
	assert.Equal(t, "vfat", devs[0].FSType, "Existing fstype should not be overwritten")
	assert.Equal(t, 0, callCount, "findmnt should not be called when fstype exists")
}

// TestLsblkJSONUnmarshalError tests handling of invalid JSON
func TestLsblkJSONUnmarshalError(t *testing.T) {
	invalidJSON := `{invalid json}`

	ctx := context.Background()
	_, err := parseLsblkJSON(ctx, []byte(invalidJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal lsblk output")
}

// TestEmptyLsblkOutput tests handling of empty output
func TestEmptyLsblkOutput(t *testing.T) {
	ctx := context.Background()
	_, err := parseLsblkJSON(ctx, []byte(""))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty input provided")
}

// TestMissingBlockdevicesKey tests handling of JSON without blockdevices key
func TestMissingBlockdevicesKey(t *testing.T) {
	jsonWithoutKey := `{"other": []}`

	ctx := context.Background()
	_, err := parseLsblkJSON(ctx, []byte(jsonWithoutKey))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing \"blockdevices\" key")
}
