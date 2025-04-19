package query

import (
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"

	"k8s.io/apimachinery/pkg/api/resource"
)

// GetSystemResourceLogicalCores returns the system GPU resource of the machine
// with the GPU count, using the type defined in "corev1.ResourceName"
// in https://pkg.go.dev/k8s.io/api/core/v1#ResourceName.
// It represents the GPU count with the key "nvidia.com/gpu" or "nvidia.com/gpu.count".
// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
//
// This is different than the device count in DCGM.
// ref. "CountDevEntry" in "nvvs/plugin_src/software/Software.cpp"
// ref. https://github.com/NVIDIA/DCGM/blob/903d745504f50153be8293f8566346f9de3b3c93/nvvs/plugin_src/software/Software.cpp#L220-L249
func GetSystemResourceGPUCount() (string, error) {
	nvmlInstance, err := nvidianvml.NewInstanceV2()
	if err != nil {
		return "", err
	}
	defer func() {
		if err := nvmlInstance.Shutdown(); err != nil {
			log.Logger.Warnw("failed to shutdown nvml instance", "error", err)
		}
	}()

	deviceCount := len(nvmlInstance.Devices())

	qty := resource.NewQuantity(int64(deviceCount), resource.DecimalSI)
	return qty.String(), nil
}
