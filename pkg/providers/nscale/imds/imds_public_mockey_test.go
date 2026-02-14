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
			require.Equal(t, imdsMetadataURL+"/instance-id", metadataURL)
			return "meta-ok", nil
		}).Build()

		got, err := FetchMetadata(context.Background(), "instance-id")
		require.NoError(t, err)
		require.Equal(t, "meta-ok", got)
	})
}

func TestFetchPublicIPv4_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchPublicIPv4 forwards default metadata URL", t, func() {
		mockey.Mock(fetchPublicIPv4).To(func(ctx context.Context, metadataURL string) (string, error) {
			require.Equal(t, imdsMetadataURL, metadataURL)
			return "203.0.113.10", nil
		}).Build()

		got, err := FetchPublicIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "203.0.113.10", got)
	})
}

func TestFetchOpenStackMetadata_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchOpenStackMetadata forwards default OpenStack metadata URL", t, func() {
		mockey.Mock(fetchOpenStackMetadata).To(func(ctx context.Context, metadataURL string) (*OpenStackMetadataResponse, error) {
			require.Equal(t, openStackMetadataJSONURL, metadataURL)
			return &OpenStackMetadataResponse{UUID: "uuid-1"}, nil
		}).Build()

		got, err := FetchOpenStackMetadata(context.Background())
		require.NoError(t, err)
		require.Equal(t, "uuid-1", got.UUID)
	})
}
