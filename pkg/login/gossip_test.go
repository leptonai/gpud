package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/version"
)

// Test version of Gossip function that uses HTTP instead of HTTPS for testing
func testGossip(endpoint string, uid string, address string, components []string) error {
	content := gossipPayload{
		Name:          uid,
		ID:            uid,
		Provider:      "personal",
		DaemonVersion: version.Version,
		Components:    strings.Join(components, ","),
	}
	rawPayload, _ := json.Marshal(&content)
	// Use HTTP instead of HTTPS for the test
	response, err := http.Post(fmt.Sprintf("http://%s/api/v1/gossip", endpoint), "application/json", bytes.NewBuffer(rawPayload))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		var errorResponse gossipRespErr
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("Error parsing error response: %v\nResponse body: %s", err, body)
		}
	}
	return nil
}

func TestGossip_Success(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/api/v1/gossip", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse the request body
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)

		// Verify request payload
		assert.Equal(t, "test-uid", reqBody["name"])
		assert.Equal(t, "test-uid", reqBody["id"])
		assert.Equal(t, "personal", reqBody["provider"])
		assert.Equal(t, version.Version, reqBody["daemon_version"])
		assert.Equal(t, "gpu,cpu", reqBody["components"])

		// Return success response
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	// Get the server URL
	serverURL := server.URL[7:] // Remove "http://" prefix

	// Call our test Gossip function that uses HTTP instead of HTTPS
	err := testGossip(serverURL, "test-uid", "test-address", []string{"gpu", "cpu"})
	assert.NoError(t, err)
}

func TestGossip_NoUsageStats(t *testing.T) {
	// Set environment variable to disable usage stats
	os.Setenv("GPUD_NO_USAGE_STATS", "true")
	defer os.Unsetenv("GPUD_NO_USAGE_STATS")

	// Call the Gossip function - should skip and return nil
	err := Gossip("example.com", "test-uid", "test-address", []string{"gpu", "cpu"})
	assert.NoError(t, err)
}

func TestGossip_ServerError(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a server error
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status": "server error", "error": "internal error"}`))
	}))
	defer server.Close()

	// Get the server URL
	serverURL := server.URL[7:] // Remove "http://" prefix

	// Call our test Gossip function that uses HTTP instead of HTTPS
	err := testGossip(serverURL, "test-uid", "test-address", []string{"gpu", "cpu"})
	assert.NoError(t, err) // Gossip doesn't return an error for non-OK status
}
