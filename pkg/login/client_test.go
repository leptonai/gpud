package login

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestSendRequest_Success(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Return successful response
		w.WriteHeader(http.StatusOK)
		resp := apiv1.LoginResponse{
			MachineID: "test-machine-id",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify response
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "test-machine-id", resp.MachineID)
}

func TestSendRequest_HttpError(t *testing.T) {
	// Setup invalid server URL to cause HTTP error
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, "http://invalid-server-url.that.does.not.exist", req)

	// Verify error is returned
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSendRequest_BadStatusCode(t *testing.T) {
	// Setup mock server returning error status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := apiv1.LoginResponse{
			Error: "invalid credentials",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "invalid-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned but response is available
	assert.Error(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "invalid credentials", resp.Error)
}

func TestSendRequest_InvalidResponseFormat(t *testing.T) {
	// Setup mock server returning invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSendRequest_ContextCancellation(t *testing.T) {
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

	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSendRequest_EmptyMachineID(t *testing.T) {
	// Setup mock server returning empty machineID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := apiv1.LoginResponse{
			MachineID: "", // Empty machine ID
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify successful response (since validation was removed from the function)
	assert.True(t, errors.Is(err, ErrEmptyMachineID))
	assert.NotNil(t, resp)
	assert.Empty(t, resp.MachineID)
}

func TestSendRequest(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Return successful response
		w.WriteHeader(http.StatusOK)
		resp := apiv1.LoginResponse{
			MachineID: "test-machine-id",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify response
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "test-machine-id", resp.MachineID)
}

func TestSendRequestWithTimeout(t *testing.T) {
	// Setup mock server with delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add a delay to simulate network latency
		time.Sleep(100 * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		resp := apiv1.LoginResponse{
			MachineID: "test-machine-id",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned due to timeout
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestSendRequestWithInvalidURL(t *testing.T) {
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, "://invalid-url", req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSendRequestWithMarshalError(t *testing.T) {
	// This test is more of a placeholder since it's hard to force a JSON marshal error
	// with the current structure. In a real-world scenario, you might use a mock for json.Marshal
	// to force an error.
	t.Skip("Skipping as it's difficult to create a marshal error with the current structure")
}

func TestSendRequest_ServerError(t *testing.T) {
	// Setup mock server returning a 500 Internal Server Error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		resp := apiv1.LoginResponse{
			Error: "internal server error",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	assert.Error(t, err)
	assert.NotNil(t, resp)
	assert.Contains(t, err.Error(), "500")
	assert.Equal(t, "internal server error", resp.Error)
}

func TestSendRequest_ReadError(t *testing.T) {
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
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSendRequest_ComplexRequest(t *testing.T) {
	// Setup mock server to verify complex request data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse and verify request body
		var receivedReq apiv1.LoginRequest
		err := json.NewDecoder(r.Body).Decode(&receivedReq)
		assert.NoError(t, err)

		// Verify all fields are correctly sent
		assert.Equal(t, "test-token", receivedReq.Token)
		assert.Equal(t, "aws", receivedReq.Provider)
		assert.Equal(t, "192.168.1.1", receivedReq.Network.PrivateIP)

		// Return successful response
		w.WriteHeader(http.StatusOK)
		resp := apiv1.LoginResponse{
			MachineID: "test-machine-id",
			Status:    "success",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token:    "test-token",
		Provider: "aws",
		Network: &apiv1.MachineNetwork{
			PrivateIP: "192.168.1.1",
			PublicIP:  "203.0.113.1",
		},
		Location: &apiv1.MachineLocation{
			Region: "us-west-2",
			Zone:   "us-west-2a",
		},
	}

	resp, err := sendRequest(ctx, server.URL, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "test-machine-id", resp.MachineID)
	assert.Equal(t, "success", resp.Status)
}

func TestSendRequest_EmptyMachineIDReturnsError(t *testing.T) {
	// Setup mock server returning empty machineID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := apiv1.LoginResponse{
			MachineID: "", // Empty machine ID
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned for empty machineID
	assert.True(t, errors.Is(err, ErrEmptyMachineID))
	assert.NotNil(t, resp)
	assert.Empty(t, resp.MachineID)
}

func TestSendRequest_EmptyMachineIDWithMessageReturnsError(t *testing.T) {
	// Setup mock server returning empty machineID but with an error message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := apiv1.LoginResponse{
			MachineID: "",
			Error:     "Registration pending",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned even when there's an error message
	assert.True(t, errors.Is(err, ErrEmptyMachineID))
	assert.NotNil(t, resp)
	assert.Empty(t, resp.MachineID)
	assert.Equal(t, "Registration pending", resp.Error)
}

func TestSendRequest_EmptyMachineIDWithStatusReturnsError(t *testing.T) {
	// Setup mock server returning empty machineID but with status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := apiv1.LoginResponse{
			MachineID: "",
			Status:    "processing",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned even with a status
	assert.True(t, errors.Is(err, ErrEmptyMachineID))
	assert.NotNil(t, resp)
	assert.Empty(t, resp.MachineID)
	assert.Equal(t, "processing", resp.Status)
}

func TestSendRequest_EmptyMachineIDWithOKStatusCodeReturnsError(t *testing.T) {
	// Setup mock server returning empty machineID but with HTTP 200 OK
	// This simulates a case where server returns success but without a machine ID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := apiv1.LoginResponse{
			MachineID: "",
			Status:    "success", // Contradicting status
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Execute test
	ctx := context.Background()
	req := apiv1.LoginRequest{
		Token: "test-token",
	}

	resp, err := sendRequest(ctx, server.URL, req)

	// Verify error is returned despite success status
	assert.True(t, errors.Is(err, ErrEmptyMachineID))
	assert.NotNil(t, resp)
	assert.Empty(t, resp.MachineID)
	assert.Equal(t, "success", resp.Status)
}
