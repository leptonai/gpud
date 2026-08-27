package pci

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultSysfsPCIDevicesDir is the sysfs directory that the kernel
// populates with one entry per PCI device. PCI devices are not
// namespaced, so this reflects the host topology even inside containers
// that do not host-mount /sys.
const DefaultSysfsPCIDevicesDir = "/sys/bus/pci/devices"

// sysfsVendorNVIDIA is the PCI vendor ID of NVIDIA as read from the
// sysfs "vendor" attribute of a PCI device.
const sysfsVendorNVIDIA = "0x10de"

// PCI class codes (class/subclass in the upper 16 bits of the sysfs
// "class" attribute) under which NVIDIA GPUs are enumerated.
// Data-center GPUs (e.g., H100) enumerate as 3D controllers, whereas
// workstation/consumer GPUs (e.g., RTX 4090) enumerate as VGA
// compatible controllers.
const (
	sysfsClassVGAController = "0x0300"
	sysfsClass3DController  = "0x0302"
)

// GPUDevice describes one NVIDIA GPU PCI device discovered via sysfs.
type GPUDevice struct {
	// Address is the PCI domain:bus:device.function address,
	// e.g., "0000:19:00.0".
	Address string

	// DeviceID is the PCI device ID in lowercase hex without the
	// "0x" prefix, e.g., "2330" for an H100 SXM5.
	DeviceID string
}

// ListNVIDIAGPUDevices returns all NVIDIA GPU PCI devices by reading the
// sysfs PCI device tree under the given directory. Unlike "lspci", this
// needs no pciutils binary and no PCI ID database, and -- most
// importantly -- no NVIDIA driver: PCI enumeration is done by the kernel,
// so the GPU devices are visible even when the driver is not loaded yet
// (e.g., still being installed by the NVIDIA GPU Operator).
func ListNVIDIAGPUDevices(sysfsDevicesDir string) ([]GPUDevice, error) {
	entries, err := os.ReadDir(sysfsDevicesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sysfs PCI devices directory %q: %w", sysfsDevicesDir, err)
	}

	devs := make([]GPUDevice, 0)
	for _, entry := range entries {
		deviceDir := filepath.Join(sysfsDevicesDir, entry.Name())

		vendor, err := readSysfsAttribute(deviceDir, "vendor")
		if err != nil {
			continue
		}
		if vendor != sysfsVendorNVIDIA {
			continue
		}

		class, err := readSysfsAttribute(deviceDir, "class")
		if err != nil {
			continue
		}
		if !strings.HasPrefix(class, sysfsClassVGAController) &&
			!strings.HasPrefix(class, sysfsClass3DController) {
			continue
		}

		deviceID, err := readSysfsAttribute(deviceDir, "device")
		if err != nil {
			continue
		}

		devs = append(devs, GPUDevice{
			Address:  entry.Name(),
			DeviceID: strings.TrimPrefix(deviceID, "0x"),
		})
	}

	sort.Slice(devs, func(i, j int) bool { return devs[i].Address < devs[j].Address })
	return devs, nil
}

// readSysfsAttribute reads a single-value sysfs attribute,
// returning the trimmed value (e.g., "0x10de").
func readSysfsAttribute(deviceDir string, name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(deviceDir, name))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
