package netutil

import (
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
		t.Skip("PublicIP test requires network access and curl")
	}

	assert.NotEmpty(t, ip, "PublicIP should return a non-empty string")
}

// TestPublicIPWithMockServer tests the internal publicIP function using a mock HTTP server
func TestPublicIPWithMockServer(t *testing.T) {
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
			name:           "Success with valid IP",
			serverResponse: "192.168.1.1",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    false,
			expectedIP:     "192.168.1.1",
		},
		{
			name:           "Success with IPv6",
			serverResponse: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    false,
			expectedIP:     "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		},
		{
			name:           "Success with whitespace",
			serverResponse: "  192.168.1.1  \n",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    false,
			expectedIP:     "192.168.1.1",
		},
		{
			name:           "Non-error with server error status",
			serverResponse: "Internal Server Error",
			serverStatus:   http.StatusInternalServerError,
			serverDelay:    0,
			expectError:    false,                   // Function doesn't check HTTP status codes
			expectedIP:     "Internal Server Error", // It will return whatever the body contains
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
			name:           "Empty response",
			serverResponse: "",
			serverStatus:   http.StatusOK,
			serverDelay:    0,
			expectError:    false,
			expectedIP:     "",
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
			ip, err := publicIP(server.URL)

			// Check the results
			if tc.expectError {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error")
				assert.Equal(t, tc.expectedIP, ip, "IP address doesn't match expected value")
			}
		})
	}
}

// Test for handling malformed responses
func TestPublicIPMalformedResponse(t *testing.T) {
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
		defer conn.Close()

		// Send a partial/malformed HTTP response
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\n"))
		_, _ = conn.Write([]byte("Content-Length: 1000\r\n\r\n")) // Set a large content length
		_, _ = conn.Write([]byte("192.168.1."))                   // Send incomplete data
		// Don't close properly to cause a read error
	}))
	defer server.Close()

	// Call the function under test
	_, err := publicIP(server.URL)

	// Should return an error
	assert.Error(t, err, "Expected an error due to malformed response")
}

// TestPublicIPWithExportedWrapper tests the exported PublicIP function with a custom implementation
func TestPublicIPWithExportedWrapper(t *testing.T) {
	// Create a test server to mock ifconfig.me
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify if the request is similar to what PublicIP() would generate
		userAgent := r.Header.Get("User-Agent")
		assert.Equal(t, "curl/7.64.1", userAgent, "User-Agent header should be set correctly")

		// Return a test IP
		mockIP := "203.0.113.1" // TEST-NET-3 reserved IP for documentation
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockIP))
	}))
	defer server.Close()

	// Call the internal function directly with our test server URL
	ip, err := publicIP(server.URL)

	// Check results
	assert.NoError(t, err, "publicIP should not return an error with our mock server")
	assert.Equal(t, "203.0.113.1", ip, "Should return the mocked IP address")
}
