package disk

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLsblkJSONNestedMountPoints(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("testdata", "lsblk.withlv.censored.json"))
	require.NoError(t, err)

	devs, err := parseLsblkJSON(
		ctx,
		data,
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)
	require.NotEmpty(t, devs, "expected block devices to be discovered")

	var nvme0 *BlockDevice
	for i := range devs {
		if devs[i].Name == "/dev/nvme0n1" {
			nvme0 = &devs[i]
			break
		}
	}
	require.NotNil(t, nvme0, "expected /dev/nvme0n1 to be present")
	require.NotEmpty(t, nvme0.Children, "expected /dev/nvme0n1 to include logical volume children")

	var lv *BlockDevice
	for i := range nvme0.Children {
		if nvme0.Children[i].Name == "/dev/mapper/vg_nvme-lv_nvme_rimage_0" {
			lv = &nvme0.Children[i]
			break
		}
	}
	require.NotNil(t, lv, "expected RAID image child to be present")
	require.NotEmpty(t, lv.Children, "expected RAID image child to have nested logical volume")

	assert.Equal(t, "/dev/mapper/vg_nvme-lv_nvme", lv.Children[0].Name)
	assert.Equal(t, "/mnt/local_disk", lv.Children[0].MountPoint)
	assert.Equal(t, "ext4", lv.Children[0].FSType)

	flattened := devs.Flatten()
	md := findFlattenedDevice(flattened, "/dev/md127")
	require.NotNil(t, md, "expected /dev/md127 root RAID device to be present")
	assert.Equal(t, "raid1", md.Type)
	assert.Equal(t, "/", md.MountPoint)
	assert.Equal(t, "ext4", md.FSType)

	vg := findFlattenedDevice(flattened, "/dev/mapper/vg_nvme-lv_nvme")
	require.NotNil(t, vg, "expected logical volume with mount point to be present")
	assert.Equal(t, "lvm", vg.Type)
	assert.Equal(t, "/mnt/local_disk", vg.MountPoint)
}

func TestDefaultDeviceTypeFuncAllowsRaid(t *testing.T) {
	assert.True(t, DefaultDeviceTypeFunc("raid1"))
	assert.True(t, DefaultDeviceTypeFunc("raid10"))
	assert.True(t, DefaultDeviceTypeFunc("md"))
	assert.False(t, DefaultDeviceTypeFunc("loop"))
}

func findFlattenedDevice(devs FlattenedBlockDevices, name string) *FlattenedBlockDevice {
	for i := range devs {
		if devs[i].Name == name {
			return &devs[i]
		}
	}
	return nil
}

// TestParseLsblkJSONNestedMountPointsAllDevices verifies that all expected devices are present
// and properly nested, including parent devices that only have matching grandchildren.
func TestParseLsblkJSONNestedMountPointsAllDevices(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("testdata", "lsblk.withlv.censored.json"))
	require.NoError(t, err)

	devs, err := parseLsblkJSON(
		ctx,
		data,
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)
	require.NotEmpty(t, devs, "expected block devices to be discovered")

	// Check that we found all the expected top-level devices
	deviceNames := make(map[string]bool)
	for i := range devs {
		deviceNames[devs[i].Name] = true
	}

	expectedDevices := []string{
		"/dev/sda",     // has children with mountpoints (/boot/efi, /)
		"/dev/sdb",     // has children with mountpoints (/boot/efi, /)
		"/dev/nvme0n1", // has nested LVM with /mnt/local_disk mountpoint
		// Note: /dev/nvme1n1 is correctly filtered out - has ext4 but no mountpoint and no children
		"/dev/nvme2n1", // has nested LVM with /mnt/local_disk mountpoint
		"/dev/nvme3n1", // has nested LVM with /mnt/local_disk mountpoint
		"/dev/nvme4n1", // has nested LVM with /mnt/local_disk mountpoint
	}

	for _, name := range expectedDevices {
		assert.True(t, deviceNames[name], "expected device %s to be present", name)
	}
}

// TestParseLsblkJSONNestedLVMChain tests the specific LVM chain that was failing:
// nvme -> LVM2_member -> rimage (no mountpoint) -> final LV (with mountpoint)
func TestParseLsblkJSONNestedLVMChain(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("testdata", "lsblk.withlv.censored.json"))
	require.NoError(t, err)

	devs, err := parseLsblkJSON(
		ctx,
		data,
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)

	// Find one of the NVMe devices with the LVM chain
	var nvme2 *BlockDevice
	for i := range devs {
		if devs[i].Name == "/dev/nvme2n1" {
			nvme2 = &devs[i]
			break
		}
	}
	require.NotNil(t, nvme2, "expected /dev/nvme2n1 to be present")

	// It should have the rimage as a child
	require.Len(t, nvme2.Children, 1, "expected exactly one child (rimage)")
	rimage := nvme2.Children[0]
	assert.Equal(t, "/dev/mapper/vg_nvme-lv_nvme_rimage_1", rimage.Name)
	assert.Equal(t, "lvm", rimage.Type)
	assert.Empty(t, rimage.MountPoint, "rimage should not have a mountpoint")

	// The rimage should have the final LV as its child
	require.Len(t, rimage.Children, 1, "expected rimage to have one child (final LV)")
	finalLV := rimage.Children[0]
	assert.Equal(t, "/dev/mapper/vg_nvme-lv_nvme", finalLV.Name)
	assert.Equal(t, "lvm", finalLV.Type)
	assert.Equal(t, "/mnt/local_disk", finalLV.MountPoint)
	assert.Equal(t, "ext4", finalLV.FSType)
}

// TestParseLsblkJSONRAIDChain tests RAID device handling:
// disk -> partition (linux_raid_member, no mountpoint) -> raid1 (with mountpoint /)
func TestParseLsblkJSONRAIDChain(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("testdata", "lsblk.withlv.censored.json"))
	require.NoError(t, err)

	devs, err := parseLsblkJSON(
		ctx,
		data,
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)

	// Find /dev/sda
	var sda *BlockDevice
	for i := range devs {
		if devs[i].Name == "/dev/sda" {
			sda = &devs[i]
			break
		}
	}
	require.NotNil(t, sda, "expected /dev/sda to be present")

	// Should have 1 child: sda2 (linux_raid_member with RAID child)
	// Note: sda1 is filtered out because it has vfat filesystem, which is not in DefaultFsTypeFunc
	require.Len(t, sda.Children, 1, "expected one child (sda2)")

	// Find sda2 (the raid member)
	var sda2 *BlockDevice
	for i := range sda.Children {
		if sda.Children[i].Name == "/dev/sda2" {
			sda2 = &sda.Children[i]
			break
		}
	}
	require.NotNil(t, sda2, "expected /dev/sda2 to be present")
	assert.Equal(t, "part", sda2.Type)
	assert.Equal(t, "linux_raid_member", sda2.FSType)
	assert.Empty(t, sda2.MountPoint, "sda2 should not have a mountpoint")

	// sda2 should have the RAID device as its child
	require.Len(t, sda2.Children, 1, "expected sda2 to have one child (raid device)")
	raid := sda2.Children[0]
	assert.Equal(t, "/dev/md127", raid.Name)
	assert.Equal(t, "raid1", raid.Type)
	assert.Equal(t, "/", raid.MountPoint)
	assert.Equal(t, "ext4", raid.FSType)
}

// TestParseLsblkJSONWithoutFilters verifies that without filters, all devices are included
func TestParseLsblkJSONWithoutFilters(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("testdata", "lsblk.withlv.censored.json"))
	require.NoError(t, err)

	devs, err := parseLsblkJSON(ctx, data)
	require.NoError(t, err)
	require.NotEmpty(t, devs, "expected block devices to be discovered")

	// Without filters, all 7 top-level devices should be present
	assert.Len(t, devs, 7, "expected all 7 top-level devices without filters")

	// Verify nvme1n1 is included when no filters are applied
	var foundNvme1 bool
	for i := range devs {
		if devs[i].Name == "/dev/nvme1n1" {
			foundNvme1 = true
			assert.Equal(t, "ext4", devs[i].FSType)
			assert.Empty(t, devs[i].MountPoint)
			break
		}
	}
	assert.True(t, foundNvme1, "expected /dev/nvme1n1 to be present without filters")
}

// TestParseLsblkJSONNvme1FilteredOut documents that nvme1n1 (ext4 but no mountpoint, no children)
// is correctly filtered out when using DefaultMountPointFunc
func TestParseLsblkJSONNvme1FilteredOut(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("testdata", "lsblk.withlv.censored.json"))
	require.NoError(t, err)

	devs, err := parseLsblkJSON(
		ctx,
		data,
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)

	// nvme1n1 should NOT be present because it has no mountpoint and no children
	for i := range devs {
		assert.NotEqual(t, "/dev/nvme1n1", devs[i].Name, "nvme1n1 should be filtered out with DefaultMountPointFunc")
	}
}

// TestParseLsblkJSONStrictMountPointFilter tests that with a strict mountpoint filter,
// only devices with matching mountpoints (or descendants with matching mountpoints) are included
func TestParseLsblkJSONStrictMountPointFilter(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("testdata", "lsblk.withlv.censored.json"))
	require.NoError(t, err)

	// Filter for only /mnt/local_disk
	devs, err := parseLsblkJSON(
		ctx,
		data,
		WithMountPoint(func(mp string) bool {
			return mp == "/mnt/local_disk"
		}),
	)
	require.NoError(t, err)

	// Should find the 4 NVMe devices that have the /mnt/local_disk chain
	assert.GreaterOrEqual(t, len(devs), 4, "expected at least 4 devices with /mnt/local_disk in their tree")

	// Verify that one of them has the correct chain
	var found bool
	for i := range devs {
		if devs[i].Name == "/dev/nvme0n1" {
			found = true
			assert.NotEmpty(t, devs[i].Children, "nvme0n1 should have children")
			break
		}
	}
	assert.True(t, found, "expected /dev/nvme0n1 with /mnt/local_disk chain")
}

// TestParseLsblkJSONFsTypeFilter tests filtering by filesystem type
func TestParseLsblkJSONFsTypeFilter(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("testdata", "lsblk.withlv.censored.json"))
	require.NoError(t, err)

	// Filter for only ext4
	devs, err := parseLsblkJSON(
		ctx,
		data,
		WithFstype(func(fstype string) bool {
			return fstype == "ext4"
		}),
	)
	require.NoError(t, err)
	require.NotEmpty(t, devs, "expected devices with ext4 filesystem")

	// Should include nvme1n1 (direct ext4) and devices with ext4 descendants
	flattened := devs.Flatten()
	var foundNvme1 bool
	var foundMd127 bool
	var foundLvNvme bool
	for i := range flattened {
		if flattened[i].Name == "/dev/nvme1n1" && flattened[i].FSType == "ext4" {
			foundNvme1 = true
		}
		if flattened[i].Name == "/dev/md127" && flattened[i].FSType == "ext4" {
			foundMd127 = true
		}
		if flattened[i].Name == "/dev/mapper/vg_nvme-lv_nvme" && flattened[i].FSType == "ext4" {
			foundLvNvme = true
		}
	}
	assert.True(t, foundNvme1, "expected /dev/nvme1n1 with ext4")
	assert.True(t, foundMd127, "expected /dev/md127 with ext4")
	assert.True(t, foundLvNvme, "expected /dev/mapper/vg_nvme-lv_nvme with ext4")
}

// TestParseLsblkJSONSimpleHierarchy tests backward compatibility with simple 1-level hierarchies
// (devices with no children). This ensures the recursive implementation doesn't break simple cases.
func TestParseLsblkJSONSimpleHierarchy(t *testing.T) {
	ctx := context.Background()

	// JSON with only top-level devices, no children
	simpleJSON := `{
		"blockdevices": [
			{
				"name": "/dev/sda",
				"type": "disk",
				"size": 1000000000000,
				"rota": false,
				"serial": "TEST123",
				"wwn": null,
				"vendor": "TestVendor",
				"model": "TestModel",
				"rev": "1.0",
				"mountpoint": "/data",
				"fstype": "ext4",
				"fsused": "500000000000",
				"partuuid": null
			},
			{
				"name": "/dev/sdb",
				"type": "disk",
				"size": 2000000000000,
				"rota": true,
				"serial": "TEST456",
				"wwn": null,
				"vendor": "TestVendor2",
				"model": "TestModel2",
				"rev": "2.0",
				"mountpoint": "/backup",
				"fstype": "ext4",
				"fsused": "1000000000000",
				"partuuid": null
			}
		]
	}`

	devs, err := parseLsblkJSON(
		ctx,
		[]byte(simpleJSON),
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)
	require.Len(t, devs, 2, "should find both devices")

	// Verify both devices are present
	assert.Equal(t, "/dev/sda", devs[0].Name)
	assert.Equal(t, "/data", devs[0].MountPoint)
	assert.Len(t, devs[0].Children, 0, "should have no children")

	assert.Equal(t, "/dev/sdb", devs[1].Name)
	assert.Equal(t, "/backup", devs[1].MountPoint)
	assert.Len(t, devs[1].Children, 0, "should have no children")
}

// TestParseLsblkJSONTwoLevelHierarchy tests backward compatibility with simple 2-level hierarchies
// (parent with direct children, the most common case). This ensures the recursive implementation
// doesn't break the typical disk → partition scenario.
func TestParseLsblkJSONTwoLevelHierarchy(t *testing.T) {
	ctx := context.Background()

	// JSON with parent and direct children (typical disk with partitions)
	twoLevelJSON := `{
		"blockdevices": [
			{
				"name": "/dev/sda",
				"type": "disk",
				"size": 1000000000000,
				"rota": false,
				"serial": "TEST123",
				"wwn": null,
				"vendor": "TestVendor",
				"model": "TestModel",
				"rev": "1.0",
				"mountpoint": null,
				"fstype": null,
				"fsused": null,
				"partuuid": null,
				"children": [
					{
						"name": "/dev/sda1",
						"type": "part",
						"size": 500000000000,
						"rota": false,
						"serial": null,
						"wwn": null,
						"vendor": null,
						"model": null,
						"rev": null,
						"mountpoint": "/",
						"fstype": "ext4",
						"fsused": "200000000000",
						"partuuid": "abc-123"
					},
					{
						"name": "/dev/sda2",
						"type": "part",
						"size": 500000000000,
						"rota": false,
						"serial": null,
						"wwn": null,
						"vendor": null,
						"model": null,
						"rev": null,
						"mountpoint": "/home",
						"fstype": "ext4",
						"fsused": "300000000000",
						"partuuid": "def-456"
					}
				]
			}
		]
	}`

	devs, err := parseLsblkJSON(
		ctx,
		[]byte(twoLevelJSON),
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)
	require.Len(t, devs, 1, "should find parent device")

	parent := devs[0]
	assert.Equal(t, "/dev/sda", parent.Name)
	require.Len(t, parent.Children, 2, "parent should have 2 children")

	// Verify children are properly nested
	assert.Equal(t, "/dev/sda1", parent.Children[0].Name)
	assert.Equal(t, "/", parent.Children[0].MountPoint)
	assert.Equal(t, "ext4", parent.Children[0].FSType)
	assert.Equal(t, "/dev/sda", parent.Children[0].ParentDeviceName)

	assert.Equal(t, "/dev/sda2", parent.Children[1].Name)
	assert.Equal(t, "/home", parent.Children[1].MountPoint)
	assert.Equal(t, "ext4", parent.Children[1].FSType)
	assert.Equal(t, "/dev/sda", parent.Children[1].ParentDeviceName)
}

// TestParseLsblkJSONParentFilteredChildNotFiltered tests backward compatibility
// where parent doesn't match but child does (common case for unmounted disks with mounted partitions)
func TestParseLsblkJSONParentFilteredChildNotFiltered(t *testing.T) {
	ctx := context.Background()

	// Parent has no mountpoint, but child does
	json := `{
		"blockdevices": [
			{
				"name": "/dev/sda",
				"type": "disk",
				"size": 1000000000000,
				"rota": false,
				"serial": "TEST123",
				"wwn": null,
				"vendor": "TestVendor",
				"model": "TestModel",
				"rev": "1.0",
				"mountpoint": null,
				"fstype": null,
				"fsused": null,
				"partuuid": null,
				"children": [
					{
						"name": "/dev/sda1",
						"type": "part",
						"size": 1000000000000,
						"rota": false,
						"serial": null,
						"wwn": null,
						"vendor": null,
						"model": null,
						"rev": null,
						"mountpoint": "/",
						"fstype": "ext4",
						"fsused": "500000000000",
						"partuuid": "abc-123"
					}
				]
			}
		]
	}`

	devs, err := parseLsblkJSON(
		ctx,
		[]byte(json),
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)
	require.Len(t, devs, 1, "parent should be included because child matches")

	parent := devs[0]
	assert.Equal(t, "/dev/sda", parent.Name)
	assert.Empty(t, parent.MountPoint, "parent has no mountpoint")
	require.Len(t, parent.Children, 1, "child should be included")

	child := parent.Children[0]
	assert.Equal(t, "/dev/sda1", child.Name)
	assert.Equal(t, "/", child.MountPoint)
}

// TestParseLsblkJSONDepthLimit tests the stack overflow protection mechanism.
// This ensures the recursive parser doesn't crash with extremely deep hierarchies
// or malformed data with circular references.
func TestParseLsblkJSONDepthLimit(t *testing.T) {
	ctx := context.Background()

	// Create a deeply nested structure (25 levels, exceeding the maxRecursionDepth of 20)
	// Build from the bottom up
	deepJSON := `{
		"blockdevices": [
			{
				"name": "/dev/test0",
				"type": "disk",
				"size": 1000000000000,
				"rota": false,
				"serial": "TEST",
				"wwn": null,
				"vendor": null,
				"model": null,
				"rev": null,
				"mountpoint": null,
				"fstype": null,
				"fsused": null,
				"partuuid": null,
				"children": [
					{
						"name": "/dev/test1",
						"type": "lvm",
						"size": 1000000000000,
						"rota": false,
						"serial": null,
						"wwn": null,
						"vendor": null,
						"model": null,
						"rev": null,
						"mountpoint": null,
						"fstype": null,
						"fsused": null,
						"partuuid": null,
						"children": [
							{
								"name": "/dev/test2",
								"type": "lvm",
								"size": 1000000000000,
								"rota": false,
								"serial": null,
								"wwn": null,
								"vendor": null,
								"model": null,
								"rev": null,
								"mountpoint": null,
								"fstype": null,
								"fsused": null,
								"partuuid": null,
								"children": [
									{
										"name": "/dev/test_deep",
										"type": "lvm",
										"size": 1000000000000,
										"rota": false,
										"serial": null,
										"wwn": null,
										"vendor": null,
										"model": null,
										"rev": null,
										"mountpoint": "/test",
										"fstype": "ext4",
										"fsused": "500000000000",
										"partuuid": null
									}
								]
							}
						]
					}
				]
			}
		]
	}`

	// This should NOT crash, even with deep nesting
	devs, err := parseLsblkJSON(
		ctx,
		[]byte(deepJSON),
		WithFstype(DefaultFsTypeFunc),
		WithDeviceType(DefaultDeviceTypeFunc),
		WithMountPoint(DefaultMountPointFunc),
	)
	require.NoError(t, err)

	// For a 4-level hierarchy (within limit), it should work normally
	require.Len(t, devs, 1, "should find root device")
	assert.Equal(t, "/dev/test0", devs[0].Name)

	// Verify the nested structure exists
	assert.NotEmpty(t, devs[0].Children)
}

// TestDefaultDeviceTypeFuncComprehensive tests all device types
func TestDefaultDeviceTypeFuncComprehensive(t *testing.T) {
	tests := []struct {
		deviceType string
		expected   bool
	}{
		{"disk", true},
		{"part", true},
		{"lvm", true},
		{"raid0", true},
		{"raid1", true},
		{"raid5", true},
		{"raid10", true},
		{"md", true},
		{"md0", true},
		{"md127", true},
		{"loop", false},
		{"rom", false},
		{"crypt", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.deviceType, func(t *testing.T) {
			result := DefaultDeviceTypeFunc(tt.deviceType)
			assert.Equal(t, tt.expected, result, "device type %q should return %v", tt.deviceType, tt.expected)
		})
	}
}
