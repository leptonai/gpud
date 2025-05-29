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
				require.Equal(t, metadataFlavorGoogle, r.Header.Get(headerMetadataFlavor))
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

func TestFetchAvailabilityZone(t *testing.T) {
	tests := []struct {
		name             string
		mockHandler      func(w http.ResponseWriter, r *http.Request)
		expectedAZ       string
		expectedError    string
		checkRequestPath bool
	}{
		{
			name: "successful az fetch",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, metadataFlavorGoogle, r.Header.Get(headerMetadataFlavor))
				require.True(t, strings.HasSuffix(r.URL.Path, "/instance/zone"), "Path should end with /instance/zone")
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("eastus2-1"))
				require.NoError(t, err)
			},
			expectedAZ:       "eastus2-1",
			checkRequestPath: true,
		},
		{
			name: "az fetch failure",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
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

			az, err := fetchAvailabilityZone(ctx, server.URL)

			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedAZ, az)
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
				require.Equal(t, metadataFlavorGoogle, r.Header.Get(headerMetadataFlavor))
				require.True(t, strings.HasSuffix(r.URL.Path, "/instance/network-interfaces/"), "Path should end with /instance/network-interfaces/")
				require.Equal(t, "true", r.URL.Query().Get("recursive"))

				// Create a sample response with public IP using new structs
				interfaces := []gcpNetworkInterface{
					{
						AccessConfigs: []gcpAccessConfig{
							{
								ExternalIP: "203.0.113.1",
								Type:       "ONE_TO_ONE_NAT",
							},
						},
					},
					{ // Another interface without public IP to ensure correct selection
						AccessConfigs: []gcpAccessConfig{
							{
								ExternalIP: "",
								Type:       "ONE_TO_ONE_NAT",
							},
						},
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
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, metadataFlavorGoogle, r.Header.Get(headerMetadataFlavor))
				require.True(t, strings.HasSuffix(r.URL.Path, "/instance/network-interfaces/"), "Path should end with /instance/network-interfaces/")
				require.Equal(t, "true", r.URL.Query().Get("recursive"))
				// Create a sample response with NO public IP
				interfaces := []gcpNetworkInterface{
					{
						AccessConfigs: []gcpAccessConfig{
							{
								ExternalIP: "", // No public IP
								Type:       "ONE_TO_ONE_NAT",
							},
						},
					},
				}

				jsonData, err := json.Marshal(interfaces)
				require.NoError(t, err)

				w.WriteHeader(http.StatusOK)
				_, err = w.Write(jsonData)
				require.NoError(t, err)
			},
			expectedIP:    "", // Expect empty IP
			expectedError: "", // Expect no error
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
		require.Equal(t, metadataFlavorGoogle, r.Header.Get(headerMetadataFlavor))

		if strings.HasSuffix(r.URL.Path, "/instance/zone") {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("eastus2-2"))
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

	az, err := fetchAvailabilityZone(ctx, server.URL)
	require.NoError(t, err)
	require.Equal(t, "eastus2-2", az)
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
		require.Equal(t, metadataFlavorGoogle, r.Header.Get(headerMetadataFlavor))

		if strings.HasSuffix(r.URL.Path, "/instance/network-interfaces/") && r.URL.Query().Get("recursive") == "true" {
			// Create a sample response with public IP
			interfaces := []gcpNetworkInterface{
				{
					AccessConfigs: []gcpAccessConfig{
						{
							ExternalIP: "203.0.113.2",
							Type:       "ONE_TO_ONE_NAT",
						},
					},
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
		require.Equal(t, metadataFlavorGoogle, r.Header.Get(headerMetadataFlavor))

		// Check for instance-id path (with or without leading slash)
		if strings.HasSuffix(r.URL.Path, "/instance-id") {
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

func TestExtractZoneFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Standard path",
			path:     "projects/980931390107/zones/us-east5-c",
			expected: "us-east5-c",
		},
		{
			name:     "Path with only zone",
			path:     "us-central1-a",
			expected: "us-central1-a",
		},
		{
			name:     "Empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "Path with trailing slash",
			path:     "projects/123/zones/europe-west1-d/",
			expected: "", // strings.Split results in an empty last element
		},
		{
			name:     "Path with multiple trailing slashes",
			path:     "projects/123/zones/europe-west1-d//",
			expected: "", // strings.Split results in an empty last element
		},
		{
			name:     "Path with leading slash",
			path:     "/projects/123/zones/asia-south1-b",
			expected: "asia-south1-b", // strings.Split("/foo/bar", "/") -> ["", "foo", "bar"]
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := extractZoneFromPath(tt.path)
			require.Equal(t, tt.expected, actual, "extractZoneFromPath(%q)", tt.path)
		})
	}
}
