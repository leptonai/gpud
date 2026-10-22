package machineinfo

import (
	"net/netip"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
)

// SelectPrivateIP returns the private IP to report for this machine.
//
// Precedence:
//  1. providerPrivateIP from provider metadata (IMDS), reported as-is.
//  2. the RFC1918 IPv4 on the default-route interface, the address kubelet
//     picks as the node InternalIP. This wins over other NICs because hosts
//     can carry RFC1918 addresses on non-LAN links that nicInfo keeps on
//     purpose (e.g., the lepton0 dummy link holding the node cluster IP, which
//     sorts before a 10.78.x LAN address).
//  3. the first RFC1918 IPv4 in nicInfo (overlay/CNI interfaces are already
//     excluded by GetMachineNICInfo). This covers hosts whose only RFC1918
//     address is on a non-default-route NIC (e.g., eni*/lepton*).
//  4. the non-RFC1918 IPv4 on the default-route interface. This covers
//     bare-metal hosts without IMDS whose host network uses non-RFC1918
//     internal ranges (e.g., 7.0.0.0/8), which step 3 cannot see.
//  5. empty.
//
// publicIP is never returned from steps 2 or 4.
func SelectPrivateIP(providerPrivateIP string, publicIP string, nicInfo *apiv1.MachineNICInfo) string {
	return selectPrivateIP(providerPrivateIP, publicIP, nicInfo, netutil.DefaultRouteHostIPv4)
}

func selectPrivateIP(
	providerPrivateIP string,
	publicIP string,
	nicInfo *apiv1.MachineNICInfo,
	getDefaultRouteHostIPv4Func func() (netutil.InterfaceAddr, error),
) string {
	if providerPrivateIP != "" {
		return providerPrivateIP
	}

	hostIP, hostIPOK := usableDefaultRouteHostIPv4(publicIP, getDefaultRouteHostIPv4Func)
	if hostIPOK && hostIP.Addr.IsPrivate() {
		log.Logger.Infow("provider private IP not available, using RFC1918 default-route host IP",
			"ip", hostIP.Addr.String(),
			"interface", hostIP.Iface.Name,
			"reason", "interface holds the default route (kubelet node InternalIP choice)")
		return hostIP.Addr.String()
	}

	if nicInfo != nil {
		for _, iface := range nicInfo.PrivateIPInterfaces {
			if iface.IP == "" {
				continue
			}
			if iface.Addr.IsPrivate() && iface.Addr.Is4() {
				log.Logger.Infow("provider private IP not available, using local network interface", "ip", iface.IP, "interface", iface.Interface)
				return iface.IP
			}
		}
	}

	if !hostIPOK {
		return ""
	}
	log.Logger.Infow("provider private IP and RFC1918 interface IP not available, using default-route host IP",
		"ip", hostIP.Addr.String(),
		"interface", hostIP.Iface.Name,
		"reason", "interface holds the default route (kubelet node InternalIP choice)")
	return hostIP.Addr.String()
}

// usableDefaultRouteHostIPv4 returns the default-route host IPv4 when it is a
// usable unicast address distinct from the public IP.
func usableDefaultRouteHostIPv4(
	publicIP string,
	getDefaultRouteHostIPv4Func func() (netutil.InterfaceAddr, error),
) (netutil.InterfaceAddr, bool) {
	if getDefaultRouteHostIPv4Func == nil {
		return netutil.InterfaceAddr{}, false
	}
	hostIP, err := getDefaultRouteHostIPv4Func()
	if err != nil {
		log.Logger.Warnw("failed to find default-route host IP", "error", err)
		return netutil.InterfaceAddr{}, false
	}
	addr := hostIP.Addr
	if !addr.Is4() || addr.IsUnspecified() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		log.Logger.Warnw("ignoring unusable default-route host IP", "ip", addr.String(), "interface", hostIP.Iface.Name)
		return netutil.InterfaceAddr{}, false
	}
	if pub, err := netip.ParseAddr(publicIP); err == nil && pub.Unmap() == addr {
		log.Logger.Warnw("ignoring default-route host IP equal to public IP", "ip", addr.String(), "interface", hostIP.Iface.Name)
		return netutil.InterfaceAddr{}, false
	}
	return hostIP, true
}
