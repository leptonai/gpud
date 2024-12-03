package nvml

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/leptonai/gpud/log"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"sigs.k8s.io/yaml"
)

// Returns true if clock events is supported by all devices.
// Returns false if any device does not support clock events.
// ref. undefined symbol: nvmlDeviceGetCurrentClocksEventReasons for older nvidia drivers
func ClockEventsSupported() (bool, error) {
	nvmlLib := nvml.New()
	if ret := nvmlLib.Init(); ret != nvml.SUCCESS {
		return false, fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
	}
	log.Logger.Debugw("successfully initialized NVML")

	deviceLib := device.New(nvmlLib)

	// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
	// "error getting device handle for index '6': Unknown Error"
	devices, err := deviceLib.GetDevices()
	if err != nil {
		return false, err
	}

	// in rare cases, this evaluates the whole system to false
	// as a result, masking clock events
	// we must monitor clock events when a single device supports clock events
	// (probably some undocumented behavior in NVML)

	for _, dev := range devices {
		supported, err := ClockEventsSupportedByDevice(dev)
		if err != nil {
			return false, err
		}
		if supported {
			return true, nil
		}
	}

	// no device supports clock events
	return false, nil
}

// Returns true if clock events is supported by this device.
func ClockEventsSupportedByDevice(dev device.Device) (bool, error) {
	// clock events are supported in versions 535 and above
	// otherwise, CGO call just exits with
	// undefined symbol: nvmlDeviceGetCurrentClocksEventReasons
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
	_, ret := dev.GetCurrentClocksEventReasons()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return false, nil
	}
	if ret != nvml.SUCCESS {
		return false, fmt.Errorf("could not get current clock events: %v", nvml.ErrorString(ret))
	}
	return true, nil
}

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

	// Represents the hardware slowdown reasons.
	HWSlowdownReasons []string `json:"hw_slowdown_reasons,omitempty"`

	// Represents other human-readable reasons for the clock events.
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

	// clock events are supported in versions 535 and above
	// otherwise, CGO call just exits with
	// undefined symbol: nvmlDeviceGetCurrentClocksEventReasons
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
	reasons, ret := dev.GetCurrentClocksEventReasons()
	if ret != nvml.SUCCESS {
		return ClockEvents{}, fmt.Errorf("failed to get device clock event reasons: %v", nvml.ErrorString(ret))
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html#group__nvmlClocksEventReasons
	clockEvents.ReasonsBitmask = reasons

	clockEvents.HWSlowdown = reasons&reasonHWSlowdown != 0
	clockEvents.HWSlowdownThermal = reasons&reasonHWSlowdownThermal != 0
	clockEvents.HWSlowdownPowerBrake = reasons&reasonHWSlowdownPowerBrake != 0

	hwReasons, otherReasons := getClockEventReasons(reasons)
	for _, reason := range hwReasons {
		clockEvents.HWSlowdownReasons = append(clockEvents.HWSlowdownReasons,
			fmt.Sprintf("%s: %s (nvml)", uuid, reason))
	}
	for _, reason := range otherReasons {
		clockEvents.Reasons = append(clockEvents.Reasons,
			fmt.Sprintf("%s: %s (nvml)", uuid, reason))
	}

	return clockEvents, nil
}

func getClockEventReasons(reasons uint64) ([]string, []string) {
	hwSlowdownReasons := make([]string, 0)
	otherReasons := make([]string, 0)

	for flag, rt := range clockEventReasonsToInclude {
		if reasons&flag != 0 {
			if rt.hwSlowdown {
				hwSlowdownReasons = append(hwSlowdownReasons, rt.description)
				continue
			}
			otherReasons = append(otherReasons, rt.description)
		}
	}

	// sort the reasons to make the output deterministic
	sort.Strings(hwSlowdownReasons)
	sort.Strings(otherReasons)

	return hwSlowdownReasons, otherReasons
}

// 0x0000000000000000 is none
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html
const (
	reasonHWSlowdown           uint64 = 0x0000000000000008
	reasonHWSlowdownThermal    uint64 = 0x0000000000000040
	reasonHWSlowdownPowerBrake uint64 = 0x0000000000000080
)

type reasonType struct {
	description string
	hwSlowdown  bool
}

// ref. https://github.com/NVIDIA/go-nvml/blob/main/gen/nvml/nvml.h
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html
var clockEventReasonsToInclude = map[uint64]reasonType{
	// ref. nvmlClocksEventReasonGpuIdle
	0x0000000000000001: {
		description: "GPU is idle and clocks are dropping to Idle state",
		hwSlowdown:  false,
	},

	// ref. nvmlClocksEventReasonApplicationsClocksSetting
	0x0000000000000002: {
		description: "GPU clocks are limited by current setting of applications clocks",
		hwSlowdown:  false,
	},

	// ref. nvmlClocksEventReasonSwPowerCap
	0x0000000000000004: {
		description: "Clocks have been optimized to not exceed currently set power limits ('SW Power Cap: Active' in nvidia-smi --query)",
		hwSlowdown:  false,
	},

	// ref. nvmlClocksThrottleReasonHwSlowdown
	reasonHWSlowdown: {
		description: "HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw ('HW Slowdown: Active' in nvidia-smi --query)",
		hwSlowdown:  true,
	},

	// ref. nvmlClocksEventReasonSyncBoost
	0x0000000000000010: {
		description: "GPU is part of a Sync boost group to maximize performance per watt",
		hwSlowdown:  false,
	},

	// ref. nvmlClocksEventReasonSwThermalSlowdown
	0x0000000000000020: {
		description: "SW Thermal Slowdown is active to keep GPU and memory temperatures within operating limits",
		hwSlowdown:  false,
	},

	// ref. nvmlClocksThrottleReasonHwThermalSlowdown
	reasonHWSlowdownThermal: {
		description: "HW Thermal Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (temperature being too high) ('HW Thermal Slowdown' in nvidia-smi --query)",
		hwSlowdown:  true,
	},

	// ref. nvmlClocksThrottleReasonHwPowerBrakeSlowdown
	reasonHWSlowdownPowerBrake: {
		description: "HW Power Brake Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (External Power Brake Assertion being triggered) ('HW Power Brake Slowdown' in nvidia-smi --query)",
		hwSlowdown:  true,
	},

	// ref. nvmlClocksEventReasonDisplayClockSetting
	0x0000000000000100: {
		description: "GPU clocks are limited by current setting of Display clocks",
		hwSlowdown:  false,
	},
}

func (inst *instance) RecvClockEvents() <-chan *ClockEvents {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	if inst.nvmlLib == nil {
		return nil
	}

	return inst.clockEventsCh
}

func (inst *instance) pollClockEvents() {
	log.Logger.Debugw("polling clock events")

	for {
		select {
		case <-inst.rootCtx.Done():
			return
		default:
		}
	}
}
