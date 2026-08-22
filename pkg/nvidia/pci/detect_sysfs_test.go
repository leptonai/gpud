package pci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakePCIDevice creates a fake sysfs PCI device entry.
func writeFakePCIDevice(t *testing.T, root string, address string, vendor string, device string, class string) {
	t.Helper()
	dir := filepath.Join(root, address)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor"), []byte(vendor+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "device"), []byte(device+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "class"), []byte(class+"\n"), 0o644))
}

func TestListNVIDIAGPUDevices(t *testing.T) {
	root := t.TempDir()

	// two H100 SXM5 3D controllers
	writeFakePCIDevice(t, root, "0000:19:00.0", "0x10de", "0x2330", "0x030200")
	writeFakePCIDevice(t, root, "0000:9b:00.0", "0x10de", "0x2330", "0x030200")
	// an RTX 4090 enumerates as a VGA compatible controller
	writeFakePCIDevice(t, root, "0000:01:00.0", "0x10de", "0x2684", "0x030000")
	// an NVIDIA NVSwitch is not a GPU and must be skipped
	writeFakePCIDevice(t, root, "0000:06:00.0", "0x10de", "0x1af1", "0x068000")
	// a non-NVIDIA device must be skipped
	writeFakePCIDevice(t, root, "0000:02:00.0", "0x8086", "0x1521", "0x020000")
	// a non-NVIDIA 3D controller must be skipped
	writeFakePCIDevice(t, root, "0000:03:00.0", "0x1002", "0x744c", "0x030200")

	devs, err := ListNVIDIAGPUDevices(root)
	require.NoError(t, err)
	require.Len(t, devs, 3)

	// sorted by PCI address for deterministic results
	assert.Equal(t, "0000:01:00.0", devs[0].Address)
	assert.Equal(t, "2684", devs[0].DeviceID)
	assert.Equal(t, "0000:19:00.0", devs[1].Address)
	assert.Equal(t, "2330", devs[1].DeviceID)
	assert.Equal(t, "0000:9b:00.0", devs[2].Address)
	assert.Equal(t, "2330", devs[2].DeviceID)
}

func TestListNVIDIAGPUDevices_NoDevices(t *testing.T) {
	root := t.TempDir()

	// only non-NVIDIA devices
	writeFakePCIDevice(t, root, "0000:02:00.0", "0x8086", "0x1521", "0x020000")

	devs, err := ListNVIDIAGPUDevices(root)
	require.NoError(t, err)
	assert.Empty(t, devs)
}

func TestListNVIDIAGPUDevices_MissingDirectory(t *testing.T) {
	devs, err := ListNVIDIAGPUDevices(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
	assert.Nil(t, devs)
}

func TestListNVIDIAGPUDevices_SkipsUnreadableEntries(t *testing.T) {
	root := t.TempDir()

	// device entry without attributes must be skipped, not fail the scan
	require.NoError(t, os.MkdirAll(filepath.Join(root, "0000:04:00.0"), 0o755))

	writeFakePCIDevice(t, root, "0000:19:00.0", "0x10de", "0x2331", "0x030200")

	devs, err := ListNVIDIAGPUDevices(root)
	require.NoError(t, err)
	require.Len(t, devs, 1)
	assert.Equal(t, "2331", devs[0].DeviceID)
}

func TestListNVIDIAGPUDevices_NonDirectoryEntriesSkipped(t *testing.T) {
	root := t.TempDir()

	// stray regular file in the devices directory must be skipped
	require.NoError(t, os.WriteFile(filepath.Join(root, "README"), []byte("not a device"), 0o644))

	writeFakePCIDevice(t, root, "0000:19:00.0", "0x10de", "0x2331", "0x030200")

	devs, err := ListNVIDIAGPUDevices(root)
	require.NoError(t, err)
	require.Len(t, devs, 1)
}
