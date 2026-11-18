package product

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRubinCapabilities(t *testing.T) {
	const generic = "NVIDIA-Graphics-Device"
	for _, tc := range []struct {
		name, product string
		arch          string
		link          HostLink
		want          Capabilities
	}{
		{
			name: "C2C host link is an external fabric candidate", product: generic, arch: "rubin", link: HostLinkC2C,
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, ExternalFabricCandidate: true, NonPCIFabricCandidate: true},
		},
		{
			name: "disabled C2C claims no local fabric", product: generic, arch: "rubin", link: HostLinkUnknown,
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, NonPCIFabricCandidate: true},
		},
		{
			name: "no evidence claims neither topology", product: generic, arch: "rubin", link: HostLinkUnknown,
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, NonPCIFabricCandidate: true},
		},
		{
			// Enabled C2C establishes candidacy independently of a marketing name.
			name: "host link outranks the name", product: "NVIDIA DGX Rubin NVL8", arch: "rubin", link: HostLinkC2C,
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, ExternalFabricCandidate: true, NonPCIFabricCandidate: true},
		},
		{
			name: "explicit NVL72 name without evidence", product: "NVIDIA Vera Rubin NVL72",
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, NonPCIFabricCandidate: true, ExplicitNVL72: true},
		},
		{
			name: "VR NVL72 driver name", product: "NVIDIA VR NVL72",
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, NonPCIFabricCandidate: true, ExplicitNVL72: true},
		},
		{
			name: "sanitized VR NVL72 driver name", product: "NVIDIA-VR-NVL72",
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, NonPCIFabricCandidate: true, ExplicitNVL72: true},
		},
		{
			name: "explicit NVL8 name without evidence", product: "NVIDIA DGX Rubin NVL8",
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, NonPCIFabricCandidate: true},
		},
		{
			name: "named Rubin retains C2C with unknown architecture", product: "NVIDIA Rubin", arch: "undefined", link: HostLinkC2C,
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, ExternalFabricCandidate: true, NonPCIFabricCandidate: true},
		},
		{
			name: "named Rubin retains PCIe with unknown architecture", product: "NVIDIA Rubin", arch: "undefined", link: HostLinkUnknown,
			want: Capabilities{Rubin: true, MemoryManagement: memMgmtCapAllSupported, FabricState: true, NonPCIFabricCandidate: true},
		},
		{name: "generic name on another architecture", product: generic, arch: "blackwell", link: HostLinkC2C},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, CapabilitiesForGPU(tc.product, tc.arch, tc.link))
		})
	}

	// Product names alone do not establish Rubin fabric ownership.
	require.False(t, HasExternalFabricByGPUProduct("NVIDIA Vera Rubin NVL72"))
	require.False(t, HasExternalFabricByGPUProduct("NVIDIA-VR-NVL72"))
	require.False(t, HasExternalFabricByGPUProduct("NVIDIA DGX Rubin NVL8"))
	require.False(t, HasExternalFabricByGPUProduct(generic))
}
