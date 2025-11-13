package hwslowdown

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/error"
)

// Returns true if clock events is supported by this device.
func ClockEventsSupportedByDevice(dev device.Device) (bool, error) {
	// clock events are supported in versions 535 and above
	// otherwise, CGO call just exits with
	// undefined symbol: nvmlDeviceGetCurrentClocksEventReasons
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
	_, ret := dev.GetCurrentClocksEventReasons()
	if nvmlerrors.IsNotSupportError(ret) {
		return false, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return false, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return false, nvmlerrors.ErrGPURequiresReset
	}
	// not a "not supported" error, not a success return, thus return an error here
	if nvmlerrors.IsNotReadyError(ret) {
		return false, fmt.Errorf("device not initialized %v", nvml.ErrorString(ret))
	}
	// not a "not supported" error, not a success return, thus return an error here
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
	// Time is the time the metrics were collected.
	Time metav1.Time `json:"time"`

	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// BusID is the GPU bus ID from the nvml API.
	//  e.g., "0000:0f:00.0"
	BusID string `json:"bus_id"`

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

	// Supported is true if the clock events are supported by the device.
	Supported bool `json:"supported"`
}

// Event creates a apiv1.Event from ClockEvents if there are hardware slowdown reasons.
// Returns nil if there are no hardware slowdown reasons.
func (evs *ClockEvents) Event() *eventstore.Event {
	if len(evs.HWSlowdownReasons) == 0 {
		return nil
	}

	return &eventstore.Event{
		Time:    evs.Time.Time,
		Name:    "hw_slowdown",
		Type:    string(apiv1.EventTypeWarning),
		Message: strings.Join(evs.HWSlowdownReasons, ", "),
		ExtraInfo: map[string]string{
			"data_source": "nvml",
			"gpu_uuid":    evs.UUID,
		},
	}
}

func GetClockEvents(uuid string, dev device.Device) (ClockEvents, error) {
	clockEvents := ClockEvents{
		Time:      metav1.Time{Time: time.Now().UTC()},
		UUID:      uuid,
		BusID:     dev.PCIBusID(),
		Supported: true,
	}

	// clock events are supported in versions 535 and above
	// otherwise, CGO call just exits with
	// undefined symbol: nvmlDeviceGetCurrentClocksEventReasons
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
	reasons, ret := dev.GetCurrentClocksEventReasons()
	if nvmlerrors.IsNotSupportError(ret) {
		clockEvents.Supported = false
		return clockEvents, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return clockEvents, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return clockEvents, nvmlerrors.ErrGPURequiresReset
	}
	// not a "not supported" error, not a success return, thus return an error here
	if nvmlerrors.IsNotReadyError(ret) {
		return clockEvents, fmt.Errorf("device %s is not initialized %v", uuid, nvml.ErrorString(ret))
	}
	if ret != nvml.SUCCESS {
		return clockEvents, fmt.Errorf("failed to get device clock event reasons: %v", nvml.ErrorString(ret))
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html#group__nvmlClocksEventReasons
	clockEvents.ReasonsBitmask = reasons

	clockEvents.HWSlowdown = reasons&reasonHWSlowdown != 0
	clockEvents.HWSlowdownThermal = reasons&reasonHWSlowdownThermal != 0
	clockEvents.HWSlowdownPowerBrake = reasons&reasonHWSlowdownPowerBrake != 0

	hwReasons, otherReasons := getClockEventReasons(reasons)
	for _, reason := range hwReasons {
		clockEvents.HWSlowdownReasons = append(clockEvents.HWSlowdownReasons,
			fmt.Sprintf("%s: %s", uuid, reason))
	}
	for _, reason := range otherReasons {
		clockEvents.Reasons = append(clockEvents.Reasons,
			fmt.Sprintf("%s: %s", uuid, reason))
	}

	return clockEvents, nil
}

func getClockEventReasons(reasons uint64) ([]string, []string) {
	hwSlowdownReasons := make([]string, 0)
	otherReasons := make([]string, 0)

	for flag, rt := range clockEventReasonsToInclude {
		if reasons&flag != 0 {
			if rt.isHWSlowdown {
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
	reasonGPUIdle                   uint64 = 0x0000000000000001
	reasonApplicationsClocksSetting uint64 = 0x0000000000000002
	reasonSWPowerCap                uint64 = 0x0000000000000004
	reasonHWSlowdown                uint64 = 0x0000000000000008
	reasonSyncBoost                 uint64 = 0x0000000000000010
	reasonSwThermalSlowdown         uint64 = 0x0000000000000020
	reasonHWSlowdownThermal         uint64 = 0x0000000000000040
	reasonHWSlowdownPowerBrake      uint64 = 0x0000000000000080
	reasonDisplayClockSetting       uint64 = 0x0000000000000100
)

type reasonType struct {
	description  string
	isHWSlowdown bool
}

// ref. https://github.com/NVIDIA/go-nvml/blob/main/gen/nvml/nvml.h
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html
var clockEventReasonsToInclude = map[uint64]reasonType{
	// ref. nvmlClocksEventReasonGpuIdle
	reasonGPUIdle: {
		description:  "GPU is idle and clocks are dropping to Idle state",
		isHWSlowdown: false,
	},

	// ref. nvmlClocksEventReasonApplicationsClocksSetting
	reasonApplicationsClocksSetting: {
		description:  "GPU clocks are limited by current setting of applications clocks",
		isHWSlowdown: false,
	},

	// ref. nvmlClocksEventReasonSwPowerCap
	reasonSWPowerCap: {
		description:  "Clocks have been optimized to not exceed currently set power limits ('SW Power Cap: Active' in nvidia-smi --query)",
		isHWSlowdown: false,
	},

	// ref. nvmlClocksThrottleReasonHwSlowdown
	reasonHWSlowdown: {
		description:  "HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw ('HW Slowdown: Active' in nvidia-smi --query)",
		isHWSlowdown: true,
	},

	// ref. nvmlClocksEventReasonSyncBoost
	reasonSyncBoost: {
		description:  "GPU is part of a Sync boost group to maximize performance per watt",
		isHWSlowdown: false,
	},

	// ref. nvmlClocksEventReasonSwThermalSlowdown
	reasonSwThermalSlowdown: {
		description:  "SW Thermal Slowdown is active to keep GPU and memory temperatures within operating limits",
		isHWSlowdown: false,
	},

	// ref. nvmlClocksThrottleReasonHwThermalSlowdown
	reasonHWSlowdownThermal: {
		description:  "HW Thermal Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (temperature being too high) ('HW Thermal Slowdown' in nvidia-smi --query)",
		isHWSlowdown: true,
	},

	// ref. nvmlClocksThrottleReasonHwPowerBrakeSlowdown
	reasonHWSlowdownPowerBrake: {
		description:  "HW Power Brake Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (External Power Brake Assertion being triggered) ('HW Power Brake Slowdown' in nvidia-smi --query)",
		isHWSlowdown: true,
	},

	// ref. nvmlClocksEventReasonDisplayClockSetting
	reasonDisplayClockSetting: {
		description:  "GPU clocks are limited by current setting of Display clocks",
		isHWSlowdown: false,
	},
}
