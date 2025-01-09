package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// ClockSpeed represents the data from the nvmlDeviceGetClockInfo API.
// Returns the graphics and memory clock speeds in MHz.
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g2efc4dd4096173f01d80b2a8bbfd97ad
type ClockSpeed struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	GraphicsMHz uint32 `json:"graphics_mhz"`
	MemoryMHz   uint32 `json:"memory_mhz"`

	// ClockGraphicsSupported is true if the clock speed is supported by the device.
	ClockGraphicsSupported bool `json:"clock_graphics_supported"`

	// ClockMemorySupported is true if the clock speed is supported by the device.
	ClockMemorySupported bool `json:"clock_memory_supported"`
}

func GetClockSpeed(uuid string, dev device.Device) (ClockSpeed, error) {
	clockSpeed := ClockSpeed{
		UUID: uuid,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g2efc4dd4096173f01d80b2a8bbfd97ad
	graphicsClock, ret := dev.GetClockInfo(nvml.CLOCK_GRAPHICS)
	if IsNotSupportError(ret) {
		clockSpeed.ClockGraphicsSupported = false
	} else if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
		return clockSpeed, fmt.Errorf("failed to get device clock info for nvml.CLOCK_GRAPHICS: %v", nvml.ErrorString(ret))
	}
	clockSpeed.ClockGraphicsSupported = true
	clockSpeed.GraphicsMHz = graphicsClock

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g2efc4dd4096173f01d80b2a8bbfd97ad
	memClock, ret := dev.GetClockInfo(nvml.CLOCK_MEM)
	if IsNotSupportError(ret) {
		clockSpeed.ClockMemorySupported = false
	} else if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
		return clockSpeed, fmt.Errorf("failed to get device clock info for nvml.CLOCK_MEM: %v", nvml.ErrorString(ret))
	}
	clockSpeed.ClockMemorySupported = true
	clockSpeed.MemoryMHz = memClock

	return clockSpeed, nil
}
