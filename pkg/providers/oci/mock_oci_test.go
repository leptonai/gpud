package oci

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/oci/imds"
)

func TestNewAndDetectProvider_WithMockey(t *testing.T) {
	mockey.PatchConvey("OCI detector reports instance metadata and accepts a non-RFC1918 primary VNIC IPv4 address", t, func() {
		mockey.Mock(imds.FetchInstanceID).Return("ocid1.instance.oc1.phx.example", nil).Build()
		privateIPFetches := 0
		mockey.Mock(imds.FetchPrimaryVNICPrivateIPv4).To(func(context.Context) (string, error) {
			privateIPFetches++
			return "203.0.113.10", nil
		}).Build()
		mockey.Mock(imds.FetchCanonicalRegionName).Return("us-phoenix-1", nil).Build()

		detector := New()
		require.Equal(t, Name, detector.Name())

		provider, err := detector.Provider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, provider)

		privateIP, err := detector.PrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "203.0.113.10", privateIP)
		require.False(t, netip.MustParseAddr(privateIP).IsPrivate())
		require.Equal(t, 1, privateIPFetches)

		instanceID, err := detector.InstanceID(context.Background())
		require.NoError(t, err)
		require.Equal(t, "ocid1.instance.oc1.phx.example", instanceID)

		regionDetector, ok := detector.(providers.RegionDetector)
		require.True(t, ok)
		region, err := regionDetector.Region(context.Background())
		require.NoError(t, err)
		require.Equal(t, "us-phoenix-1", region)
	})
}

func TestFetchPrivateIPv4_InvalidAddress_WithMockey(t *testing.T) {
	mockey.PatchConvey("OCI private IP lookup ignores an invalid primary VNIC address", t, func() {
		mockey.Mock(imds.FetchPrimaryVNICPrivateIPv4).To(func(context.Context) (string, error) {
			return "not-an-ip", nil
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Empty(t, privateIP)
	})
}

func TestDetectProvider_EmptyInstanceID_WithMockey(t *testing.T) {
	mockey.PatchConvey("OCI detector ignores an empty instance ID", t, func() {
		mockey.Mock(imds.FetchInstanceID).Return("", nil).Build()

		provider, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Empty(t, provider)
	})
}

func TestDetectProvider_Error_WithMockey(t *testing.T) {
	mockey.PatchConvey("OCI detector propagates metadata fetch errors", t, func() {
		mockey.Mock(imds.FetchInstanceID).Return("", errors.New("fetch failed")).Build()

		_, err := detectProvider(context.Background())
		require.Error(t, err)
	})
}
