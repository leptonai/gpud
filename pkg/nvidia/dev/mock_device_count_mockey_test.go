package dev

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- canRead direct tests ---

func TestCanRead_ExistingFile(t *testing.T) {
	mockey.PatchConvey("canRead returns true for readable file", t, func() {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "readable")
		require.NoError(t, os.WriteFile(tmpFile, []byte("data"), 0644))

		assert.True(t, canRead(tmpFile))
	})
}

func TestCanRead_NonExistentFile(t *testing.T) {
	mockey.PatchConvey("canRead returns false for non-existent file", t, func() {
		assert.False(t, canRead("/nonexistent/path/file"))
	})
}

func TestCanRead_NoReadPermission(t *testing.T) {
	mockey.PatchConvey("canRead returns false for unreadable file", t, func() {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "unreadable")
		require.NoError(t, os.WriteFile(tmpFile, []byte("data"), 0000))
		defer func() { _ = os.Chmod(tmpFile, 0644) }()

		assert.False(t, canRead(tmpFile))
	})
}

func TestCanRead_Directory(t *testing.T) {
	mockey.PatchConvey("canRead returns true for readable directory", t, func() {
		tmpDir := t.TempDir()
		// Directories can be opened for reading
		result := canRead(tmpDir)
		assert.True(t, result)
	})
}

// --- countAllDevicesFromDir edge cases ---

func TestCountAllDevicesFromDir_EmptyDir(t *testing.T) {
	mockey.PatchConvey("countAllDevicesFromDir returns 0 for empty dir", t, func() {
		tmpDir := t.TempDir()

		count, err := countAllDevicesFromDir(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestCountAllDevicesFromDir_MixedFiles(t *testing.T) {
	mockey.PatchConvey("countAllDevicesFromDir counts only nvidia devices", t, func() {
		tmpDir := t.TempDir()

		// Create valid nvidia device files
		for i := 0; i < 3; i++ {
			f := filepath.Join(tmpDir, "nvidia"+string(rune('0'+i)))
			require.NoError(t, os.WriteFile(f, []byte{}, 0644))
		}
		// Create non-nvidia files
		for _, name := range []string{"nvidia", "nvidiactl", "nvidia-uvm", "sda1", "ttyS0"} {
			f := filepath.Join(tmpDir, name)
			require.NoError(t, os.WriteFile(f, []byte{}, 0644))
		}

		count, err := countAllDevicesFromDir(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})
}

func TestCountAllDevicesFromDir_UnreadableDevice(t *testing.T) {
	mockey.PatchConvey("countAllDevicesFromDir skips unreadable devices", t, func() {
		tmpDir := t.TempDir()

		// Create readable nvidia0
		f0 := filepath.Join(tmpDir, "nvidia0")
		require.NoError(t, os.WriteFile(f0, []byte{}, 0644))

		// Create unreadable nvidia1
		f1 := filepath.Join(tmpDir, "nvidia1")
		require.NoError(t, os.WriteFile(f1, []byte{}, 0000))
		defer func() { _ = os.Chmod(f1, 0644) }()

		// Create readable nvidia2
		f2 := filepath.Join(tmpDir, "nvidia2")
		require.NoError(t, os.WriteFile(f2, []byte{}, 0644))

		count, err := countAllDevicesFromDir(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, 2, count) // Only nvidia0 and nvidia2
	})
}

func TestCountAllDevicesFromDir_NonExistentDir(t *testing.T) {
	mockey.PatchConvey("countAllDevicesFromDir errors for non-existent dir", t, func() {
		_, err := countAllDevicesFromDir("/nonexistent/dir/path")
		require.Error(t, err)
	})
}

func TestCountAllDevicesFromDir_OnlyInvalidFiles(t *testing.T) {
	mockey.PatchConvey("countAllDevicesFromDir returns 0 when no valid devices", t, func() {
		tmpDir := t.TempDir()

		// Create non-nvidia files only
		for _, name := range []string{"nvidiactl", "nvidia-uvm", "nvidia-modeset", "sda1"} {
			f := filepath.Join(tmpDir, name)
			require.NoError(t, os.WriteFile(f, []byte{}, 0644))
		}

		count, err := countAllDevicesFromDir(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

// --- CountAllDevicesFromDevDir with mocked os.Stat ---

func TestCountAllDevicesFromDevDir_MockedNonExistentDev(t *testing.T) {
	mockey.PatchConvey("CountAllDevicesFromDevDir returns 0 when /dev doesn't exist", t, func() {
		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}).Build()

		count, err := CountAllDevicesFromDevDir()
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

// --- countDevEntry additional tests ---

func TestCountDevEntry_NvidiaControl(t *testing.T) {
	mockey.PatchConvey("countDevEntry rejects nvidiactl", t, func() {
		assert.False(t, countDevEntry("nvidiactl"))
	})
}

func TestCountDevEntry_NvidiaUVM(t *testing.T) {
	mockey.PatchConvey("countDevEntry rejects nvidia-uvm", t, func() {
		assert.False(t, countDevEntry("nvidia-uvm"))
	})
}

func TestCountDevEntry_NvidiaModeset(t *testing.T) {
	mockey.PatchConvey("countDevEntry rejects nvidia-modeset", t, func() {
		assert.False(t, countDevEntry("nvidia-modeset"))
	})
}

func TestCountDevEntry_HighNumberDevice(t *testing.T) {
	mockey.PatchConvey("countDevEntry accepts high-numbered devices", t, func() {
		assert.True(t, countDevEntry("nvidia15"))
		assert.True(t, countDevEntry("nvidia127"))
	})
}

func TestCountDevEntry_MultiDigitPath(t *testing.T) {
	mockey.PatchConvey("countDevEntry handles full paths with multi-digit numbers", t, func() {
		assert.True(t, countDevEntry("/dev/nvidia10"))
		assert.True(t, countDevEntry("/dev/nvidia99"))
	})
}
