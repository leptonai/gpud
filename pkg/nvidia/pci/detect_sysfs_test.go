package pci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSysfsPCIDevice(t *testing.T, root, address, vendor, class string) {
	t.Helper()
	dir := filepath.Join(root, address)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor"), []byte(vendor+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "class"), []byte(class+"\n"), 0o644))
}

func TestHasNVIDIAGPU(t *testing.T) {
	t.Run("NVIDIA 3D controller", func(t *testing.T) {
		root := t.TempDir()
		writeSysfsPCIDevice(t, root, "0000:19:00.0", "0x10de", "0x030200")

		exists, err := HasNVIDIAGPU(root)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("NVIDIA VGA controller", func(t *testing.T) {
		root := t.TempDir()
		writeSysfsPCIDevice(t, root, "0000:01:00.0", "0x10de", "0x030000")

		exists, err := HasNVIDIAGPU(root)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("ignores non-GPU NVIDIA device", func(t *testing.T) {
		root := t.TempDir()
		writeSysfsPCIDevice(t, root, "0000:02:00.0", "0x10de", "0x060400")

		exists, err := HasNVIDIAGPU(root)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("ignores another vendor GPU", func(t *testing.T) {
		root := t.TempDir()
		writeSysfsPCIDevice(t, root, "0000:03:00.0", "0x1002", "0x030200")

		exists, err := HasNVIDIAGPU(root)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("missing sysfs", func(t *testing.T) {
		exists, err := HasNVIDIAGPU(filepath.Join(t.TempDir(), "missing"))
		assert.False(t, exists)
		require.Error(t, err)
	})
}
