package query

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
		"a100": memMgmtCapAllSupported,
		"b100": memMgmtCapAllSupported,
		"b200": memMgmtCapAllSupported,
		"h100": memMgmtCapAllSupported,
		"h200": memMgmtCapAllSupported,
		"a10":  memMgmtCapOnlyRowRemappingSupported,
	}
)

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
	// even for "NVIDIA GeForce RTX 4090", nvml returns no error
	// thus "NVML.Supported" is not a reliable way to check if row remapping is supported
	// thus we track a separate boolean value based on the GPU product name
	//
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#row-remapping
	RowRemapping bool `json:"row_remapping"`

	// Message contains the message to the user about the memory error management capabilities.
	Message string `json:"message,omitempty"`
}
