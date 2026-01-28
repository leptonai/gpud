package persistencemode

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

// PersistenceMode is the persistence mode of the device.
// Implements "DCGM_FR_PERSISTENCE_MODE" in DCGM.
// ref. https://github.com/NVIDIA/DCGM/blob/903d745504f50153be8293f8566346f9de3b3c93/nvvs/plugin_src/software/Software.cpp#L526-L553
//
// Persistence mode controls whether the NVIDIA driver stays loaded when no active clients are connected to the GPU.
// ref. https://developer.nvidia.com/management-library-nvml
//
// Once all clients have closed the device file, the GPU state will be unloaded unless persistence mode is enabled.
// ref. https://docs.nvidia.com/deploy/driver-persistence/index.html
//
// NVIDIA Persistence Daemon provides a more robust implementation of persistence mode on Linux.
// ref. https://docs.nvidia.com/deploy/driver-persistence/index.html#usage
//
// To enable persistence mode, we need to check if "nvidia-persistenced" is running.
// Or run "nvidia-smi -pm 1" to enable persistence mode.
type PersistenceMode struct {
	UUID    string `json:"uuid"`
	BusID   string `json:"bus_id"`
	Enabled bool   `json:"enabled"`
	// Supported is true if the persistence mode is supported by the device.
	Supported bool `json:"supported"`
}

func GetPersistenceMode(uuid string, dev device.Device) (PersistenceMode, error) {
	mode := PersistenceMode{
		UUID:      uuid,
		BusID:     dev.PCIBusID(),
		Supported: true,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g1224ad7b15d7407bebfff034ec094c6b
	pm, ret := dev.GetPersistenceMode()
	if nvmlerrors.IsNotSupportError(ret) {
		mode.Supported = false
		return mode, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return mode, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return mode, nvmlerrors.ErrGPURequiresReset
	}
	// not a "not supported" error, not a success return, thus return an error here
	if ret != nvml.SUCCESS {
		return mode, fmt.Errorf("failed to get device persistence mode: %v", nvml.ErrorString(ret))
	}
	mode.Enabled = pm == nvml.FEATURE_ENABLED

	return mode, nil
}
