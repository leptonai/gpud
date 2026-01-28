package temperature

import (
	"fmt"
	"strconv"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type Temperature struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// BusID is the GPU bus ID from the nvml API.
	//  e.g., "0000:0f:00.0"
	BusID string `json:"bus_id"`

	CurrentCelsiusGPUCore uint32 `json:"current_celsius_gpu_core"`
	CurrentCelsiusHBM     uint32 `json:"current_celsius_hbm"`

	// HBMTemperatureSupported indicates whether NVML provided a memory temperature reading.
	HBMTemperatureSupported bool `json:"hbm_temperature_supported"`

	// ThresholdCelsiusSlowdownMargin is the thermal headroom (in °C) to the nearest
	// slowdown threshold as defined by NVML. NVML does not specify whether the threshold
	// is for GPU core or HBM; it is whichever slowdown threshold is nearest (driver-defined).
	ThresholdCelsiusSlowdownMargin int32 `json:"threshold_celsius_slowdown_margin"`

	// MarginTemperatureSupported indicates whether NVML provided a margin temperature reading.
	MarginTemperatureSupported bool `json:"margin_temperature_supported"`

	// Threshold at which the GPU starts to shut down to prevent hardware damage.
	ThresholdCelsiusShutdown uint32 `json:"threshold_celsius_shutdown"`
	// Threshold at which the GPU starts to throttle its performance.
	ThresholdCelsiusSlowdown uint32 `json:"threshold_celsius_slowdown"`
	// Maximum safe operating temperature for the GPU's memory.
	ThresholdCelsiusMemMax uint32 `json:"threshold_celsius_mem_max"`
	// Maximum safe operating temperature for the GPU core.
	ThresholdCelsiusGPUMax uint32 `json:"threshold_celsius_gpu_max"`

	UsedPercentShutdown string `json:"used_percent_shutdown"`
	UsedPercentSlowdown string `json:"used_percent_slowdown"`
	UsedPercentMemMax   string `json:"used_percent_mem_max"`
	UsedPercentGPUMax   string `json:"used_percent_gpu_max"`
}

func (temp Temperature) GetUsedPercentShutdown() (float64, error) {
	return strconv.ParseFloat(temp.UsedPercentShutdown, 64)
}

func (temp Temperature) GetUsedPercentSlowdown() (float64, error) {
	return strconv.ParseFloat(temp.UsedPercentSlowdown, 64)
}

func (temp Temperature) GetUsedPercentMemMax() (float64, error) {
	return strconv.ParseFloat(temp.UsedPercentMemMax, 64)
}

func (temp Temperature) GetUsedPercentGPUMax() (float64, error) {
	return strconv.ParseFloat(temp.UsedPercentGPUMax, 64)
}

// NVML_TEMPERATURE_MEM is not exposed in go-nvml v0.13.0-1 yet.
// Use the nvml.h enum value to query memory (HBM/GDDR) temperature when supported.
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g92d1c5182a14dd4be7090e3c1480b121
const temperatureSensorMemory nvml.TemperatureSensors = 1

func GetTemperature(uuid string, dev device.Device) (Temperature, error) {
	temp := Temperature{
		UUID:  uuid,
		BusID: dev.PCIBusID(),
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g92d1c5182a14dd4be7090e3c1480b121
	tempCur, ret := dev.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret == nvml.SUCCESS {
		temp.CurrentCelsiusGPUCore = tempCur
	} else {
		log.Logger.Warnw("failed to get device temperature", "error", nvml.ErrorString(ret))
		if nvmlerrors.IsGPULostError(ret) {
			return temp, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return temp, nvmlerrors.ErrGPURequiresReset
		}
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g92d1c5182a14dd4be7090e3c1480b121
	tempCurHBM, ret := dev.GetTemperature(temperatureSensorMemory)
	if ret == nvml.SUCCESS {
		temp.CurrentCelsiusHBM = tempCurHBM
		temp.HBMTemperatureSupported = true
	} else {
		if ret == nvml.ERROR_NOT_SUPPORTED || ret == nvml.ERROR_INVALID_ARGUMENT {
			log.Logger.Debugw("device HBM temperature not supported", "error", nvml.ErrorString(ret))
		} else {
			log.Logger.Warnw("failed to get device HBM temperature", "error", nvml.ErrorString(ret))
		}
		if nvmlerrors.IsGPULostError(ret) {
			return temp, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return temp, nvmlerrors.ErrGPURequiresReset
		}
	}

	// nvmlDeviceGetMarginTemperature returns the thermal margin (°C) to the nearest
	// slowdown threshold as defined by NVML. NVML does not specify GPU core vs HBM;
	// it is whichever slowdown threshold is nearest (driver-defined).
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g42db93dc04fc99d253eadc2037a5232d
	margin, ret := dev.GetMarginTemperature()
	if ret == nvml.SUCCESS {
		temp.ThresholdCelsiusSlowdownMargin = margin.MarginTemperature
		temp.MarginTemperatureSupported = true
	} else {
		if ret == nvml.ERROR_NOT_SUPPORTED || ret == nvml.ERROR_INVALID_ARGUMENT {
			log.Logger.Debugw("device margin temperature not supported", "error", nvml.ErrorString(ret))
		} else {
			log.Logger.Warnw("failed to get device margin temperature", "error", nvml.ErrorString(ret))
		}
		if nvmlerrors.IsGPULostError(ret) {
			return temp, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return temp, nvmlerrors.ErrGPURequiresReset
		}
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g271ba78911494f33fc079b204a929405
	tempLimitShutdown, ret := dev.GetTemperatureThreshold(nvml.TEMPERATURE_THRESHOLD_SHUTDOWN)
	if ret == nvml.SUCCESS {
		temp.ThresholdCelsiusShutdown = tempLimitShutdown
		if tempLimitShutdown > 0 {
			temp.UsedPercentShutdown = fmt.Sprintf("%.2f", float64(tempCur)/float64(tempLimitShutdown)*100)
		} else {
			temp.UsedPercentShutdown = "0.0"
		}
	} else {
		log.Logger.Warnw("failed to get device temperature shutdown limit", "error", nvml.ErrorString(ret))
		if nvmlerrors.IsGPULostError(ret) {
			return temp, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return temp, nvmlerrors.ErrGPURequiresReset
		}
		temp.UsedPercentShutdown = "0.0"
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g271ba78911494f33fc079b204a929405
	tempLimitSlowdown, ret := dev.GetTemperatureThreshold(nvml.TEMPERATURE_THRESHOLD_SLOWDOWN)
	if ret == nvml.SUCCESS {
		temp.ThresholdCelsiusSlowdown = tempLimitSlowdown
		if tempLimitSlowdown > 0 {
			temp.UsedPercentSlowdown = fmt.Sprintf("%.2f", float64(tempCur)/float64(tempLimitSlowdown)*100)
		} else {
			temp.UsedPercentSlowdown = "0.0"
		}
	} else {
		log.Logger.Warnw("failed to get device temperature slowdown limit", "error", nvml.ErrorString(ret))
		if nvmlerrors.IsGPULostError(ret) {
			return temp, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return temp, nvmlerrors.ErrGPURequiresReset
		}
		temp.UsedPercentSlowdown = "0.0"
	}

	// same logic as DCGM "VerifyHBMTemperature" that alerts  "DCGM_FR_TEMP_VIOLATION",
	// use "DCGM_FI_DEV_MEM_MAX_OP_TEMP" to get the max HBM temperature threshold "NVML_TEMPERATURE_THRESHOLD_MEM_MAX"
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g271ba78911494f33fc079b204a929405
	// ref. https://github.com/NVIDIA/DCGM/blob/a33560c9c138c617f3ee6cb50df11561302e5743/dcgmlib/src/DcgmCacheManager.cpp#L7738-L7767
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g271ba78911494f33fc079b204a929405
	tempLimitMemMax, ret := dev.GetTemperatureThreshold(nvml.TEMPERATURE_THRESHOLD_MEM_MAX)
	if ret == nvml.SUCCESS {
		temp.ThresholdCelsiusMemMax = tempLimitMemMax
		if tempLimitMemMax > 0 && temp.HBMTemperatureSupported {
			temp.UsedPercentMemMax = fmt.Sprintf("%.2f", float64(tempCurHBM)/float64(tempLimitMemMax)*100)
		} else {
			temp.UsedPercentMemMax = "0.0"
		}
	} else {
		log.Logger.Debugw("failed to get device temperature memory max limit", "error", nvml.ErrorString(ret))
		if nvmlerrors.IsGPULostError(ret) {
			return temp, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return temp, nvmlerrors.ErrGPURequiresReset
		}
		temp.UsedPercentMemMax = "0.0"
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g271ba78911494f33fc079b204a929405
	tempLimitGPUMax, ret := dev.GetTemperatureThreshold(nvml.TEMPERATURE_THRESHOLD_GPU_MAX)
	if ret == nvml.SUCCESS {
		temp.ThresholdCelsiusGPUMax = tempLimitGPUMax
		if tempLimitGPUMax > 0 {
			temp.UsedPercentGPUMax = fmt.Sprintf("%.2f", float64(tempCur)/float64(tempLimitGPUMax)*100)
		} else {
			temp.UsedPercentGPUMax = "0.0"
		}
	} else {
		log.Logger.Warnw("failed to get device temperature gpu max limit", "error", nvml.ErrorString(ret))
		if nvmlerrors.IsGPULostError(ret) {
			return temp, nvmlerrors.ErrGPULost
		}
		temp.UsedPercentGPUMax = "0.0"
	}

	return temp, nil
}
