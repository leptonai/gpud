package testutil

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
)

// CreateGSPFirmwareDevice creates a new mock device specifically for GSP firmware tests
func CreateGSPFirmwareDevice(
	uuid string,
	gspEnabled bool,
	gspSupported bool,
	gspFirmwareRet nvml.Return,
) device.Device {
	mockDevice := &mock.Device{
		GetGspFirmwareModeFunc: func() (bool, bool, nvml.Return) {
			return gspEnabled, gspSupported, gspFirmwareRet
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}

	return NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")
}
