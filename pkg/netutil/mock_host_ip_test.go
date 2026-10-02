package netutil

import (
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utilnet "k8s.io/apimachinery/pkg/util/net"
)

func mockHostInterfaces() {
	mockey.Mock(net.Interfaces).To(func() ([]net.Interface, error) {
		return []net.Interface{
			{Name: "broken", Flags: net.FlagUp},
			{Name: "cilium_host", Flags: net.FlagUp},
			{Name: "bond0", Flags: net.FlagUp},
		}, nil
	}).Build()
	mockey.Mock((*net.Interface).Addrs).To(func(ifi *net.Interface) ([]net.Addr, error) {
		switch ifi.Name {
		case "cilium_host":
			return []net.Addr{&net.IPNet{IP: net.ParseIP("10.128.20.49"), Mask: net.CIDRMask(32, 32)}}, nil
		case "bond0":
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("7.251.157.169"), Mask: net.CIDRMask(24, 32)},
			}, nil
		default:
			return nil, errors.New("addrs failed")
		}
	}).Build()
}

func TestDefaultRouteHostIPv4_WithMockey(t *testing.T) {
	mockey.PatchConvey("16-byte IPv4 from ChooseHostInterface is unmapped and resolved to its interface", t, func() {
		mockHostInterfaces()
		// ChooseHostInterface returns net.IP in 16-byte form for IPv4.
		mockey.Mock(utilnet.ChooseHostInterface).To(func() (net.IP, error) {
			return net.ParseIP("7.251.157.169"), nil
		}).Build()

		got, err := DefaultRouteHostIPv4()
		require.NoError(t, err)
		assert.Equal(t, netip.MustParseAddr("7.251.157.169"), got.Addr)
		assert.True(t, got.Addr.Is4())
		assert.Equal(t, "bond0", got.Iface.Name)
	})

	mockey.PatchConvey("address not on any interface still returns the IP", t, func() {
		mockHostInterfaces()
		mockey.Mock(utilnet.ChooseHostInterface).To(func() (net.IP, error) {
			return net.ParseIP("192.0.2.10").To4(), nil
		}).Build()

		got, err := DefaultRouteHostIPv4()
		require.NoError(t, err)
		assert.Equal(t, "192.0.2.10", got.Addr.String())
		assert.Empty(t, got.Iface.Name)
	})

	mockey.PatchConvey("interface listing failure still returns the IP", t, func() {
		mockey.Mock(net.Interfaces).To(func() ([]net.Interface, error) {
			return nil, errors.New("interfaces failed")
		}).Build()
		mockey.Mock(utilnet.ChooseHostInterface).To(func() (net.IP, error) {
			return net.ParseIP("7.251.157.169"), nil
		}).Build()

		got, err := DefaultRouteHostIPv4()
		require.NoError(t, err)
		assert.Equal(t, "7.251.157.169", got.Addr.String())
		assert.Empty(t, got.Iface.Name)
	})

	mockey.PatchConvey("ChooseHostInterface error is returned", t, func() {
		mockey.Mock(utilnet.ChooseHostInterface).To(func() (net.IP, error) {
			return nil, errors.New("no default route")
		}).Build()

		_, err := DefaultRouteHostIPv4()
		require.ErrorContains(t, err, "no default route")
	})

	mockey.PatchConvey("IPv6 host IP is rejected", t, func() {
		mockey.Mock(utilnet.ChooseHostInterface).To(func() (net.IP, error) {
			return net.ParseIP("2001:db8::1"), nil
		}).Build()

		_, err := DefaultRouteHostIPv4()
		require.ErrorContains(t, err, "not IPv4")
	})

	mockey.PatchConvey("malformed IP is rejected", t, func() {
		mockey.Mock(utilnet.ChooseHostInterface).To(func() (net.IP, error) {
			return net.IP{1, 2, 3}, nil
		}).Build()

		_, err := DefaultRouteHostIPv4()
		require.ErrorContains(t, err, "invalid host IP")
	})
}
