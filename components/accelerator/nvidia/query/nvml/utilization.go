package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
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

	// Percent of time over the past sample period during which one or more kernels was executing on the GPU.
	GPUUsedPercent uint32 `json:"gpu_used_percent"`
	// Percent of time over the past sample period during which global (device) memory was being read or written.
	MemoryUsedPercent uint32 `json:"memory_used_percent"`
}

func GetUtilization(uuid string, dev device.Device) (Utilization, error) {
	util := Utilization{
		UUID: uuid,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g540824faa6cef45500e0d1dc2f50b321
	rates, ret := dev.GetUtilizationRates()
	if ret != nvml.SUCCESS {
		return Utilization{}, fmt.Errorf("failed to get device utilization rates: %v", nvml.ErrorString(ret))
	}
	util.GPUUsedPercent = rates.Gpu
	util.MemoryUsedPercent = rates.Memory

	return util, nil
}
