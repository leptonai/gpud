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
			}`))
	}))
	defer server.Close()

	data, err := fetchInstanceData(context.Background(), server.URL)
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
