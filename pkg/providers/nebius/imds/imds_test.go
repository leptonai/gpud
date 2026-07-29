package imds

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchInstanceDataAndRegion(t *testing.T) {
	setDefaultTransport(t, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, metadataTrue, r.Header.Get(headerMetadata))
		assert.Equal(t, "metadata.nebius.internal", r.URL.Host)

		var body string
		switch r.URL.Path {
		case "/v1/instance-data":
			body = `{
				"id": "computeinstance-inst789",
				"parent_id": "project-test123",
				"name": "example-vm",
				"hostname": "example-hostname",
				"platform": "gpu-h200-sxm",
				"preset": "1gpu-16vcpu-200gb",
				"labels": {"environment": "test"},
				"resource_version": 7,
				"created_at": "2026-07-29T00:00:00Z",
				"service_account_id": "serviceaccount-sa123",
				"gpu_cluster_id": "computegpucluster-gpu456",
				"infiniband_fabric": "fabric-5",
				"infiniband_topology_path": ["hash-1", "hash-2", "hash-3"],
				"region": "eu-west1"
			}`
		case "/v1/instance-data/region":
			body = "eu-west1"
		default:
			t.Fatalf("unexpected IMDS path %q", r.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	}))

	data, err := FetchInstanceData(context.Background())
	require.NoError(t, err)
	require.Equal(t, "computeinstance-inst789", data.ID)
	require.Equal(t, "project-test123", data.ParentID)
	require.Equal(t, "example-vm", data.Name)
	require.Equal(t, "example-hostname", data.Hostname)
	require.Equal(t, "gpu-h200-sxm", data.Platform)
	require.Equal(t, "1gpu-16vcpu-200gb", data.Preset)
	require.Equal(t, map[string]string{"environment": "test"}, data.Labels)
	require.EqualValues(t, 7, data.ResourceVersion)
	require.Equal(t, "2026-07-29T00:00:00Z", data.CreatedAt)
	require.Equal(t, "serviceaccount-sa123", data.ServiceAccountID)
	require.Equal(t, "computegpucluster-gpu456", data.GPUClusterID)
	require.Equal(t, "fabric-5", data.InfinibandFabric)
	require.Equal(t, []string{"hash-1", "hash-2", "hash-3"}, data.InfinibandTopologyPath)
	require.Equal(t, "eu-west1", data.Region)

	region, err := FetchRegion(context.Background())
	require.NoError(t, err)
	require.Equal(t, "eu-west1", region)
}

func TestFetchMetadataByPath(t *testing.T) {
	t.Run("trims response and sends required header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, metadataTrue, r.Header.Get(headerMetadata))
			_, _ = w.Write([]byte(" eu-west1\n"))
		}))
		defer server.Close()

		region, err := fetchMetadataByPath(context.Background(), server.URL)
		require.NoError(t, err)
		require.Equal(t, "eu-west1", region)
	})

	t.Run("retries each documented transient status", func(t *testing.T) {
		for _, statusCode := range []int{
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusServiceUnavailable,
		} {
			t.Run(http.StatusText(statusCode), func(t *testing.T) {
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if requests.Add(1) == 1 {
						w.WriteHeader(statusCode)
						return
					}
					_, _ = w.Write([]byte("eu-west1"))
				}))
				defer server.Close()

				region, err := fetchMetadataByPath(context.Background(), server.URL)
				require.NoError(t, err)
				require.Equal(t, "eu-west1", region)
				require.EqualValues(t, 2, requests.Load())
			})
		}
	})

	t.Run("returns terminal status error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		_, err := fetchMetadataByPath(context.Background(), server.URL)
		require.ErrorContains(t, err, "status code 400")
	})

	t.Run("returns request creation error", func(t *testing.T) {
		_, err := fetchMetadataByPath(context.Background(), "://invalid")
		require.ErrorContains(t, err, "failed to create Nebius metadata request")
	})

	t.Run("returns transport error", func(t *testing.T) {
		transportErr := errors.New("transport failed")
		setDefaultTransport(t, roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, transportErr
		}))

		_, err := fetchMetadataByPath(context.Background(), "http://metadata.test")
		require.ErrorIs(t, err, transportErr)
	})

	t.Run("returns response read error", func(t *testing.T) {
		readErr := errors.New("read failed")
		setDefaultTransport(t, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(iotest.ErrReader(readErr)),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		}))

		_, err := fetchMetadataByPath(context.Background(), "http://metadata.test")
		require.ErrorIs(t, err, readErr)
	})

	t.Run("returns after exhausting retries", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		_, err := fetchMetadataByPath(context.Background(), server.URL)
		require.ErrorContains(t, err, "status code 503")
		require.EqualValues(t, maxMetadataRetries+1, requests.Load())
	})

	t.Run("returns when context ends during retry wait", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := fetchMetadataByPath(ctx, server.URL)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestFetchInstanceDataInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer server.Close()

	_, err := fetchInstanceData(context.Background(), server.URL)
	require.ErrorContains(t, err, "failed to parse Nebius instance metadata")
}

func TestFetchInstanceDataRequestError(t *testing.T) {
	_, err := fetchInstanceData(context.Background(), "://invalid")
	require.ErrorContains(t, err, "failed to create Nebius metadata request")
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func setDefaultTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = original
	})
}
