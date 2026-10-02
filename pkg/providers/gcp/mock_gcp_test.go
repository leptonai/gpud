package gcp

import (
	"context"
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/gcp/imds"
)

func TestNewAndDetectProvider_WithMockey(t *testing.T) {
	mockey.PatchConvey("New and detectProvider succeed when zone is set", t, func() {
		mockey.Mock(imds.FetchAvailabilityZone).To(func(ctx context.Context) (string, error) {
			return "us-central1-a", nil
		}).Build()

		detector := New()
		require.NotNil(t, detector)
		require.Equal(t, Name, detector.Name())

		provider, err := detector.Provider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, provider)

		zone, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, zone)
	})
}

func TestNewImplementsRegionDetector_WithMockey(t *testing.T) {
	mockey.PatchConvey("New returns a RegionDetector reporting the zone-based region", t, func() {
		mockey.Mock(imds.FetchRegion).To(func(ctx context.Context) (string, error) {
			return "us-east5-a", nil
		}).Build()

		detector := New()
		require.NotNil(t, detector)

		regionDetector, ok := detector.(providers.RegionDetector)
		require.True(t, ok, "gcp detector should implement providers.RegionDetector")

		region, err := regionDetector.Region(context.Background())
		require.NoError(t, err)
		require.Equal(t, "us-east5-a", region)
	})
}

func TestDetectProvider_EmptyZone_WithMockey(t *testing.T) {
	mockey.PatchConvey("detectProvider returns empty when zone is empty", t, func() {
		mockey.Mock(imds.FetchAvailabilityZone).To(func(ctx context.Context) (string, error) {
			return "", nil
		}).Build()

		zone, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Equal(t, "", zone)
	})
}

func TestDetectProvider_Error_WithMockey(t *testing.T) {
	mockey.PatchConvey("detectProvider returns error when fetch fails", t, func() {
		mockey.Mock(imds.FetchAvailabilityZone).To(func(ctx context.Context) (string, error) {
			return "", errors.New("fetch failed")
		}).Build()

		_, err := detectProvider(context.Background())
		require.Error(t, err)
	})
}

func TestFetchPrivateIPv4_WithMockey(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		fetchErr error
		want     string
		wantErr  bool
	}{
		{name: "non-RFC1918 VPC address is authoritative", metadata: "7.246.74.36", want: "7.246.74.36"},
		{name: "RFC1918 VPC address", metadata: "10.128.0.2", want: "10.128.0.2"},
		{name: "invalid address is ignored", metadata: "not-an-ip", want: ""},
		{name: "IPv6 address is ignored", metadata: "fd20::2", want: ""},
		{name: "metadata error is returned", fetchErr: errors.New("metadata unavailable"), wantErr: true},
	}

	for _, tt := range tests {
		mockey.PatchConvey(tt.name, t, func() {
			mockey.Mock(imds.FetchPrimaryPrivateIPv4).To(func(context.Context) (string, error) {
				return tt.metadata, tt.fetchErr
			}).Build()

			got, err := New().PrivateIPv4(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
