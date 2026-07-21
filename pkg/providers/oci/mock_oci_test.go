package oci

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers/oci/imds"
)

func TestNewAndDetectProvider_WithMockey(t *testing.T) {
	mockey.PatchConvey("OCI detector accepts a non-RFC1918 primary VNIC IPv4 address", t, func() {
		mockey.Mock(imds.FetchPrimaryVNICPrivateIPv4).To(func(context.Context) (string, error) {
			return "203.0.113.10", nil
		}).Build()

		detector := New()
		require.Equal(t, Name, detector.Name())

		provider, err := detector.Provider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, provider)

		privateIP, err := detector.PrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "203.0.113.10", privateIP)
		require.False(t, netip.MustParseAddr(privateIP).IsPrivate())
	})
}

func TestDetectProvider_InvalidAddress_WithMockey(t *testing.T) {
	mockey.PatchConvey("OCI detector ignores an invalid primary VNIC address", t, func() {
		mockey.Mock(imds.FetchPrimaryVNICPrivateIPv4).To(func(context.Context) (string, error) {
			return "not-an-ip", nil
		}).Build()

		provider, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Empty(t, provider)
	})
}

func TestDetectProvider_Error_WithMockey(t *testing.T) {
	mockey.PatchConvey("OCI detector propagates metadata fetch errors", t, func() {
		mockey.Mock(imds.FetchPrimaryVNICPrivateIPv4).To(func(context.Context) (string, error) {
			return "", errors.New("fetch failed")
		}).Build()

		_, err := detectProvider(context.Background())
		require.Error(t, err)
	})
}
