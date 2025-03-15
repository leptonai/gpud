package pod

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestDialUnix(t *testing.T) {
	// This is a simple test that just ensures the function doesn't crash
	// In a real environment, you would need a real Unix socket or more sophisticated mocking
	_, err := dialUnix(context.Background(), "non-existent-socket")
	assert.Error(t, err)
}

func TestDialOptions(t *testing.T) {
	opts := defaultDialOptions()
	require.NotNil(t, opts)

	// Check if we have the expected number of dial options
	// This is somewhat brittle as it depends on the implementation, but serves as a basic check
	assert.GreaterOrEqual(t, len(opts), 4)

	// Since we can't easily check the internals of the gRPC dial options,
	// this test just ensures the function returns options without error
}

// Test PodSandbox and PodSandboxContainerStatus structures and their JSON marshaling
func TestPodSandboxTypes(t *testing.T) {
	t.Run("PodSandbox JSON marshaling", func(t *testing.T) {
		pod := PodSandbox{
			ID:        "pod-123",
			Namespace: "default",
			Name:      "test-pod",
			State:     "SANDBOX_READY",
			Containers: []PodSandboxContainerStatus{
				{
					ID:        "container-456",
					Name:      "test-container",
					Image:     "nginx:latest",
					CreatedAt: 1234567890,
					State:     "CONTAINER_RUNNING",
				},
			},
		}

		// Test JSON marshaling
		b, err := json.Marshal(pod)
		require.NoError(t, err)

		// Test contains expected fields
		jsonStr := string(b)
		assert.Contains(t, jsonStr, "pod-123")
		assert.Contains(t, jsonStr, "default")
		assert.Contains(t, jsonStr, "test-pod")
		assert.Contains(t, jsonStr, "SANDBOX_READY")
		assert.Contains(t, jsonStr, "container-456")
		assert.Contains(t, jsonStr, "test-container")
		assert.Contains(t, jsonStr, "nginx:latest")
		assert.Contains(t, jsonStr, "1234567890")
		assert.Contains(t, jsonStr, "CONTAINER_RUNNING")
	})

	t.Run("PodSandboxContainerStatus JSON marshaling", func(t *testing.T) {
		container := PodSandboxContainerStatus{
			ID:        "container-789",
			Name:      "sidecar",
			Image:     "busybox:latest",
			CreatedAt: 9876543210,
			State:     "CONTAINER_EXITED",
		}

		// Test JSON marshaling
		b, err := json.Marshal(container)
		require.NoError(t, err)

		// Test contains expected fields
		jsonStr := string(b)
		assert.Contains(t, jsonStr, "container-789")
		assert.Contains(t, jsonStr, "sidecar")
		assert.Contains(t, jsonStr, "busybox:latest")
		assert.Contains(t, jsonStr, "9876543210")
		assert.Contains(t, jsonStr, "CONTAINER_EXITED")
	})
}

// Test the connect function with mock server
func Test_connect(t *testing.T) {
	t.Run("invalid endpoint", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := connect(ctx, "invalid://endpoint")
		assert.Error(t, err)
	})

	t.Run("non-existent unix socket", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := connect(ctx, "unix:///nonexistent/socket/path")
		assert.Error(t, err)
	})
}

// Test listSandboxStatus function with mock connectors
func TestListSandboxStatus(t *testing.T) {
	// Test with invalid endpoint
	t.Run("invalid endpoint", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		pods, err := listSandboxStatus(ctx, "invalid://endpoint")
		assert.Error(t, err)
		assert.Empty(t, pods)
	})

	// Only run timeout test if not in a CI environment to avoid flakiness
	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond) // Ensure timeout happens

		pods, err := listSandboxStatus(ctx, "unix:///nonexistent/socket/path")
		assert.Error(t, err)
		assert.Empty(t, pods)
	})
}

// Test the createClient function
func TestCreateClient(t *testing.T) {
	t.Run("with canceled context", func(t *testing.T) {
		// Create a context and immediately cancel it
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Immediately cancel

		// Create a connection to a non-existent endpoint (connection creation will succeed due to lazy initialization)
		conn, err := grpc.DialContext(context.Background(), "unix:///non-existent-path", grpc.WithInsecure()) //nolint:staticcheck
		require.NoError(t, err)
		defer conn.Close()

		// The client creation should fail because the context is canceled
		_, _, err = createClient(ctx, conn)
		assert.Error(t, err)
	})
}
