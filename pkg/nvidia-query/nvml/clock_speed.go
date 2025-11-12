package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
)

// ClockSpeed represents the data from the nvmlDeviceGetClockInfo API.
// Returns the graphics and memory clock speeds in MHz.
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g2efc4dd4096173f01d80b2a8bbfd97ad
type ClockSpeed struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// BusID is the GPU bus ID from the nvml API.
	//  e.g., "0000:0f:00.0"
	BusID string `json:"bus_id"`

	GraphicsMHz uint32 `json:"graphics_mhz"`
	MemoryMHz   uint32 `json:"memory_mhz"`

	// ClockGraphicsSupported is true if the clock speed is supported by the device.
	ClockGraphicsSupported bool `json:"clock_graphics_supported"`

	// ClockMemorySupported is true if the clock speed is supported by the device.
	ClockMemorySupported bool `json:"clock_memory_supported"`
}

func GetClockSpeed(uuid string, dev device.Device) (ClockSpeed, error) {
	clockSpeed := ClockSpeed{
		UUID:  uuid,
		BusID: dev.PCIBusID(),
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g2efc4dd4096173f01d80b2a8bbfd97ad
	graphicsClock, ret := dev.GetClockInfo(nvml.CLOCK_GRAPHICS)
	if nvmlerrors.IsNotSupportError(ret) {
		clockSpeed.ClockGraphicsSupported = false
	} else if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
		if nvmlerrors.IsGPULostError(ret) {
			return clockSpeed, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return clockSpeed, nvmlerrors.ErrGPURequiresReset
		}
		return clockSpeed, fmt.Errorf("failed to get device clock info for nvml.CLOCK_GRAPHICS: %v", nvml.ErrorString(ret))
	} else {
		clockSpeed.ClockGraphicsSupported = true
		clockSpeed.GraphicsMHz = graphicsClock
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g2efc4dd4096173f01d80b2a8bbfd97ad
	memClock, ret := dev.GetClockInfo(nvml.CLOCK_MEM)
	if nvmlerrors.IsNotSupportError(ret) {
		clockSpeed.ClockMemorySupported = false
	} else if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
		if nvmlerrors.IsGPULostError(ret) {
			return clockSpeed, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return clockSpeed, nvmlerrors.ErrGPURequiresReset
		}
		return clockSpeed, fmt.Errorf("failed to get device clock info for nvml.CLOCK_MEM: %v", nvml.ErrorString(ret))
	} else {
		clockSpeed.ClockMemorySupported = true
		clockSpeed.MemoryMHz = memClock
	}

	return clockSpeed, nil
}
