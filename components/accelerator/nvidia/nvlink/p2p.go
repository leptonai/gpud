package nvlink

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

const (
	p2pStatusOK                   = "OK"
	p2pStatusChipsetNotSupported  = "CNS"
	p2pStatusGPUNotSupported      = "GNS"
	p2pStatusTopologyNotSupported = "TNS"
	p2pStatusDisabledByRegkey     = "DR"
	p2pStatusNotSupported         = "NS"
	p2pStatusUnknown              = "U"
)

func getPeerNVLinkP2PStatus(dev device.Device, peer device.Device) (string, error) {
	// WHY: this NVML query is the programmatic equivalent of
	// `nvidia-smi topo -p2p n` for NVLink capability between two GPUs. We use it
	// because operators diagnose the broken-topology case from that command's
	// `OK`/`NS` matrix, while per-link NVLink port state alone can miss it.
	status, ret := dev.GetP2PStatus(peer, nvml.P2P_CAPS_INDEX_NVLINK)
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get nvlink p2p status: %s", nvml.ErrorString(ret))
	}
	return toP2PStatusCode(status), nil
}

func toP2PStatusCode(status nvml.GpuP2PStatus) string {
	switch status {
	case nvml.P2P_STATUS_OK:
		return p2pStatusOK
	case nvml.P2P_STATUS_CHIPSET_NOT_SUPPORTED:
		return p2pStatusChipsetNotSupported
	case nvml.P2P_STATUS_GPU_NOT_SUPPORTED:
		return p2pStatusGPUNotSupported
	case nvml.P2P_STATUS_IOH_TOPOLOGY_NOT_SUPPORTED:
		return p2pStatusTopologyNotSupported
	case nvml.P2P_STATUS_DISABLED_BY_REGKEY:
		return p2pStatusDisabledByRegkey
	case nvml.P2P_STATUS_NOT_SUPPORTED:
		return p2pStatusNotSupported
	default:
		return p2pStatusUnknown
	}
}
