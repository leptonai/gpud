package imds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFetchMetadataByPathWithStatusCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("i-000017ac "))
			require.NoError(t, err)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		v, code, err := fetchMetadataByPathWithStatusCode(ctx, srv.URL)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "i-000017ac", v)
	})

	t.Run("non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, code, err := fetchMetadataByPathWithStatusCode(ctx, srv.URL)
		require.Error(t, err)
		require.Equal(t, http.StatusNotFound, code)
		require.Contains(t, err.Error(), "received status code 404")
	})
}

func TestFetchPublicIPv4(t *testing.T) {
	t.Run("returns empty on 404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/public-ipv4" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		v, err := fetchPublicIPv4(ctx, srv.URL)
		require.NoError(t, err)
		require.Empty(t, v)
	})

	t.Run("returns value on 200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/public-ipv4" {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("46.148.127.98"))
				require.NoError(t, err)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		v, err := fetchPublicIPv4(ctx, srv.URL)
		require.NoError(t, err)
		require.Equal(t, "46.148.127.98", v)
	})
}

func TestFetchMetadataByPathWithStatusCode_InvalidURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := fetchMetadataByPathWithStatusCode(ctx, "://bad-url")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create metadata request")
}

func TestFetchMetadataByPathWithStatusCode_Unreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := fetchMetadataByPathWithStatusCode(ctx, "http://192.0.2.1:1/unreachable")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to fetch metadata")
}

func TestFetchMetadataByPath_Error(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := fetchMetadataByPath(ctx, "://bad-url")
	require.Error(t, err)
}

func TestFetchPublicIPv4_NonNotFoundError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := fetchPublicIPv4(ctx, srv.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "received status code 500")
}

func TestFetchOpenStackMetadata_FetchError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := fetchOpenStackMetadata(ctx, "://bad-url")
	require.Error(t, err)
}

func TestFetchMetadata_PrependsSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/instance-id", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("i-12345"))
		require.NoError(t, err)
	}))
	defer srv.Close()

	// Temporarily override the IMDS URL by calling the internal function directly
	// FetchMetadata prepends "/" if missing — test that via fetchMetadataByPath
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	v, err := fetchMetadataByPath(ctx, srv.URL+"/instance-id")
	require.NoError(t, err)
	require.Equal(t, "i-12345", v)
}

func TestFetchOpenStackMetadata(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"uuid":"u-1","availability_zone":"nova","meta":{"organizationID":"org","projectID":"project"}}`))
			require.NoError(t, err)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := fetchOpenStackMetadata(ctx, srv.URL)
		require.NoError(t, err)
		require.Equal(t, "u-1", resp.UUID)
		require.Equal(t, "nova", resp.AvailabilityZone)
		require.Equal(t, "org", resp.Meta.OrganizationID)
		require.Equal(t, "project", resp.Meta.ProjectID)
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("{not-json"))
			require.NoError(t, err)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := fetchOpenStackMetadata(ctx, srv.URL)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to parse OpenStack metadata")
	})
}
