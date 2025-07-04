package testutil

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
)

// CreateGPMSupportedDevice creates a mock device for GPM support testing
func CreateGPMSupportedDevice(
	uuid string,
	gpmDeviceSupport nvml.GpmSupport,
	gpmDeviceSupportRet nvml.Return,
) device.Device {
	mockDevice := &mock.Device{
		GpmQueryDeviceSupportFunc: func() (nvml.GpmSupport, nvml.Return) {
			return gpmDeviceSupport, gpmDeviceSupportRet
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}

	return NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")
}

// CreateGPMSampleDevice creates a mock device for GPM sample testing
func CreateGPMSampleDevice(
	uuid string,
	sampleGetRet nvml.Return,
) device.Device {
	mockDevice := &mock.Device{
		GpmQueryDeviceSupportFunc: func() (nvml.GpmSupport, nvml.Return) {
			return nvml.GpmSupport{IsSupportedDevice: 1}, nvml.SUCCESS
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
		GpmSampleGetFunc: func(sample nvml.GpmSample) nvml.Return {
			return sampleGetRet
		},
	}

	return NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")
}
