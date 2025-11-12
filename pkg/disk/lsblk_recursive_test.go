package disk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file contains tests for recursive block device processing and device type filtering.
//
// Background (Issue #1107):
// Complex block device hierarchies require TWO key capabilities:
//
// Capability 1: Recursive Hierarchy Traversal
//   - Scenario: disk → LVM PV → LVM rimage (no mount) → LVM LV (mounted)
//   - Challenge: Mountpoints may exist at any depth in the hierarchy
//   - Solution: Recursive processBlockDevice checks all descendant levels
//
// Capability 2: Comprehensive Device Type Support
//   - Scenario: disk → partition → RAID device (type 'raid1', 'raid5', etc.)
//   - Challenge: RAID/MD device types must be accepted by the filter
//   - Solution: DefaultDeviceTypeFunc includes RAID/MD type patterns
//
// Both capabilities are essential:
//   ✓ Without recursion: Deeply nested devices are missed
//   ✓ Without RAID support: RAID devices are incorrectly filtered out
//
// Together, these capabilities handle real-world storage configurations that were
// previously causing "no block device found" errors in production.

// TestRecursiveProcessing_NestedAndRAID demonstrates that BOTH recursive processing
// and RAID device type support are necessary for handling complex block device hierarchies.
func TestRecursiveProcessing_NestedAndRAID(t *testing.T) {
	ctx := context.Background()

	// Test case 1: Nested LVM requires RECURSIVE processing
	// Without processBlockDevice recursion, deeply nested devices would be missed
	// even with proper device type support
	t.Run("nested_lvm_requires_recursive_processing", func(t *testing.T) {
		// Structure: disk (LVM2_member) → rimage (no mount) → LV (mounted)
		// The mountpoint is at the grandchild level, requiring recursive checks
		nestedLVM := &BlockDevice{
			Name:       "/dev/nvme0n1",
			Type:       "disk",
			FSType:     "LVM2_member",
			MountPoint: "",
			Children: []BlockDevice{
				{
					Name:       "/dev/mapper/vg-rimage",
					Type:       "lvm",
					FSType:     "",
					MountPoint: "", // No mountpoint at this level
					Children: []BlockDevice{
						{
							Name:       "/dev/mapper/vg-lv",
							Type:       "lvm",
							FSType:     "ext4",
							MountPoint: "/mnt/local_disk", // Mountpoint at grandchild level
						},
					},
				},
			},
		}

		op := &Op{}
		require.NoError(t, op.applyOpts([]OpOption{
			WithMountPoint(DefaultMountPointFunc),
			WithDeviceType(DefaultDeviceTypeFunc),
			WithFstype(DefaultFsTypeFunc),
		}))

		fstypeCache := make(map[string]string)
		result := processBlockDevice(ctx, nestedLVM, "", 0, op, fstypeCache)

		// Recursive processing allows detection of mountpoints at any depth
		assert.True(t, result, "nested LVM should be included with recursive processing")
		require.Len(t, nestedLVM.Children, 1, "rimage should be preserved")
		require.Len(t, nestedLVM.Children[0].Children, 1, "final LV should be preserved")
	})

	// Test case 2: RAID devices require proper device type support
	// Without RAID type acceptance in the filter, RAID devices would be rejected
	// even with recursive processing in place
	t.Run("raid_devices_require_device_type_support", func(t *testing.T) {
		// Structure: disk → partition (linux_raid_member) → RAID device (mounted)
		// The RAID device type (raid1, raid5, etc.) must be accepted by the filter
		raidDevice := &BlockDevice{
			Name:       "/dev/sda",
			Type:       "disk",
			FSType:     "",
			MountPoint: "",
			Children: []BlockDevice{
				{
					Name:       "/dev/sda2",
					Type:       "part",
					FSType:     "linux_raid_member",
					MountPoint: "",
					Children: []BlockDevice{
						{
							Name:       "/dev/md127",
							Type:       "raid1", // Must be accepted by device type filter
							FSType:     "ext4",
							MountPoint: "/",
						},
					},
				},
			},
		}

		op := &Op{}
		require.NoError(t, op.applyOpts([]OpOption{
			WithMountPoint(DefaultMountPointFunc),
			WithDeviceType(DefaultDeviceTypeFunc), // Must accept "raid*" types
			WithFstype(DefaultFsTypeFunc),
		}))

		fstypeCache := make(map[string]string)
		result := processBlockDevice(ctx, raidDevice, "", 0, op, fstypeCache)

		// RAID support in DefaultDeviceTypeFunc enables RAID device detection
		assert.True(t, result, "RAID device should be included with RAID support")
		require.Len(t, raidDevice.Children, 1, "partition should be preserved")
		require.Len(t, raidDevice.Children[0].Children, 1, "RAID device should be preserved")
		assert.Equal(t, "raid1", raidDevice.Children[0].Children[0].Type)
	})

	// Test case 3: Validate that the filter properly rejects unsupported device types
	t.Run("device_type_filter_validation", func(t *testing.T) {
		// Simulating a device type filter without RAID support
		// to demonstrate the necessity of comprehensive type support
		limitedDeviceTypeFunc := func(dt string) bool {
			return dt == "disk" || dt == "lvm" || dt == "part" // No RAID support
		}

		raidDevice := &BlockDevice{
			Name:       "/dev/md127",
			Type:       "raid1",
			FSType:     "ext4",
			MountPoint: "/",
		}

		op := &Op{}
		require.NoError(t, op.applyOpts([]OpOption{
			WithMountPoint(DefaultMountPointFunc),
			WithDeviceType(limitedDeviceTypeFunc), // Limited filter without RAID
			WithFstype(DefaultFsTypeFunc),
		}))

		fstypeCache := make(map[string]string)
		result := processBlockDevice(ctx, raidDevice, "", 0, op, fstypeCache)

		// Without RAID support, the device is correctly rejected by the filter
		assert.False(t, result, "RAID device should be rejected by limited filter")
	})
}

// TestRecursiveProcessing_ComplexHierarchies validates recursive processing with
// real-world complex device structures that combine multiple layering technologies
// (e.g., structures similar to those found in production that triggered issue #1107).
func TestRecursiveProcessing_ComplexHierarchies(t *testing.T) {
	ctx := context.Background()

	// Simulate complex structures combining multiple storage technologies:
	// 1. Multi-level LVM with rimage intermediaries
	// 2. RAID arrays built on partitions
	problematicStructure := []BlockDevice{
		// Case 1: Multi-level LVM hierarchy (requires recursive descent)
		{
			Name:       "/dev/nvme2n1",
			Type:       "disk",
			FSType:     "LVM2_member",
			MountPoint: "",
			Children: []BlockDevice{
				{
					Name:       "/dev/mapper/vg_nvme-lv_nvme_rimage_1",
					Type:       "lvm",
					FSType:     "",
					MountPoint: "", // Intermediate layer without mountpoint
					Children: []BlockDevice{
						{
							Name:       "/dev/mapper/vg_nvme-lv_nvme",
							Type:       "lvm",
							FSType:     "ext4",
							MountPoint: "/mnt/local_disk", // Final mounted volume
						},
					},
				},
			},
		},
		// Case 2: RAID device hierarchy (requires RAID type support)
		{
			Name:       "/dev/sda",
			Type:       "disk",
			FSType:     "",
			MountPoint: "",
			Children: []BlockDevice{
				{
					Name:       "/dev/sda2",
					Type:       "part",
					FSType:     "linux_raid_member",
					MountPoint: "",
					Children: []BlockDevice{
						{
							Name:       "/dev/md127",
							Type:       "raid1",
							FSType:     "ext4",
							MountPoint: "/", // RAID device with root mount
						},
					},
				},
			},
		},
	}

	op := &Op{}
	require.NoError(t, op.applyOpts([]OpOption{
		WithMountPoint(DefaultMountPointFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithFstype(DefaultFsTypeFunc),
	}))

	fstypeCache := make(map[string]string)
	foundDevices := 0

	// Process all devices and count those successfully detected
	for i := range problematicStructure {
		dev := &problematicStructure[i]
		if processBlockDevice(ctx, dev, "", 0, op, fstypeCache) {
			foundDevices++
		}
	}

	// Both complex hierarchies should be successfully processed
	assert.Equal(t, 2, foundDevices, "both complex device hierarchies should be detected")
	assert.NotZero(t, foundDevices, "should NOT trigger 'no block device found' error")
}
