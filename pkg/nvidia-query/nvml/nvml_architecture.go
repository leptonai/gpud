// Package nvml implements the NVIDIA Management Library (NVML) interface.
// See https://docs.nvidia.com/deploy/nvml-api/nvml-api-reference.html#nvml-api-reference for more details.
package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// GetArchitecture returns the GPU architecture name based on the device architecture.
func GetArchitecture(dev device.Device) (string, error) {
	arch, ret := dev.GetArchitecture()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get device architecture: %v", nvml.ErrorString(ret))
	}

	// Map architecture values to human-readable names based on NVML_DEVICE_ARCH_* constants
	switch arch {
	case nvml.DEVICE_ARCH_KEPLER:
		return "Kepler", nil
	case nvml.DEVICE_ARCH_MAXWELL:
		return "Maxwell", nil
	case nvml.DEVICE_ARCH_PASCAL:
		return "Pascal", nil
	case nvml.DEVICE_ARCH_VOLTA:
		return "Volta", nil
	case nvml.DEVICE_ARCH_TURING:
		return "Turing", nil
	case nvml.DEVICE_ARCH_AMPERE:
		return "Ampere", nil
	case nvml.DEVICE_ARCH_ADA:
		return "Ada", nil
	case nvml.DEVICE_ARCH_HOPPER:
		return "Hopper", nil
	// Blackwell constant might not be defined in the current version of go-nvml
	// case nvml.DEVICE_ARCH_BLACKWELL:
	// 	return "Blackwell", nil
	case nvml.DEVICE_ARCH_UNKNOWN:
		return "Unknown", nil
	default:
		return fmt.Sprintf("UnknownArchitecture(%d)", arch), nil
	}
}
