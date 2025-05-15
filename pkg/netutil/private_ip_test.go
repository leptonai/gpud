package netutil

import (
	"bytes"
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPrivateIPsWithFilter(t *testing.T) {
	ips, err := GetPrivateIPs()

	// We should not get an error
	assert.NoError(t, err)

	// We should get some IPs, but since this depends on the machine running the test,
	// we can only make general assertions
	for _, ip := range ips {
		// Check that the interface name is not empty
		assert.NotEmpty(t, ip.Iface.Name)

		// Check that the IP address is valid
		assert.True(t, ip.Addr.IsValid())

		// Check that it's either a private IPv4 or IPv6 address
		if ip.Addr.Is4() {
			assert.True(t, ip.Addr.IsPrivate(), "IPv4 address should be private: %s", ip.Addr)
		} else if ip.Addr.Is6() {
			assert.True(t, ip.Addr.IsPrivate() || ip.Addr.IsLinkLocalUnicast(),
				"IPv6 address should be private or link-local: %s", ip.Addr)
		}
	}
}

func TestGetPrivateIPs(t *testing.T) {
	ips, err := GetPrivateIPs(WithPrefixToSkip("lo"), WithPrefixToSkip("docker"))

	// We should not get an error
	assert.NoError(t, err)

	// Check that we filter out common interface types
	for _, ip := range ips {
		// Check that loopback interfaces are filtered out
		assert.False(t, ip.Addr.IsLoopback(), "Loopback addresses should be filtered out")

		// Check that interfaces with common prefixes are filtered out
		for _, prefix := range []string{"lo", "docker"} {
			assert.False(t,
				strings.HasPrefix(ip.Iface.Name, prefix),
				"Interface with prefix %s should be filtered out", prefix)
		}
	}
}

func TestConvertNetAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     net.Addr
		wantAddr netip.Addr
		wantOk   bool
	}{
		{
			name:     "IPv4 net.IPNet",
			addr:     &net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
			wantAddr: netip.MustParseAddr("192.168.1.1"),
			wantOk:   true,
		},
		{
			name:     "IPv6 net.IPNet",
			addr:     &net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
			wantAddr: netip.MustParseAddr("2001:db8::1"),
			wantOk:   true,
		},
		{
			name:     "IPv4 net.IPAddr",
			addr:     &net.IPAddr{IP: net.ParseIP("10.0.0.1")},
			wantAddr: netip.MustParseAddr("10.0.0.1"),
			wantOk:   true,
		},
		{
			name:     "IPv6 net.IPAddr",
			addr:     &net.IPAddr{IP: net.ParseIP("fd00::1")},
			wantAddr: netip.MustParseAddr("fd00::1"),
			wantOk:   true,
		},
		{
			name:     "IPv4-mapped IPv6 address",
			addr:     &net.IPAddr{IP: net.ParseIP("::ffff:192.0.2.1")},
			wantAddr: netip.MustParseAddr("192.0.2.1"),
			wantOk:   true,
		},
		{
			name:     "Unsupported address type",
			addr:     &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 8080},
			wantAddr: netip.Addr{},
			wantOk:   false,
		},
		{
			name:     "nil address",
			addr:     nil,
			wantAddr: netip.Addr{},
			wantOk:   false,
		},
		{
			name:     "Invalid IP address",
			addr:     &net.IPAddr{IP: []byte{1, 2, 3}}, // Invalid IP (too short)
			wantAddr: netip.Addr{},
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAddr, gotOk := convertNetAddr(tt.addr)
			assert.Equal(t, tt.wantOk, gotOk, "convertNetAddr() returned unexpected ok value")
			if gotOk {
				assert.Equal(t, tt.wantAddr, gotAddr, "convertNetAddr() returned unexpected addr value")
			}
		})
	}
}

func TestInterfaceAddrsRenderTable(t *testing.T) {
	// Create test data
	addrs := InterfaceAddrs{
		{
			Iface: net.Interface{Name: "eth0", Index: 1},
			Addr:  netip.MustParseAddr("192.168.1.1"),
		},
		{
			Iface: net.Interface{Name: "wlan0", Index: 2},
			Addr:  netip.MustParseAddr("10.0.0.1"),
		},
		{
			Iface: net.Interface{Name: "eth1", Index: 3},
			Addr:  netip.MustParseAddr("fd00::1"),
		},
	}

	// Create a buffer to capture output
	buf := new(bytes.Buffer)

	// Call the RenderTable method
	addrs.RenderTable(buf)

	// Check output contains expected data
	output := buf.String()
	assert.Contains(t, output, "INTERFACE NAME")
	assert.Contains(t, output, "ADDRESS")
	assert.Contains(t, output, "eth0")
	assert.Contains(t, output, "192.168.1.1")
	assert.Contains(t, output, "wlan0")
	assert.Contains(t, output, "10.0.0.1")
	assert.Contains(t, output, "eth1")
	assert.Contains(t, output, "fd00::1")
}
