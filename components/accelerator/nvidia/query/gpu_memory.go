package query

import "strings"

// GetMemoryErrorManagementCapabilities returns the GPU memory error management capabilities
// based on the GPU product name.
// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#supported-gpus
func GetMemoryErrorManagementCapabilities(gpuProductName string) MemoryErrorManagementCapabilities {
	p := strings.ToLower(gpuProductName)
	switch {
	case strings.Contains(p, "h100"):
		return MemoryErrorManagementCapabilities{
			ErrorContainment:     true,
			DynamicPageOfflining: true,
			RowRemapping:         true,
		}

	case strings.Contains(p, "a100"):
		return MemoryErrorManagementCapabilities{
			ErrorContainment:     true,
			DynamicPageOfflining: true,
			RowRemapping:         true,
		}

	case strings.Contains(p, "a10"):
		return MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}

	default:
		return MemoryErrorManagementCapabilities{}
	}
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
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#row-remapping
	RowRemapping bool `json:"row_remapping"`
}
