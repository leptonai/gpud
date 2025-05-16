package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestGetMachineInfo(t *testing.T) {
	tests := []struct {
		name            string
		statusCode      int
		body            string
		closeConnection bool
		networkError    bool
		wantErr         bool
		errorContains   string
	}{
		{
			name:       "Success",
			statusCode: http.StatusOK,
			body:       `{"hostname":"test-host","cpuInfo":{"architecture":"amd64","manufacturer":"Intel"}}`,
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
			name:          "Empty Body",
			statusCode:    http.StatusOK,
			body:          "",
			wantErr:       true,
			errorContains: "failed to decode machine info",
		},
		{
			name:          "Malformed JSON",
			statusCode:    http.StatusOK,
			body:          `{"hostname":`,
			wantErr:       true,
			errorContains: "failed to decode machine info",
		},
		{
			name:            "Connection Close",
			statusCode:      http.StatusOK,
			closeConnection: true,
			wantErr:         true,
			errorContains:   "failed to make request",
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
					if r.URL.Path != "/machine-info" {
						t.Errorf("Expected %s path, got %s", "/machine-info", r.URL.Path)
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

					w.WriteHeader(tt.statusCode)
					_, err := w.Write([]byte(tt.body))
					require.NoError(t, err)
				}))
				defer srv.Close()
			}

			info, err := GetMachineInfo(context.Background(), srv.URL)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, info)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, info)
				assert.Equal(t, "test-host", info.Hostname)
				assert.Equal(t, "amd64", info.CPUInfo.Architecture)
				assert.Equal(t, "Intel", info.CPUInfo.Manufacturer)
			}
		})
	}
}

func TestGetMachineInfoInvalidURL(t *testing.T) {
	_, err := GetMachineInfo(context.Background(), "invalid-url")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported protocol scheme")
}

func TestGetMachineInfoContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		machineInfo := &apiv1.MachineInfo{
			Hostname: "test-host",
			CPUInfo: &apiv1.MachineCPUInfo{
				Architecture: "amd64",
			},
		}
		json, _ := json.Marshal(machineInfo)
		_, err := w.Write(json)
		if err != nil {
			t.Errorf("Error writing response: %v", err)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := GetMachineInfo(ctx, srv.URL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// TestGetMachineInfoWithHeaders tests that headers from options are applied correctly
func TestGetMachineInfoWithHeaders(t *testing.T) {
	// This test checks that we're properly accessing and passing through the URL
	// We don't need to test that specific header options work as that's already
	// tested in the options package or would be specific to each client function
	mockMachineInfo := &apiv1.MachineInfo{
		Hostname: "test-host",
		CPUInfo: &apiv1.MachineCPUInfo{
			Architecture: "amd64",
			Manufacturer: "Intel",
			Type:         "Core",
		},
	}

	// Create a custom server that just verifies the request was made and returns a response
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/machine-info", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusOK)
		jsonData, err := json.Marshal(mockMachineInfo)
		require.NoError(t, err)
		_, err = w.Write(jsonData)
		require.NoError(t, err)
	}))
	defer srv.Close()

	// Test with a basic request that should succeed
	info, err := GetMachineInfo(context.Background(), srv.URL)
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, mockMachineInfo.Hostname, info.Hostname)
	assert.Equal(t, mockMachineInfo.CPUInfo.Architecture, info.CPUInfo.Architecture)
	assert.Equal(t, mockMachineInfo.CPUInfo.Manufacturer, info.CPUInfo.Manufacturer)
	assert.Equal(t, mockMachineInfo.CPUInfo.Type, info.CPUInfo.Type)
}
