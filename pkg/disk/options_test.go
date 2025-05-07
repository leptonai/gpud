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
}
