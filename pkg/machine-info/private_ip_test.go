package machineinfo

import (
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/netutil"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/providers"
)

func noDefaultRouteHostIPv4() (netutil.InterfaceAddr, error) {
	return netutil.InterfaceAddr{}, errors.New("no default route")
}

func defaultRouteHostIPv4(iface, ip string) func() (netutil.InterfaceAddr, error) {
	return func() (netutil.InterfaceAddr, error) {
		return netutil.InterfaceAddr{Iface: net.Interface{Name: iface}, Addr: netip.MustParseAddr(ip)}, nil
	}
}

func nic(name, ip string) apiv1.MachineNetworkInterface {
	return apiv1.MachineNetworkInterface{Interface: name, IP: ip, Addr: netip.MustParseAddr(ip)}
}

func TestSelectPrivateIP(t *testing.T) {
	tests := []struct {
		name              string
		providerPrivateIP string
		publicIP          string
		nics              []apiv1.MachineNetworkInterface
		defaultRoute      func() (netutil.InterfaceAddr, error)
		want              string
	}{
		{
			name:              "provider private IP wins over NIC and default route",
			providerPrivateIP: "172.16.0.10",
			publicIP:          "203.0.113.1",
			nics:              []apiv1.MachineNetworkInterface{nic("eth0", "10.0.0.5")},
			defaultRoute:      defaultRouteHostIPv4("bond0", "7.251.157.169"),
			want:              "172.16.0.10",
		},
		{
			name:         "RFC1918 NIC IPv4 wins over default route",
			publicIP:     "203.0.113.1",
			nics:         []apiv1.MachineNetworkInterface{nic("eth1", "fe80::1"), nic("eth0", "10.0.0.5")},
			defaultRoute: defaultRouteHostIPv4("bond0", "7.251.157.169"),
			want:         "10.0.0.5",
		},
		{
			// Mistral Compute BYOK: no IMDS; GetMachineNICInfo now drops cilium_host,
			// leaving only IPv6 link-local entries. The host IP is non-RFC1918.
			name:         "no RFC1918 NIC uses non-RFC1918 default-route host IP",
			publicIP:     "203.0.113.1",
			nics:         []apiv1.MachineNetworkInterface{nic("enP22s22f0np0", "fe80::a288:c2ff:fe3c:1")},
			defaultRoute: defaultRouteHostIPv4("bond0", "7.251.157.169"),
			want:         "7.251.157.169",
		},
		{
			name:         "nil NIC info uses default-route host IP",
			defaultRoute: defaultRouteHostIPv4("eth0", "192.0.2.10"),
			want:         "192.0.2.10",
		},
		{
			name:         "default-route host IP equal to public IP is rejected",
			publicIP:     "198.51.100.7",
			defaultRoute: defaultRouteHostIPv4("eth0", "198.51.100.7"),
			want:         "",
		},
		{
			name:         "loopback default-route host IP is rejected",
			defaultRoute: defaultRouteHostIPv4("lo", "127.0.0.2"),
			want:         "",
		},
		{
			name:         "link-local default-route host IP is rejected",
			defaultRoute: defaultRouteHostIPv4("eth0", "169.254.10.1"),
			want:         "",
		},
		{
			name:         "unspecified default-route host IP is rejected",
			defaultRoute: defaultRouteHostIPv4("eth0", "0.0.0.0"),
			want:         "",
		},
		{
			name:         "IPv6 default-route host IP is rejected",
			defaultRoute: defaultRouteHostIPv4("eth0", "2001:db8::1"),
			want:         "",
		},
		{
			name:         "default-route lookup error yields empty",
			defaultRoute: noDefaultRouteHostIPv4,
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectPrivateIP(tt.providerPrivateIP, tt.publicIP, &apiv1.MachineNICInfo{PrivateIPInterfaces: tt.nics}, tt.defaultRoute)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCreateLoginRequest_BareMetalUsesDefaultRouteHostIP covers the Mistral
// Compute BYOK shape end to end: no IMDS private IP, no RFC1918 address on a
// non-overlay NIC, and the host network on non-RFC1918 7.251.x via bond0.
func TestCreateLoginRequest_BareMetalUsesDefaultRouteHostIP(t *testing.T) {
	req, err := createLoginRequest(
		"token", "machine-id", "", "8", &mockNvmlInstance{},
		func() (string, error) { return "203.0.113.1", nil },
		func() *apiv1.MachineLocation { return &apiv1.MachineLocation{Region: "eu-west-3"} },
		func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				CPUInfo:    &apiv1.MachineCPUInfo{LogicalCores: 8},
				MemoryInfo: &apiv1.MachineMemoryInfo{TotalBytes: 1024},
				NICInfo: &apiv1.MachineNICInfo{PrivateIPInterfaces: []apiv1.MachineNetworkInterface{
					nic("enP22s22f0np0", "fe80::a288:c2ff:fe3c:1"),
				}},
			}, nil
		},
		func(ip string) *providers.Info {
			return &providers.Info{Provider: "mistral-compute-mistralcomputeholdingsas", PublicIP: ip}
		},
		func() (string, error) { return "100Gi", nil },
		func(nvidianvml.Instance) (string, error) { return "8", nil },
		defaultRouteHostIPv4("bond0", "7.251.157.169"),
	)
	require.NoError(t, err)
	assert.Equal(t, "7.251.157.169", req.Network.PrivateIP)
	assert.Equal(t, "203.0.113.1", req.Network.PublicIP)
}
