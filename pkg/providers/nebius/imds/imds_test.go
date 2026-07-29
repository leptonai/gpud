package imds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchInstanceData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/instance-data", r.URL.Path)
		assert.Equal(t, metadataTrue, r.Header.Get(headerMetadata))
		_, _ = w.Write([]byte(`{
			"id": "computeinstance-inst789",
			"parent_id": "project-test123",
			"gpu_cluster_id": "computegpucluster-gpu456",
			"region": "eu-west1"
		}`))
	}))
	defer server.Close()

	data, err := fetchInstanceData(context.Background(), server.URL)
	require.NoError(t, err)
	require.Equal(t, "computeinstance-inst789", data.ID)
	require.Equal(t, "project-test123", data.ParentID)
	require.Equal(t, "computegpucluster-gpu456", data.GPUClusterID)
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

	t.Run("retries documented transient status", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if requests.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte("eu-west1"))
		}))
		defer server.Close()

		region, err := fetchMetadataByPath(context.Background(), server.URL)
		require.NoError(t, err)
		require.Equal(t, "eu-west1", region)
		require.EqualValues(t, 3, requests.Load())
	})

	t.Run("returns terminal status error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		_, err := fetchMetadataByPath(context.Background(), server.URL)
		require.ErrorContains(t, err, "status code 400")
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
