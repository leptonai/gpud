package disk

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseLsblkJSONWithXFSAndVFAT tests that xfs and vfat filesystems are correctly
// detected and included. This test uses real-world data from a system with:
//   - /dev/nvme4n1p1: vfat filesystem mounted at /boot/efi (EFI System Partition)
//   - /dev/nvme4n1p2: xfs filesystem mounted at / (root partition)
//   - /dev/nvme0n1: LVM2_member with ext4 child mounted at /lepton-data-disk
//
// This test was added to verify the fix for the "no block device found" warning
// that occurred when systems used xfs (common in RHEL/CentOS) or vfat (required for EFI).
//
// See: DefaultFsTypeFunc in options.go for supported filesystem types.
func TestParseLsblkJSONWithXFSAndVFAT(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/lsblk.xfs_vfat.json")
	require.NoError(t, err)

	ctx := context.Background()

	// Parse with default filters - this should NOT filter out xfs/vfat devices
	devs, err := parseLsblkJSON(
		ctx,
		data,
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)

	// Should find devices (not empty - this was the original bug)
	require.NotEmpty(t, devs, "should find block devices with xfs/vfat filesystems")

	// Build a map for easier lookup
	devMap := make(map[string]*BlockDevice)
	for i := range devs {
		devMap[devs[i].Name] = &devs[i]
	}

	// Test 1: Verify nvme4n1 is present (has xfs and vfat children)
	nvme4n1, ok := devMap["/dev/nvme4n1"]
	require.True(t, ok, "nvme4n1 should be present because it has children with valid mountpoints")
	require.NotEmpty(t, nvme4n1.Children, "nvme4n1 should have children")

	// Test 2: Verify xfs root partition is included
	foundXFSRoot := false
	for _, child := range nvme4n1.Children {
		if child.Name == "/dev/nvme4n1p2" {
			foundXFSRoot = true
			assert.Equal(t, "xfs", child.FSType, "root partition should have xfs filesystem")
			assert.Equal(t, "/", child.MountPoint, "root partition should be mounted at /")
			assert.Equal(t, "part", child.Type)
		}
	}
	assert.True(t, foundXFSRoot, "xfs root partition (/dev/nvme4n1p2) should be included")

	// Test 3: Verify vfat EFI partition is included
	foundVFATEFI := false
	for _, child := range nvme4n1.Children {
		if child.Name == "/dev/nvme4n1p1" {
			foundVFATEFI = true
			assert.Equal(t, "vfat", child.FSType, "EFI partition should have vfat filesystem")
			assert.Equal(t, "/boot/efi", child.MountPoint, "EFI partition should be mounted at /boot/efi")
			assert.Equal(t, "part", child.Type)
		}
	}
	assert.True(t, foundVFATEFI, "vfat EFI partition (/dev/nvme4n1p1) should be included")

	// Test 4: Verify LVM with ext4 is also included
	nvme0n1, ok := devMap["/dev/nvme0n1"]
	require.True(t, ok, "nvme0n1 should be present (LVM2_member)")
	assert.Equal(t, "LVM2_member", nvme0n1.FSType)
	require.NotEmpty(t, nvme0n1.Children, "nvme0n1 should have LVM children")

	foundLVM := false
	for _, child := range nvme0n1.Children {
		if child.Name == "/dev/mapper/lepton_vg-lepton_lv" {
			foundLVM = true
			assert.Equal(t, "lvm", child.Type)
			assert.Equal(t, "ext4", child.FSType)
			assert.Equal(t, "/lepton-data-disk", child.MountPoint)
		}
	}
	assert.True(t, foundLVM, "LVM logical volume should be included")

	// Test 5: Verify loop devices are filtered out (type "loop" not in DefaultDeviceTypeFunc)
	for _, dev := range devs {
		assert.NotEqual(t, "loop", dev.Type, "loop devices should be filtered out by DefaultDeviceTypeFunc")
	}

	// Test 6: Verify sda (empty disk with no mountpoint) is filtered out
	_, hasSDA := devMap["/dev/sda"]
	assert.False(t, hasSDA, "sda should be filtered out (no mountpoint and no children with mountpoints)")
}

// TestDefaultFsTypeFuncIncludesXFSAndVFAT verifies that xfs and vfat are in DefaultFsTypeFunc.
// These filesystem types are widely used:
//   - xfs: Default filesystem for RHEL 7+, CentOS 7+, Rocky Linux, AlmaLinux, Fedora Server
//   - vfat: Required for EFI System Partitions (ESP) per UEFI specification
func TestDefaultFsTypeFuncIncludesXFSAndVFAT(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		fstype   string
		expected bool
		desc     string
	}{
		// Should be included
		{"", true, "empty string (unformatted parent disks)"},
		{"ext4", true, "ext4 (most common Linux filesystem)"},
		{"xfs", true, "xfs (default for RHEL/CentOS/Rocky Linux)"},
		{"vfat", true, "vfat (required for EFI System Partitions)"},
		{"LVM2_member", true, "LVM2_member (LVM physical volumes)"},
		{"linux_raid_member", true, "linux_raid_member (software RAID)"},
		{"raid0", true, "raid0 (RAID-0 devices)"},
		{"nfs", true, "nfs (Network File System)"},
		{"nfs4", true, "nfs4 (NFS version 4)"},

		// Should NOT be included
		{"squashfs", false, "squashfs (snap/loop mounts)"},
		{"tmpfs", false, "tmpfs (temporary filesystem)"},
		{"devtmpfs", false, "devtmpfs (device filesystem)"},
		{"btrfs", false, "btrfs (not included by default)"},
		{"ntfs", false, "ntfs (Windows filesystem)"},
	}

	for _, tc := range testCases {
		t.Run(tc.fstype, func(t *testing.T) {
			result := DefaultFsTypeFunc(tc.fstype)
			assert.Equal(t, tc.expected, result, "DefaultFsTypeFunc(%q) - %s", tc.fstype, tc.desc)
		})
	}
}
