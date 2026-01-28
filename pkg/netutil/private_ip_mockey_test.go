package netutil

import (
	"errors"
	"net"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrivateIPs_InterfacesErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("GetPrivateIPs returns interfaces error", t, func() {
		mockey.Mock(net.Interfaces).To(func() ([]net.Interface, error) {
			return nil, errors.New("interfaces failed")
		}).Build()

		ips, err := GetPrivateIPs()
		require.Error(t, err)
		assert.Nil(t, ips)
	})
}

func TestGetPrivateIPs_MockedInterfacesWithMockey(t *testing.T) {
	mockey.PatchConvey("GetPrivateIPs filters interfaces and selects IPv6 when needed", t, func() {
		mockey.Mock(net.Interfaces).To(func() ([]net.Interface, error) {
			return []net.Interface{
				{Name: "lo", Flags: net.FlagUp},
				{Name: "eth0", Flags: 0},
				{Name: "eth1", Flags: net.FlagUp},
				{Name: "eth2", Flags: net.FlagUp},
			}, nil
		}).Build()

		mockey.Mock((*net.Interface).Addrs).To(func(ifi *net.Interface) ([]net.Addr, error) {
			switch ifi.Name {
			case "lo":
				return []net.Addr{&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)}}, nil
			case "eth1":
				return []net.Addr{&net.IPNet{IP: net.ParseIP("fd00::1"), Mask: net.CIDRMask(64, 128)}}, nil
			case "eth2":
				return []net.Addr{&net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(24, 32)}}, nil
			default:
				return nil, errors.New("addr error")
			}
		}).Build()

		ips, err := GetPrivateIPs(WithPrefixesToSkip("lo"))
		require.NoError(t, err)
		require.Len(t, ips, 1)
		assert.Equal(t, "eth1", ips[0].Iface.Name)
		assert.True(t, ips[0].Addr.Is6())
	})
}

func TestGetPrivateIPs_SuffixSkipWithMockey(t *testing.T) {
	mockey.PatchConvey("GetPrivateIPs skips interfaces by suffix", t, func() {
		mockey.Mock(net.Interfaces).To(func() ([]net.Interface, error) {
			return []net.Interface{
				{Name: "eth0v", Flags: net.FlagUp},
			}, nil
		}).Build()

		mockey.Mock((*net.Interface).Addrs).To(func(ifi *net.Interface) ([]net.Addr, error) {
			return []net.Addr{&net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.CIDRMask(24, 32)}}, nil
		}).Build()

		ips, err := GetPrivateIPs(WithSuffixesToSkip("v"))
		require.NoError(t, err)
		assert.Len(t, ips, 0)
	})
}
