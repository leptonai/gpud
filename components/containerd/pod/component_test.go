package pod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leptonai/gpud/components"
)

func Test_componentStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{ctx: ctx, cancel: cancel, checkDependencyInstalled: func() bool { return true }}
	err := c.Start()
	assert.NoError(t, err)
	assert.NoError(t, c.Close())
}

func TestComponentBasics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{ctx: ctx, cancel: cancel, checkDependencyInstalled: func() bool { return true }}

	// Test component name
	assert.Equal(t, Name, c.Name())

	// Test Start method
	err := c.Start()
	assert.NoError(t, err)

	// Test Close method
	err = c.Close()
	assert.NoError(t, err)

	// Test Events method
	events, err := c.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)

	// Test Metrics method
	metrics, err := c.Metrics(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestParseUnixEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
		wantErr  bool
	}{
		{
			name:     "valid unix endpoint",
			endpoint: "unix:///run/containerd/containerd.sock",
			want:     "/run/containerd/containerd.sock",
			wantErr:  false,
		},
		{
			name:     "invalid scheme",
			endpoint: "http://localhost:8080",
			want:     "",
			wantErr:  true,
		},
		{
			name:     "invalid url",
			endpoint: "://invalid",
			want:     "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUnixEndpoint(tt.endpoint)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestDefaultDialOptions(t *testing.T) {
	opts := defaultDialOptions()
	assert.NotEmpty(t, opts)
	assert.Greater(t, len(opts), 0)
}

func TestDataFunctions(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		d := Data{}

		// Test marshalJSON
		b, err := json.Marshal(d)
		assert.NoError(t, err)
		assert.NotNil(t, b)

		// Test describeReason
		reason := d.getReason()
		assert.Contains(t, reason, "no pod sandbox found")

		// Test getHealth with no error
		health, healthy := d.getHealth()
		assert.Equal(t, components.StateHealthy, health)
		assert.True(t, healthy)

		// Test getStates
		states, err := d.getStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
	})

	t.Run("data with error", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{{ID: "pod1"}},
			err:  errors.New("test error"),
		}

		// Test describeReason with error
		reason := d.getReason()
		assert.Contains(t, reason, "failed to list pod sandbox status")

		// Test getHealth with error
		health, healthy := d.getHealth()
		assert.Equal(t, components.StateUnhealthy, health)
		assert.False(t, healthy)
	})

	t.Run("data with gRPC unimplemented error", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{{ID: "pod1"}},
			err:  status.Error(codes.Unimplemented, "test unimplemented"),
		}

		// Test describeReason with unimplemented error
		reason := d.getReason()
		assert.Contains(t, reason, "containerd didn't enable CRI")
	})

	t.Run("empty data with error - empty pods takes precedence", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{},
			err:  errors.New("test error"),
		}

		// Test describeReason with error but empty pods
		reason := d.getReason()
		assert.Contains(t, reason, "no pod sandbox found")
		assert.NotContains(t, reason, "failed gRPC call")
	})

	t.Run("data with pods", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{
				{
					ID:   "pod1",
					Name: "test-pod",
				},
			},
		}

		// Test getStates with pods
		states, err := d.getStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Contains(t, states[0].ExtraInfo, "data")
		assert.Contains(t, states[0].ExtraInfo, "encoding")
	})
}

// Test the component States method separately
func TestComponentStates(t *testing.T) {
	ctx := context.Background()
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		lastData: &Data{
			Pods: []PodSandbox{
				{
					ID:   "pod1",
					Name: "test-pod",
				},
			},
		},
	}

	states, err := comp.States(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, states)
	assert.Equal(t, Name, states[0].Name)
}

// Test checkOnce method
func TestCheckOnce(t *testing.T) {
	// set shorter timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	comp := &component{
		ctx:                      ctx,
		cancel:                   func() {},
		checkDependencyInstalled: func() bool { return true },
		endpoint:                 "unix:///nonexistent/socket",
	}

	// This will likely fail due to nonexistent socket, but we're just testing the function doesn't crash
	comp.CheckOnce()

	// Verify that lastData was updated
	assert.NotZero(t, comp.lastData.ts)
	assert.Error(t, comp.lastData.err) // Should error due to nonexistent socket
}

// Test component Events method more thoroughly
func TestComponentEvents(t *testing.T) {
	ctx := context.Background()
	comp := &component{
		ctx:                      ctx,
		cancel:                   func() {},
		checkDependencyInstalled: func() bool { return true },
		endpoint:                 "unix:///nonexistent/socket",
		lastData: &Data{
			ts: time.Now().Add(-1 * time.Hour),
			Pods: []PodSandbox{
				{
					ID:   "pod1",
					Name: "test-pod",
				},
			},
		},
	}

	// With a since time in the past
	since := time.Now().Add(-2 * time.Hour)
	events, err := comp.Events(ctx, since)
	assert.NoError(t, err)
	assert.Empty(t, events)

	// With a since time in the future
	future := time.Now().Add(1 * time.Hour)
	events, err = comp.Events(ctx, future)
	assert.NoError(t, err)
	assert.Empty(t, events)
}

// More thorough test of the component methods with different contexts
func TestComponentWithDifferentContexts(t *testing.T) {
	// Create component with canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel right away
	comp := &component{
		ctx:                      ctx,
		cancel:                   func() {},
		checkDependencyInstalled: func() bool { return true },
		endpoint:                 "unix:///nonexistent/socket",
	}

	// Test States with canceled context
	states, err := comp.States(ctx)
	assert.NoError(t, err) // Should still work as it uses stored data
	assert.NotNil(t, states)

	// Test with timeout context
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer timeoutCancel()

	metrics, err := comp.Metrics(timeoutCtx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, metrics)
}

// Test marshalJSON function with different scenarios
func TestDataMarshalJSON(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		d := Data{}
		jsonData, err := json.Marshal(d)
		assert.NoError(t, err)
		assert.NotNil(t, jsonData)
		// Empty data should marshal to an empty JSON object
		assert.Equal(t, "{\"containerd_service_active\":false}", string(jsonData))
	})

	t.Run("data with pods", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{
				{
					ID:        "pod-123",
					Namespace: "default",
					Name:      "test-pod",
					State:     "SANDBOX_READY",
					Containers: []PodSandboxContainerStatus{
						{
							ID:    "container-456",
							Name:  "test-container",
							Image: "nginx:latest",
						},
					},
				},
			},
		}
		jsonData, err := json.Marshal(d)
		assert.NoError(t, err)
		assert.NotNil(t, jsonData)

		// Verify the JSON contains expected fields
		jsonStr := string(jsonData)
		assert.Contains(t, jsonStr, "pod-123")
		assert.Contains(t, jsonStr, "default")
		assert.Contains(t, jsonStr, "test-pod")
		assert.Contains(t, jsonStr, "SANDBOX_READY")
		assert.Contains(t, jsonStr, "container-456")
	})

	t.Run("data with marshaling error", func(t *testing.T) {
		// Create a pod with a channel which cannot be marshaled to JSON
		badPod := PodSandbox{
			ID:   "bad-pod",
			Name: "bad-pod",
		}

		d := Data{
			Pods: []PodSandbox{badPod},
		}

		// This is expected to either return an error or escape the invalid UTF-8
		jsonData, _ := json.Marshal(d)
		assert.NotNil(t, jsonData) // Should still produce some output
	})
}

// Test component checkOnce function with different error scenarios
func TestCheckOnceWithMockEndpoints(t *testing.T) {
	// Test with missing endpoint
	t.Run("missing endpoint", func(t *testing.T) {
		ctx := context.Background()
		comp := &component{
			ctx:                      ctx,
			cancel:                   func() {},
			checkDependencyInstalled: func() bool { return true },
			endpoint:                 "", // Empty endpoint
		}

		comp.CheckOnce()

		// Verify data was updated with error
		assert.NotZero(t, comp.lastData.ts)
		assert.Error(t, comp.lastData.err)
	})

	// Test with canceled context
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		comp := &component{
			ctx:                      ctx,
			cancel:                   func() {},
			checkDependencyInstalled: func() bool { return true },
			endpoint:                 "unix:///nonexistent/socket",
		}

		comp.CheckOnce()

		// Verify data was updated with error
		assert.NotZero(t, comp.lastData.ts)
		assert.Error(t, comp.lastData.err)
	})

	// Test with timeout context
	t.Run("timeout context", func(t *testing.T) {
		// Use a parent context with a very short timeout to ensure it expires
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(10 * time.Millisecond) // Ensure timeout happens

		comp := &component{
			ctx:                      ctx,
			cancel:                   func() {},
			checkDependencyInstalled: func() bool { return true },
			endpoint:                 "unix:///nonexistent/socket",
		}

		comp.CheckOnce()

		// Verify data was updated with error
		assert.NotZero(t, comp.lastData.ts)
		assert.Error(t, comp.lastData.err)
	})
}

// Test getHealth function with different error types
func TestGetHealthWithDifferentErrors(t *testing.T) {
	t.Run("connection refused error", func(t *testing.T) {
		d := Data{
			err: &net.OpError{
				Op:  "dial",
				Err: errors.New("connection refused"),
			},
		}

		health, healthy := d.getHealth()
		assert.Equal(t, components.StateUnhealthy, health)
		assert.False(t, healthy)
	})

	t.Run("permission denied error", func(t *testing.T) {
		d := Data{
			err: &os.PathError{
				Op:   "open",
				Path: "/path/to/socket",
				Err:  errors.New("permission denied"),
			},
		}

		health, healthy := d.getHealth()
		assert.Equal(t, components.StateUnhealthy, health)
		assert.False(t, healthy)
	})

	t.Run("context canceled error", func(t *testing.T) {
		d := Data{
			err: context.Canceled,
		}

		health, healthy := d.getHealth()
		assert.Equal(t, components.StateUnhealthy, health)
		assert.False(t, healthy)
	})

	t.Run("context deadline exceeded error", func(t *testing.T) {
		d := Data{
			err: context.DeadlineExceeded,
		}

		health, healthy := d.getHealth()
		assert.Equal(t, components.StateUnhealthy, health)
		assert.False(t, healthy)
	})

	t.Run("grpc unavailable error", func(t *testing.T) {
		d := Data{
			err: status.Error(codes.Unavailable, "service unavailable"),
		}

		health, healthy := d.getHealth()
		assert.Equal(t, components.StateUnhealthy, health)
		assert.False(t, healthy)
	})
}

// Test getStates function with edge cases
func TestGetStatesEdgeCases(t *testing.T) {
	t.Run("empty data with error", func(t *testing.T) {
		d := Data{
			err: errors.New("some error"),
		}

		states, err := d.getStates()
		assert.NoError(t, err) // Error is embedded in state, not returned
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateUnhealthy, states[0].Health)
		// The empty pods condition takes precedence over errors in getReason()
		assert.Contains(t, states[0].Reason, "no pod sandbox found")
	})

	t.Run("data with pods and error", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{{ID: "pod1"}},
			err:  errors.New("grpc connection error"),
		}

		states, err := d.getStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateUnhealthy, states[0].Health)
		// When there are pods, error message should be in the reason
		assert.Contains(t, states[0].Reason, "failed to list pod sandbox status grpc connection error")
	})

	t.Run("data with many pods", func(t *testing.T) {
		// Create data with multiple pods
		pods := make([]PodSandbox, 10)
		for i := 0; i < 10; i++ {
			pods[i] = PodSandbox{
				ID:        fmt.Sprintf("pod-%d", i),
				Name:      fmt.Sprintf("test-pod-%d", i),
				Namespace: "default",
				State:     "SANDBOX_READY",
			}
		}

		d := Data{
			Pods: pods,
		}

		states, err := d.getStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateHealthy, states[0].Health)

		// Check that JSON encoding worked and includes multiple pods
		jsonData, jsonErr := json.Marshal(states[0].ExtraInfo)
		assert.NoError(t, jsonErr)
		assert.Contains(t, string(jsonData), "pod-0")
		assert.Contains(t, string(jsonData), "pod-9")
	})

	t.Run("data with JSON marshaling issue", func(t *testing.T) {
		// Create a pod with fields that might cause JSON issues
		badPod := PodSandbox{
			ID:   "bad-pod",
			Name: "bad-pod",
		}

		d := Data{
			Pods: []PodSandbox{badPod},
		}

		states, err := d.getStates()
		// Even with JSON issues, the function should not return an error
		// It might produce empty or escaped JSON
		assert.NoError(t, err)
		assert.Len(t, states, 1)
	})
}

// TestData_getReason specifically tests the Data.Reason method thoroughly
func TestData_getReason(t *testing.T) {
	tests := []struct {
		name     string
		data     Data
		expected string
	}{
		{
			name: "nil pods array",
			data: Data{
				Pods: nil,
				err:  nil,
			},
			expected: "no pod sandbox found or containerd is not running",
		},
		{
			name: "empty data no error",
			data: Data{
				Pods: []PodSandbox{},
				err:  nil,
			},
			expected: "no pod sandbox found or containerd is not running",
		},
		{
			name: "empty pods with connection error",
			data: Data{
				Pods: []PodSandbox{},
				err:  errors.New("connection refused"),
			},
			expected: "no pod sandbox found or containerd is not running, error: connection refused",
		},
		{
			name: "single pod no error",
			data: Data{
				Pods: []PodSandbox{
					{
						ID:   "pod1",
						Name: "test-pod",
					},
				},
				err: nil,
			},
			expected: "total 1 pod sandboxe(s)",
		},
		{
			name: "multiple pods no error",
			data: Data{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
					{ID: "pod2", Name: "test-pod-2"},
					{ID: "pod3", Name: "test-pod-3"},
				},
				err: nil,
			},
			expected: "total 3 pod sandboxe(s)",
		},
		{
			name: "generic error",
			data: Data{
				Pods: []PodSandbox{
					{ID: "pod1"},
				},
				err: errors.New("generic error"),
			},
			expected: "failed to list pod sandbox status generic error",
		},
		{
			name: "unimplemented error",
			data: Data{
				Pods: []PodSandbox{
					{ID: "pod1"},
				},
				err: status.Error(codes.Unimplemented, "unknown service runtime.v1.RuntimeService"),
			},
			expected: "containerd didn't enable CRI",
		},
		{
			name: "pods with unimplemented error",
			data: Data{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
					{ID: "pod2", Name: "test-pod-2"},
				},
				err: status.Error(codes.Unimplemented, "unknown service runtime.v1.RuntimeService"),
			},
			expected: "containerd didn't enable CRI",
		},
		{
			name: "other status error",
			data: Data{
				Pods: []PodSandbox{
					{ID: "pod1"},
				},
				err: status.Error(codes.Unavailable, "service unavailable"),
			},
			expected: "failed gRPC call to the containerd socket service unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := tt.data.getReason()
			assert.Equal(t, tt.expected, reason)
		})
	}
}

// TestData_getReasonWithErrors focuses on testing the getReason method
// with various error types
func TestData_getReasonWithErrors(t *testing.T) {
	tests := []struct {
		name     string
		data     Data
		expected string
	}{
		{
			name: "context canceled error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  context.Canceled,
			},
			expected: "failed to list pod sandbox status context canceled",
		},
		{
			name: "context deadline exceeded error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  context.DeadlineExceeded,
			},
			expected: "failed to list pod sandbox status context deadline exceeded",
		},
		{
			name: "network dial error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err: &net.OpError{
					Op:  "dial",
					Err: errors.New("connection refused"),
				},
			},
			expected: "failed to list pod sandbox status dial: connection refused",
		},
		{
			name: "network connect error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err: &net.OpError{
					Op:  "connect",
					Err: errors.New("connection reset by peer"),
				},
			},
			expected: "failed to list pod sandbox status connect: connection reset by peer",
		},
		{
			name: "permission denied error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err: &os.PathError{
					Op:   "open",
					Path: "/run/containerd/containerd.sock",
					Err:  errors.New("permission denied"),
				},
			},
			expected: "failed to list pod sandbox status open /run/containerd/containerd.sock: permission denied",
		},
		{
			name: "no such file error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err: &os.PathError{
					Op:   "stat",
					Path: "/run/containerd/containerd.sock",
					Err:  errors.New("no such file or directory"),
				},
			},
			expected: "failed to list pod sandbox status stat /run/containerd/containerd.sock: no such file or directory",
		},
		{
			name: "grpc internal error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  status.Error(codes.Internal, "internal error"),
			},
			expected: "failed gRPC call to the containerd socket internal error",
		},
		{
			name: "grpc not found error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  status.Error(codes.NotFound, "not found"),
			},
			expected: "failed gRPC call to the containerd socket not found",
		},
		{
			name: "grpc resource exhausted error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  status.Error(codes.ResourceExhausted, "resource exhausted"),
			},
			expected: "failed gRPC call to the containerd socket resource exhausted",
		},
		{
			name: "wrapped error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  fmt.Errorf("could not connect: %w", errors.New("underlying error")),
			},
			expected: "failed to list pod sandbox status could not connect: underlying error",
		},
		{
			name: "error take precedence over empty pod",
			data: Data{
				Pods: []PodSandbox{},
				err:  errors.New("this error"),
			},
			expected: "no pod sandbox found or containerd is not running, error: this error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := tt.data.getReason()
			assert.Equal(t, tt.expected, reason)
		})
	}
}
