package pci

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultSysfsPCIDevicesDir = "/sys/bus/pci/devices"

// HasNVIDIAGPU reports whether sysfs contains an NVIDIA VGA or 3D controller.
// PCI enumeration is available before the NVIDIA driver is installed.
func HasNVIDIAGPU(sysfsDevicesDir string) (bool, error) {
	entries, err := os.ReadDir(sysfsDevicesDir)
	if err != nil {
		return false, fmt.Errorf("failed to read sysfs PCI devices directory %q: %w", sysfsDevicesDir, err)
	}

	for _, entry := range entries {
		deviceDir := filepath.Join(sysfsDevicesDir, entry.Name())
		vendor, err := os.ReadFile(filepath.Join(deviceDir, "vendor"))
		if err != nil || strings.TrimSpace(string(vendor)) != "0x10de" {
			continue
		}

		class, err := os.ReadFile(filepath.Join(deviceDir, "class"))
		if err != nil {
			continue
		}
		classValue := strings.TrimSpace(string(class))
		if strings.HasPrefix(classValue, "0x0300") || strings.HasPrefix(classValue, "0x0302") {
			return true, nil
		}
	}

	return false, nil
}
