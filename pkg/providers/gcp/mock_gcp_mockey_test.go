package gcp

import (
	"context"
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

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
