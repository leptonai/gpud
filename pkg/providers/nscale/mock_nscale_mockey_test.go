package nscale

import (
	"context"
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers/nscale/imds"
)

func TestNewAndDetectProvider_WithMockey(t *testing.T) {
	mockey.PatchConvey("New and detectProvider succeed when OpenStack metadata has nscale fields", t, func() {
		mockey.Mock(imds.FetchOpenStackMetadata).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, error) {
			return &imds.OpenStackMetadataResponse{
				UUID:             "9af4ad95-5cf9-4085-b4ac-2dcf39427166",
				AvailabilityZone: "nova",
				Meta: imds.OpenStackMetadataMeta{
					OrganizationID: "org-nscale",
					ProjectID:      "proj-nscale",
				},
			}, nil
		}).Build()

		detector := New()
		require.NotNil(t, detector)
		require.Equal(t, Name, detector.Name())

		provider, err := detector.Provider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, provider)

		provider, err = detectProvider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, provider)
	})
}

func TestDetectProvider_WithMockey(t *testing.T) {
	mockey.PatchConvey("detectProvider returns empty when OpenStack metadata misses nscale fields", t, func() {
		mockey.Mock(imds.FetchOpenStackMetadata).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, error) {
			return &imds.OpenStackMetadataResponse{
				UUID: "9af4ad95-5cf9-4085-b4ac-2dcf39427166",
				Meta: imds.OpenStackMetadataMeta{
					OrganizationID: "",
					ProjectID:      "proj-nscale",
				},
			}, nil
		}).Build()

		provider, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Empty(t, provider)
	})

	mockey.PatchConvey("detectProvider returns error when OpenStack metadata fetch fails", t, func() {
		mockey.Mock(imds.FetchOpenStackMetadata).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, error) {
			return nil, errors.New("metadata unavailable")
		}).Build()

		_, err := detectProvider(context.Background())
		require.Error(t, err)
	})
}

func TestFetchPrivateIPv4_WithMockey(t *testing.T) {
	mockey.PatchConvey("fetchPrivateIPv4 returns RFC1918 IPv4", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "10.50.85.108", nil
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "10.50.85.108", privateIP)
	})

	mockey.PatchConvey("fetchPrivateIPv4 drops non-RFC1918 address", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "7.247.195.201", nil
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Empty(t, privateIP)
	})

	mockey.PatchConvey("fetchPrivateIPv4 drops invalid IP", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "not-an-ip", nil
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Empty(t, privateIP)
	})

	mockey.PatchConvey("fetchPrivateIPv4 returns metadata error", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "", errors.New("metadata unavailable")
		}).Build()

		_, err := fetchPrivateIPv4(context.Background())
		require.Error(t, err)
	})
}

func TestFetchVMEnvironment_WithMockey(t *testing.T) {
	mockey.PatchConvey("fetchVMEnvironment returns OpenStack availability zone", t, func() {
		mockey.Mock(imds.FetchOpenStackMetadata).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, error) {
			return &imds.OpenStackMetadataResponse{
				AvailabilityZone: "nova",
			}, nil
		}).Build()

		zone, err := fetchVMEnvironment(context.Background())
		require.NoError(t, err)
		require.Equal(t, "nova", zone)
	})
}

func TestInstanceID_WithMockey(t *testing.T) {
	mockey.PatchConvey("InstanceID uses EC2-style metadata endpoint value", t, func() {
		mockey.Mock(imds.FetchInstanceID).To(func(ctx context.Context) (string, error) {
			return "i-000017ac", nil
		}).Build()

		detector := New()
		instanceID, err := detector.InstanceID(context.Background())
		require.NoError(t, err)
		require.Equal(t, "i-000017ac", instanceID)
	})
}
