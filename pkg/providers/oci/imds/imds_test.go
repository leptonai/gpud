package imds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestFetchMetadataByPath_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchMetadataByPath(context.Background(), srv.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "received status code 404")
}
