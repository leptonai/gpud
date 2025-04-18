package gossip

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestCreateURL(t *testing.T) {
	tests := []struct {
		endpoint string
		expected string
	}{
		{"example.com", "https://example.com/api/v1/gossip"},
		{"api.leptonai.com", "https://api.leptonai.com/api/v1/gossip"},
	}

	for _, tc := range tests {
		url := createURL(tc.endpoint)
		assert.Equal(t, tc.expected, url)
	}
}

func TestSendRequest_Success(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Return successful response
		w.WriteHeader(http.StatusOK)
		resp := apiv1.GossipResponse{
			Status: "success",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1", "component2"},
	}

	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify response
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "success", resp.Status)
}

func TestSendRequest_NoUsageStats(t *testing.T) {
	// Set environment variable to disable stats
	os.Setenv("GPUD_NO_USAGE_STATS", "true")
	defer os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Even with a valid server, no request should be made
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler should never be called
		t.Error("Server should not receive request when GPUD_NO_USAGE_STATS=true")
	}))
	defer server.Close()

	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify no response and no error
	assert.Nil(t, resp)
	assert.Nil(t, err)
}

func TestSendRequest_HttpError(t *testing.T) {
	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Setup invalid server URL to cause HTTP error
	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	resp, err := sendRequest(ctx, "http://invalid-server-url.that.does.not.exist", req)

	// Verify error is returned
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSendRequest_BadStatusCode(t *testing.T) {
	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Setup mock server returning error status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := apiv1.GossipResponse{
			Error: "invalid request",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned but response is available
	assert.Error(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "invalid request", resp.Error)
}

func TestSendRequest_InvalidResponseFormat(t *testing.T) {
	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Setup mock server returning invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "error unmarshaling gossip response")
}

func TestSendRequest_ContextCancellation(t *testing.T) {
	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Setup mock server with delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This sleep is intentionally left in place for the test
		<-r.Context().Done()
		// Context was canceled, connection should be aborted
	}))
	defer server.Close()

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context immediately

	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSendRequestWithTimeout(t *testing.T) {
	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Setup mock server with delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add a delay to simulate network latency
		time.Sleep(100 * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		resp := apiv1.GossipResponse{
			Status: "success",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned due to timeout
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestSendRequestWithInvalidURL(t *testing.T) {
	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	resp, err := sendRequest(ctx, "://invalid-url", req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSendRequest_ServerError(t *testing.T) {
	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Setup mock server returning a 500 Internal Server Error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		resp := apiv1.GossipResponse{
			Error: "internal server error",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	resp, err := sendRequest(ctx, server.URL, req)

	assert.Error(t, err)
	assert.NotNil(t, resp)
	assert.Contains(t, err.Error(), "500")
	assert.Equal(t, "internal server error", resp.Error)
}

func TestSendRequest_ReadError(t *testing.T) {
	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Setup mock server that will close the connection without sending a complete response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force-close the connection by hijacking it
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Skip("Hijacking not supported")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("Failed to hijack connection: %v", err)
		}
		conn.Close() // This forces the connection to close abruptly
	}))
	defer server.Close()

	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	resp, err := sendRequest(ctx, server.URL, req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSendRequest_ComplexRequest(t *testing.T) {
	// Ensure GPUD_NO_USAGE_STATS is not set to "true"
	os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Setup mock server to verify complex request data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse and verify request body
		var receivedReq apiv1.GossipRequest
		err := json.NewDecoder(r.Body).Decode(&receivedReq)
		assert.NoError(t, err)

		// Verify all fields are correctly sent
		assert.Equal(t, "test-machine-id", receivedReq.MachineID)
		assert.Equal(t, "2.0.0", receivedReq.DaemonVersion)
		assert.Equal(t, 3, len(receivedReq.Components))
		assert.Contains(t, receivedReq.Components, "component1")
		assert.Contains(t, receivedReq.Components, "component2")
		assert.Contains(t, receivedReq.Components, "component3")

		// Return successful response
		w.WriteHeader(http.StatusOK)
		resp := apiv1.GossipResponse{
			Status: "success",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "2.0.0",
		Components:    []string{"component1", "component2", "component3"},
	}

	resp, err := sendRequest(ctx, server.URL, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "success", resp.Status)
}

func TestSendRequest_PublicWrapper(t *testing.T) {
	if os.Getenv("TEST_GOSSIP_CLIENT") != "true" {
		t.Skip("Skipping test that would make a real external API call")
	}

	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify we're hitting the correct endpoint format in createURL
		assert.True(t, r.URL.Path == "/api/v1/gossip")

		// Return successful response
		w.WriteHeader(http.StatusOK)
		resp := apiv1.GossipResponse{
			Status: "success",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Extract just the host:port part from the URL
	serverURL := server.URL[7:] // Remove "http://" prefix

	ctx := context.Background()
	req := apiv1.GossipRequest{
		MachineID:     "test-machine-id",
		DaemonVersion: "1.0.0",
		Components:    []string{"component1"},
	}

	// Use the public function which should call createURL
	// We're not actually using the server's URL directly since SendRequest will create its own URL
	// Instead, we'll modify the mock server handler to accept a different path
	resp, err := SendRequest(ctx, serverURL, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}
