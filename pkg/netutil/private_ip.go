package netutil

import (
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"

	"github.com/olekukonko/tablewriter"
)

// InterfaceAddrs is a slice of InterfaceAddr.
type InterfaceAddrs []InterfaceAddr

// RenderTable renders the InterfaceAddrs as a table.
func (addrs InterfaceAddrs) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Interface Name", "Address"})
	for _, addr := range addrs {
		table.Append([]string{addr.Iface.Name, addr.Addr.String()})
	}
	table.Render()
}

// InterfaceAddr represents a network interface and its associated InterfaceAddr address.
type InterfaceAddr struct {
	Iface net.Interface
	Addr  netip.Addr
}

// GetPrivateIPs finds private IP addresses using an optional interface filter.
// It returns a slice of IP structs, each containing interface and address information.
func GetPrivateIPs(opts ...OpOption) (InterfaceAddrs, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("error getting network interfaces: %w", err)
	}

	var addresses InterfaceAddrs
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			// skip interfaces that are down
			continue
		}

		// skip interfaces based on prefix
		shouldSkip := false
		for skipPrefix := range op.prefixesToSkip {
			if strings.HasPrefix(iface.Name, skipPrefix) {
				shouldSkip = true
				break
			}
		}
		if shouldSkip {
			continue
		}

		// skip interfaces based on suffix
		for skipSuffix := range op.suffixesToSkip {
			if strings.HasSuffix(iface.Name, skipSuffix) {
				shouldSkip = true
				break
			}
		}
		if shouldSkip {
			continue
		}

		// list addresses for this interface
		addrs, err := iface.Addrs()
		if err != nil {
			// skip interfaces where we can't get addresses
			continue
		}

		// first try to find a private IPv4 address
		// matches: 10.*.*.*, 172.16-31.*.*, 192.168.*.*
		for _, addr := range addrs {
			ip, ok := convertNetAddr(addr)
			if !ok || !ip.IsValid() || ip.IsLoopback() {
				continue
			}

			if ip.Is4() && ip.IsPrivate() {
				addresses = append(addresses, InterfaceAddr{
					Iface: iface,
					Addr:  ip,
				})
				break
			}
		}

		// if no IPv4 found, try to find a private/link-local IPv6 address
		// matches: fd00::/8 (ULA) or fe80::/10 (Link-Local)
		if len(addresses) == 0 || addresses[len(addresses)-1].Iface.Name != iface.Name {
			for _, addr := range addrs {
				ip, ok := convertNetAddr(addr)
				if !ok || !ip.IsValid() || ip.IsLoopback() {
					continue
				}

				if ip.Is6() && (ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
					addresses = append(addresses, InterfaceAddr{
						Iface: iface,
						Addr:  ip,
					})
					break
				}
			}
		}
	}

	return addresses, nil
}

// convertNetAddr converts a standard net.Addr to netip.Addr.
// Returns the IP and true if conversion was successful, or
// an invalid IP and false otherwise.
func convertNetAddr(addr net.Addr) (netip.Addr, bool) {
	var ip net.IP
	switch v := addr.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		return netip.Addr{}, false
	}

	ipAddr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return ipAddr.Unmap(), true
}
