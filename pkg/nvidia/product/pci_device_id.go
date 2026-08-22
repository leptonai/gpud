package product

import "strings"

// pciDeviceIDToProductName maps NVIDIA PCI device IDs (lowercase hex, no
// "0x" prefix) to the product names that NVML "nvmlDeviceGetName" returns
// for the same hardware.
//
// This is the fallback source for GPU product detection when NVML is
// unavailable -- e.g., when the NVIDIA driver is installed and managed by
// the NVIDIA GPU Operator, whose userspace libraries are injected only into
// GPU workload containers, or when the Operator-managed driver is still
// being installed during initial node discovery. PCI enumeration is done by
// the kernel and needs no NVIDIA driver, so these IDs are always readable
// from sysfs ("/sys/bus/pci/devices/<addr>/device").
//
// Keep the values identical to the NVML product names: downstream consumers
// (e.g., the Lepton control plane) match on the sanitized product name, and
// the value reported here must equal what the NVML code path would have
// reported once the driver becomes available.
//
// Device IDs follow the public PCI ID database (https://pci-ids.ucw.cz).
var pciDeviceIDToProductName = map[string]string{
	// Turing
	"1eb8": "Tesla T4", // TU104GL [Tesla T4]

	// Volta
	"1db1": "Tesla V100-SXM2-16GB", // GV100GL [Tesla V100 SXM2 16GB]
	"1db4": "Tesla V100-PCIE-16GB", // GV100GL [Tesla V100 PCIe 16GB]
	"1db5": "Tesla V100-SXM2-32GB", // GV100GL [Tesla V100 SXM2 32GB]
	"1db6": "Tesla V100-PCIE-32GB", // GV100GL [Tesla V100 PCIe 32GB]

	// Ampere
	"20b0": "NVIDIA A100-SXM4-40GB", // GA100 [A100 SXM4 40GB]
	"20b1": "NVIDIA A100-PCIE-40GB", // GA100 [A100 PCIe 40GB]
	"20f1": "NVIDIA A100-PCIE-40GB", // GA100 [A100 PCIe 40GB]
	"20b2": "NVIDIA A100-SXM4-80GB", // GA100 [A100 SXM4 80GB]
	"20b5": "NVIDIA A100 80GB PCIe", // GA100 [A100 PCIe 80GB]
	"20b7": "NVIDIA A30",            // GA100GL [A30 PCIe]
	"2235": "NVIDIA A40",            // GA102GL [A40]
	"2236": "NVIDIA A10",            // GA102GL [A10]
	"2237": "NVIDIA A10G",           // GA102GL [A10G]
	"2230": "NVIDIA RTX A6000",      // GA102GL [RTX A6000]
	"2231": "NVIDIA RTX A5000",      // GA102GL [RTX A5000]

	// Ada Lovelace
	"26b5": "NVIDIA L40",              // AD102GL [L40]
	"26b9": "NVIDIA L40S",             // AD102GL [L40S]
	"27b8": "NVIDIA L4",               // AD104GL [L4]
	"2684": "NVIDIA GeForce RTX 4090", // AD102 [GeForce RTX 4090]

	// Hopper
	"2330": "NVIDIA H100 80GB HBM3", // GH100 [H100 SXM5 80GB]
	"2331": "NVIDIA H100 PCIe",      // GH100 [H100 PCIe]
	"2321": "NVIDIA H100 NVL",       // GH100 [H100L 94GB]
	"2335": "NVIDIA H200",           // GH100 [H200 SXM 141GB]
	"233b": "NVIDIA H200 NVL",       // GH100 [H200 NVL]
	"2322": "NVIDIA H800 PCIe",      // GH100 [H800 PCIe]
	"2324": "NVIDIA H800 80GB HBM3", // GH100 [H800]
	"2329": "NVIDIA H20",            // GH100 [H20]

	// Blackwell
	"29bc": "NVIDIA B100",  // GB102 [B100]
	"2901": "NVIDIA B200",  // GB100 [B200]
	"2941": "NVIDIA GB200", // GB100 [HGX GB200]
	"31c2": "NVIDIA GB300", // GB110 [GB300]
	"31c3": "NVIDIA GB300", // GB110 [GB300]
}

// GetProductNameByPCIDeviceID returns the NVML-style product name for an
// NVIDIA PCI device ID, or an empty string when the device ID is not a
// known NVIDIA GPU. The input is normalized case-insensitively and may
// carry a "0x" prefix (as read from sysfs) or not.
func GetProductNameByPCIDeviceID(deviceID string) string {
	id := strings.ToLower(strings.TrimSpace(deviceID))
	id = strings.TrimPrefix(id, "0x")
	if id == "" {
		return ""
	}
	return pciDeviceIDToProductName[id]
}
