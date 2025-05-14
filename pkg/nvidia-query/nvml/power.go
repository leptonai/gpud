package nvml

import (
	"fmt"
	"strconv"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Power struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	UsageMilliWatts           uint32 `json:"usage_milli_watts"`
	EnforcedLimitMilliWatts   uint32 `json:"enforced_limit_milli_watts"`
	ManagementLimitMilliWatts uint32 `json:"management_limit_milli_watts"`

	UsedPercent string `json:"used_percent"`

	GetPowerUsageSupported           bool `json:"get_power_usage_supported"`
	GetPowerLimitSupported           bool `json:"get_power_limit_supported"`
	GetPowerManagementLimitSupported bool `json:"get_power_management_limit_supported"`
}

func (power Power) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(power.UsedPercent, 64)
}

func GetPower(uuid string, dev device.Device) (Power, error) {
	power := Power{
		UUID: uuid,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7ef7dff0ff14238d08a19ad7fb23fc87
	powerUsage, ret := dev.GetPowerUsage()
	if IsNotSupportError(ret) {
		power.GetPowerUsageSupported = false
	} else if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
		if IsGPULostError(ret) {
			return power, ErrGPULost
		}
		return power, fmt.Errorf("failed to get device power usage: %v", nvml.ErrorString(ret))
	}
	power.UsageMilliWatts = powerUsage

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g263b5bf552d5ec7fcd29a088264d10ad
	enforcedPowerLimit, ret := dev.GetEnforcedPowerLimit()
	if IsNotSupportError(ret) {
		power.GetPowerLimitSupported = false
	} else if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
		if IsGPULostError(ret) {
			return power, ErrGPULost
		}
		return power, fmt.Errorf("failed to get device power limit: %v", nvml.ErrorString(ret))
	}
	power.EnforcedLimitMilliWatts = enforcedPowerLimit

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1gf754f109beca3a4a8c8c1cd650d7d66c
	managementPowerLimit, ret := dev.GetPowerManagementLimit()
	if IsNotSupportError(ret) {
		power.GetPowerManagementLimitSupported = false
	} else if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
		if IsGPULostError(ret) {
			return power, ErrGPULost
		}
		return power, fmt.Errorf("failed to get device power management limit: %v", nvml.ErrorString(ret))
	}
	power.ManagementLimitMilliWatts = managementPowerLimit

	total := enforcedPowerLimit
	if total == 0 {
		total = managementPowerLimit
	}
	if total > 0 {
		power.UsedPercent = fmt.Sprintf("%.2f", float64(power.UsageMilliWatts)/float64(total)*100)
	} else {
		power.UsedPercent = "0.0"
	}

	return power, nil
}
