package netutil

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPublicIP testing the PublicIP function
func TestPublicIP(t *testing.T) {
	ip, err := PublicIP()
	if err != nil {
		t.Logf("PublicIP test skipped due to error: %v", err)
		t.Skip("PublicIP test requires network access")
	}

	assert.NotEmpty(t, ip, "PublicIP should return a non-empty string")

	// Validate that the returned IP is actually a valid IP address
	parsed := net.ParseIP(ip)
	assert.NotNil(t, parsed, "Returned IP should be valid: %s", ip)
}

// TestDiscoverPublicIPWithMockServer tests the internal discoverPublicIP function using a mock HTTP server
func TestDiscoverPublicIPWithMockServer(t *testing.T) {
	// Define the test cases
	testCases := []struct {
		name           string
		serverResponse string
		serverStatus   int
		serverDelay    time.Duration
		expectError    bool
		expectedIP     string
	}{
		{
			name:           "Success with valid IPv4",
			serverResponse: "192.168.1.1",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    false,
			expectedIP:     "192.168.1.1",
		},
		{
			name:           "Success with valid IPv6",
			serverResponse: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    false,
			expectedIP:     "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		},
		{
			name:           "Success with whitespace trimmed",
			serverResponse: "  192.168.1.1  \n",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    false,
			expectedIP:     "192.168.1.1",
		},
		{
			name:           "Error with server error status",
			serverResponse: "Internal Server Error",
			serverStatus:   http.StatusInternalServerError,
			serverDelay:    0,
			expectError:    true, // Function checks HTTP status codes
			expectedIP:     "",
		},
		{
			name:           "Error with timeout",
			serverResponse: "192.168.1.1",
			serverStatus:   http.StatusOK,
			serverDelay:    11 * time.Second, // Client timeout is 10 seconds
			expectError:    true,
			expectedIP:     "",
		},
		{
			name:           "Error with empty response",
			serverResponse: "",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    true, // Function validates IP addresses
			expectedIP:     "",
		},
		{
			name:           "Error with invalid IP address",
			serverResponse: "not.an.ip.address",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    true,
			expectedIP:     "",
		},
		{
			name:           "Error with 404 status",
			serverResponse: "Not Found",
			serverStatus:   http.StatusNotFound,
			serverDelay:    0,
			expectError:    true,
			expectedIP:     "",
		},
		{
			name:           "Success with loopback IP",
			serverResponse: "127.0.0.1",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    false,
			expectedIP:     "127.0.0.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check if the User-Agent header is set properly
				userAgent := r.Header.Get("User-Agent")
				assert.Equal(t, "curl/7.64.1", userAgent, "User-Agent header should be set correctly")

				// Add delay if specified
				if tc.serverDelay > 0 {
					time.Sleep(tc.serverDelay)
				}

				// Set the response status code
				w.WriteHeader(tc.serverStatus)
				// Write the response
				_, _ = w.Write([]byte(tc.serverResponse))
			}))
			defer server.Close()

			// Call the function under test
			ip, err := discoverPublicIP(server.URL)

			// Check the results
			if tc.expectError {
				assert.Error(t, err, "Expected an error but got none")
				assert.Empty(t, ip, "IP should be empty when there's an error")
			} else {
				assert.NoError(t, err, "Unexpected error: %v", err)
				assert.Equal(t, tc.expectedIP, ip, "IP address doesn't match expected value")
			}
		})
	}
}

// TestDiscoverPublicIPMalformedResponse tests handling of malformed responses
func TestDiscoverPublicIPMalformedResponse(t *testing.T) {
	// Create a server that will return a malformed response (e.g., truncated)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack the connection to send a malformed response
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("httptest.ResponseRecorder does not implement http.Hijacker")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = conn.Close()
		}()

		// Send a partial/malformed HTTP response
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\n"))
		_, _ = conn.Write([]byte("Content-Length: 1000\r\n\r\n")) // Set a large content length
		_, _ = conn.Write([]byte("192.168.1."))                   // Send incomplete data
		// Don't close properly to cause a read error
	}))
	defer server.Close()

	// Call the function under test
	_, err := discoverPublicIP(server.URL)

	// Should return an error
	assert.Error(t, err, "Expected an error due to malformed response")
}

// TestPublicIPRetryLogic tests the retry logic in PublicIP function
func TestPublicIPRetryLogic(t *testing.T) {
	// Save original URLs and restore after test
	originalURLs := publicIPDiscoverURLs
	defer func() {
		publicIPDiscoverURLs = originalURLs
	}()

	t.Run("Success on first URL", func(t *testing.T) {
		// Create a server that returns a valid IP
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("203.0.113.1"))
		}))
		defer server.Close()

		// Override URLs to use our test server
		publicIPDiscoverURLs = []string{server.URL}

		ip, err := PublicIP()
		assert.NoError(t, err)
		assert.Equal(t, "203.0.113.1", ip)
	})

	t.Run("Success on second URL after first fails", func(t *testing.T) {
		// Create two servers - first fails, second succeeds
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Error"))
		}))
		defer failServer.Close()

		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("203.0.113.2"))
		}))
		defer successServer.Close()

		// Override URLs to use our test servers
		publicIPDiscoverURLs = []string{failServer.URL, successServer.URL}

		ip, err := PublicIP()
		assert.NoError(t, err)
		assert.Equal(t, "203.0.113.2", ip)
	})

	t.Run("All URLs fail", func(t *testing.T) {
		// Create servers that all fail
		failServer1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer failServer1.Close()

		failServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer failServer2.Close()

		// Override URLs to use failing servers
		publicIPDiscoverURLs = []string{failServer1.URL, failServer2.URL}

		_, err := PublicIP()
		assert.Error(t, err, "Expected error when all URLs fail")
	})
}

// TestDiscoverPublicIPInvalidURL tests handling of invalid URLs
func TestDiscoverPublicIPInvalidURL(t *testing.T) {
	testCases := []struct {
		name string
		url  string
	}{
		{
			name: "Invalid URL",
			url:  "not-a-valid-url",
		},
		{
			name: "Unsupported protocol",
			url:  "ftp://example.com",
		},
		{
			name: "Non-existent domain",
			url:  "http://this-domain-should-not-exist.invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := discoverPublicIP(tc.url)
			assert.Error(t, err, "Expected error for invalid URL: %s", tc.url)
		})
	}
}

// TestDiscoverPublicIPContextCancellation tests proper handling of context cancellation
func TestDiscoverPublicIPContextCancellation(t *testing.T) {
	// Create a server with a long delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request context is properly set up
		select {
		case <-r.Context().Done():
			// Context was canceled
			return
		case <-time.After(15 * time.Second):
			// This should not happen in our test
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("192.168.1.1"))
		}
	}))
	defer server.Close()

	// This should timeout due to the 10-second client timeout
	start := time.Now()
	_, err := discoverPublicIP(server.URL)
	duration := time.Since(start)

	assert.Error(t, err, "Expected timeout error")
	// Should timeout around 10 seconds, give some leeway for test execution
	assert.True(t, duration >= 9*time.Second && duration <= 12*time.Second,
		"Expected timeout around 10 seconds, got %v", duration)
}

// TestDiscoverPublicIPHTTPMethodAndHeaders tests that the function uses correct HTTP method and headers
func TestDiscoverPublicIPHTTPMethodAndHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify HTTP method
		assert.Equal(t, "GET", r.Method, "Should use GET method")

		// Verify User-Agent header
		userAgent := r.Header.Get("User-Agent")
		assert.Equal(t, "curl/7.64.1", userAgent, "User-Agent should be set correctly")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("203.0.113.1"))
	}))
	defer server.Close()

	ip, err := discoverPublicIP(server.URL)
	assert.NoError(t, err)
	assert.Equal(t, "203.0.113.1", ip)
}

// TestDiscoverPublicIPIPv4Enforcement tests that the function enforces IPv4 connections
func TestDiscoverPublicIPIPv4Enforcement(t *testing.T) {
	// This test verifies that the custom dialer is set up correctly
	// We can't easily test the actual IPv4 enforcement without complex network setup,
	// but we can verify the function works with valid responses

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("203.0.113.1"))
	}))
	defer server.Close()

	ip, err := discoverPublicIP(server.URL)
	assert.NoError(t, err)
	assert.Equal(t, "203.0.113.1", ip)

	// Verify the returned IP is valid
	parsed := net.ParseIP(ip)
	assert.NotNil(t, parsed, "Returned IP should be valid")
}

// TestPublicIPDiscoverURLsNotEmpty tests that the discover URLs are not empty
func TestPublicIPDiscoverURLsNotEmpty(t *testing.T) {
	assert.NotEmpty(t, publicIPDiscoverURLs, "publicIPDiscoverURLs should not be empty")

	for i, url := range publicIPDiscoverURLs {
		assert.NotEmpty(t, url, "URL at index %d should not be empty", i)
		assert.Contains(t, url, "http", "URL at index %d should be a valid HTTP URL", i)
	}
}
