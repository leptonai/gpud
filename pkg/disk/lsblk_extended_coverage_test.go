package disk

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetBlockDevicesWithLsblkErrors tests error handling in GetBlockDevicesWithLsblk
func TestGetBlockDevicesWithLsblkErrors(t *testing.T) {
	t.Run("lsblk command execution error", func(t *testing.T) {
		ctx := context.Background()
		_, err := getBlockDevicesWithLsblk(ctx, getBlockDevicesDeps{
			getLsblkBinPathAndVersion: func(context.Context) (string, string, error) {
				return "/usr/bin/lsblk", "lsblk from util-linux 2.37.2", nil
			},
			executeLsblkCommand: func(context.Context, string, string) ([]byte, error) {
				return nil, fmt.Errorf("lsblk command failed: permission denied")
			},
			findMnt: FindMnt,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})

	t.Run("version detection error", func(t *testing.T) {
		ctx := context.Background()
		_, err := getBlockDevicesWithLsblk(ctx, getBlockDevicesDeps{
			getLsblkBinPathAndVersion: func(context.Context) (string, string, error) {
				return "", "", fmt.Errorf("lsblk not found in PATH")
			},
			executeLsblkCommand: executeLsblkCommand,
			findMnt:             FindMnt,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "lsblk not found")
	})

	t.Run("invalid JSON parsing", func(t *testing.T) {
		ctx := context.Background()
		_, err := getBlockDevicesWithLsblk(ctx, getBlockDevicesDeps{
			getLsblkBinPathAndVersion: func(context.Context) (string, string, error) {
				return "/usr/bin/lsblk", "lsblk from util-linux 2.37.2", nil
			},
			executeLsblkCommand: func(context.Context, string, string) ([]byte, error) {
				return []byte(`{"blockdevices": [invalid json}`), nil
			},
			findMnt: FindMnt,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal")
	})
}

// TestParseLsblkPairsFormat tests parsing of older lsblk --pairs format
func TestParseLsblkPairsFormat(t *testing.T) {
	// Test data in pairs format (older lsblk versions)
	pairsData := `NAME="/dev/sda" TYPE="disk" SIZE="137438953472" ROTA="1" SERIAL="" WWN="" VENDOR="Msft    " MODEL="Virtual Disk    " REV="1.0 " MOUNTPOINT="" FSTYPE="" FSUSED="" PARTUUID="" PKNAME=""
NAME="/dev/sda1" TYPE="part" SIZE="137322544640" ROTA="1" SERIAL="" WWN="" VENDOR="" MODEL="" REV="" MOUNTPOINT="/boot" FSTYPE="ext4" FSUSED="1073741824" PARTUUID="" PKNAME="/dev/sda"`

	findMnt := func(ctx context.Context, target string) (*FindMntOutput, error) {
		if target == "/boot" {
			return &FindMntOutput{
				Filesystems: []FoundMnt{{Fstype: "ext4"}},
			}, nil
		}
		return nil, fmt.Errorf("not found")
	}

	ctx := context.Background()
	devs, err := parseLsblkPairsWithFindMnt(ctx, []byte(pairsData), findMnt)
	require.NoError(t, err)

	assert.Equal(t, 1, len(devs), "Should have 1 top-level device")
	assert.Equal(t, "/dev/sda", devs[0].Name)
	assert.Equal(t, "disk", devs[0].Type)
	assert.Equal(t, uint64(137438953472), devs[0].Size.Uint64)

	assert.Equal(t, 1, len(devs[0].Children), "Should have 1 child")
	assert.Equal(t, "/dev/sda1", devs[0].Children[0].Name)
	assert.Equal(t, "ext4", devs[0].Children[0].FSType)
}

// TestDecideLsblkFlagVersions tests version detection for different lsblk versions
func TestDecideLsblkFlagVersions(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expectJSON  bool
		expectError bool
	}{
		{
			name:        "modern version with JSON support",
			version:     "lsblk from util-linux 2.37.2",
			expectJSON:  true,
			expectError: false,
		},
		{
			name:        "older version without JSON support",
			version:     "lsblk from util-linux 2.25.2",
			expectJSON:  false,
			expectError: false,
		},
		{
			name:        "version 2.27 exact boundary",
			version:     "lsblk from util-linux 2.27",
			expectJSON:  false,
			expectError: false,
		},
		{
			name:        "version 2.37 exact JSON support",
			version:     "lsblk from util-linux 2.37",
			expectJSON:  true,
			expectError: false,
		},
		{
			name:        "invalid version format",
			version:     "invalid version string",
			expectJSON:  false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			flags, _, err := decideLsblkFlag(ctx, tt.version)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectJSON {
					assert.Contains(t, flags, "--json")
				} else {
					assert.Contains(t, flags, "--pairs")
				}
			}
		})
	}
}

// TestFillFstypeFromFindmntEdgeCases tests edge cases in fstype fallback
func TestFillFstypeFromFindmntEdgeCases(t *testing.T) {
	t.Run("device with existing fstype not overwritten", func(t *testing.T) {
		callCount := 0
		findMnt := func(ctx context.Context, target string) (*FindMntOutput, error) {
			callCount++
			return &FindMntOutput{
				Filesystems: []FoundMnt{{Fstype: "xfs"}},
			}, nil
		}

		ctx := context.Background()
		cache := make(map[string]string)
		dev := &BlockDevice{
			Name:       "/dev/sda1",
			MountPoint: "/data",
			FSType:     "ext4", // Already has fstype
		}

		fillFstypeFromFindmntWithFindMnt(ctx, dev, cache, findMnt)

		assert.Equal(t, "ext4", dev.FSType, "Existing fstype should not be changed")
		assert.Equal(t, 0, callCount, "findmnt should not be called")
	})

	t.Run("device without mount point", func(t *testing.T) {
		callCount := 0
		findMnt := func(ctx context.Context, target string) (*FindMntOutput, error) {
			callCount++
			return nil, fmt.Errorf("should not be called")
		}

		ctx := context.Background()
		cache := make(map[string]string)
		dev := &BlockDevice{
			Name:       "/dev/sda1",
			MountPoint: "", // No mount point
			FSType:     "",
		}

		fillFstypeFromFindmntWithFindMnt(ctx, dev, cache, findMnt)

		assert.Equal(t, "", dev.FSType, "FSType should remain empty")
		assert.Equal(t, 0, callCount, "findmnt should not be called")
	})

	t.Run("cache hit prevents redundant calls", func(t *testing.T) {
		callCount := 0
		findMnt := func(ctx context.Context, target string) (*FindMntOutput, error) {
			callCount++
			return &FindMntOutput{
				Filesystems: []FoundMnt{{Fstype: "btrfs"}},
			}, nil
		}

		ctx := context.Background()
		cache := make(map[string]string)
		cache["/mnt/data"] = "cached-fs" // Pre-populate cache

		dev := &BlockDevice{
			Name:       "/dev/sda1",
			MountPoint: "/mnt/data",
			FSType:     "",
		}

		fillFstypeFromFindmntWithFindMnt(ctx, dev, cache, findMnt)

		assert.Equal(t, "cached-fs", dev.FSType, "Should use cached value")
		assert.Equal(t, 0, callCount, "findmnt should not be called due to cache hit")
	})
}

// TestGetFstypeFromFindmntErrorHandling tests error scenarios in getFstypeFromFindmnt
func TestGetFstypeFromFindmntErrorHandling(t *testing.T) {
	t.Run("findmnt returns error", func(t *testing.T) {
		findMnt := func(ctx context.Context, target string) (*FindMntOutput, error) {
			return nil, errors.New("findmnt: permission denied")
		}

		ctx := context.Background()
		fstype := getFstypeFromFindmntWithFindMnt(ctx, "/mnt/test", findMnt)
		assert.Equal(t, "", fstype, "Should return empty string on error")
	})

	t.Run("findmnt returns empty filesystems", func(t *testing.T) {
		findMnt := func(ctx context.Context, target string) (*FindMntOutput, error) {
			return &FindMntOutput{
				Filesystems: []FoundMnt{}, // Empty slice
			}, nil
		}

		ctx := context.Background()
		fstype := getFstypeFromFindmntWithFindMnt(ctx, "/mnt/test", findMnt)
		assert.Equal(t, "", fstype, "Should return empty string for no filesystems")
	})

	t.Run("findmnt returns empty fstype", func(t *testing.T) {
		findMnt := func(ctx context.Context, target string) (*FindMntOutput, error) {
			return &FindMntOutput{
				Filesystems: []FoundMnt{{Fstype: ""}}, // Empty fstype
			}, nil
		}

		ctx := context.Background()
		fstype := getFstypeFromFindmntWithFindMnt(ctx, "/mnt/test", findMnt)
		assert.Equal(t, "", fstype, "Should return empty string for empty fstype")
	})
}

// TestComplexDeviceHierarchy tests handling of complex device hierarchies
func TestComplexDeviceHierarchy(t *testing.T) {
	// Complex hierarchy with LVM, RAID, and nested partitions
	complexJSON := `{
   "blockdevices": [
      {
         "name": "/dev/sda",
         "type": "disk",
         "children": [
            {"name": "/dev/sda1", "type": "part", "mountpoint": "/boot", "fstype": null},
            {"name": "/dev/sda2", "type": "part", "mountpoint": null, "fstype": null}
         ]
      },
      {
         "name": "/dev/nvme0n1",
         "type": "disk",
         "children": [
            {"name": "/dev/nvme0n1p1", "type": "part", "mountpoint": "/home", "fstype": null}
         ]
      }
   ]
}`

	findMntCalls := make(map[string]int)
	findMnt := func(ctx context.Context, target string) (*FindMntOutput, error) {
		findMntCalls[target]++
		switch target {
		case "/boot":
			return &FindMntOutput{Filesystems: []FoundMnt{{Fstype: "ext2"}}}, nil
		case "/home":
			return &FindMntOutput{Filesystems: []FoundMnt{{Fstype: "btrfs"}}}, nil
		}
		return nil, fmt.Errorf("not mounted")
	}

	ctx := context.Background()
	devs, err := parseLsblkJSONWithFindMnt(ctx, []byte(complexJSON), findMnt)
	require.NoError(t, err)

	// Verify hierarchy
	assert.Equal(t, 2, len(devs), "Should have 2 top-level devices")

	// Find devices by name (order may vary due to sorting)
	var sda, nvme *BlockDevice
	for i := range devs {
		switch devs[i].Name {
		case "/dev/sda":
			sda = &devs[i]
		case "/dev/nvme0n1":
			nvme = &devs[i]
		}
	}

	// Check /dev/sda hierarchy
	require.NotNil(t, sda, "Should find /dev/sda")
	assert.Equal(t, 2, len(sda.Children))
	assert.Equal(t, "ext2", sda.Children[0].FSType, "/boot should have ext2")

	// Check NVMe device
	require.NotNil(t, nvme, "Should find /dev/nvme0n1")
	assert.Equal(t, 1, len(nvme.Children))
	assert.Equal(t, "btrfs", nvme.Children[0].FSType, "/home should have btrfs")

	// Verify caching worked - each mount point called only once
	for mountPoint, count := range findMntCalls {
		assert.Equal(t, 1, count, "Mount point %s should be queried only once", mountPoint)
	}
}

// TestParseLsblkJSONWithOptions tests parsing with various filter options
func TestParseLsblkJSONWithOptions(t *testing.T) {
	testJSON := `{
   "blockdevices": [
      {
         "name": "/dev/sda",
         "type": "disk",
         "children": [
            {"name": "/dev/sda1", "type": "part", "mountpoint": "/boot", "fstype": "vfat"},
            {"name": "/dev/sda2", "type": "part", "mountpoint": "/", "fstype": "ext4"},
            {"name": "/dev/sda3", "type": "part", "mountpoint": "/home", "fstype": "ext4"}
         ]
      },
      {
         "name": "/dev/sdb",
         "type": "disk",
         "fstype": "ntfs",
         "mountpoint": "/mnt/windows"
      }
   ]
}`

	ctx := context.Background()

	t.Run("filter by fstype", func(t *testing.T) {
		devs, err := parseLsblkJSON(ctx, []byte(testJSON), WithFstype(func(fstype string) bool {
			return fstype == "ext4"
		}))
		require.NoError(t, err)

		// Should get sda with only ext4 children
		assert.Equal(t, 1, len(devs))
		assert.Equal(t, 2, len(devs[0].Children), "Should have 2 ext4 partitions")
	})

	t.Run("filter by device type", func(t *testing.T) {
		devs, err := parseLsblkJSON(ctx, []byte(testJSON), WithDeviceType(func(deviceType string) bool {
			return deviceType == "disk"
		}))
		require.NoError(t, err)

		// Should get only disk devices, no partitions
		assert.Equal(t, 2, len(devs))
		assert.Equal(t, 0, len(devs[0].Children), "Should have no children when filtering for disks only")
	})

	t.Run("filter by mount point", func(t *testing.T) {
		devs, err := parseLsblkJSON(ctx, []byte(testJSON), WithMountPoint(func(mountPoint string) bool {
			return mountPoint == "/" || mountPoint == "/home"
		}))
		require.NoError(t, err)

		// Should get sda with only matching mount points
		assert.Equal(t, 1, len(devs))
		assert.Equal(t, 2, len(devs[0].Children), "Should have 2 matching mount points")
	})

	t.Run("combined filters", func(t *testing.T) {
		devs, err := parseLsblkJSON(ctx, []byte(testJSON),
			WithFstype(func(fstype string) bool {
				return fstype == "ext4"
			}),
			WithMountPoint(func(mountPoint string) bool {
				return mountPoint == "/"
			}))
		require.NoError(t, err)

		// Should get only root partition
		assert.Equal(t, 1, len(devs))
		assert.Equal(t, 1, len(devs[0].Children), "Should have only root partition")
		assert.Equal(t, "/dev/sda2", devs[0].Children[0].Name)
	})
}

// TestEmptyAndNullScenarios tests various empty and null scenarios
func TestEmptyAndNullScenarios(t *testing.T) {
	t.Run("empty blockdevices array", func(t *testing.T) {
		emptyJSON := `{"blockdevices": []}`
		ctx := context.Background()
		devs, err := parseLsblkJSON(ctx, []byte(emptyJSON))
		require.NoError(t, err)
		assert.Equal(t, 0, len(devs), "Should handle empty devices list")
	})

	t.Run("all null values", func(t *testing.T) {
		nullJSON := `{
   "blockdevices": [{
      "name": "/dev/sda",
      "type": "disk",
      "size": null,
      "rota": false,
      "serial": null,
      "wwn": null,
      "vendor": null,
      "model": null,
      "rev": null,
      "mountpoint": null,
      "fstype": null,
      "fsused": null,
      "partuuid": null
   }]
}`
		ctx := context.Background()
		devs, err := parseLsblkJSON(ctx, []byte(nullJSON))
		require.NoError(t, err)
		assert.Equal(t, 1, len(devs))
		assert.Equal(t, "/dev/sda", devs[0].Name)
		assert.Equal(t, uint64(0), devs[0].Size.Uint64)
		assert.Equal(t, false, devs[0].Rota.Bool)
	})
}
