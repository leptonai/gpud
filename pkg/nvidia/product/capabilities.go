package product

import "strings"

var (
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#supported-gpus
	memMgmtCapAllSupported = MemoryErrorManagementCapabilities{
		ErrorContainment:     true,
		DynamicPageOfflining: true,
		RowRemapping:         true,
	}
	memMgmtCapOnlyRowRemappingSupported = MemoryErrorManagementCapabilities{
		RowRemapping: true,
	}
	gpuProductToMemMgmtCaps = map[string]MemoryErrorManagementCapabilities{
		"a100":  memMgmtCapAllSupported,
		"b100":  memMgmtCapAllSupported,
		"b200":  memMgmtCapAllSupported,
		"gb200": memMgmtCapAllSupported,
		"h100":  memMgmtCapAllSupported,
		"h200":  memMgmtCapAllSupported,
		"a10":   memMgmtCapOnlyRowRemappingSupported,
	}

	gpuProductToFMSupported = map[string]bool{
		"a100": true,
		"b100": true,
		"b200": true,
		// GB200 compute nodes rely on NVOS-managed fabric services that run on
		// NVLink Switch Trays, while the nodes themselves run NVSM. The traditional
		// nv-fabricmanager daemon (port 6666) never starts on the compute nodes and
		// returns NV_WARN_NOTHING_TO_DO because no NVSwitch kernel devices exist.
		// See NVIDIA NVOS, NMX Controller, and NVSM documentation referenced in the
		// reverted GB200 NVOS monitoring commit for architectural details. Since
		// SupportedFMByGPUProduct tracks support for the on-node nv-fabricmanager
		// service only, we explicitly report GB200 as unsupported here.
		"gb200": false,
		"h100":  true,
		"h200":  true,
		"a10":   false,
	}
)

// SupportedFMByGPUProduct returns the GPU fabric manager support status
// based on the GPU product name. This only reflects the legacy nv-fabricmanager
// daemon that listens on port 6666 on compute nodes and does not cover NVOS/
// NVSM-based fabric state telemetry available on newer systems like GB200.
func SupportedFMByGPUProduct(gpuProductName string) bool {
	p := strings.ToLower(gpuProductName)
	longestName, supported := "", false
	for k, v := range gpuProductToFMSupported {
		if !strings.Contains(p, k) {
			continue
		}
		if len(longestName) < len(k) {
			longestName = k
			supported = v
		}
	}
	return supported
}

// SupportFabricStateByGPUProduct reports whether the GPU surface exposes NVML
// fabric state telemetry (nvmlDeviceGetGpuFabricInfo*). This is available on
// Hopper + NVSwitch systems (e.g., H100, H200) where GPUs are registered with NVIDIA
// Fabric Manager, as well as on newer NVOS-managed systems like GB200.
//
// The nvmlDeviceGetGpuFabricInfo API is specifically designed for Hopper architecture GPUs
// with NVSwitch, allowing monitoring of GPU registration status with the NVLink fabric.
// GPU fabric registration status is exposed through the NVML APIs and nvidia-smi.
//
// References:
//   - NVML API: https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html
//     "On Hopper + NVSwitch systems, GPU is registered with the NVIDIA Fabric Manager.
//     This API reports the current state of the GPU in the NVLink fabric."
//   - Fabric Manager Guide: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html
//     Documents H100/H200 fabric state monitoring and registration process
func SupportFabricStateByGPUProduct(gpuProductName string) bool {
	p := strings.ToLower(gpuProductName)
	return strings.Contains(p, "gb200") ||
		strings.Contains(p, "h100") ||
		strings.Contains(p, "h200")
}

// SupportedMemoryMgmtCapsByGPUProduct returns the GPU memory error management capabilities
// based on the GPU product name.
// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#supported-gpus
func SupportedMemoryMgmtCapsByGPUProduct(gpuProductName string) MemoryErrorManagementCapabilities {
	p := strings.ToLower(gpuProductName)
	longestName, memCaps := "", MemoryErrorManagementCapabilities{}
	for k, v := range gpuProductToMemMgmtCaps {
		if !strings.Contains(p, k) {
			continue
		}
		if len(longestName) < len(k) {
			longestName = k
			memCaps = v
		}
	}
	return memCaps
}

// Contains information about the GPU's memory error management capabilities.
// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#supported-gpus
type MemoryErrorManagementCapabilities struct {
	// (If supported) GPU can limit the impact of uncorrectable ECC errors to GPU applications.
	// Existing/new workloads will run unaffected, both in terms of accuracy and performance.
	// Thus, does not require a GPU reset when memory errors occur.
	//
	// Note thtat there are some rarer cases, where uncorrectable errors are still uncontained
	// thus impacting all other workloads being procssed in the GPU.
	//
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#error-containments
	ErrorContainment bool `json:"error_containment"`

	// (If supported) GPU can dynamically mark the page containing uncorrectable errors
	// as unusable, and any existing or new workloads will not be allocating this page.
	//
	// Thus, does not require a GPU reset to recover from most uncorrectable ECC errors.
	//
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#dynamic-page-offlining
	DynamicPageOfflining bool `json:"dynamic_page_offlining"`

	// (If supported) GPU can replace degrading memory cells with spare ones
	// to avoid offlining regions of memory. And the row remapping is different
	// from dynamic page offlining which is fixed at a hardware level.
	//
	// The row remapping requires a GPU reset to take effect.
	//
	// Even for "NVIDIA GeForce RTX 4090", NVML API returns no error on the remapped rows API,
	// thus "NVML.Supported" is not a reliable way to check if row remapping is supported.
	// We track a separate boolean value based on the GPU product name.
	//
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#row-remapping
	RowRemapping bool `json:"row_remapping"`

	// Message contains the message to the user about the memory error management capabilities.
	Message string `json:"message,omitempty"`
}
