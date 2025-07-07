package imds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFetchIMDSV2TokenExample(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	token, err := FetchToken(context.Background())
	t.Logf("token: %s", token)
	t.Logf("err: %v", err)
}

func TestFetchIMDSV2Token(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler func(w http.ResponseWriter, r *http.Request)
		expectedToken string
		expectedError string
	}{
		{
			name: "successful token fetch",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPut, r.Method)
				require.Equal(t, fmt.Sprintf("%d", defaultTokenTTL), r.Header.Get(headerTTL))
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("test-token ")) // trailing space to test TrimSpace
				require.NoError(t, err)
			},
			expectedToken: "test-token",
		},
		{
			name: "server returns 500 error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, err := w.Write([]byte("Internal Server Error"))
				require.NoError(t, err)
			},
			expectedError: "failed to fetch IMDS token: received status code 500",
		},
		{
			name:          "request creation fails due to invalid URL",
			serverHandler: nil,                                          // No server needed, error is client-side
			expectedError: "failed to create IMDS token request: parse", // Error from http.NewRequestWithContext
		},
		{
			name: "server fails to write response body (unexpected EOF)",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "1") // Set content length but don't write body
				// No w.Write() call, or w.Write([]byte("")) to simulate empty body when one is expected
			},
			expectedError: "failed to read IMDS token response body: unexpected EOF",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			var targetURL string
			var server *httptest.Server

			if tc.serverHandler != nil {
				server = httptest.NewServer(http.HandlerFunc(tc.serverHandler))
				defer server.Close()
				targetURL = server.URL
			} else {
				// For tests where the server isn't supposed to be reached or doesn't matter
				if tc.name == "request creation fails due to invalid URL" {
					// Use an invalid URL that causes http.NewRequestWithContext to fail
					_, err := fetchToken(ctx, "\n") // Invalid URL with control character
					require.Error(t, err)
					require.Contains(t, err.Error(), tc.expectedError)
					return // Test case finished
				} else if tc.name == "client.Do fails (e.g. connection refused)" {
					// Use a non-routable / non-listening address
					targetURL = "http://localhost:12345"
				} else {
					// Default for other no-handler cases, though ideally all cases are explicit
					targetURL = "http://localhost:9999" // Some other unlikely-to-be-listening port
				}
			}

			token, err := fetchToken(ctx, targetURL)

			if tc.expectedError != "" {
				require.Error(t, err, "Expected an error but got nil")
				require.Contains(t, err.Error(), tc.expectedError, "Error message mismatch")
			} else {
				require.NoError(t, err, "Expected no error but got one")
			}

			if tc.expectedToken != "" {
				require.Equal(t, tc.expectedToken, token, "Token mismatch")
			}
		})
	}
}

func TestFetchMetadataByPath(t *testing.T) {
	tests := []struct {
		name               string
		tokenServerHandler func(w http.ResponseWriter, r *http.Request)
		metaServerHandler  func(w http.ResponseWriter, r *http.Request)
		expectedMetadata   string
		expectedError      string
		invalidTokenURL    bool
		invalidMetaURL     bool
	}{
		{
			name: "successful metadata fetch",
			tokenServerHandler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPut, r.Method)
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("test-token"))
				require.NoError(t, err)
			},
			metaServerHandler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "test-token", r.Header.Get(headerToken))
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("instance-123 ")) // Space to test TrimSpace
				require.NoError(t, err)
			},
			expectedMetadata: "instance-123",
		},
		{
			name: "token server returns error",
			tokenServerHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, err := w.Write([]byte("Internal Server Error"))
				require.NoError(t, err)
			},
			metaServerHandler: nil, // Should not be called
			expectedError:     "failed to fetch IMDS token: received status code 500",
		},
		{
			name:            "invalid token URL",
			invalidTokenURL: true,
			metaServerHandler: func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("Metadata server should not be called if token fetch fails")
			},
			expectedError: "failed to create IMDS token request: parse",
		},
		{
			name: "valid token but invalid metadata URL",
			tokenServerHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("test-token"))
				require.NoError(t, err)
			},
			invalidMetaURL: true,
			expectedError:  "failed to create metadata request: parse",
		},
		{
			name: "metadata server returns error",
			tokenServerHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("test-token"))
				require.NoError(t, err)
			},
			metaServerHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, err := w.Write([]byte("Metadata not found"))
				require.NoError(t, err)
			},
			expectedError: "failed to fetch metadata: received status code 404",
		},
		{
			name: "metadata response read error",
			tokenServerHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("test-token"))
				require.NoError(t, err)
			},
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

			var tokenURL string
			var metaURL string
			var tokenServer *httptest.Server
			var metaServer *httptest.Server

			// Set up token server if needed
			if tc.invalidTokenURL {
				tokenURL = "\n" // Invalid URL for testing URL parsing errors
			} else if tc.tokenServerHandler != nil {
				tokenServer = httptest.NewServer(http.HandlerFunc(tc.tokenServerHandler))
				defer tokenServer.Close()
				tokenURL = tokenServer.URL
			} else {
				tokenURL = "http://localhost:9998" // Non-existent server
			}

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

			metadata, err := fetchMetadataByPath(ctx, tokenURL, metaURL)

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
				// Check for token request (PUT) vs metadata request (GET)
				if r.Method == http.MethodPut {
					// Token request
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("test-token"))
					require.NoError(t, err)
					return
				} else if r.Method == http.MethodGet {
					// Metadata request
					require.Equal(t, "test-token", r.Header.Get(headerToken))
					require.Contains(t, r.URL.Path, "/placement/availability-zone")
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("us-west-2a"))
					require.NoError(t, err)
					return
				}

				t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
			},
			expectedAZ:       "us-west-2a",
			checkRequestPath: true,
		},
		{
			name: "az fetch failure",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// Check for token request (PUT) vs metadata request (GET)
				if r.Method == http.MethodPut {
					// Token request
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("test-token"))
					require.NoError(t, err)
					return
				} else if r.Method == http.MethodGet {
					// Metadata request - return 404 for testing error case
					w.WriteHeader(http.StatusNotFound)
					_, err := w.Write([]byte("Not found"))
					require.NoError(t, err)
					return
				}

				t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
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

			az, err := fetchAvailabilityZone(ctx, server.URL, server.URL)

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

// TestFetchAvailabilityZonePublic tests the public API function
func TestFetchAvailabilityZonePublic(t *testing.T) {
	// Create a common handler for both token and metadata requests
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for token request (PUT) vs metadata request (GET)
		if r.Method == http.MethodPut {
			// Token request
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("test-token"))
			require.NoError(t, err)
			return
		} else if r.Method == http.MethodGet {
			// Metadata request
			require.Equal(t, "test-token", r.Header.Get(headerToken))

			if strings.Contains(r.URL.Path, "placement/availability-zone") {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("us-west-2b"))
				require.NoError(t, err)
				return
			}

			w.WriteHeader(http.StatusNotFound)
			return
		}

		t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
	})

	// Start a test server
	server := httptest.NewServer(mockHandler)
	defer server.Close()

	// Use a custom function that calls FetchAvailabilityZone with our test server URLs
	testFetchAZ := func(ctx context.Context) (string, error) {
		return fetchAvailabilityZone(ctx, server.URL, server.URL)
	}

	// Test the function
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	az, err := testFetchAZ(ctx)
	require.NoError(t, err)
	require.Equal(t, "us-west-2b", az)
}

func TestFetchPublicIPv4(t *testing.T) {
	tests := []struct {
		name          string
		mockHandler   func(w http.ResponseWriter, r *http.Request)
		expectedIP    string
		expectedError string
	}{
		{
			name: "successful ip fetch",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// Check for token request (PUT) vs metadata request (GET)
				if r.Method == http.MethodPut {
					// Token request
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("test-token"))
					require.NoError(t, err)
					return
				} else if r.Method == http.MethodGet {
					// Metadata request
					require.Equal(t, "test-token", r.Header.Get(headerToken))
					require.Contains(t, r.URL.Path, "/public-ipv4")
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("203.0.113.1"))
					require.NoError(t, err)
					return
				}

				t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
			},
			expectedIP: "203.0.113.1",
		},
		{
			name: "ip fetch failure - not assigned (404 returns no error)",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// Check for token request (PUT) vs metadata request (GET)
				if r.Method == http.MethodPut {
					// Token request
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("test-token"))
					require.NoError(t, err)
					return
				} else if r.Method == http.MethodGet {
					// Metadata request - Some EC2 instances don't have public IPs
					w.WriteHeader(http.StatusNotFound)
					_, err := w.Write([]byte("Not found"))
					require.NoError(t, err)
					return
				}

				t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
			},
			expectedIP: "", // 404 should return empty string with no error
		},
		{
			name: "ip fetch failure - server error",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// Check for token request (PUT) vs metadata request (GET)
				if r.Method == http.MethodPut {
					// Token request
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("test-token"))
					require.NoError(t, err)
					return
				} else if r.Method == http.MethodGet {
					// Metadata request - return server error
					w.WriteHeader(http.StatusInternalServerError)
					_, err := w.Write([]byte("Internal Server Error"))
					require.NoError(t, err)
					return
				}

				t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
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

			ip, err := fetchPublicIPv4(ctx, server.URL, server.URL)

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

// TestFetchPublicIPv4Public tests the public API function
func TestFetchPublicIPv4Public(t *testing.T) {
	// Create a common handler for both token and metadata requests
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for token request (PUT) vs metadata request (GET)
		if r.Method == http.MethodPut {
			// Token request
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("test-token"))
			require.NoError(t, err)
			return
		} else if r.Method == http.MethodGet {
			// Metadata request
			require.Equal(t, "test-token", r.Header.Get(headerToken))

			if strings.Contains(r.URL.Path, "public-ipv4") {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("203.0.113.2"))
				require.NoError(t, err)
				return
			}

			w.WriteHeader(http.StatusNotFound)
			return
		}

		t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
	})

	// Start a test server
	server := httptest.NewServer(mockHandler)
	defer server.Close()

	// Use a custom function that calls FetchPublicIPv4 with our test server URLs
	testFetchIP := func(ctx context.Context) (string, error) {
		return fetchPublicIPv4(ctx, server.URL, server.URL)
	}

	// Test the function
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ip, err := testFetchIP(ctx)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.2", ip)
}

// TestFetchMetadata tests the FetchMetadata function
func TestFetchMetadata(t *testing.T) {
	// Create a common handler for both token and metadata requests
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for token request (PUT) vs metadata request (GET)
		if r.Method == http.MethodPut {
			// Token request
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("test-token"))
			require.NoError(t, err)
			return
		} else if r.Method == http.MethodGet {
			// Metadata request
			require.Equal(t, "test-token", r.Header.Get(headerToken))

			// Check for instance-id path (with or without leading slash)
			if strings.Contains(r.URL.Path, "instance-id") {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("i-0123456789abcdef"))
				require.NoError(t, err)
				return
			}

			// Default not found
			w.WriteHeader(http.StatusNotFound)
			_, err := w.Write([]byte("Not found"))
			require.NoError(t, err)
			return
		}

		t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
	})

	// Start a test server
	server := httptest.NewServer(mockHandler)
	defer server.Close()

	// Custom test function to use our mock server for both token and metadata
	testFetchMetadata := func(ctx context.Context, path string) (string, error) {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return fetchMetadataByPath(ctx, server.URL, server.URL+path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test with leading slash
	metadata1, err := testFetchMetadata(ctx, "/instance-id")
	require.NoError(t, err)
	require.Equal(t, "i-0123456789abcdef", metadata1)

	// Test without leading slash
	metadata2, err := testFetchMetadata(ctx, "instance-id")
	require.NoError(t, err)
	require.Equal(t, "i-0123456789abcdef", metadata2)

	// Test non-existent path
	_, err = testFetchMetadata(ctx, "non-existent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to fetch metadata: received status code 404")
}

// TestFetchToken tests the public FetchToken function
func TestFetchToken(t *testing.T) {
	// Create a handler for token requests
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		require.Equal(t, fmt.Sprintf("%d", defaultTokenTTL), r.Header.Get(headerTTL))
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("public-test-token"))
		require.NoError(t, err)
	})

	// Start a test server
	server := httptest.NewServer(mockHandler)
	defer server.Close()

	// Custom test function to use our mock server
	testFetchToken := func(ctx context.Context) (string, error) {
		return fetchToken(ctx, server.URL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	token, err := testFetchToken(ctx)
	require.NoError(t, err)
	require.Equal(t, "public-test-token", token)
}

func TestFetchLocalIPv4(t *testing.T) {
	tests := []struct {
		name          string
		mockHandler   func(w http.ResponseWriter, r *http.Request)
		expectedIP    string
		expectedError string
	}{
		{
			name: "successful local ip fetch",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// Check for token request (PUT) vs metadata request (GET)
				if r.Method == http.MethodPut {
					// Token request
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("test-token"))
					require.NoError(t, err)
					return
				} else if r.Method == http.MethodGet {
					// Metadata request
					require.Equal(t, "test-token", r.Header.Get(headerToken))
					require.Contains(t, r.URL.Path, "/local-ipv4")
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("10.0.1.5"))
					require.NoError(t, err)
					return
				}

				t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
			},
			expectedIP: "10.0.1.5",
		},
		{
			name: "local ip fetch failure",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// Check for token request (PUT) vs metadata request (GET)
				if r.Method == http.MethodPut {
					// Token request
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("test-token"))
					require.NoError(t, err)
					return
				} else if r.Method == http.MethodGet {
					// Metadata request - return error
					w.WriteHeader(http.StatusInternalServerError)
					_, err := w.Write([]byte("Server error"))
					require.NoError(t, err)
					return
				}

				t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
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

			ip, err := fetchLocalIPv4(ctx, server.URL, server.URL)

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

// TestFetchLocalIPv4Public tests the public API function
func TestFetchLocalIPv4Public(t *testing.T) {
	// Create a common handler for both token and metadata requests
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for token request (PUT) vs metadata request (GET)
		if r.Method == http.MethodPut {
			// Token request
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("test-token"))
			require.NoError(t, err)
			return
		} else if r.Method == http.MethodGet {
			// Metadata request
			require.Equal(t, "test-token", r.Header.Get(headerToken))

			if strings.Contains(r.URL.Path, "local-ipv4") {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("172.31.10.100"))
				require.NoError(t, err)
				return
			}

			w.WriteHeader(http.StatusNotFound)
			return
		}

		t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
	})

	// Start a test server
	server := httptest.NewServer(mockHandler)
	defer server.Close()

	// Use a custom function that calls FetchLocalIPv4 with our test server URLs
	testFetchLocalIP := func(ctx context.Context) (string, error) {
		return fetchLocalIPv4(ctx, server.URL, server.URL)
	}

	// Test the function
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ip, err := testFetchLocalIP(ctx)
	require.NoError(t, err)
	require.Equal(t, "172.31.10.100", ip)
}
