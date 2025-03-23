package testutil

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

// CreateClockSpeedDevice creates a new mock device specifically for clock speed testing
func CreateClockSpeedDevice(graphicsClock uint32, graphicsClockRet nvml.Return, memClock uint32, memClockRet nvml.Return, uuid string) nvml.Device {
	return &MockDevice{
		Device: &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				switch clockType {
				case nvml.CLOCK_GRAPHICS:
					return graphicsClock, graphicsClockRet
				case nvml.CLOCK_MEM:
					return memClock, memClockRet
				default:
					return 0, nvml.ERROR_UNKNOWN
				}
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		},
	}
}
