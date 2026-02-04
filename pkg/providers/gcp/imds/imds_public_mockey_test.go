package imds

import (
	"context"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
)

func TestFetchMetadata_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchMetadata prepends path and uses default URL", t, func() {
		mockey.Mock(fetchMetadataByPath).To(func(ctx context.Context, metadataURL string) (string, error) {
			require.Equal(t, imdsMetadataURL+"/instance/hostname", metadataURL)
			return "instance-1", nil
		}).Build()

		got, err := FetchMetadata(context.Background(), "instance/hostname")
		require.NoError(t, err)
		require.Equal(t, "instance-1", got)
	})
}

func TestFetchAvailabilityZone_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchAvailabilityZone forwards default URL", t, func() {
		mockey.Mock(fetchAvailabilityZone).To(func(ctx context.Context, metadataURL string) (string, error) {
			require.Equal(t, imdsMetadataURL, metadataURL)
			return "us-central1-a", nil
		}).Build()

		got, err := FetchAvailabilityZone(context.Background())
		require.NoError(t, err)
		require.Equal(t, "us-central1-a", got)
	})
}

func TestFetchPublicIPv4_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchPublicIPv4 forwards default URL", t, func() {
		mockey.Mock(fetchPublicIPv4).To(func(ctx context.Context, metadataURL string) (string, error) {
			require.Equal(t, imdsMetadataURL, metadataURL)
			return "198.51.100.10", nil
		}).Build()

		got, err := FetchPublicIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "198.51.100.10", got)
	})
}

func TestFetchInstanceID_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchInstanceID uses default URL", t, func() {
		mockey.Mock(fetchMetadataByPath).To(func(ctx context.Context, metadataURL string) (string, error) {
			require.Equal(t, imdsMetadataURL+"/instance/id", metadataURL)
			return "1234567890", nil
		}).Build()

		got, err := FetchInstanceID(context.Background())
		require.NoError(t, err)
		require.Equal(t, "1234567890", got)
	})
}
