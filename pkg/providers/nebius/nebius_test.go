package nebius

import (
	"context"
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
		require.Equal(t, "project-test123/computegpucluster-gpu456/computeinstance-inst789", instanceID)

		require.Equal(t, "project-test123/computeinstance-inst789",
			formatInstanceID("project-test123", "", "computeinstance-inst789"))
	})
}
