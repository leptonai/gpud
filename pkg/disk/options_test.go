package disk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultFsTypeFuncs(t *testing.T) {
	t.Run("DefaultExt4FsTypeFunc", func(t *testing.T) {
		assert.True(t, DefaultExt4FsTypeFunc("ext4"))
		assert.False(t, DefaultExt4FsTypeFunc("fuse.juicefs"))
		assert.False(t, DefaultExt4FsTypeFunc("wekafs"))
		assert.False(t, DefaultExt4FsTypeFunc(""))
	})

	t.Run("DefaultNFSFsTypeFunc", func(t *testing.T) {
		assert.False(t, DefaultNFSFsTypeFunc("fuse.juicefs"))
		assert.True(t, DefaultNFSFsTypeFunc("wekafs"))
		assert.True(t, DefaultNFSFsTypeFunc("virtiofs"))
		assert.True(t, DefaultNFSFsTypeFunc("nfs"))
		assert.True(t, DefaultNFSFsTypeFunc("nfs4"))
		assert.False(t, DefaultNFSFsTypeFunc("ext4"))
		assert.False(t, DefaultNFSFsTypeFunc(""))
	})

	t.Run("DefaultFsTypeFunc", func(t *testing.T) {
		assert.True(t, DefaultFsTypeFunc("ext4"))
		assert.True(t, DefaultFsTypeFunc("LVM2_member"))
		assert.True(t, DefaultFsTypeFunc("linux_raid_member"))
		assert.True(t, DefaultFsTypeFunc("raid0"))
		assert.True(t, DefaultFsTypeFunc(""))
		assert.False(t, DefaultFsTypeFunc("fuse.juicefs"))
		assert.False(t, DefaultFsTypeFunc("wekafs"))
	})

	t.Run("DefaultDeviceTypeFunc", func(t *testing.T) {
		assert.True(t, DefaultDeviceTypeFunc("disk"))
		assert.True(t, DefaultDeviceTypeFunc("lvm"))
		assert.True(t, DefaultDeviceTypeFunc("part"))
		assert.False(t, DefaultDeviceTypeFunc("loop"))
		assert.False(t, DefaultDeviceTypeFunc(""))
	})

	t.Run("DefaultMountPointFunc", func(t *testing.T) {
		// Test empty mount point
		assert.False(t, DefaultMountPointFunc(""))

		// Test normal mount points
		assert.True(t, DefaultMountPointFunc("/"))
		assert.True(t, DefaultMountPointFunc("/home"))
		assert.True(t, DefaultMountPointFunc("/var"))
		assert.True(t, DefaultMountPointFunc("/usr"))
		assert.True(t, DefaultMountPointFunc("/opt"))
		assert.True(t, DefaultMountPointFunc("/mnt"))
		assert.True(t, DefaultMountPointFunc("/mnt/data"))

		// Test provider-specific mount points that should be filtered
		assert.False(t, DefaultMountPointFunc("/mnt/cloud-metadata"))
		assert.False(t, DefaultMountPointFunc("/mnt/cloud-metadata/"))
		assert.False(t, DefaultMountPointFunc("/mnt/cloud-metadata/instance"))

		// Test edge cases - similar but different paths
		assert.True(t, DefaultMountPointFunc("/mnt/customfs"))
		assert.True(t, DefaultMountPointFunc("/mnt/customfs/"))
		assert.True(t, DefaultMountPointFunc("/mnt/customfs/subfolder"))
		assert.True(t, DefaultMountPointFunc("/mnt/customfs-data"))
		assert.False(t, DefaultMountPointFunc("/mnt/cloud-metadata-backup")) // /mnt/cloud-metadata is a prefix
		assert.True(t, DefaultMountPointFunc("/mnt/custom"))
		assert.True(t, DefaultMountPointFunc("/mnt/cloud"))
		assert.True(t, DefaultMountPointFunc("/customfs"))
		assert.True(t, DefaultMountPointFunc("/cloud-metadata"))
	})
}
