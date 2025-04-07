package login

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/leptonai/gpud/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogin(t *testing.T) {
	mockPublicIP := "192.0.2.1"
	mockUID := "test-uid"
	mockToken := "test-token"
	mockComponents := "component1,component2"
	mockName := "test-name"

	mockGetPublicIPSuccess := func() (string, error) {
		return mockPublicIP, nil
	}

	mockGetPublicIPError := func() (string, error) {
		return "", errors.New("failed to get public IP")
	}

	t.Run("Successful login", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req LoginRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)

			assert.Equal(t, mockName, req.Name)
			assert.Equal(t, mockUID, req.ID)
			assert.Equal(t, mockPublicIP, req.PublicIP)
			assert.Equal(t, "personal", req.Provider)
			assert.Equal(t, mockComponents, req.Components)
			assert.Equal(t, mockToken, req.Token)

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := login(mockName, mockToken, server.URL, mockComponents, mockUID, mockGetPublicIPSuccess)
		assert.NoError(t, err)
	})

	t.Run("Failed to get public IP", func(t *testing.T) {
		err := login(mockName, mockToken, "dummy-url", mockComponents, mockUID, mockGetPublicIPError)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch public ip")
		assert.Contains(t, err.Error(), "failed to get public IP")
	})

	t.Run("HTTP request failed (server down)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.Close() // Close server immediately

		err := login(mockName, mockToken, server.URL, mockComponents, mockUID, mockGetPublicIPSuccess)
		assert.Error(t, err)
	})

	t.Run("HTTP non-OK status (invalid token)", func(t *testing.T) {
		errorResp := LoginErrorResponse{
			Error:  "token mismatch",
			Status: "invalid workspace token provided",
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized) // Or any non-OK status
			err := json.NewEncoder(w).Encode(errorResp)
			require.NoError(t, err)
		}))
		defer server.Close()

		err := login(mockName, mockToken, server.URL, mockComponents, mockUID, mockGetPublicIPSuccess)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token provided")
		assert.Contains(t, err.Error(), "gpud login --token yourToken")
	})

	t.Run("HTTP non-OK status (generic error)", func(t *testing.T) {
		errorResp := LoginErrorResponse{
			Error:  "some backend error",
			Status: "processing failed",
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError) // Or any non-OK status
			err := json.NewEncoder(w).Encode(errorResp)
			require.NoError(t, err)
		}))
		defer server.Close()

		err := login(mockName, mockToken, server.URL, mockComponents, mockUID, mockGetPublicIPSuccess)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Currently, we only support machines with a public IP address")
		assert.Contains(t, err.Error(), fmt.Sprintf("%s:%d", mockPublicIP, config.DefaultGPUdPort))
		assert.Contains(t, err.Error(), "error: {some backend error processing failed}") // Check underlying error
	})

	t.Run("HTTP non-OK status (bad error response body)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, err := w.Write([]byte("this is not json"))
			require.NoError(t, err)
		}))
		defer server.Close()

		err := login(mockName, mockToken, server.URL, mockComponents, mockUID, mockGetPublicIPSuccess)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error parsing error response")
		assert.Contains(t, err.Error(), "Response body: this is not json")
	})
}

// Note: Testing the top-level Login function directly would require mocking netutil.PublicIP,
// which is less clean than testing the internal login function with dependency injection.
