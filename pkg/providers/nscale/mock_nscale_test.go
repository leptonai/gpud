package nscale

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers/nscale/imds"
)

// fakeAddrConn is a net.Conn whose LocalAddr is fixed, so route-source
// lookups can be tested without real network access.
type fakeAddrConn struct {
	net.Conn
	addr net.Addr
}

func (f *fakeAddrConn) LocalAddr() net.Addr { return f.addr }
func (f *fakeAddrConn) Close() error        { return nil }

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
	mockey.PatchConvey("detectProvider returns empty when OpenStack metadata misses UUID", t, func() {
		mockey.Mock(imds.FetchOpenStackMetadata).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, error) {
			return &imds.OpenStackMetadataResponse{
				UUID: "",
				Meta: imds.OpenStackMetadataMeta{
					OrganizationID: "org-nscale",
					ProjectID:      "proj-nscale",
				},
			}, nil
		}).Build()

		provider, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Empty(t, provider)
	})

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

	mockey.PatchConvey("fetchPrivateIPv4 accepts routable IPv4 from metadata", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "7.247.195.201", nil
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "7.247.195.201", privateIP)
	})

	mockey.PatchConvey("fetchPrivateIPv4 returns empty when metadata value is invalid and route fallback fails", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "not-an-ip", nil
		}).Build()
		mockey.Mock(routeSourceIPv4).To(func() (string, error) {
			return "", errors.New("no route")
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Empty(t, privateIP)
	})

	mockey.PatchConvey("fetchPrivateIPv4 returns empty for IPv6 metadata value when route fallback fails", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "2600:1f18:b1::1", nil
		}).Build()
		mockey.Mock(routeSourceIPv4).To(func() (string, error) {
			return "", errors.New("no route")
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Empty(t, privateIP)
	})

	mockey.PatchConvey("fetchPrivateIPv4 returns metadata error when route fallback also fails", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "", errors.New("metadata unavailable")
		}).Build()
		mockey.Mock(routeSourceIPv4).To(func() (string, error) {
			return "", errors.New("no route")
		}).Build()

		_, err := fetchPrivateIPv4(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "metadata unavailable")
	})

	mockey.PatchConvey("fetchPrivateIPv4 falls back to route source when metadata errors", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "", errors.New("metadata unavailable")
		}).Build()
		mockey.Mock(routeSourceIPv4).To(func() (string, error) {
			return "7.247.35.250", nil
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "7.247.35.250", privateIP)
	})

	mockey.PatchConvey("fetchPrivateIPv4 falls back to route source when metadata value is invalid", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "not-an-ip", nil
		}).Build()
		mockey.Mock(routeSourceIPv4).To(func() (string, error) {
			return "7.247.35.250", nil
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "7.247.35.250", privateIP)
	})

	mockey.PatchConvey("fetchPrivateIPv4 prefers metadata value over route source", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "7.247.195.201", nil
		}).Build()
		mockey.Mock(routeSourceIPv4).To(func() (string, error) {
			return "10.0.0.1", nil
		}).Build()

		privateIP, err := fetchPrivateIPv4(context.Background())
		require.NoError(t, err)
		require.Equal(t, "7.247.195.201", privateIP)
	})
}

func TestParseRouteSourceIPv4(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "accepts publicly routable fabric IPv4", input: "7.247.35.250", want: "7.247.35.250"},
		{name: "accepts RFC1918 IPv4", input: "10.50.85.108", want: "10.50.85.108"},
		{name: "rejects link-local", input: "169.254.169.254", wantErr: true},
		{name: "rejects loopback", input: "127.0.0.1", wantErr: true},
		{name: "rejects unspecified", input: "0.0.0.0", wantErr: true},
		{name: "rejects IPv6", input: "2600:1f18:b1::1", wantErr: true},
		{name: "rejects IPv6 loopback", input: "::1", wantErr: true},
		{name: "rejects non-IP", input: "not-an-ip", wantErr: true},
		{name: "rejects empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRouteSourceIPv4(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRouteSourceIPv4_WithMockey(t *testing.T) {
	mockey.PatchConvey("routeSourceIPv4 returns error when dial fails", t, func() {
		mockey.Mock(net.Dial).To(func(network, address string) (net.Conn, error) {
			return nil, errors.New("no route to host")
		}).Build()

		_, err := routeSourceIPv4()
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to resolve route to metadata service")
	})

	mockey.PatchConvey("routeSourceIPv4 returns source IPv4 from the UDP local address", t, func() {
		mockey.Mock(net.Dial).To(func(network, address string) (net.Conn, error) {
			return &fakeAddrConn{addr: &net.UDPAddr{IP: net.ParseIP("7.247.35.250"), Port: 12345}}, nil
		}).Build()

		ip, err := routeSourceIPv4()
		require.NoError(t, err)
		require.Equal(t, "7.247.35.250", ip)
	})

	mockey.PatchConvey("routeSourceIPv4 rejects non-UDP local address", t, func() {
		mockey.Mock(net.Dial).To(func(network, address string) (net.Conn, error) {
			return &fakeAddrConn{addr: &net.TCPAddr{IP: net.ParseIP("7.247.35.250"), Port: 12345}}, nil
		}).Build()

		_, err := routeSourceIPv4()
		require.Error(t, err)
	})

	mockey.PatchConvey("routeSourceIPv4 rejects nil IP local address", t, func() {
		mockey.Mock(net.Dial).To(func(network, address string) (net.Conn, error) {
			return &fakeAddrConn{addr: &net.UDPAddr{}}, nil
		}).Build()

		_, err := routeSourceIPv4()
		require.Error(t, err)
	})

	mockey.PatchConvey("routeSourceIPv4 rejects loopback source address", t, func() {
		mockey.Mock(net.Dial).To(func(network, address string) (net.Conn, error) {
			return &fakeAddrConn{addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}, nil
		}).Build()

		_, err := routeSourceIPv4()
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

func TestRegion_WithMockey(t *testing.T) {
	mockey.PatchConvey("New returns a Detector that implements RegionDetector", t, func() {
		mockey.Mock(imds.FetchRegion).To(func(ctx context.Context) (string, error) {
			return "eu-north-1", nil
		}).Build()

		detector := New()
		require.NotNil(t, detector)

		// Type-assert that the detector implements RegionDetector.
		regionDetector, ok := detector.(interface {
			Region(context.Context) (string, error)
		})
		require.True(t, ok, "New() must return a detector that implements RegionDetector")

		region, err := regionDetector.Region(context.Background())
		require.NoError(t, err)
		require.Equal(t, "eu-north-1", region)
	})

	mockey.PatchConvey("Region returns empty when FetchRegion returns empty", t, func() {
		mockey.Mock(imds.FetchRegion).To(func(ctx context.Context) (string, error) {
			return "", nil
		}).Build()

		detector := New()
		regionDetector, ok := detector.(interface {
			Region(context.Context) (string, error)
		})
		require.True(t, ok)
		region, err := regionDetector.Region(context.Background())
		require.NoError(t, err)
		require.Empty(t, region)
	})

	mockey.PatchConvey("Region returns error when FetchRegion fails", t, func() {
		mockey.Mock(imds.FetchRegion).To(func(ctx context.Context) (string, error) {
			return "", errors.New("metadata unavailable")
		}).Build()

		detector := New()
		regionDetector, ok := detector.(interface {
			Region(context.Context) (string, error)
		})
		require.True(t, ok)
		_, err := regionDetector.Region(context.Background())
		require.Error(t, err)
	})
}
