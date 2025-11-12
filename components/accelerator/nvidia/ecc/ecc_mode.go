package ecc

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
)

type ECCMode struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// BusID is the GPU bus ID from the nvml API.
	//  e.g., "0000:0f:00.0"
	BusID string `json:"bus_id"`

	EnabledCurrent bool `json:"enabled_current"`

	// "pending" ECC mode refers to the target mode following the next reboot.
	EnabledPending bool `json:"enabled_pending"`

	// Supported is true if the ECC mode is supported by the device.
	Supported bool `json:"supported"`
}

// Returns the current and pending ECC modes.
// "pending" ECC mode refers to the target mode following the next reboot.
func GetECCModeEnabled(uuid string, dev device.Device) (ECCMode, error) {
	result := ECCMode{
		UUID:      uuid,
		BusID:     dev.PCIBusID(),
		Supported: true,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1gbf6a8f2d0ed49e920e8ec20365381100
	current, pending, ret := dev.GetEccMode()
	if nvmlerrors.IsNotSupportError(ret) {
		result.Supported = false
		return result, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return result, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return result, nvmlerrors.ErrGPURequiresReset
	}
	// not a "not supported" error, not a success return, thus return an error here
	if ret != nvml.SUCCESS {
		return result, fmt.Errorf("failed to get current/pending ecc mode: %s", nvml.ErrorString(ret))
	}

	result.EnabledCurrent = current == nvml.FEATURE_ENABLED
	result.EnabledPending = pending == nvml.FEATURE_ENABLED

	return result, nil
}
