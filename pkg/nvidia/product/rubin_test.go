package product

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Product names below are either the generic name observed on Rubin
// development drivers or the documented system names (DGX Vera Rubin NVL72,
// DGX Rubin NVL8); NVIDIA has not published NVML marketing names for Rubin.
func TestRubinCapabilities(t *testing.T) {
	const generic = "NVIDIA-Graphics-Device"
	for _, tc := range []struct {
		name, product string
		arch          string
		link          HostLink
		want          Capabilities
	}{
		{
			name: "C2C host link is an NVL72 compute tray", product: generic, arch: "rubin", link: HostLinkC2C,
			want: Capabilities{MemoryManagement: memMgmtCapAllSupported, FabricState: true, ExternalFabricCandidate: true, NonPCIFabricCandidate: true},
		},
		{
			name: "PCIe host link is an NVL8 baseboard", product: generic, arch: "rubin", link: HostLinkPCIe,
			want: Capabilities{MemoryManagement: memMgmtCapAllSupported, FabricState: true, FabricManagerIfFabric: true, NonPCIFabricCandidate: true},
		},
		{
			name: "no evidence claims neither topology", product: generic, arch: "rubin", link: HostLinkUnknown,
			want: Capabilities{MemoryManagement: memMgmtCapAllSupported, FabricState: true, NonPCIFabricCandidate: true},
		},
		{
			// Hardware evidence outranks a name; a C2C GPU cannot be an NVL8 baseboard.
			name: "host link outranks the name", product: "NVIDIA DGX Rubin NVL8", arch: "rubin", link: HostLinkC2C,
			want: Capabilities{MemoryManagement: memMgmtCapAllSupported, FabricState: true, ExternalFabricCandidate: true, NonPCIFabricCandidate: true},
		},
		{
			name: "explicit NVL72 name without evidence", product: "NVIDIA Vera Rubin NVL72",
			want: Capabilities{MemoryManagement: memMgmtCapAllSupported, FabricState: true, ExternalFabricCandidate: true, NonPCIFabricCandidate: true},
		},
		{
			name: "explicit NVL8 name without evidence", product: "NVIDIA DGX Rubin NVL8",
			want: Capabilities{MemoryManagement: memMgmtCapAllSupported, FabricState: true, FabricManagerIfFabric: true, NonPCIFabricCandidate: true},
		},
		{
			name: "named Rubin retains C2C with unknown architecture", product: "NVIDIA Rubin", arch: "undefined", link: HostLinkC2C,
			want: Capabilities{MemoryManagement: memMgmtCapAllSupported, FabricState: true, ExternalFabricCandidate: true, NonPCIFabricCandidate: true},
		},
		{
			name: "named Rubin retains PCIe with unknown architecture", product: "NVIDIA Rubin", arch: "undefined", link: HostLinkPCIe,
			want: Capabilities{MemoryManagement: memMgmtCapAllSupported, FabricState: true, FabricManagerIfFabric: true, NonPCIFabricCandidate: true},
		},
		{name: "generic name on another architecture", product: generic, arch: "blackwell", link: HostLinkC2C},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, CapabilitiesForGPU(tc.product, tc.arch, tc.link))
		})
	}

	// The InfiniBand NVL72 profile needs an explicit NVL72 name; a Rubin GPU
	// name does not identify the compute tray's NICs.
	require.True(t, HasExternalFabricByGPUProduct("NVIDIA Vera Rubin NVL72"))
	require.False(t, HasExternalFabricByGPUProduct("NVIDIA DGX Rubin NVL8"))
	require.False(t, HasExternalFabricByGPUProduct(generic))
}
