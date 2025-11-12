package utilization

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
)

// Utilization represents the data from the nvmlDeviceGetUtilizationRates API.
// Utilization information for a device.
// Each sample period may be between 1 second and 1/6 second, depending on the product being queried.
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g540824faa6cef45500e0d1dc2f50b321
// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlUtilization__t.html#structnvmlUtilization__t
// c.f., "DCGM_FI_PROF_GR_ENGINE_ACTIVE" https://docs.nvidia.com/datacenter/dcgm/1.7/dcgm-api/group__dcgmFieldIdentifiers.html#group__dcgmFieldIdentifiers_1g5a93634d6e8574ab6af4bfab102709dc
type Utilization struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// BusID is the GPU bus ID from the nvml API.
	//  e.g., "0000:0f:00.0"
	BusID string `json:"bus_id"`

	// Percent of time over the past sample period during which one or more kernels was executing on the GPU.
	GPUUsedPercent uint32 `json:"gpu_used_percent"`
	// Percent of time over the past sample period during which global (device) memory was being read or written.
	MemoryUsedPercent uint32 `json:"memory_used_percent"`

	// Supported is true if the utilization is supported by the device.
	Supported bool `json:"supported"`
}

func GetUtilization(uuid string, dev device.Device) (Utilization, error) {
	util := Utilization{
		UUID:      uuid,
		BusID:     dev.PCIBusID(),
		Supported: true,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g540824faa6cef45500e0d1dc2f50b321
	rates, ret := dev.GetUtilizationRates()
	if nvmlerrors.IsNotSupportError(ret) {
		util.Supported = false
		return util, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return util, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return util, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
		return util, fmt.Errorf("failed to get device utilization rates: %v", nvml.ErrorString(ret))
	}
	util.GPUUsedPercent = rates.Gpu
	util.MemoryUsedPercent = rates.Memory

	return util, nil
}
