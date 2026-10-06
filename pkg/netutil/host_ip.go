package netutil

import (
	"fmt"
	"net"
	"net/netip"

	utilnet "k8s.io/apimachinery/pkg/util/net"
)

// DefaultRouteHostIPv4 returns the IPv4 address kubelet would choose as the
// node InternalIP when no address is configured: the global unicast address on
// the interface holding the default route (k8s.io/apimachinery ChooseHostInterface).
// On hosts without /proc/net/route (e.g., macOS), ChooseHostInterface falls back
// to the first up, non-loopback interface with a global unicast address.
//
// The address is not required to be RFC1918; bare-metal hosts commonly use
// non-RFC1918 internal ranges (e.g., 7.0.0.0/8) for the host network.
func DefaultRouteHostIPv4() (InterfaceAddr, error) {
	ip, err := utilnet.ChooseHostInterface()
	if err != nil {
		return InterfaceAddr{}, err
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return InterfaceAddr{}, fmt.Errorf("invalid host IP %q", ip)
	}
	addr = addr.Unmap()
	if !addr.Is4() {
		return InterfaceAddr{}, fmt.Errorf("host IP %s is not IPv4", addr)
	}

	return InterfaceAddr{Iface: interfaceWithAddr(addr), Addr: addr}, nil
}

// interfaceWithAddr returns the interface that holds addr, or a zero
// interface if none matches.
func interfaceWithAddr(addr netip.Addr) net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ip, ok := convertNetAddr(a); ok && ip == addr {
				return iface
			}
		}
	}
	return net.Interface{}
}
