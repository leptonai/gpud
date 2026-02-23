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
	mockey.PatchConvey("FetchPublicIPv4 forwards default URL", t, func() {
		mockey.Mock(fetchPublicIPv4).To(func(ctx context.Context, metadataURL string) (string, error) {
			require.Equal(t, imdsMetadataURL, metadataURL)
			return "203.0.113.20", nil
		}).Build()

		got, err := FetchPublicIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "203.0.113.20", got)
	})
}

func TestFetchAvailabilityZone_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchAvailabilityZone returns location from compute response", t, func() {
		mockey.Mock(fetchComputeResponse).To(func(ctx context.Context, metadataURL string) (*computeResponse, error) {
			require.Equal(t, imdsMetadataURL, metadataURL)
			return &computeResponse{Location: "eastus2"}, nil
		}).Build()

		got, err := FetchAvailabilityZone(context.Background())
		require.NoError(t, err)
		require.Equal(t, "eastus2", got)
	})
}

func TestFetchAZEnvironment_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchAZEnvironment returns environment string", t, func() {
		mockey.Mock(fetchComputeResponse).To(func(ctx context.Context, metadataURL string) (*computeResponse, error) {
			require.Equal(t, imdsMetadataURL, metadataURL)
			return &computeResponse{AZEnvironment: "AzureGovernmentCloud"}, nil
		}).Build()

		got, err := FetchAZEnvironment(context.Background())
		require.NoError(t, err)
		require.Equal(t, "AzureGovernmentCloud", got)
	})
}

func TestFetchInstanceID_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchInstanceID returns resource ID", t, func() {
		mockey.Mock(fetchComputeResponse).To(func(ctx context.Context, metadataURL string) (*computeResponse, error) {
			require.Equal(t, imdsMetadataURL, metadataURL)
			return &computeResponse{ResourceID: "/subscriptions/123/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm1"}, nil
		}).Build()

		got, err := FetchInstanceID(context.Background())
		require.NoError(t, err)
		require.Contains(t, got, "virtualMachines/vm1")
	})
}
