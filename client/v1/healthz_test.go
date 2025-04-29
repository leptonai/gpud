package v1

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/server"
)

func TestCheckHealthz(t *testing.T) {
	tests := []struct {
		name            string
		statusCode      int
		body            string
		gzip            bool
		networkError    bool
		closeConnection bool
		wantErr         bool
		errorContains   string
	}{
		{
			name:       "Success",
			statusCode: http.StatusOK,
			body:       `{"status":"ok","version":"v1"}`,
			wantErr:    false,
		},
		{
			name:       "Success with gzip",
			statusCode: http.StatusOK,
			body:       `{"status":"ok","version":"v1"}`,
			gzip:       true,
			wantErr:    false,
		},
		{
			name:          "Wrong Status",
			statusCode:    http.StatusInternalServerError,
			body:          "",
			wantErr:       true,
			errorContains: "server not ready",
		},
		{
			name:          "Wrong Body",
			statusCode:    http.StatusOK,
			body:          `{"status":"error"}`,
			wantErr:       true,
			errorContains: "unexpected healthz response",
		},
		{
			name:          "Empty Body",
			statusCode:    http.StatusOK,
			body:          "",
			wantErr:       true,
			errorContains: "unexpected healthz response",
		},
		{
			name:          "Malformed JSON",
			statusCode:    http.StatusOK,
			body:          `{"status":`,
			wantErr:       true,
			errorContains: "unexpected healthz response",
		},
		{
			name:          "Extra Fields",
			statusCode:    http.StatusOK,
			body:          `{"status":"ok","version":"v1","extra":"field"}`,
			wantErr:       true,
			errorContains: "unexpected healthz response",
		},
		{
			name:            "Connection Close",
			statusCode:      http.StatusOK,
			closeConnection: true,
			wantErr:         true,
			errorContains:   "EOF",
		},
		{
			name:          "Network Error",
			networkError:  true,
			wantErr:       true,
			errorContains: "failed to make request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var srv *httptest.Server
			if tt.networkError {
				// Use a port that's unlikely to be in use but will cause connection refused
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				srv.Close()
			} else {
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/healthz" {
						t.Errorf("Expected /healthz path, got %s", r.URL.Path)
						http.NotFound(w, r)
						return
					}

					if tt.closeConnection {
						// Hijack the connection and close it immediately
						conn, _, err := w.(http.Hijacker).Hijack()
						require.NoError(t, err)
						conn.Close()
						return
					}

					if tt.gzip {
						w.Header().Set("Content-Encoding", "gzip")
						gz := gzip.NewWriter(w)
						_, err := gz.Write([]byte(tt.body))
						require.NoError(t, err)
						require.NoError(t, gz.Close())
						return
					}

					w.WriteHeader(tt.statusCode)
					_, err := w.Write([]byte(tt.body))
					require.NoError(t, err)
				}))
				defer srv.Close()
			}

			err := CheckHealthz(context.Background(), srv.URL)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckHealthzInvalidURL(t *testing.T) {
	err := CheckHealthz(context.Background(), "invalid-url")
	if err == nil {
		t.Error("CheckHealthz() with invalid URL should return error")
	}
}

func TestCheckHealthzContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json, _ := json.Marshal(server.DefaultHealthz)
		_, err := w.Write(json)
		if err != nil {
			t.Errorf("Error writing response: %v", err)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := CheckHealthz(ctx, srv.URL)
	if err == nil {
		t.Error("CheckHealthz() with canceled context should return error")
	}
}

func TestBlockUntilServerReady(t *testing.T) {
	t.Run("server becomes ready immediately", func(t *testing.T) {
		expectedHealthz, err := json.Marshal(server.DefaultHealthz)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/healthz", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(expectedHealthz)
			require.NoError(t, err)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = BlockUntilServerReady(ctx, srv.URL)
		require.NoError(t, err)
	})

	t.Run("server becomes ready after delay", func(t *testing.T) {
		expectedHealthz, err := json.Marshal(server.DefaultHealthz)
		require.NoError(t, err)

		// Track number of requests to simulate the server becoming ready after a few attempts
		requestCount := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/healthz", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)

			requestCount++
			if requestCount <= 2 {
				// First two requests fail
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}

			// Later requests succeed
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(expectedHealthz)
			require.NoError(t, err)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = BlockUntilServerReady(ctx, srv.URL)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, requestCount, 3, "Expected at least 3 requests")
	})

	t.Run("context canceled", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/healthz", r.URL.Path)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := BlockUntilServerReady(ctx, srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context")
	})
}
