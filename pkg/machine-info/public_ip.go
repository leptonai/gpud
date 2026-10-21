// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package machineinfo

import "net"

// An external IP echo is also a NAT/proxy egress address. It can identify the
// provider's ASN without identifying a public address assigned to this host.
func assignedPublicIP(value string) string {
	ip := net.ParseIP(value)
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return ""
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return "" // RFC 6598 shared space is not an assigned public address.
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		var assigned net.IP
		switch a := addr.(type) {
		case *net.IPNet:
			assigned = a.IP
		case *net.IPAddr:
			assigned = a.IP
		}
		if assigned.Equal(ip) {
			return value
		}
	}
	return ""
}
