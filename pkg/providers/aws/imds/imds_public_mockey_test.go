package imds

import (
	"context"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
)

func TestFetchMetadata_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchMetadata prepends path and uses default URLs", t, func() {
		mockey.Mock(fetchMetadataByPath).To(func(ctx context.Context, tokenURL string, metadataURL string) (string, error) {
			require.Equal(t, imdsTokenURL, tokenURL)
			require.Equal(t, imdsMetadataURL+"/instance-id", metadataURL)
			return "meta-ok", nil
		}).Build()

		got, err := FetchMetadata(context.Background(), "instance-id")
		require.NoError(t, err)
		require.Equal(t, "meta-ok", got)
	})
}

func TestFetchAvailabilityZone_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchAvailabilityZone forwards default URLs", t, func() {
		mockey.Mock(fetchAvailabilityZone).To(func(ctx context.Context, tokenURL string, metadataURL string) (string, error) {
			require.Equal(t, imdsTokenURL, tokenURL)
			require.Equal(t, imdsMetadataURL, metadataURL)
			return "us-west-2a", nil
		}).Build()

		got, err := FetchAvailabilityZone(context.Background())
		require.NoError(t, err)
		require.Equal(t, "us-west-2a", got)
	})
}

func TestFetchPublicIPv4_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchPublicIPv4 forwards default URLs", t, func() {
		mockey.Mock(fetchPublicIPv4).To(func(ctx context.Context, tokenURL string, metadataURL string) (string, error) {
			require.Equal(t, imdsTokenURL, tokenURL)
			require.Equal(t, imdsMetadataURL, metadataURL)
			return "203.0.113.10", nil
		}).Build()

		got, err := FetchPublicIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "203.0.113.10", got)
	})
}

func TestFetchLocalIPv4_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchLocalIPv4 forwards default URLs", t, func() {
		mockey.Mock(fetchLocalIPv4).To(func(ctx context.Context, tokenURL string, metadataURL string) (string, error) {
			require.Equal(t, imdsTokenURL, tokenURL)
			require.Equal(t, imdsMetadataURL, metadataURL)
			return "10.0.0.10", nil
		}).Build()

		got, err := FetchLocalIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "10.0.0.10", got)
	})
}

func TestFetchInstanceID_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchInstanceID uses default URLs", t, func() {
		mockey.Mock(fetchMetadataByPath).To(func(ctx context.Context, tokenURL string, metadataURL string) (string, error) {
			require.Equal(t, imdsTokenURL, tokenURL)
			require.Equal(t, imdsMetadataURL+"/instance-id", metadataURL)
			return "i-0123456789abcdef", nil
		}).Build()

		got, err := FetchInstanceID(context.Background())
		require.NoError(t, err)
		require.Equal(t, "i-0123456789abcdef", got)
	})
}
