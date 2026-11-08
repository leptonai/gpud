package product

import "strings"

type capabilities struct {
	memoryManagement MemoryErrorManagementCapabilities
	fabricManager    bool
	fabricState      bool
	externalFabric   bool
}

// Keep memory recovery, fabric telemetry, and management topology in one
// product registry so additions can be reviewed and tested together.
// Memory features: https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/latest/supported-gpus.html
// HGX/DGX fabric: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html
// Rack fabric: https://docs.nvidia.com/dgx/dgxgb200-user-guide/software.html
var (
	memMgmtCapAllSupported = MemoryErrorManagementCapabilities{
		ErrorContainment: true, DynamicPageOfflining: true, RowRemapping: true,
	}
	gpuProductCapabilities = map[string]capabilities{
		"a10":  {memoryManagement: MemoryErrorManagementCapabilities{RowRemapping: true}},
		"a100": {memoryManagement: memMgmtCapAllSupported, fabricManager: true},
		"h100": {memoryManagement: memMgmtCapAllSupported, fabricManager: true, fabricState: true},
		"h200": {memoryManagement: memMgmtCapAllSupported, fabricManager: true, fabricState: true},
		// Preserve memory support without treating standalone Grace Hopper as HGX H200.
		"gh200": {memoryManagement: memMgmtCapAllSupported},
		"b100":  {memoryManagement: memMgmtCapAllSupported, fabricManager: true, fabricState: true},
		"b200":  {memoryManagement: memMgmtCapAllSupported, fabricManager: true, fabricState: true},
		"b300":  {memoryManagement: memMgmtCapAllSupported, fabricManager: true, fabricState: true},
		// NVL72 fabric services run on switch trays, not on compute nodes.
		"gb200": {memoryManagement: memMgmtCapAllSupported, fabricState: true, externalFabric: true},
		"gb300": {memoryManagement: memMgmtCapAllSupported, fabricState: true, externalFabric: true},
	}
)

func capabilitiesByGPUProduct(gpuProductName string) capabilities {
	p := strings.ToLower(gpuProductName)
	longestName := ""
	var caps capabilities
	for name, candidate := range gpuProductCapabilities {
		if len(name) > len(longestName) && strings.Contains(p, name) {
			longestName, caps = name, candidate
		}
	}
	// PCIe variants retain memory recovery but do not use NVSwitch fabrics.
	if strings.Contains(p, "pcie") {
		caps.fabricManager, caps.fabricState, caps.externalFabric = false, false, false
	}
	return caps
}

// SupportedFMByGPUProduct reports support for the on-node nv-fabricmanager
// daemon. This is separate from NVML fabric telemetry on NVOS-managed racks.
func SupportedFMByGPUProduct(gpuProductName string) bool {
	return capabilitiesByGPUProduct(gpuProductName).fabricManager
}

// SupportFabricStateByGPUProduct reports support for NVML GPU fabric telemetry.
func SupportFabricStateByGPUProduct(gpuProductName string) bool {
	return capabilitiesByGPUProduct(gpuProductName).fabricState
}

// HasExternalFabricByGPUProduct reports whether fabric switches and management
// reside outside the compute node, so local PCI enumeration is not a prerequisite
// for querying NVML fabric telemetry.
func HasExternalFabricByGPUProduct(gpuProductName string) bool {
	return capabilitiesByGPUProduct(gpuProductName).externalFabric
}

// SupportedMemoryMgmtCapsByGPUProduct returns memory error management capabilities.
// Product policy is intentional: some unsupported GPUs return successful NVML
// remapped-row queries, so API success alone is not sufficient feature detection.
func SupportedMemoryMgmtCapsByGPUProduct(gpuProductName string) MemoryErrorManagementCapabilities {
	return capabilitiesByGPUProduct(gpuProductName).memoryManagement
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
