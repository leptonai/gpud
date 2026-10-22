// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package machineinfo

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers"
)

func TestProviderPublicIPDoesNotReportNATEgress(t *testing.T) {
	ipNet := func(value string) net.Addr { return &net.IPNet{IP: net.ParseIP(value), Mask: net.CIDRMask(32, 32)} }
	cases := []struct {
		name         string
		egress       string
		metadata     string
		imds         bool
		addrs        []net.Addr
		addressError error
		want         string
	}{
		{name: "NAT and overlay addresses are not public assignment", egress: "1.2.3.4", addrs: []net.Addr{ipNet("10.48.0.106"), ipNet("100.64.0.162"), ipNet("10.50.0.1")}},
		{name: "metadata address wins over different egress", egress: "9.9.9.9", metadata: "1.2.3.4", imds: true, addrs: []net.Addr{ipNet("10.0.0.1")}, want: "1.2.3.4"},
		{name: "assigned public IPv4 survives missing metadata", egress: "1.2.3.4", imds: true, addrs: []net.Addr{ipNet("1.2.3.4")}, want: "1.2.3.4"},
		{name: "assigned public IPv6 is retained", egress: "2001:4860:4860::8888", addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("2001:4860:4860::8888")}}, want: "2001:4860:4860::8888"},
		{name: "unavailable assignments remain unknown", egress: "1.2.3.4", addressError: errors.New("interface enumeration unavailable")},
		{name: "private echo is not public", egress: "10.48.0.106", addrs: []net.Addr{ipNet("10.48.0.106")}},
		{name: "shared CGNAT echo is not public", egress: "100.64.0.162", addrs: []net.Addr{ipNet("100.64.0.162")}},
		{name: "loopback echo is not public", egress: "127.0.0.1", addrs: []net.Addr{ipNet("127.0.0.1")}},
		{name: "link local echo is not public", egress: "169.254.1.1", addrs: []net.Addr{ipNet("169.254.1.1")}},
		{name: "invalid echo is not an assignment", egress: "not-an-address"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				mockey.Mock(net.InterfaceAddrs).To(func() ([]net.Addr, error) { return tc.addrs, tc.addressError }).Build()
				got := getProviderWithContext(context.Background(), tc.egress, func(context.Context) (*providers.Info, error) {
					return &providers.Info{Provider: "aws", PublicIP: tc.metadata, IMDSDetected: tc.imds}, nil
				})
				require.Equal(t, tc.want, got.PublicIP)
			})
		})
	}
}
