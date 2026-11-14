package product

import "strings"

type capabilities struct {
	memoryManagement        MemoryErrorManagementCapabilities
	fabricManager           bool
	fabricManagerIfFabric   bool
	fabricState             bool
	externalFabricCandidate bool
	switchBehindBridge      bool
}

// Capabilities describes GPU health features separately from fabric topology.
type Capabilities struct {
	MemoryManagement MemoryErrorManagementCapabilities
	// FabricManager requires the on-node nv-fabricmanager unconditionally.
	FabricManager bool
	// FabricManagerIfFabric requires the on-node nv-fabricmanager only after NVML
	// reports NVLink fabric telemetry. A product family alone cannot tell whether
	// the NVSwitches are on the local baseboard or in external switch trays.
	FabricManagerIfFabric   bool
	FabricState             bool
	ExternalFabricCandidate bool
	NonPCIFabricCandidate   bool
}

// HostLink is NVML evidence of how a GPU attaches to its host CPU.
// It separates superchip compute trays from x86 baseboards of the same GPU family.
type HostLink uint8

const (
	// HostLinkUnknown means NVML could not answer (older driver, missing symbol,
	// query failure, or a product-name override that deliberately bypasses hardware).
	HostLinkUnknown HostLink = iota
	// HostLinkPCIe means the GPU has no NVLink-C2C link to its CPU.
	HostLinkPCIe
	// HostLinkC2C means NVLink-C2C to a Grace or Vera CPU (a superchip).
	HostLinkC2C
)

// CapabilitiesForGPU uses architecture evidence when early drivers report a
// generic product name. A fabric candidate permits probing, not an NVL72 claim.
func CapabilitiesForGPU(name, architecture string, link HostLink) Capabilities {
	caps := capabilitiesByGPUProduct(name)
	if IsRubin(name, architecture) {
		// Either identity source establishes Rubin, but neither establishes topology.
		// Preserve measured C2C/PCIe evidence even when architecture is unavailable:
		// Vera Rubin uses C2C, while Rubin NVL8 uses an x86 host.
		// https://docs.nvidia.com/dgx/dgxvr-user-guide/software.html
		// https://resources.nvidia.com/en-us-dgx-software/dgx-rubin-nvl8-datasheet
		caps = rubinCapabilities(name, link)
	}
	return Capabilities{
		MemoryManagement:        caps.memoryManagement,
		FabricManager:           caps.fabricManager,
		FabricManagerIfFabric:   caps.fabricManagerIfFabric,
		FabricState:             caps.fabricState,
		ExternalFabricCandidate: caps.externalFabricCandidate,
		NonPCIFabricCandidate:   caps.externalFabricCandidate || caps.switchBehindBridge,
	}
}

// IsRubin identifies the family without inferring a rack size or product SKU.
func IsRubin(name, architecture string) bool {
	return strings.EqualFold(architecture, "rubin") || strings.Contains(strings.ToLower(name), "rubin")
}

// rubinCapabilities resolves Rubin fabric ownership from hardware evidence first
// and an explicit product name second.
//
// Every Rubin GPU supports row remapping (NVML: Ampere and newer) and NVML
// fabric telemetry, so those are enabled for the whole family. Fabric ownership
// differs by system and must not be guessed:
//   - DGX Vera Rubin NVL72 compute trays pair Rubin GPUs with Vera CPUs over
//     NVLink-C2C. FM, NVLSM and NMX-C run on NVOS switch trays, not the host.
//     https://docs.nvidia.com/dgx/dgxvr-user-guide/software.html
//   - DGX Rubin NVL8 uses Intel Xeon hosts (no C2C) and on-board NVSwitches.
//     https://resources.nvidia.com/en-us-dgx-software/dgx-rubin-nvl8-datasheet
//     NVIDIA documents host FM and NVLSM for the previous 8-GPU NVSwitch
//     baseboards, whose switches sit behind a CX7 bridge instead of host PCIe.
//     https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html#nvidia-hgx-b200-b300-gpu-baseboard
//     Rubin NVL8 FM placement is not documented yet, so the local daemon is
//     required only after NVML shows that a fabric exists, rather than for
//     every PCIe-attached Rubin GPU.
//   - With no evidence, probe NVML telemetry but claim neither topology. That
//     avoids both a false missing-FM failure on NVL72 and suppressed local-fault
//     handling on NVL8.
func rubinCapabilities(name string, link HostLink) capabilities {
	p := strings.ToLower(name)
	caps := capabilities{memoryManagement: memMgmtCapAllSupported, fabricState: true}
	switch {
	case link == HostLinkC2C,
		// Overrides and older drivers lack C2C evidence. "Vera" names the
		// superchip CPU, and NVL72 is the documented superchip rack.
		link == HostLinkUnknown && (strings.Contains(p, "vera") || strings.Contains(p, "nvl72")):
		caps.externalFabricCandidate = true
	case link == HostLinkPCIe,
		link == HostLinkUnknown && strings.Contains(p, "nvl8"):
		caps.switchBehindBridge = true
		caps.fabricManagerIfFabric = true
	default:
		// Switches are not host PCIe devices on either documented Rubin system,
		// so NVML must be probed before fabric telemetry is ruled out.
		caps.switchBehindBridge = true
	}
	return withoutNonSwitchFabric(name, caps)
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
		"b100":  {memoryManagement: memMgmtCapAllSupported, fabricManager: true, fabricState: true, switchBehindBridge: true},
		"b200":  {memoryManagement: memMgmtCapAllSupported, fabricManager: true, fabricState: true, switchBehindBridge: true},
		"b300":  {memoryManagement: memMgmtCapAllSupported, fabricManager: true, fabricState: true, switchBehindBridge: true},
		// GB families can use external NVL72 fabrics, but GB200 also has NVL4.
		"gb200": {memoryManagement: memMgmtCapAllSupported, fabricState: true, externalFabricCandidate: true},
		"gb300": {memoryManagement: memMgmtCapAllSupported, fabricState: true, externalFabricCandidate: true},
		// Rubin is resolved by rubinCapabilities: its topology depends on
		// host-link evidence that a product-name table cannot express.
	}
)

func capabilitiesByGPUProduct(gpuProductName string) capabilities {
	// Name-only callers (product-name overrides and name-based helpers) have no
	// host-link evidence; keep them on the same Rubin rules as the NVML path.
	if IsRubin(gpuProductName, "") {
		return rubinCapabilities(gpuProductName, HostLinkUnknown)
	}
	p := strings.ToLower(gpuProductName)
	longestName := ""
	var caps capabilities
	for name, candidate := range gpuProductCapabilities {
		if len(name) > len(longestName) && strings.Contains(p, name) {
			longestName, caps = name, candidate
		}
	}
	return withoutNonSwitchFabric(gpuProductName, caps)
}

func withoutNonSwitchFabric(name string, caps capabilities) capabilities {
	p := strings.ToLower(name)
	// PCIe and NVL4 variants retain memory recovery without NVSwitch fabrics.
	// https://docs.nvidia.com/multi-node-nvlink-systems/nvfupd-guide/examples/gb200NVL.html
	if strings.Contains(p, "pcie") || strings.Contains(p, "nvl4") {
		caps.fabricManager, caps.fabricManagerIfFabric, caps.fabricState, caps.externalFabricCandidate, caps.switchBehindBridge = false, false, false, false, false
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

// HasExternalFabricByGPUProduct requires an explicit NVL72 topology in the name.
// A bare GB200 name cannot distinguish NVL4 from an NVL72 compute tray.
func HasExternalFabricByGPUProduct(gpuProductName string) bool {
	return MayHaveExternalFabricByGPUProduct(gpuProductName) && strings.Contains(strings.ToLower(gpuProductName), "nvl72")
}

// MayHaveExternalFabricByGPUProduct permits probing rack telemetry without a
// local NVSwitch. It does not establish that the system has an external fabric.
func MayHaveExternalFabricByGPUProduct(gpuProductName string) bool {
	return capabilitiesByGPUProduct(gpuProductName).externalFabricCandidate
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

// MayHaveNonPCIFabricByGPUProduct allows NVML probing when switches are external
// or accessed through CX7 bridges rather than enumerated as host PCIe devices.
// https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html#nvidia-hgx-b200-b300-gpu-baseboard
func MayHaveNonPCIFabricByGPUProduct(name string) bool {
	caps := capabilitiesByGPUProduct(name)
	return caps.externalFabricCandidate || caps.switchBehindBridge
}
