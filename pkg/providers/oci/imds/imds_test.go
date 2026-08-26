package imds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchPrimaryVNICPrivateIPv4(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/opc/v2/vnics/0/privateIp", r.URL.Path)
		require.Equal(t, bearerOracle, r.Header.Get(headerAuthorization))
		_, err := w.Write([]byte(" 203.0.113.10\n"))
		require.NoError(t, err)
	}))
	defer srv.Close()

	privateIP, err := fetchPrimaryVNICPrivateIPv4(context.Background(), srv.URL+"/opc/v2")
	require.NoError(t, err)
	require.Equal(t, "203.0.113.10", privateIP)
}

func TestFetchInstanceMetadata(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		response string
		fetch    func(context.Context, string) (string, error)
	}{
		{
			name:     "instance ID",
			path:     "/opc/v2/instance/id",
			response: "ocid1.instance.oc1.phx.example",
			fetch:    fetchInstanceID,
		},
		{
			name:     "canonical region name",
			path:     "/opc/v2/instance/canonicalRegionName",
			response: "us-phoenix-1",
			fetch:    fetchCanonicalRegionName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, tt.path, r.URL.Path)
				_, err := w.Write([]byte(tt.response))
				require.NoError(t, err)
			}))
			defer srv.Close()

			got, err := tt.fetch(context.Background(), srv.URL+"/opc/v2")
			require.NoError(t, err)
			require.Equal(t, tt.response, got)
		})
	}
}

func TestFetchMetadataByPath_NonOK(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchMetadataByPath(context.Background(), srv.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "received status code 404")
	require.Equal(t, 1, attempts)
}

func TestFetchMetadataByPath_DoesNotRetryOtherStatuses(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := fetchMetadataByPath(context.Background(), srv.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "received status code 400")
	require.Equal(t, 1, attempts)
}

func TestFetchMetadataByPath_InvalidURL(t *testing.T) {
	_, err := fetchMetadataByPath(context.Background(), "://bad-url")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create OCI metadata request")
}

func TestFetchMetadataByPath_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	metadataURL := srv.URL
	srv.Close()

	_, err := fetchMetadataByPath(context.Background(), metadataURL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch OCI metadata")
}

func TestFetchMetadataByPath_ReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := fetchMetadataByPath(context.Background(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read OCI metadata response body")
}
