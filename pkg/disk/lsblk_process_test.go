package disk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessBlockDevice_SingleDevice tests processing a single device with no children
func TestProcessBlockDevice_SingleDevice(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts(nil))

	dev := &BlockDevice{
		Name:       "/dev/sda",
		Type:       "disk",
		FSType:     "ext4",
		MountPoint: "/data",
	}

	fstypeCache := make(map[string]string)
	result := processBlockDevice(ctx, dev, "", 0, op, fstypeCache)

	assert.True(t, result, "device with mountpoint should be included")
	assert.Len(t, dev.Children, 0, "single device should have no children")
}

// TestProcessBlockDevice_DeviceWithMatchingChild tests a device that doesn't match but has a matching child
func TestProcessBlockDevice_DeviceWithMatchingChild(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts([]OpOption{
		WithMountPoint(func(mp string) bool { return mp != "" }),
	}))

	dev := &BlockDevice{
		Name:       "/dev/sda",
		Type:       "disk",
		FSType:     "",
		MountPoint: "", // Parent has no mountpoint
		Children: []BlockDevice{
			{
				Name:       "/dev/sda1",
				Type:       "part",
				FSType:     "ext4",
				MountPoint: "/", // Child has mountpoint
			},
		},
	}

	fstypeCache := make(map[string]string)
	result := processBlockDevice(ctx, dev, "", 0, op, fstypeCache)

	assert.True(t, result, "parent should be included because child matches")
	assert.Len(t, dev.Children, 1, "matching child should be preserved")
	assert.Equal(t, "/dev/sda", dev.Children[0].ParentDeviceName, "child should have parent name set")
}

// TestProcessBlockDevice_ThreeLevelNesting tests nested LVM hierarchy
func TestProcessBlockDevice_ThreeLevelNesting(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts([]OpOption{
		WithMountPoint(func(mp string) bool { return mp != "" }),
		WithDeviceType(DefaultDeviceTypeFunc),
	}))

	// disk → lvm rimage → lvm final (with mountpoint)
	dev := &BlockDevice{
		Name:       "/dev/nvme0n1",
		Type:       "disk",
		FSType:     "LVM2_member",
		MountPoint: "",
		Children: []BlockDevice{
			{
				Name:       "/dev/mapper/vg-rimage",
				Type:       "lvm",
				FSType:     "",
				MountPoint: "",
				Children: []BlockDevice{
					{
						Name:       "/dev/mapper/vg-lv",
						Type:       "lvm",
						FSType:     "ext4",
						MountPoint: "/mnt/data",
					},
				},
			},
		},
	}

	fstypeCache := make(map[string]string)
	result := processBlockDevice(ctx, dev, "", 0, op, fstypeCache)

	assert.True(t, result, "grandparent should be included because grandchild matches")
	require.Len(t, dev.Children, 1, "parent should be preserved")
	require.Len(t, dev.Children[0].Children, 1, "grandchild should be preserved")
	assert.Equal(t, "/mnt/data", dev.Children[0].Children[0].MountPoint)
}

// TestProcessBlockDevice_FilteredOutDevice tests a device that doesn't match and has no matching children
func TestProcessBlockDevice_FilteredOutDevice(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts([]OpOption{
		WithMountPoint(func(mp string) bool { return mp != "" }),
	}))

	dev := &BlockDevice{
		Name:       "/dev/loop0",
		Type:       "loop",
		FSType:     "",
		MountPoint: "",
	}

	fstypeCache := make(map[string]string)
	result := processBlockDevice(ctx, dev, "", 0, op, fstypeCache)

	assert.False(t, result, "device without mountpoint and no children should be filtered out")
}

// TestProcessBlockDevice_DepthLimitExceeded tests stack overflow protection
func TestProcessBlockDevice_DepthLimitExceeded(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts(nil))

	dev := &BlockDevice{
		Name:       "/dev/test",
		Type:       "disk",
		FSType:     "ext4",
		MountPoint: "/test",
	}

	fstypeCache := make(map[string]string)

	// Call with depth exceeding the limit
	result := processBlockDevice(ctx, dev, "", maxRecursionDepth+1, op, fstypeCache)

	assert.False(t, result, "should return false when depth limit exceeded")
}

// TestProcessBlockDevice_FiltersByFstype tests filtering by filesystem type
func TestProcessBlockDevice_FiltersByFstype(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts([]OpOption{
		WithFstype(func(fs string) bool { return fs == "ext4" }),
	}))

	ext4Dev := &BlockDevice{
		Name:       "/dev/sda1",
		Type:       "part",
		FSType:     "ext4",
		MountPoint: "/data",
	}

	xfsDev := &BlockDevice{
		Name:       "/dev/sdb1",
		Type:       "part",
		FSType:     "xfs",
		MountPoint: "/backup",
	}

	fstypeCache := make(map[string]string)

	result1 := processBlockDevice(ctx, ext4Dev, "", 0, op, fstypeCache)
	assert.True(t, result1, "ext4 device should match")

	result2 := processBlockDevice(ctx, xfsDev, "", 0, op, fstypeCache)
	assert.False(t, result2, "xfs device should not match")
}

// TestProcessBlockDevice_FiltersByDeviceType tests filtering by device type
func TestProcessBlockDevice_FiltersByDeviceType(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts([]OpOption{
		WithDeviceType(func(dt string) bool { return dt == "disk" }),
	}))

	diskDev := &BlockDevice{
		Name:       "/dev/sda",
		Type:       "disk",
		FSType:     "ext4",
		MountPoint: "/data",
	}

	partDev := &BlockDevice{
		Name:       "/dev/sda1",
		Type:       "part",
		FSType:     "ext4",
		MountPoint: "/data",
	}

	fstypeCache := make(map[string]string)

	result1 := processBlockDevice(ctx, diskDev, "", 0, op, fstypeCache)
	assert.True(t, result1, "disk device should match")

	result2 := processBlockDevice(ctx, partDev, "", 0, op, fstypeCache)
	assert.False(t, result2, "part device should not match")
}

// TestProcessBlockDevice_MixedChildren tests parent with both matching and non-matching children
func TestProcessBlockDevice_MixedChildren(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts([]OpOption{
		WithMountPoint(func(mp string) bool { return mp != "" }),
	}))

	dev := &BlockDevice{
		Name:       "/dev/sda",
		Type:       "disk",
		FSType:     "",
		MountPoint: "",
		Children: []BlockDevice{
			{
				Name:       "/dev/sda1",
				Type:       "part",
				FSType:     "ext4",
				MountPoint: "/", // Matches
			},
			{
				Name:       "/dev/sda2",
				Type:       "part",
				FSType:     "swap",
				MountPoint: "", // Doesn't match
			},
			{
				Name:       "/dev/sda3",
				Type:       "part",
				FSType:     "ext4",
				MountPoint: "/home", // Matches
			},
		},
	}

	fstypeCache := make(map[string]string)
	result := processBlockDevice(ctx, dev, "", 0, op, fstypeCache)

	assert.True(t, result, "parent should be included because some children match")
	assert.Len(t, dev.Children, 2, "only matching children should be preserved")

	// Verify the matching children
	mountpoints := []string{}
	for _, child := range dev.Children {
		mountpoints = append(mountpoints, child.MountPoint)
	}
	assert.Contains(t, mountpoints, "/")
	assert.Contains(t, mountpoints, "/home")
	assert.NotContains(t, mountpoints, "")
}

// TestProcessBlockDevice_ParentNamePropagation tests that ParentDeviceName is set correctly
func TestProcessBlockDevice_ParentNamePropagation(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts(nil))

	dev := &BlockDevice{
		Name:       "/dev/sda",
		Type:       "disk",
		FSType:     "ext4",
		MountPoint: "/",
		Children: []BlockDevice{
			{
				Name:       "/dev/sda1",
				Type:       "part",
				FSType:     "ext4",
				MountPoint: "/data",
				Children: []BlockDevice{
					{
						Name:       "/dev/mapper/lv",
						Type:       "lvm",
						FSType:     "ext4",
						MountPoint: "/data/lv",
					},
				},
			},
		},
	}

	fstypeCache := make(map[string]string)
	processBlockDevice(ctx, dev, "parent", 0, op, fstypeCache)

	assert.Equal(t, "/dev/sda", dev.Children[0].ParentDeviceName, "child should have parent name")
	assert.Equal(t, "/dev/sda1", dev.Children[0].Children[0].ParentDeviceName, "grandchild should have child name as parent")
}

// TestProcessBlockDevice_RAIDDevice tests RAID device handling
func TestProcessBlockDevice_RAIDDevice(t *testing.T) {
	ctx := context.Background()
	op := &Op{}
	require.NoError(t, op.applyOpts([]OpOption{
		WithMountPoint(func(mp string) bool { return mp != "" }),
		WithDeviceType(DefaultDeviceTypeFunc),
	}))

	// disk → partition → raid1 (with mountpoint)
	dev := &BlockDevice{
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
	}

	fstypeCache := make(map[string]string)
	result := processBlockDevice(ctx, dev, "", 0, op, fstypeCache)

	assert.True(t, result, "parent should be included because RAID grandchild matches")
	require.Len(t, dev.Children, 1)
	require.Len(t, dev.Children[0].Children, 1)
	assert.Equal(t, "raid1", dev.Children[0].Children[0].Type)
	assert.Equal(t, "/", dev.Children[0].Children[0].MountPoint)
}
