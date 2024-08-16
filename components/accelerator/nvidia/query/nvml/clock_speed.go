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
}

func GetClockSpeed(uuid string, dev device.Device) (ClockSpeed, error) {
	clockSpeed := ClockSpeed{
		UUID: uuid,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g2efc4dd4096173f01d80b2a8bbfd97ad
	graphicsClock, ret := dev.GetClockInfo(nvml.CLOCK_GRAPHICS)
	if ret != nvml.SUCCESS {
		return ClockSpeed{}, fmt.Errorf("failed to get device clock info for nvml.CLOCK_GRAPHICS: %v", nvml.ErrorString(ret))
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g2efc4dd4096173f01d80b2a8bbfd97ad
	memClock, ret := dev.GetClockInfo(nvml.CLOCK_MEM)
	if ret != nvml.SUCCESS {
		return ClockSpeed{}, fmt.Errorf("failed to get device clock info for nvml.CLOCK_MEM: %v", nvml.ErrorString(ret))
	}

	clockSpeed.GraphicsMHz = graphicsClock
	clockSpeed.MemoryMHz = memClock

	return clockSpeed, nil
}
