package nscale

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers/nscale/imds"
)

func setIMDSRetryBackoffs(t *testing.T, backoffs []time.Duration) {
	t.Helper()
	orig := imdsRetryBackoffs
	imdsRetryBackoffs = backoffs
	t.Cleanup(func() { imdsRetryBackoffs = orig })
}

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
		mockey.Mock(imds.FetchOpenStackMetadataWithStatusCode).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, int, error) {
			return &imds.OpenStackMetadataResponse{
				UUID:             "9af4ad95-5cf9-4085-b4ac-2dcf39427166",
				AvailabilityZone: "nova",
				Meta: imds.OpenStackMetadataMeta{
					OrganizationID: "org-nscale",
					ProjectID:      "proj-nscale",
				},
			}, http.StatusOK, nil
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
		mockey.Mock(imds.FetchOpenStackMetadataWithStatusCode).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, int, error) {
			return &imds.OpenStackMetadataResponse{
				UUID: "",
				Meta: imds.OpenStackMetadataMeta{
					OrganizationID: "org-nscale",
					ProjectID:      "proj-nscale",
				},
			}, http.StatusOK, nil
		}).Build()

		provider, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Empty(t, provider)
	})

	mockey.PatchConvey("detectProvider returns empty when OpenStack metadata misses nscale fields", t, func() {
		mockey.Mock(imds.FetchOpenStackMetadataWithStatusCode).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, int, error) {
			return &imds.OpenStackMetadataResponse{
				UUID: "9af4ad95-5cf9-4085-b4ac-2dcf39427166",
				Meta: imds.OpenStackMetadataMeta{
					OrganizationID: "",
					ProjectID:      "proj-nscale",
				},
			}, http.StatusOK, nil
		}).Build()

		provider, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Empty(t, provider)
	})

	mockey.PatchConvey("detectProvider retries transient 5xx and 429 OpenStack metadata errors and succeeds", t, func() {
		setIMDSRetryBackoffs(t, []time.Duration{0, time.Millisecond, time.Millisecond, time.Millisecond})
		attempts := 0
		mockey.Mock(imds.FetchOpenStackMetadataWithStatusCode).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, int, error) {
			attempts++
			if attempts == 1 {
				return nil, http.StatusServiceUnavailable, errors.New("received status code 503")
			}
			if attempts == 2 {
				return nil, http.StatusTooManyRequests, errors.New("received status code 429")
			}
			return &imds.OpenStackMetadataResponse{
				UUID: "9af4ad95-5cf9-4085-b4ac-2dcf39427166",
				Meta: imds.OpenStackMetadataMeta{
					OrganizationID: "org-nscale",
					ProjectID:      "proj-nscale",
				},
			}, http.StatusOK, nil
		}).Build()

		provider, err := detectProvider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, provider)
		require.Equal(t, 3, attempts)
	})

	mockey.PatchConvey("detectProvider does not retry 4xx or non-HTTP errors on non-nscale hosts", t, func() {
		setIMDSRetryBackoffs(t, []time.Duration{0, time.Hour, time.Hour})
		var currentCode int
		attempts := 0
		mockey.Mock(imds.FetchOpenStackMetadataWithStatusCode).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, int, error) {
			attempts++
			return nil, currentCode, errors.New("non-retryable metadata error")
		}).Build()

		for _, code := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, 0} {
			currentCode = code
			attempts = 0
			_, err := detectProvider(context.Background())
			require.Error(t, err)
			require.Equal(t, 1, attempts, "statusCode=%d must not be retried", code)
		}
	})

	mockey.PatchConvey("detectProvider returns error when 503 persists across retries", t, func() {
		setIMDSRetryBackoffs(t, []time.Duration{0, time.Millisecond, time.Millisecond})
		attempts := 0
		mockey.Mock(imds.FetchOpenStackMetadataWithStatusCode).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, int, error) {
			attempts++
			return nil, http.StatusServiceUnavailable, errors.New("metadata 503")
		}).Build()

		_, err := detectProvider(context.Background())
		require.Error(t, err)
		require.Equal(t, 3, attempts)
	})

	mockey.PatchConvey("detectProvider stops retrying when context is canceled and preserves last error", t, func() {
		setIMDSRetryBackoffs(t, []time.Duration{0, time.Hour})
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0
		sentinel := errors.New("metadata 503")
		mockey.Mock(imds.FetchOpenStackMetadataWithStatusCode).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, int, error) {
			attempts++
			cancel()
			return nil, http.StatusServiceUnavailable, sentinel
		}).Build()

		_, err := detectProvider(ctx)
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorIs(t, err, sentinel)
		require.Equal(t, 1, attempts)
	})
}

func TestGetInstanceID_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetInstanceID retries transient errors and trims whitespace", t, func() {
		setIMDSRetryBackoffs(t, []time.Duration{0, time.Millisecond, time.Millisecond})
		attempts := 0
		mockey.Mock(imds.FetchInstanceID).To(func(ctx context.Context) (string, error) {
			attempts++
			if attempts == 1 {
				return "", errors.New("received status code 503")
			}
			if attempts == 2 {
				return "   ", nil
			}
			return " i-00014a3a\n", nil
		}).Build()

		id, err := GetInstanceID(context.Background())
		require.NoError(t, err)
		require.Equal(t, "i-00014a3a", id)
		require.Equal(t, 3, attempts)
	})

	mockey.PatchConvey("GetInstanceID returns error after retry exhaustion", t, func() {
		setIMDSRetryBackoffs(t, []time.Duration{0, time.Millisecond})
		attempts := 0
		mockey.Mock(imds.FetchInstanceID).To(func(ctx context.Context) (string, error) {
			attempts++
			return "", errors.New("metadata unavailable")
		}).Build()

		_, err := GetInstanceID(context.Background())
		require.Error(t, err)
		require.Equal(t, 2, attempts)
	})

	mockey.PatchConvey("GetInstanceID stops retrying when context is canceled", t, func() {
		setIMDSRetryBackoffs(t, []time.Duration{0, time.Hour})
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0
		mockey.Mock(imds.FetchInstanceID).To(func(ctx context.Context) (string, error) {
			attempts++
			cancel()
			return "", nil
		}).Build()

		_, err := GetInstanceID(ctx)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, attempts)
	})
}

func TestFetchPrivateIPv4_WithMockey(t *testing.T) {
	mockey.PatchConvey("fetchPrivateIPv4 and PrivateIPv4 return RFC1918 IPv4", t, func() {
		mockey.Mock(imds.FetchLocalIPv4).To(func(ctx context.Context) (string, error) {
			return "10.50.85.108", nil
		}).Build()

		privateIP, err := PrivateIPv4(context.Background())
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
	mockey.PatchConvey("fetchVMEnvironment and VMEnvironment return OpenStack availability zone", t, func() {
		mockey.Mock(imds.FetchOpenStackMetadata).To(func(ctx context.Context) (*imds.OpenStackMetadataResponse, error) {
			return &imds.OpenStackMetadataResponse{
				AvailabilityZone: "nova",
			}, nil
		}).Build()

		zone, err := VMEnvironment(context.Background())
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

		region, err = Region(context.Background())
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
