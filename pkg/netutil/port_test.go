package netutil

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsPortOpen tests the IsPortOpen function
func TestIsPortOpen(t *testing.T) {
	// Start a test server on a random available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	// Get the port that was assigned
	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to extract port: %v", err)
	}

	// Convert port string to int
	var openPort int
	_, _ = fmt.Sscanf(portStr, "%d", &openPort)

	// Test with definitely open port (the one we just opened)
	t.Run("Open port", func(t *testing.T) {
		assert.True(t, IsPortOpen(openPort), "Port %d should be detected as open", openPort)
	})

	// Close the listener to free up the port
	listener.Close()

	// Find a port that's very likely to be closed
	// Using a high port number that's unlikely to be in use
	closedPort := 54321
	for i := 0; i < 10; i++ {
		if !IsPortOpen(closedPort + i) {
			closedPort = closedPort + i
			break
		}
	}

	// Test with closed port
	t.Run("Closed port", func(t *testing.T) {
		assert.False(t, IsPortOpen(closedPort), "Port %d should be detected as closed", closedPort)
	})

	// Test with edge cases
	t.Run("Invalid port", func(t *testing.T) {
		assert.False(t, IsPortOpen(65536), "Port 65536 is outside valid range and should return false")
	})
}
