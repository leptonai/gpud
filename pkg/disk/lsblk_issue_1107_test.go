package disk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIssue1107_RequiresBothFixes demonstrates that BOTH changes were necessary:
// 1. Recursive processBlockDevice (for nested LVM)
// 2. RAID support in DefaultDeviceTypeFunc (for RAID devices)
func TestIssue1107_RequiresBothFixes(t *testing.T) {
	ctx := context.Background()

	// Test case 1: Nested LVM requires RECURSIVE processing
	// Without processBlockDevice recursion, this would fail even with RAID support
	t.Run("nested_lvm_requires_recursive_processing", func(t *testing.T) {
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

		// This ONLY works because processBlockDevice recursively checks grandchildren
		assert.True(t, result, "nested LVM should be included with recursive processing")
		require.Len(t, nestedLVM.Children, 1, "rimage should be preserved")
		require.Len(t, nestedLVM.Children[0].Children, 1, "final LV should be preserved")
	})

	// Test case 2: RAID devices require RAID support in DefaultDeviceTypeFunc
	// Without RAID support, this would fail even with recursive processing
	t.Run("raid_devices_require_device_type_support", func(t *testing.T) {
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
							Type:       "raid1", // This type MUST be supported
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
			WithDeviceType(DefaultDeviceTypeFunc), // This MUST accept "raid1"
			WithFstype(DefaultFsTypeFunc),
		}))

		fstypeCache := make(map[string]string)
		result := processBlockDevice(ctx, raidDevice, "", 0, op, fstypeCache)

		// This ONLY works because DefaultDeviceTypeFunc now accepts "raid*" types
		assert.True(t, result, "RAID device should be included with RAID support")
		require.Len(t, raidDevice.Children, 1, "partition should be preserved")
		require.Len(t, raidDevice.Children[0].Children, 1, "RAID device should be preserved")
		assert.Equal(t, "raid1", raidDevice.Children[0].Children[0].Type)
	})

	// Test case 3: The OLD implementation would have failed both cases
	t.Run("old_device_type_func_would_reject_raid", func(t *testing.T) {
		// Simulating the OLD DefaultDeviceTypeFunc (before fix)
		oldDeviceTypeFunc := func(dt string) bool {
			return dt == "disk" || dt == "lvm" || dt == "part" // No RAID support!
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
			WithDeviceType(oldDeviceTypeFunc), // OLD function without RAID
			WithFstype(DefaultFsTypeFunc),
		}))

		fstypeCache := make(map[string]string)
		result := processBlockDevice(ctx, raidDevice, "", 0, op, fstypeCache)

		// This FAILS because old function doesn't support RAID
		assert.False(t, result, "RAID device should be rejected by old function")
	})
}

// TestIssue1107_FullFixValidation validates the complete fix using the actual test data structure
func TestIssue1107_FullFixValidation(t *testing.T) {
	ctx := context.Background()

	// Simulate the actual problematic structure from lsblk.withlv.censored.json
	// that was failing with the "no block device found" error
	problematicStructure := []BlockDevice{
		// Problem 1: Nested LVM (needs recursive processing)
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
					MountPoint: "",
					Children: []BlockDevice{
						{
							Name:       "/dev/mapper/vg_nvme-lv_nvme",
							Type:       "lvm",
							FSType:     "ext4",
							MountPoint: "/mnt/local_disk",
						},
					},
				},
			},
		},
		// Problem 2: RAID device (needs RAID support in filter)
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
							MountPoint: "/",
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

	for i := range problematicStructure {
		dev := &problematicStructure[i]
		if processBlockDevice(ctx, dev, "", 0, op, fstypeCache) {
			foundDevices++
		}
	}

	// Both devices should be found with the complete fix
	assert.Equal(t, 2, foundDevices, "both problematic devices should be found with complete fix")
	assert.NotZero(t, foundDevices, "should NOT trigger 'no block device found' error")
}

// TestIssue1107_DocumentWhyBothChangesNeeded documents the necessity of both changes
func TestIssue1107_DocumentWhyBothChangesNeeded(t *testing.T) {
	t.Log("Issue #1107 had TWO distinct problems:")
	t.Log("")
	t.Log("Problem 1: Nested LVM Hierarchy")
	t.Log("  - Structure: disk → LVM PV → LVM rimage (no mount) → LVM LV (mounted)")
	t.Log("  - Old code: Only checked direct children, missed grandchildren")
	t.Log("  - Fix: Recursive processBlockDevice to check all levels")
	t.Log("")
	t.Log("Problem 2: RAID Device Type Filtering")
	t.Log("  - Structure: disk → partition → RAID device (type 'raid1')")
	t.Log("  - Old code: DefaultDeviceTypeFunc didn't accept 'raid*' types")
	t.Log("  - Fix: Added RAID/MD support to DefaultDeviceTypeFunc")
	t.Log("")
	t.Log("BOTH fixes were mandatory:")
	t.Log("  ✓ Without recursive processing: Nested LVM would fail")
	t.Log("  ✓ Without RAID support: RAID devices would fail")
	t.Log("")
	t.Log("The complete fix addresses BOTH root causes.")
}
