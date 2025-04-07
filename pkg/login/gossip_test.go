package login

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leptonai/gpud/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGossip(t *testing.T) {
	mockUID := "test-uid"
	mockComponents := []string{"compA", "compB"}
	mockEndpoint := ""

	t.Run("Gossip skipped due to env var", func(t *testing.T) {
		t.Setenv("GPUD_NO_USAGE_STATS", "true")
		err := Gossip(mockEndpoint, mockUID, "dummy-url", mockComponents)
		assert.NoError(t, err)
	})

	t.Run("Successful gossip", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req GossipRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)

			assert.Equal(t, mockUID, req.Name)
			assert.Equal(t, mockUID, req.ID)
			assert.Equal(t, "personal", req.Provider)
			assert.Equal(t, version.Version, req.DaemonVersion)
			assert.Equal(t, strings.Join(mockComponents, ","), req.Components)

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := Gossip(mockEndpoint, mockUID, server.URL, mockComponents)
		assert.NoError(t, err)
	})

	t.Run("HTTP request failed (server down)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.Close() // Close server immediately

		err := Gossip(mockEndpoint, mockUID, server.URL, mockComponents)
		assert.Error(t, err)
	})

	t.Run("HTTP non-OK status", func(t *testing.T) {
		errorResp := GossipErrorResponse{
			Error:  "gossip rejected",
			Status: "failed",
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			err := json.NewEncoder(w).Encode(errorResp)
			require.NoError(t, err)
		}))
		defer server.Close()

		// Note: The current Gossip implementation doesn't return an error on non-200 status
		// unless parsing the error response fails. It logs the error but returns nil.
		// This test verifies that behavior.
		err := Gossip(mockEndpoint, mockUID, server.URL, mockComponents)
		assert.NoError(t, err) // Expecting nil error despite non-200 status
	})

	t.Run("HTTP non-OK status (bad error response body)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte("invalid json"))
			require.NoError(t, err)
		}))
		defer server.Close()

		err := Gossip(mockEndpoint, mockUID, server.URL, mockComponents)
		assert.Error(t, err) // Expecting error because error response parsing fails
		assert.Contains(t, err.Error(), "Error parsing error response")
		assert.Contains(t, err.Error(), "Response body: invalid json")
	})
}
