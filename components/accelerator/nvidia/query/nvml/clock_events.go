package nvml

import (
	"encoding/json"
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"sigs.k8s.io/yaml"
)

// ClockEvents represents the current clock events from the nvmlDeviceGetCurrentClocksEventReasons API.
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1ga115e41a14b747cb334a0e7b49ae1941
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html#group__nvmlClocksEventReasons
type ClockEvents struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// Represents the bitmask of active clocks event reasons.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html#group__nvmlClocksEventReasons
	ReasonsBitmask uint64 `json:"reasons_bitmask"`
	// Represents the human-readable reasons for the clock events.
	Reasons []string `json:"reasons,omitempty"`

	// Set true if the HW Slowdown reason due to the high temperature is active.
	HWSlowdown bool `json:"hw_slowdown"`
	// Set true if the HW Thermal Slowdown reason due to the high temperature is active.
	HWSlowdownThermal bool `json:"hw_thermal_slowdown"`
	// Set true if the HW Power Brake Slowdown reason due to the external power brake assertion is active.
	HWSlowdownPowerBrake bool `json:"hw_slowdown_power_brake"`
}

func (evs *ClockEvents) JSON() ([]byte, error) {
	if evs == nil {
		return nil, nil
	}
	return json.Marshal(evs)
}

func (evs *ClockEvents) YAML() ([]byte, error) {
	if evs == nil {
		return nil, nil
	}
	return yaml.Marshal(evs)
}

func GetClockEvents(uuid string, dev device.Device) (ClockEvents, error) {
	clockEvents := ClockEvents{
		UUID: uuid,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
	reasons, ret := dev.GetCurrentClocksEventReasons()
	if ret != nvml.SUCCESS {
		return ClockEvents{}, fmt.Errorf("failed to get device clock event reasons: %v", nvml.ErrorString(ret))
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html#group__nvmlClocksEventReasons
	clockEvents.ReasonsBitmask = reasons

	for flag, description := range clockEventReasons {
		if reasons&flag != 0 {
			clockEvents.Reasons = append(clockEvents.Reasons, fmt.Sprintf("%s: %s", uuid, description))
		}
	}

	clockEvents.HWSlowdown = reasons&reasonHWSlowdown != 0
	clockEvents.HWSlowdownThermal = reasons&reasonHWSlowdownThermal != 0
	clockEvents.HWSlowdownPowerBrake = reasons&reasonHWSlowdownPowerBrake != 0

	return clockEvents, nil
}

// 0x0000000000000000 is none
const (
	reasonHWSlowdown           uint64 = 0x0000000000000008
	reasonHWSlowdownThermal    uint64 = 0x0000000000000040
	reasonHWSlowdownPowerBrake uint64 = 0x0000000000000080
)

// ref. https://github.com/NVIDIA/go-nvml/blob/main/gen/nvml/nvml.h
var clockEventReasons = map[uint64]string{
	// ref. nvmlClocksEventReasonGpuIdle
	0x0000000000000001: "GPU is idle and clocks are dropping to Idle state",

	// ref. nvmlClocksEventReasonApplicationsClocksSetting
	0x0000000000000002: "GPU clocks are limited by current setting of applications clocks",

	// ref. nvmlClocksEventReasonSwPowerCap
	0x0000000000000004: "Clocks have been optimized to not exceed currently set power limits ('SW Power Cap: Active' in nvidia-smi --query)",

	// ref. nvmlClocksThrottleReasonHwSlowdown
	0x0000000000000008: "HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw ('HW Slowdown: Active' in nvidia-smi --query)",

	// ref. nvmlClocksEventReasonSyncBoost
	0x0000000000000010: "GPU is part of a Sync boost group to maximize performance per watt",

	// ref. nvmlClocksEventReasonSwThermalSlowdown
	0x0000000000000020: "SW Thermal Slowdown is active to keep GPU and memory temperatures within operating limits",

	// ref. nvmlClocksThrottleReasonHwThermalSlowdown
	0x0000000000000040: "HW Thermal Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (temperature being too high) ('HW Thermal Slowdown' in nvidia-smi --query)",

	// ref. nvmlClocksThrottleReasonHwPowerBrakeSlowdown
	0x0000000000000080: "HW Power Brake Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (External Power Brake Assertion being triggered) ('HW Power Brake Slowdown' in nvidia-smi --query)",

	// ref. nvmlClocksEventReasonDisplayClockSetting
	0x0000000000000100: "GPU clocks are limited by current setting of Display clocks",
}
