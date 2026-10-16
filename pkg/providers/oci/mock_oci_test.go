package oci

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/oci/imds"
)

func setInstanceIDFallbackPaths(t *testing.T, paths []string) {
	t.Helper()
	orig := instanceIDFallbackPaths
	instanceIDFallbackPaths = paths
	t.Cleanup(func() { instanceIDFallbackPaths = orig })
}

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
	setInstanceIDFallbackPaths(t, nil)
	mockey.PatchConvey("OCI detector ignores an empty instance ID", t, func() {
		mockey.Mock(imds.FetchInstanceID).Return("", nil).Build()

		provider, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Empty(t, provider)
	})
}

func TestDetectProvider_Error_WithMockey(t *testing.T) {
	setInstanceIDFallbackPaths(t, nil)
	mockey.PatchConvey("OCI detector propagates metadata fetch errors", t, func() {
		mockey.Mock(imds.FetchInstanceID).Return("", errors.New("fetch failed")).Build()

		_, err := detectProvider(context.Background())
		require.Error(t, err)
	})
}

func TestOCIFallbackFromHostFile_WithMockey(t *testing.T) {
	dir := t.TempDir()
	idFile := filepath.Join(dir, "instance-id")
	const wantOCID = "ocid1.instance.oc1.iad.anuwcljtd5i4peic5etxeqel3xg6rexkedi6tqmnmcuylhxhc2kohxndkg2q"
	require.NoError(t, os.WriteFile(idFile, []byte(wantOCID+"\n"), 0o600))
	setInstanceIDFallbackPaths(t, []string{filepath.Join(dir, "missing"), idFile})

	mockey.PatchConvey("OCI detector falls back to cloud-init instance-id and derives region when IMDS fails", t, func() {
		mockey.Mock(imds.FetchInstanceID).Return("", errors.New("imds timeout")).Build()
		mockey.Mock(imds.FetchCanonicalRegionName).Return("", errors.New("imds timeout")).Build()

		detector := New()
		provider, err := detector.Provider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, provider)

		instanceID, err := detector.InstanceID(context.Background())
		require.NoError(t, err)
		require.Equal(t, wantOCID, instanceID)

		regionDetector, ok := detector.(providers.RegionDetector)
		require.True(t, ok)
		region, err := regionDetector.Region(context.Background())
		require.NoError(t, err)
		require.Equal(t, "us-ashburn-1", region)
	})
}

func TestParseInstanceOCIDAndRegion(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantOCID   string
		wantOK     bool
		wantRegion string
	}{
		{
			name:       "iad instance ocid",
			raw:        "ocid1.instance.oc1.iad.anuwcljtd5i4peic5etxeqel3xg6rexkedi6tqmnmcuylhxhc2kohxndkg2q\n",
			wantOCID:   "ocid1.instance.oc1.iad.anuwcljtd5i4peic5etxeqel3xg6rexkedi6tqmnmcuylhxhc2kohxndkg2q",
			wantOK:     true,
			wantRegion: "us-ashburn-1",
		},
		{
			name:       "ord instance ocid",
			raw:        "OCID1.INSTANCE.OC1.ORD.ANUWCLJTD5I4PEICEXAMPLE",
			wantOCID:   "ocid1.instance.oc1.ord.anuwcljtd5i4peicexample",
			wantOK:     true,
			wantRegion: "us-chicago-1",
		},
		{
			name:       "jnb instance ocid",
			raw:        "ocid1.instance.oc1.jnb.anuwcljtd5i4peicexample",
			wantOCID:   "ocid1.instance.oc1.jnb.anuwcljtd5i4peicexample",
			wantOK:     true,
			wantRegion: "af-johannesburg-1",
		},
		{
			name:       "hyphenated region identifier in ocid",
			raw:        "ocid1.instance.oc1.us-ashburn-1.anuwcljtd5i4peicexample",
			wantOCID:   "ocid1.instance.oc1.us-ashburn-1.anuwcljtd5i4peicexample",
			wantOK:     true,
			wantRegion: "us-ashburn-1",
		},
		{
			name:   "uuid is not an ocid",
			raw:    "2b1434ed-d9fc-489f-9fd4-7859292d309f",
			wantOK: false,
		},
		{
			name:   "compartment ocid is not an instance ocid",
			raw:    "ocid1.compartment.oc1..aaaaaaaasih2j2sbpwhqlvakbshnatjxkxfveb2r726qzkh4fblyuncu6llq",
			wantOK: false,
		},
		{
			name:   "cloud-init placeholder",
			raw:    "iid-datasource-none",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseInstanceOCID(tt.raw)
			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			require.Equal(t, tt.wantOCID, got)
			require.Equal(t, tt.wantRegion, regionFromInstanceOCID(got))
		})
	}
}
