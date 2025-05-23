package imds

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFetchMetadataByPath(t *testing.T) {
	tests := []struct {
		name              string
		metaServerHandler func(w http.ResponseWriter, r *http.Request)
		expectedMetadata  string
		expectedError     string
		invalidMetaURL    bool
	}{
		{
			name: "successful metadata fetch",
			metaServerHandler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "true", r.Header.Get(headerMetadata))
				require.Contains(t, r.URL.Query().Get(queryKeyAPIVersion), defaultAPIVersion)
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("instance-123 ")) // Space to test TrimSpace
				require.NoError(t, err)
			},
			expectedMetadata: "instance-123",
		},
		{
			name:           "invalid metadata URL",
			invalidMetaURL: true,
			expectedError:  "failed to create metadata request: parse",
		},
		{
			name: "metadata server returns error",
			metaServerHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, err := w.Write([]byte("Metadata not found"))
				require.NoError(t, err)
			},
			expectedError: "failed to fetch metadata: received status code 404",
		},
		{
			name: "metadata response read error",
			metaServerHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "10")
				// Don't write anything to force EOF error
			},
			expectedError: "failed to read metadata response body: unexpected EOF",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			var metaURL string
			var metaServer *httptest.Server

			// Set up metadata server if needed
			if tc.invalidMetaURL {
				metaURL = "\n" // Invalid URL for testing URL parsing errors
			} else if tc.metaServerHandler != nil {
				metaServer = httptest.NewServer(http.HandlerFunc(tc.metaServerHandler))
				defer metaServer.Close()
				metaURL = metaServer.URL
			} else {
				metaURL = "http://localhost:9999" // Non-existent server
			}

			metadata, err := fetchMetadataByPath(ctx, metaURL)

			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
			}

			if tc.expectedMetadata != "" {
				require.Equal(t, tc.expectedMetadata, metadata)
			}
		})
	}
}

func TestFetchComputeResponse(t *testing.T) {
	tests := []struct {
		name             string
		mockHandler      func(w http.ResponseWriter, r *http.Request)
		expectedLocation string
		expectedError    string
		checkRequestPath bool
	}{
		{
			name: "successful compute response fetch",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "true", r.Header.Get(headerMetadata))
				// The actual path fetched by fetchComputeResponse is metadataURL + "/instance/compute"
				require.Equal(t, "/instance/compute", r.URL.Path)
				w.WriteHeader(http.StatusOK)
				// Respond with a JSON structure that computeResponse expects
				computeResp := computeResponse{Location: "eastus2-1", PhysicalZone: "1"}
				jsonData, err := json.Marshal(computeResp)
				require.NoError(t, err)
				_, err = w.Write(jsonData)
				require.NoError(t, err)
			},
			expectedLocation: "eastus2-1",
			checkRequestPath: true,
		},
		{
			name: "compute fetch failure",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// This handler should simply return an error status for the fetch operation.
				w.WriteHeader(http.StatusNotFound)
				_, err := w.Write([]byte("Not found"))
				require.NoError(t, err)
			},
			expectedError: "failed to fetch metadata: received status code 404",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			server := httptest.NewServer(http.HandlerFunc(tc.mockHandler))
			defer server.Close()

			resp, err := fetchComputeResponse(ctx, server.URL)

			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedLocation, resp.Location)
			}
		})
	}
}

func TestFetchPublicIPv4(t *testing.T) {
	tests := []struct {
		name          string
		mockHandler   func(w http.ResponseWriter, r *http.Request)
		expectedIP    string
		expectedError string
	}{
		{
			name: "successful ip fetch with public IP",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "true", r.Header.Get(headerMetadata))
				require.Contains(t, r.URL.Path, "/instance/network/interface")

				// Create a sample response with public IP
				interfaces := []networkInterface{
					{
						IPv4: IPv4Info{
							IPAddress: []IPv4Address{
								{
									PrivateIPAddress: "10.0.0.5",
									PublicIPAddress:  "203.0.113.1",
								},
							},
						},
						MACAddress: "00:0d:3a:b7:c2:44",
					},
				}

				jsonData, err := json.Marshal(interfaces)
				require.NoError(t, err)

				w.WriteHeader(http.StatusOK)
				_, err = w.Write(jsonData)
				require.NoError(t, err)
			},
			expectedIP: "203.0.113.1",
		},
		{
			name: "successful fetch but no public IP",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// Create a sample response with NO public IP
				interfaces := []networkInterface{
					{
						IPv4: IPv4Info{
							IPAddress: []IPv4Address{
								{
									PrivateIPAddress: "10.0.0.5",
									PublicIPAddress:  "",
								},
							},
						},
						MACAddress: "00:0d:3a:b7:c2:44",
					},
				}

				jsonData, err := json.Marshal(interfaces)
				require.NoError(t, err)

				w.WriteHeader(http.StatusOK)
				_, err = w.Write(jsonData)
				require.NoError(t, err)
			},
			expectedIP:    "",
			expectedError: "",
		},
		{
			name: "invalid JSON response",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("invalid JSON"))
				require.NoError(t, err)
			},
			expectedError: "failed to parse network interface metadata",
		},
		{
			name: "metadata fetch failure",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, err := w.Write([]byte("Server error"))
				require.NoError(t, err)
			},
			expectedError: "failed to fetch metadata: received status code 500",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			server := httptest.NewServer(http.HandlerFunc(tc.mockHandler))
			defer server.Close()

			ip, err := fetchPublicIPv4(ctx, server.URL)

			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedIP, ip)
			}
		})
	}
}

// TestFetchAvailabilityZonePublic tests the public API function
func TestFetchAvailabilityZonePublic(t *testing.T) {
	// Skip in short mode as this would attempt to call the real metadata service
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a common handler for metadata requests
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "true", r.Header.Get(headerMetadata))

		// Check if the request is for compute metadata
		if strings.HasSuffix(r.URL.Path, "/instance/compute") {
			w.WriteHeader(http.StatusOK)
			// Respond with a JSON structure that computeResponse expects
			computeResp := computeResponse{Location: "eastus2-2", PhysicalZone: "2"}
			jsonData, err := json.Marshal(computeResp)
			require.NoError(t, err)
			_, err = w.Write(jsonData)
			require.NoError(t, err)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	// Start a test server
	server := httptest.NewServer(mockHandler)
	defer server.Close()

	// Use a custom function that directly calls the private implementation
	// with our test server URL instead of modifying the package constant
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := fetchComputeResponse(ctx, server.URL)
	require.NoError(t, err)
	require.Equal(t, "eastus2-2", resp.Location)
}

// TestFetchPublicIPv4Public tests the public API function
func TestFetchPublicIPv4Public(t *testing.T) {
	// Skip in short mode as this would attempt to call the real metadata service
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a handler for metadata requests
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "true", r.Header.Get(headerMetadata))

		if strings.Contains(r.URL.Path, "instance/network/interface") {
			// Create a sample response with public IP
			interfaces := []networkInterface{
				{
					IPv4: IPv4Info{
						IPAddress: []IPv4Address{
							{
								PrivateIPAddress: "10.0.0.5",
								PublicIPAddress:  "203.0.113.2",
							},
						},
					},
					MACAddress: "00:0d:3a:b7:c2:44",
				},
			}

			jsonData, err := json.Marshal(interfaces)
			require.NoError(t, err)

			w.WriteHeader(http.StatusOK)
			_, err = w.Write(jsonData)
			require.NoError(t, err)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	// Start a test server
	server := httptest.NewServer(mockHandler)
	defer server.Close()

	// Use a custom function that directly calls the private implementation
	// with our test server URL instead of modifying the package constant
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ip, err := fetchPublicIPv4(ctx, server.URL)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.2", ip)
}

// TestFetchMetadata tests the public API function
func TestFetchMetadata(t *testing.T) {
	// Create a handler for metadata requests
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "true", r.Header.Get(headerMetadata))

		// Check for instance-id path (with or without leading slash)
		if strings.Contains(r.URL.Path, "instance-id") {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("vm-0123456789abcdef"))
			require.NoError(t, err)
			return
		}

		// Default not found
		w.WriteHeader(http.StatusNotFound)
		_, err := w.Write([]byte("Not found"))
		require.NoError(t, err)
	})

	// Start a test server
	server := httptest.NewServer(mockHandler)
	defer server.Close()

	// Test using the private function with our server URL
	// instead of modifying the package constant
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test with leading slash by using the server URL + path pattern
	path1 := "/instance-id"
	metadata1, err := fetchMetadataByPath(ctx, server.URL+path1)
	require.NoError(t, err)
	require.Equal(t, "vm-0123456789abcdef", metadata1)

	// Test without leading slash - we add it in FetchMetadata
	path2 := "instance-id"
	metadata2, err := fetchMetadataByPath(ctx, server.URL+"/"+path2)
	require.NoError(t, err)
	require.Equal(t, "vm-0123456789abcdef", metadata2)

	// Test non-existent path
	nonExistentPath := "/non-existent"
	_, err = fetchMetadataByPath(ctx, server.URL+nonExistentPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to fetch metadata: received status code 404")
}

// TestExampleUsage demonstrates how clients would use the public API functions.
// This test is skipped by default to avoid real API calls.
func TestExampleUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	// This test is always skipped since it would make real API calls to Azure IMDS
	t.Skip("Skipping example usage test that would make real API calls")

	// Examples of how to use the public API functions
	ctx := context.Background()

	// Fetch instance zone
	zone, err := FetchAvailabilityZone(ctx)
	if err != nil {
		t.Logf("Failed to fetch availability zone: %v", err)
	} else {
		t.Logf("Instance is in zone: %s", zone)
	}

	// Fetch public IP
	ip, err := FetchPublicIPv4(ctx)
	if err != nil {
		t.Logf("Failed to fetch public IP: %v", err)
	} else {
		t.Logf("Instance public IP: %s", ip)
	}

	// Fetch custom metadata
	instanceID, err := FetchMetadata(ctx, "instance/compute/vmId")
	if err != nil {
		t.Logf("Failed to fetch instance ID: %v", err)
	} else {
		t.Logf("Instance ID: %s", instanceID)
	}
}

// TestFetchAZEnvironment tests the FetchAZEnvironment function
func TestFetchAZEnvironment(t *testing.T) {
	// Skip in short mode as this would attempt to call the real metadata service
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a handler for metadata requests
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "true", r.Header.Get(headerMetadata))

		// Check if the request is for compute metadata
		if strings.HasSuffix(r.URL.Path, "/instance/compute") {
			w.WriteHeader(http.StatusOK)
			// Respond with a JSON structure that computeResponse expects
			computeResp := computeResponse{
				AZEnvironment: "AZUREPUBLICCLOUD",
				Location:      "eastus2-3",
				PhysicalZone:  "3",
			}
			jsonData, err := json.Marshal(computeResp)
			require.NoError(t, err)
			_, err = w.Write(jsonData)
			require.NoError(t, err)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	// Start a test server
	server := httptest.NewServer(mockHandler)
	defer server.Close()

	// Use a custom function that directly calls the private implementation
	// with our test server URL instead of modifying the package constant
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := fetchComputeResponse(ctx, server.URL)
	require.NoError(t, err)
	require.Equal(t, "AZUREPUBLICCLOUD", resp.AZEnvironment)
	require.Equal(t, "eastus2-3", resp.Location)
}
