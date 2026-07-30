package nebius

import (
	"context"
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/nebius/imds"
)

func TestNewAndDetectProvider_WithMockey(t *testing.T) {
	mockey.PatchConvey("New returns a region-capable Nebius detector", t, func() {
		mockey.Mock(imds.FetchRegion).To(func(context.Context) (string, error) {
			return "eu-west1", nil
		}).Build()
		mockey.Mock(imds.FetchInstanceData).To(func(context.Context) (*imds.InstanceData, error) {
			return &imds.InstanceData{
				ID:           "computeinstance-inst789",
				ParentID:     "project-test123",
				GPUClusterID: "computegpucluster-gpu456",
			}, nil
		}).Build()

		detector := New()
		require.Equal(t, Name, detector.Name())

		provider, err := detector.Provider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, provider)

		regionDetector, ok := detector.(providers.RegionDetector)
		require.True(t, ok)
		region, err := regionDetector.Region(context.Background())
		require.NoError(t, err)
		require.Equal(t, "eu-west1", region)

		instanceID, err := detector.InstanceID(context.Background())
		require.NoError(t, err)
		require.Equal(t, "computeinstance-inst789", instanceID)
	})
}

func TestProviderMetadataErrors(t *testing.T) {
	metadataErr := errors.New("metadata unavailable")

	mockey.PatchConvey("detectProvider returns the region error", t, func() {
		mockey.Mock(imds.FetchRegion).To(func(context.Context) (string, error) {
			return "", metadataErr
		}).Build()

		provider, err := detectProvider(context.Background())
		require.ErrorIs(t, err, metadataErr)
		require.Empty(t, provider)
	})

	mockey.PatchConvey("detectProvider rejects an empty region", t, func() {
		mockey.Mock(imds.FetchRegion).To(func(context.Context) (string, error) {
			return "", nil
		}).Build()

		provider, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Empty(t, provider)
	})

	mockey.PatchConvey("GetInstanceID returns the IMDS error", t, func() {
		mockey.Mock(imds.FetchInstanceData).To(func(context.Context) (*imds.InstanceData, error) {
			return nil, metadataErr
		}).Build()

		instanceID, err := GetInstanceID(context.Background())
		require.ErrorIs(t, err, metadataErr)
		require.Empty(t, instanceID)
	})

	mockey.PatchConvey("GetInstanceID requires id", t, func() {
		mockey.Mock(imds.FetchInstanceData).To(func(context.Context) (*imds.InstanceData, error) {
			return &imds.InstanceData{}, nil
		}).Build()

		instanceID, err := GetInstanceID(context.Background())
		require.ErrorContains(t, err, "missing id")
		require.Empty(t, instanceID)
	})
}
