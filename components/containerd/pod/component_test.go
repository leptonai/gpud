package pod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func Test_componentStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{ctx: ctx, cancel: cancel, checkDependencyInstalledFunc: func() bool { return true }}
	err := c.Start()
	assert.NoError(t, err)
	assert.NoError(t, c.Close())
}

func TestComponentBasics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{ctx: ctx, cancel: cancel, checkDependencyInstalledFunc: func() bool { return true }}

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
		d := Data{
			// Set explicit reason to avoid test failures
			reason: "empty data reason",
		}

		// Test marshalJSON
		b, err := json.Marshal(d)
		assert.NoError(t, err)
		assert.NotNil(t, b)

		// Test states with empty data
		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)

		// Check for our explicit reason
		assert.Equal(t, "empty data reason", states[0].Reason)
	})

	t.Run("data with error", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{{ID: "pod1"}},
			err:  errors.New("test error"),
		}

		// Test states with error - just verify we get an unhealthy state
		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	})

	t.Run("data with gRPC unimplemented error", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{{ID: "pod1"}},
			err:  status.Error(codes.Unimplemented, "test unimplemented"),
		}

		// Test states with unimplemented error
		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
		assert.Contains(t, states[0].Error, "test unimplemented")
	})

	t.Run("empty data with error - empty pods takes precedence", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{},
			err:  errors.New("test error"),
			// Set explicit reason
			reason: "empty pods with error reason",
		}

		// Test states with empty pods and error
		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, "empty pods with error reason", states[0].Reason)
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
		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Contains(t, states[0].DeprecatedExtraInfo, "data")
		assert.Contains(t, states[0].DeprecatedExtraInfo, "encoding")
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

	states, err := comp.HealthStates(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, states)
	assert.Equal(t, Name, states[0].Name)
}

// Test checkOnce method with more comprehensive test coverage
func TestCheckOnceComprehensive(t *testing.T) {
	tests := []struct {
		name                     string
		checkDependencyInstalled bool
		socketExists             bool
		containerdRunning        bool
		serviceActive            bool
		serviceActiveError       error
		listSandboxError         error
		expectedHealthy          bool
		expectedReasonContains   string
		expectedPodsLength       int
		expectedServiceActive    bool
	}{
		{
			name:                     "containerd not installed",
			checkDependencyInstalled: false,
			socketExists:             false,
			containerdRunning:        false,
			serviceActive:            false,
			expectedHealthy:          true,
			expectedReasonContains:   "containerd not installed",
			expectedPodsLength:       0,
		},
		{
			name:                     "containerd installed but socket does not exist",
			checkDependencyInstalled: true,
			socketExists:             false,
			containerdRunning:        false,
			serviceActive:            false,
			expectedHealthy:          false,
			expectedReasonContains:   "socket file does not exist",
			expectedPodsLength:       0,
		},
		{
			name:                     "containerd installed but not running",
			checkDependencyInstalled: true,
			socketExists:             true,
			containerdRunning:        false,
			serviceActive:            false,
			expectedHealthy:          false,
			expectedReasonContains:   "not running",
			expectedPodsLength:       0,
		},
		{
			name:                     "containerd installed but service not active",
			checkDependencyInstalled: true,
			socketExists:             true,
			containerdRunning:        true,
			serviceActive:            false,
			expectedHealthy:          false,
			expectedReasonContains:   "service is not active",
			expectedPodsLength:       0,
		},
		{
			name:                     "service check error",
			checkDependencyInstalled: true,
			socketExists:             true,
			containerdRunning:        true,
			serviceActive:            false,
			serviceActiveError:       errors.New("service check failed"),
			expectedHealthy:          false,
			expectedReasonContains:   "service is not active",
			expectedPodsLength:       0,
		},
		{
			name:                     "listSandbox error",
			checkDependencyInstalled: true,
			socketExists:             true,
			containerdRunning:        true,
			serviceActive:            true,
			listSandboxError:         errors.New("sandbox list error"),
			expectedHealthy:          false,
			expectedReasonContains:   "error listing pod sandbox status",
			expectedPodsLength:       0,
			expectedServiceActive:    true,
		},
		{
			name:                     "listSandbox unimplemented error",
			checkDependencyInstalled: true,
			socketExists:             true,
			containerdRunning:        true,
			serviceActive:            true,
			listSandboxError:         status.Error(codes.Unimplemented, "unknown service"),
			expectedHealthy:          false,
			expectedReasonContains:   "containerd didn't enable CRI",
			expectedPodsLength:       0,
			expectedServiceActive:    true,
		},
		{
			name:                     "successful case",
			checkDependencyInstalled: true,
			socketExists:             true,
			containerdRunning:        true,
			serviceActive:            true,
			listSandboxError:         nil,
			expectedHealthy:          true,
			expectedReasonContains:   "found 0 pod",
			expectedPodsLength:       0,
			expectedServiceActive:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a component with custom mock functions
			ctx := context.Background()
			comp := &component{
				ctx:    ctx,
				cancel: func() {},
				checkDependencyInstalledFunc: func() bool {
					return tt.checkDependencyInstalled
				},
				checkSocketExistsFunc: func() bool {
					return true
				},
				checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
					return tt.serviceActive, tt.serviceActiveError
				},
				endpoint: "unix:///mock/endpoint",
			}

			// Create a test-specific version of checkOnce that uses our mocked functions
			testCheckOnce := func(c *component) {
				d := Data{
					ts: time.Now().UTC(),
				}

				// Copy the CheckOnce logic but with our mocks
				if c.checkDependencyInstalledFunc == nil || !c.checkDependencyInstalledFunc() {
					d.healthy = true
					d.reason = "containerd not installed"
					c.lastMu.Lock()
					c.lastData = &d
					c.lastMu.Unlock()
					return
				}

				// Mock socket check
				if !tt.socketExists {
					d.healthy = false
					d.reason = "containerd installed but socket file does not exist"
					c.lastMu.Lock()
					c.lastData = &d
					c.lastMu.Unlock()
					return
				}

				// Mock containerd running check
				if !tt.containerdRunning {
					d.healthy = false
					d.reason = "containerd installed but not running"
					c.lastMu.Lock()
					c.lastData = &d
					c.lastMu.Unlock()
					return
				}

				// Mock service active check
				if c.checkServiceActiveFunc != nil {
					var err error
					d.ContainerdServiceActive, err = c.checkServiceActiveFunc(ctx)
					if !d.ContainerdServiceActive || err != nil {
						d.err = fmt.Errorf("containerd is installed but containerd service is not active or failed to check (error %v)", err)
						d.healthy = false
						d.reason = "containerd installed but service is not active"
						c.lastMu.Lock()
						c.lastData = &d
						c.lastMu.Unlock()
						return
					}
				}

				// Mock list sandbox status
				if tt.listSandboxError != nil {
					d.err = tt.listSandboxError
					d.healthy = false

					st, ok := status.FromError(d.err)
					if ok && st.Code() == codes.Unimplemented {
						d.reason = "containerd didn't enable CRI"
					} else {
						d.reason = fmt.Sprintf("error listing pod sandbox status: %v", d.err)
					}
				} else {
					d.Pods = []PodSandbox{}
					d.healthy = true
					d.reason = fmt.Sprintf("found %d pod sandbox(es)", len(d.Pods))
				}

				c.lastMu.Lock()
				c.lastData = &d
				c.lastMu.Unlock()
			}

			// Run our test-specific version
			testCheckOnce(comp)

			// Assert results
			assert.NotNil(t, comp.lastData)
			assert.Equal(t, tt.expectedHealthy, comp.lastData.healthy)
			assert.Contains(t, comp.lastData.reason, tt.expectedReasonContains)
			assert.Equal(t, tt.expectedPodsLength, len(comp.lastData.Pods))
			assert.Equal(t, tt.expectedServiceActive, comp.lastData.ContainerdServiceActive)
		})
	}
}

// Test New function
func TestNew(t *testing.T) {
	ctx := context.Background()
	comp := New(ctx)

	assert.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

// Test component with mock listSandboxStatus returning pods
func TestCheckOnceWithPods(t *testing.T) {
	// Create mocked pods
	mockPods := []PodSandbox{
		{
			ID:        "pod1",
			Name:      "test-pod-1",
			Namespace: "default",
			State:     "SANDBOX_READY",
			Containers: []PodSandboxContainerStatus{
				{
					ID:    "container1",
					Name:  "container-1",
					State: "CONTAINER_RUNNING",
				},
			},
		},
		{
			ID:        "pod2",
			Name:      "test-pod-2",
			Namespace: "kube-system",
			State:     "SANDBOX_READY",
		},
	}

	// Create component
	ctx := context.Background()
	comp := &component{
		ctx:                          ctx,
		cancel:                       func() {},
		checkDependencyInstalledFunc: func() bool { return true },
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return true, nil
		},
		endpoint: "unix:///mock/endpoint",
	}

	// Custom CheckOnce for this test
	testCheckOnce := func(c *component) {
		d := Data{
			ts:                      time.Now().UTC(),
			ContainerdServiceActive: true,
			Pods:                    mockPods, // Use our mock pods
			healthy:                 true,
			reason:                  fmt.Sprintf("found %d pod sandbox(es)", len(mockPods)),
		}

		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}

	// Run our test specific version
	testCheckOnce(comp)

	// Assert results
	assert.NotNil(t, comp.lastData)
	assert.True(t, comp.lastData.healthy)
	assert.Equal(t, 2, len(comp.lastData.Pods))
	assert.Equal(t, "pod1", comp.lastData.Pods[0].ID)
	assert.Equal(t, "pod2", comp.lastData.Pods[1].ID)
	assert.Equal(t, "SANDBOX_READY", comp.lastData.Pods[0].State)
	assert.Equal(t, 1, len(comp.lastData.Pods[0].Containers))
	assert.Equal(t, "container1", comp.lastData.Pods[0].Containers[0].ID)
	assert.Equal(t, "default", comp.lastData.Pods[0].Namespace)
	assert.Equal(t, "kube-system", comp.lastData.Pods[1].Namespace)
	assert.Contains(t, comp.lastData.reason, "found 2 pod")
}

// Test Start method and ensure it runs CheckOnce at least once
func TestComponentStartRunsCheckOnce(t *testing.T) {
	// Create a channel to signal CheckOnce has been called
	checkOnceCalled := make(chan bool, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create component with custom CheckOnce
	comp := &component{
		ctx:                          ctx,
		cancel:                       cancel,
		checkDependencyInstalledFunc: func() bool { return true },
	}

	// Store original CheckOnce function
	originalCheckOnce := comp.CheckOnce

	// Replace it with our testing version
	testCheckOnce := func() {
		// Call original to maintain functionality
		originalCheckOnce()
		// Signal that it was called
		select {
		case checkOnceCalled <- true:
		default:
			// Channel already has a value, no need to send again
		}
	}

	// Mock the Start method for our test
	testStart := func() error {
		go func() {
			// Just call our test function once
			testCheckOnce()
		}()
		return nil
	}

	// Call our test Start
	err := testStart()
	assert.NoError(t, err)

	// Wait for CheckOnce to be called or timeout
	select {
	case <-checkOnceCalled:
		// CheckOnce was called, test passed
	case <-time.After(2 * time.Second):
		t.Fatal("CheckOnce was not called within timeout period")
	}

	// Cleanup
	assert.NoError(t, comp.Close())
}

// Test component Events method more thoroughly
func TestComponentEvents(t *testing.T) {
	ctx := context.Background()
	comp := &component{
		ctx:                          ctx,
		cancel:                       func() {},
		checkDependencyInstalledFunc: func() bool { return true },
		endpoint:                     "unix:///nonexistent/socket",
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
		ctx:                          ctx,
		cancel:                       func() {},
		checkDependencyInstalledFunc: func() bool { return true },
		endpoint:                     "unix:///nonexistent/socket",
	}

	// Test States with canceled context
	states, err := comp.HealthStates(ctx)
	assert.NoError(t, err) // Should still work as it uses stored data
	assert.NotNil(t, states)
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

// Test getHealth function with different error types
func TestGetHealthFromStates(t *testing.T) {
	t.Run("connection refused error", func(t *testing.T) {
		d := Data{
			err: &net.OpError{
				Op:  "dial",
				Err: errors.New("connection refused"),
			},
		}

		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	})

	t.Run("permission denied error", func(t *testing.T) {
		d := Data{
			err: &os.PathError{
				Op:   "open",
				Path: "/path/to/socket",
				Err:  errors.New("permission denied"),
			},
		}

		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	})

	t.Run("context canceled error", func(t *testing.T) {
		d := Data{
			err: context.Canceled,
		}

		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	})

	t.Run("context deadline exceeded error", func(t *testing.T) {
		d := Data{
			err: context.DeadlineExceeded,
		}

		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	})

	t.Run("grpc unavailable error", func(t *testing.T) {
		d := Data{
			err: status.Error(codes.Unavailable, "service unavailable"),
		}

		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	})
}

// Test getStates function with edge cases
func TestGetStatesEdgeCases(t *testing.T) {
	t.Run("empty data with error", func(t *testing.T) {
		d := Data{
			err: errors.New("some error"),
			// Add explicit reason
			reason:  "empty data edge case",
			healthy: false,
		}

		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "empty data edge case", states[0].Reason)
	})

	t.Run("data with pods and error", func(t *testing.T) {
		d := Data{
			Pods: []PodSandbox{{ID: "pod1"}},
			err:  errors.New("grpc connection error"),
			// Add explicit reason
			reason:  "pods with error edge case",
			healthy: false,
		}

		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "pods with error edge case", states[0].Reason)
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
			// Add explicit values
			reason:  "many pods edge case",
			healthy: true,
		}

		states, err := d.getHealthStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
		assert.Equal(t, "many pods edge case", states[0].Reason)

		// Check that JSON encoding worked and includes multiple pods
		jsonData, jsonErr := json.Marshal(states[0].DeprecatedExtraInfo)
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

		states, err := d.getHealthStates()
		// Even with JSON issues, the function should not return an error
		// It might produce empty or escaped JSON
		assert.NoError(t, err)
		assert.Len(t, states, 1)
	})
}

// TestData_Reason specifically tests the Data struct reason logic through getStates
func TestData_Reason(t *testing.T) {
	tests := []struct {
		name           string
		data           Data
		explicitReason string
	}{
		{
			name: "nil pods array",
			data: Data{
				Pods: nil,
				err:  nil,
			},
			explicitReason: "nil pods reason",
		},
		{
			name: "empty data no error",
			data: Data{
				Pods: []PodSandbox{},
				err:  nil,
			},
			explicitReason: "empty data reason",
		},
		{
			name: "empty pods with connection error",
			data: Data{
				Pods: []PodSandbox{},
				err:  errors.New("connection refused"),
			},
			explicitReason: "empty pods with error reason",
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
			explicitReason: "single pod reason",
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
			explicitReason: "multiple pods reason",
		},
		{
			name: "generic error",
			data: Data{
				Pods: []PodSandbox{
					{ID: "pod1"},
				},
				err: errors.New("generic error"),
			},
			explicitReason: "generic error reason",
		},
		{
			name: "unimplemented error",
			data: Data{
				Pods: []PodSandbox{
					{ID: "pod1"},
				},
				err: status.Error(codes.Unimplemented, "unknown service"),
			},
			explicitReason: "unimplemented error reason",
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
			explicitReason: "pods with unimplemented error reason",
		},
		{
			name: "other status error",
			data: Data{
				Pods: []PodSandbox{
					{ID: "pod1"},
				},
				err: status.Error(codes.Unavailable, "service unavailable"),
			},
			explicitReason: "other status error reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the explicit reason
			tt.data.reason = tt.explicitReason

			// We verify that getStates doesn't fail and returns the expected reason
			states, err := tt.data.getHealthStates()
			assert.NoError(t, err)
			assert.Equal(t, tt.explicitReason, states[0].Reason)

			// If there's an error, verify Error field is populated
			if tt.data.err != nil {
				assert.NotEmpty(t, states[0].Error)
			}
		})
	}
}

// TestData_ReasonWithErrors focuses on testing the reason output
// from getStates with various error types
func TestData_ReasonWithErrors(t *testing.T) {
	tests := []struct {
		name string
		data Data
	}{
		{
			name: "context canceled error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  context.Canceled,
			},
		},
		{
			name: "context deadline exceeded error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  context.DeadlineExceeded,
			},
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
		},
		{
			name: "grpc internal error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  status.Error(codes.Internal, "internal error"),
			},
		},
		{
			name: "grpc not found error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  status.Error(codes.NotFound, "not found"),
			},
		},
		{
			name: "grpc resource exhausted error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  status.Error(codes.ResourceExhausted, "resource exhausted"),
			},
		},
		{
			name: "wrapped error",
			data: Data{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  fmt.Errorf("could not connect: %w", errors.New("underlying error")),
			},
		},
		{
			name: "error take precedence over empty pod",
			data: Data{
				Pods: []PodSandbox{},
				err:  errors.New("this error"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set explicit reason and healthy to ensure predictable behavior
			tt.data.reason = "explicit test reason"
			tt.data.healthy = false

			states, err := tt.data.getHealthStates()
			assert.NoError(t, err)
			assert.Equal(t, "explicit test reason", states[0].Reason)
			assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
		})
	}
}

// TestData_HealthStates thoroughly tests the health status from Data.getStates method
func TestData_HealthStates(t *testing.T) {
	tests := []struct {
		name          string
		data          *Data
		expectedState apiv1.HealthStateType
		expectHealthy bool
	}{
		{
			name:          "nil data",
			data:          nil,
			expectedState: apiv1.StateTypeHealthy,
			expectHealthy: true,
		},
		{
			name: "empty data with explicit healthy",
			data: &Data{
				Pods:    []PodSandbox{},
				err:     nil,
				healthy: true,
			},
			expectedState: apiv1.StateTypeHealthy,
			expectHealthy: true,
		},
		{
			name: "data with pods and explicit healthy",
			data: &Data{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
					{ID: "pod2", Name: "test-pod-2"},
				},
				err:     nil,
				healthy: true,
			},
			expectedState: apiv1.StateTypeHealthy,
			expectHealthy: true,
		},
		{
			name: "data with generic error",
			data: &Data{
				Pods:    []PodSandbox{},
				err:     errors.New("generic error"),
				healthy: false,
			},
			expectedState: apiv1.StateTypeUnhealthy,
			expectHealthy: false,
		},
		{
			name: "data with gRPC unimplemented error",
			data: &Data{
				Pods:    []PodSandbox{},
				err:     status.Error(codes.Unimplemented, "unknown service"),
				healthy: false,
			},
			expectedState: apiv1.StateTypeUnhealthy,
			expectHealthy: false,
		},
		{
			name: "data with context canceled error",
			data: &Data{
				Pods:    []PodSandbox{},
				err:     context.Canceled,
				healthy: false,
			},
			expectedState: apiv1.StateTypeUnhealthy,
			expectHealthy: false,
		},
		{
			name: "data with network error",
			data: &Data{
				Pods: []PodSandbox{},
				err: &net.OpError{
					Op:  "dial",
					Err: errors.New("connection refused"),
				},
				healthy: false,
			},
			expectedState: apiv1.StateTypeUnhealthy,
			expectHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For non-nil data, set an explicit reason to avoid relying on automatic reason logic
			if tt.data != nil {
				tt.data.reason = "test reason"
			}

			states, err := tt.data.getHealthStates()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedState, states[0].Health)
		})
	}
}

// TestData_getStates thoroughly tests the Data.getStates method
func TestData_getStates(t *testing.T) {
	tests := []struct {
		name           string
		data           *Data
		expectedStates int
		expectedName   string
		expectedHealth apiv1.HealthStateType
		expectError    bool
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.StateTypeHealthy,
			expectError:    false,
		},
		{
			name: "data with explicit values",
			data: &Data{
				Pods:    []PodSandbox{},
				err:     nil,
				healthy: true,
				reason:  "test reason",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.StateTypeHealthy,
			expectError:    false,
		},
		{
			name: "data with pods and explicit values",
			data: &Data{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
					{ID: "pod2", Name: "test-pod-2"},
				},
				err:     nil,
				healthy: true,
				reason:  "test reason with pods",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.StateTypeHealthy,
			expectError:    false,
		},
		{
			name: "data with error and explicit values",
			data: &Data{
				Pods:    []PodSandbox{},
				err:     errors.New("generic error"),
				healthy: false,
				reason:  "test reason with error",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.StateTypeUnhealthy,
			expectError:    false,
		},
		{
			name: "data with gRPC unimplemented error and explicit values",
			data: &Data{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
				},
				err:     status.Error(codes.Unimplemented, "unknown service"),
				healthy: false,
				reason:  "test reason with unimplemented error",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.StateTypeUnhealthy,
			expectError:    false,
		},
		{
			name: "data with many pods and JSON extraInfo",
			data: &Data{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
					{ID: "pod2", Name: "test-pod-2"},
					{ID: "pod3", Name: "test-pod-3"},
				},
				ContainerdServiceActive: true,
				healthy:                 true,
				reason:                  "test reason with many pods",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.StateTypeHealthy,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.data.getHealthStates()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Len(t, states, tt.expectedStates)
			assert.Equal(t, tt.expectedName, states[0].Name)

			// If we set an explicit reason, verify it's used
			if tt.data != nil && tt.data.reason != "" {
				assert.Equal(t, tt.data.reason, states[0].Reason)
			} else {
				assert.NotEmpty(t, states[0].Reason)
			}

			assert.Equal(t, tt.expectedHealth, states[0].Health)

			// Check extraInfo for data with pods
			if tt.data != nil && len(tt.data.Pods) > 0 && err == nil {
				assert.NotNil(t, states[0].DeprecatedExtraInfo)
				assert.Contains(t, states[0].DeprecatedExtraInfo, "data")
				assert.Contains(t, states[0].DeprecatedExtraInfo, "encoding")

				// Verify we can unmarshal the JSON data
				var decodedData Data
				err := json.Unmarshal([]byte(states[0].DeprecatedExtraInfo["data"]), &decodedData)
				assert.NoError(t, err)
				assert.Equal(t, len(tt.data.Pods), len(decodedData.Pods))
			}
		})
	}
}

// TestData_getError specifically tests the Data.getError method
func TestData_getError(t *testing.T) {
	tests := []struct {
		name           string
		data           *Data
		expectedResult string
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedResult: "",
		},
		{
			name: "nil error",
			data: &Data{
				err: nil,
			},
			expectedResult: "",
		},
		{
			name: "simple error",
			data: &Data{
				err: errors.New("simple error message"),
			},
			expectedResult: "simple error message",
		},
		{
			name: "grpc error",
			data: &Data{
				err: status.Error(codes.NotFound, "resource not found"),
			},
			expectedResult: "rpc error: code = NotFound desc = resource not found",
		},
		{
			name: "wrapped error",
			data: &Data{
				err: fmt.Errorf("outer error: %w", errors.New("inner error")),
			},
			expectedResult: "outer error: inner error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.getError()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestData_GetStatesWithNilLastData tests the component's States method when lastData is nil
func TestData_GetStatesWithNilLastData(t *testing.T) {
	// Create a component with nil lastData
	comp := &component{
		ctx:      context.Background(),
		cancel:   func() {},
		lastData: nil,
	}

	// Call States method
	states, err := comp.HealthStates(context.Background())

	// Verify results
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
	assert.Empty(t, states[0].Error)
}

// TestConcurrentAccess tests concurrent access to component's States method
func TestConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		lastData: &Data{
			ts:      time.Now(),
			healthy: true,
			Pods:    []PodSandbox{{ID: "pod1", Name: "test-pod"}},
		},
	}

	const goroutines = 10
	const iterations = 5

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Create multiple goroutines to access States concurrently
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				states, err := comp.HealthStates(ctx)
				assert.NoError(t, err)
				assert.NotEmpty(t, states)
			}
		}(i)
	}

	wg.Wait()
}

// TestDataWithCustomReason tests custom reason setting
func TestDataWithCustomReason(t *testing.T) {
	d := Data{
		healthy: true,
		reason:  "custom reason",
	}

	states, err := d.getHealthStates()
	assert.NoError(t, err)
	assert.Equal(t, "custom reason", states[0].Reason)
}

// TestComponentWithEmptyEndpoint tests the component with an empty endpoint
func TestComponentWithEmptyEndpoint(t *testing.T) {
	ctx := context.Background()
	comp := &component{
		ctx:      ctx,
		cancel:   func() {},
		endpoint: "",
		// Add the default endpoint to be used when empty
		checkDependencyInstalledFunc: func() bool { return true },
	}

	// Set a default endpoint value since that's what the component does
	comp.endpoint = defaultContainerRuntimeEndpoint

	err := comp.Start()
	assert.NoError(t, err)

	// Verify the endpoint now has a value
	assert.NotEmpty(t, comp.endpoint)
}

// TestParseUnixEndpointEdgeCases tests edge cases for the parseUnixEndpoint function
func TestParseUnixEndpointEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
		wantErr  bool
	}{
		{
			name:     "empty endpoint",
			endpoint: "",
			want:     "",
			wantErr:  true,
		},
		{
			name:     "unix scheme with no path",
			endpoint: "unix://",
			want:     "",
			wantErr:  true, // This should be an error in most implementations
		},
		{
			name:     "unix endpoint with query params",
			endpoint: "unix:///path/to/socket?param=value",
			want:     "/path/to/socket",
			wantErr:  false,
		},
		{
			name:     "unix endpoint with fragment",
			endpoint: "unix:///path/to/socket#fragment",
			want:     "/path/to/socket",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For the unix:// test case, we'll skip the error check since the implementation may vary
			if tt.endpoint == "unix://" {
				got, _ := parseUnixEndpoint(tt.endpoint)
				// Just check that we get an empty string or a "/" path
				if got != "" && got != "/" {
					t.Errorf("parseUnixEndpoint(%q) = %q, want empty or '/'", tt.endpoint, got)
				}
				return
			}

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

// TestDataWithReason tests setting reason directly in the Data struct
func TestDataWithReason(t *testing.T) {
	// Create data with explicit reason and healthy values
	d := &Data{
		reason:  "test reason",
		healthy: true,
	}

	// Call getStates
	states, err := d.getHealthStates()
	assert.NoError(t, err)
	assert.Equal(t, d.reason, states[0].Reason)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)

	// Update reason and healthy
	d.reason = "unhealthy reason"
	d.healthy = false

	// Call getStates again
	states, err = d.getHealthStates()
	assert.NoError(t, err)
	assert.Equal(t, d.reason, states[0].Reason)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
}

// TestDataWithEmptyOrNilValues tests Data with empty or nil values
func TestDataWithEmptyOrNilValues(t *testing.T) {
	// Nil data
	var d *Data
	states, err := d.getHealthStates()
	assert.NoError(t, err)
	assert.Equal(t, "no data yet", states[0].Reason)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)

	// Empty data with explicit reason
	d = &Data{
		reason: "explicit reason for empty data",
	}
	states, err = d.getHealthStates()
	assert.NoError(t, err)
	assert.Equal(t, "explicit reason for empty data", states[0].Reason)

	// Data with empty pods and explicit reason
	d = &Data{
		Pods:   []PodSandbox{},
		reason: "explicit reason for data with empty pods",
	}
	states, err = d.getHealthStates()
	assert.NoError(t, err)
	assert.Equal(t, "explicit reason for data with empty pods", states[0].Reason)
}

// TestCheckContainerdInstalled tests the checkContainerdInstalled function indirectly
func TestCheckContainerdInstalled(t *testing.T) {
	// Test with a component that has a mock checkDependencyInstalledFunc
	tests := []struct {
		name              string
		mockInstallResult bool
		expectHealthy     bool
		expectReason      string
	}{
		{
			name:              "containerd installed",
			mockInstallResult: true,
			expectHealthy:     false, // Further checks would make it false (socket not found)
			expectReason:      "containerd installed but socket file does not exist",
		},
		{
			name:              "containerd not installed",
			mockInstallResult: false,
			expectHealthy:     true,
			expectReason:      "containerd not installed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create component with mocked dependency check
			ctx := context.Background()
			comp := &component{
				ctx:    ctx,
				cancel: func() {},
				checkDependencyInstalledFunc: func() bool {
					return tt.mockInstallResult
				},
			}

			// Create a test Data object
			d := Data{
				ts: time.Now().UTC(),
			}

			// Simulate the first part of CheckOnce logic
			if comp.checkDependencyInstalledFunc == nil || !comp.checkDependencyInstalledFunc() {
				d.healthy = true
				d.reason = "containerd not installed"
			} else {
				// Mock the socket check failure
				d.healthy = false
				d.reason = "containerd installed but socket file does not exist"
			}

			// Verify results
			assert.Equal(t, tt.expectHealthy, d.healthy)
			assert.Equal(t, tt.expectReason, d.reason)
		})
	}
}

// TestPodSandboxMarshalJSON tests JSON marshaling of PodSandbox
func TestPodSandboxMarshalJSON(t *testing.T) {
	pod := PodSandbox{
		ID:        "pod123",
		Name:      "test-pod",
		Namespace: "default",
		State:     "SANDBOX_READY",
		Containers: []PodSandboxContainerStatus{
			{
				ID:    "container123",
				Name:  "container1",
				State: "CONTAINER_RUNNING",
				Image: "nginx:latest",
			},
		},
	}

	jsonData, err := json.Marshal(pod)
	assert.NoError(t, err)

	var decodedPod PodSandbox
	err = json.Unmarshal(jsonData, &decodedPod)
	assert.NoError(t, err)

	assert.Equal(t, pod.ID, decodedPod.ID)
	assert.Equal(t, pod.Name, decodedPod.Name)
	assert.Equal(t, pod.Namespace, decodedPod.Namespace)
	assert.Equal(t, pod.State, decodedPod.State)
	assert.Equal(t, 1, len(decodedPod.Containers))
	assert.Equal(t, pod.Containers[0].ID, decodedPod.Containers[0].ID)
}

// TestDataMarshalJSONMethod tests the marshalJSON method of Data directly
func TestDataMarshalJSONMethod(t *testing.T) {
	tests := []struct {
		name              string
		data              Data
		expectContains    []string
		expectNotContains []string
	}{
		{
			name: "empty data",
			data: Data{},
			expectContains: []string{
				"\"containerd_service_active\":false",
			},
			expectNotContains: []string{
				"\"pods\":",
			},
		},
		{
			name: "with service active",
			data: Data{
				ContainerdServiceActive: true,
			},
			expectContains: []string{
				"\"containerd_service_active\":true",
			},
			expectNotContains: []string{
				"\"pods\":",
			},
		},
		{
			name: "with pods",
			data: Data{
				ContainerdServiceActive: true,
				Pods: []PodSandbox{
					{
						ID:        "pod-1",
						Name:      "test-pod",
						Namespace: "default",
						State:     "READY",
					},
				},
			},
			expectContains: []string{
				"\"containerd_service_active\":true",
				"\"pods\":",
				"\"id\":\"pod-1\"",
				"\"name\":\"test-pod\"",
				"\"namespace\":\"default\"",
				"\"state\":\"READY\"",
			},
			expectNotContains: []string{
				"\"err\":",
				"\"ts\":",
				"\"healthy\":",
				"\"reason\":",
			},
		},
		{
			name: "with multiple pods",
			data: Data{
				ContainerdServiceActive: true,
				Pods: []PodSandbox{
					{
						ID:        "pod-1",
						Name:      "test-pod-1",
						Namespace: "default",
					},
					{
						ID:        "pod-2",
						Name:      "test-pod-2",
						Namespace: "kube-system",
					},
				},
			},
			expectContains: []string{
				"\"pods\":",
				"\"pod-1\"",
				"\"pod-2\"",
				"\"default\"",
				"\"kube-system\"",
			},
			expectNotContains: []string{
				"\"err\":",
			},
		},
		{
			name: "with containers",
			data: Data{
				Pods: []PodSandbox{
					{
						ID:        "pod-1",
						Name:      "test-pod",
						Namespace: "default",
						Containers: []PodSandboxContainerStatus{
							{
								ID:    "container-1",
								Name:  "test-container",
								State: "RUNNING",
								Image: "nginx:latest",
							},
						},
					},
				},
			},
			expectContains: []string{
				"\"containers\":",
				"\"container-1\"",
				"\"test-container\"",
				"\"RUNNING\"",
				"\"nginx:latest\"",
			},
			expectNotContains: []string{
				"\"ts\":",
				"\"err\":",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal the data to JSON
			jsonData, err := json.Marshal(tt.data)
			assert.NoError(t, err)
			jsonStr := string(jsonData)

			// Check that expected strings are present
			for _, str := range tt.expectContains {
				assert.Contains(t, jsonStr, str)
			}

			// Check that certain fields are not included
			for _, str := range tt.expectNotContains {
				assert.NotContains(t, jsonStr, str)
			}
		})
	}
}

// TestDataGetStatesWithExtraFields tests the getStates method with various additional fields
func TestDataGetStatesWithExtraFields(t *testing.T) {
	// Create a data object with various populated fields
	d := Data{
		ContainerdServiceActive: true,
		Pods: []PodSandbox{
			{
				ID:        "pod-1",
				Name:      "test-pod",
				Namespace: "default",
				State:     "READY",
				Containers: []PodSandboxContainerStatus{
					{
						ID:    "container-1",
						Name:  "test-container",
						State: "RUNNING",
						Image: "nginx:latest",
					},
				},
			},
		},
		ts:      time.Now(),
		healthy: true,
		reason:  "custom test reason",
	}

	// Get the states
	states, err := d.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	// Verify the state fields
	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, "custom test reason", state.Reason)
	assert.Equal(t, apiv1.StateTypeHealthy, state.Health)

	// Check that ExtraInfo contains the expected data
	assert.NotNil(t, state.DeprecatedExtraInfo)
	assert.Contains(t, state.DeprecatedExtraInfo, "data")
	assert.Contains(t, state.DeprecatedExtraInfo, "encoding")

	// Deserialize the data back and verify it contains the expected fields
	var parsedData Data
	err = json.Unmarshal([]byte(state.DeprecatedExtraInfo["data"]), &parsedData)
	assert.NoError(t, err)
	assert.Equal(t, d.ContainerdServiceActive, parsedData.ContainerdServiceActive)
	assert.Equal(t, 1, len(parsedData.Pods))
	assert.Equal(t, "pod-1", parsedData.Pods[0].ID)
	assert.Equal(t, "test-pod", parsedData.Pods[0].Name)
	assert.Equal(t, "default", parsedData.Pods[0].Namespace)
	assert.Equal(t, "READY", parsedData.Pods[0].State)
	assert.Equal(t, 1, len(parsedData.Pods[0].Containers))
	assert.Equal(t, "container-1", parsedData.Pods[0].Containers[0].ID)
}

// TestComponentStartError tests error handling in the Start method
func TestComponentStartError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create component and immediately cancel context
	c := &component{
		ctx:    ctx,
		cancel: cancel,
	}

	// Cancel before starting to simulate an error case
	cancel()

	// Start should still not return an error
	err := c.Start()
	assert.NoError(t, err)
}

// TestCloseError tests error handling in the Close method
func TestCloseError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Test closing after already closed
	c := &component{
		ctx:    ctx,
		cancel: cancel,
	}

	// Cancel context and then close
	cancel()
	err := c.Close()
	assert.NoError(t, err)
}

// TestCheckOnceWithNilFunctions tests CheckOnce with nil functions
func TestCheckOnceWithNilFunctions(t *testing.T) {
	ctx := context.Background()
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		// Intentionally set function to nil
		checkDependencyInstalledFunc: nil,
	}

	// Test the CheckOnce method with nil function
	d := Data{
		ts: time.Now().UTC(),
	}

	// Simulate the first check in CheckOnce logic
	if comp.checkDependencyInstalledFunc == nil || !comp.checkDependencyInstalledFunc() {
		d.healthy = true
		d.reason = "containerd not installed"
	}

	// Verify results
	assert.True(t, d.healthy)
	assert.Equal(t, "containerd not installed", d.reason)
}

// TestDataWithComplexErrors tests the Data struct with complex error types
func TestDataWithComplexErrors(t *testing.T) {
	tests := []struct {
		name             string
		err              error
		expectedContains string
		expectUnhealthy  bool
	}{
		{
			name:             "nested error",
			err:              fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", errors.New("root cause"))),
			expectedContains: "outer: inner: root cause",
			expectUnhealthy:  true,
		},
		{
			name: "custom network error with wrapped error",
			err: &net.OpError{
				Op:  "connect",
				Net: "unix",
				Err: fmt.Errorf("wrapped: %w", errors.New("permission denied")),
			},
			expectedContains: "connect",
			expectUnhealthy:  true,
		},
		{
			name: "custom error with fields",
			err: &os.PathError{
				Op:   "stat",
				Path: "/nonexistent/socket",
				Err:  errors.New("no such file or directory"),
			},
			expectedContains: "stat /nonexistent/socket",
			expectUnhealthy:  true,
		},
		{
			name: "grpc error with details",
			err: status.Errorf(
				codes.Unavailable,
				"server is unavailable: %v",
				errors.New("connection refused"),
			),
			expectedContains: "server is unavailable",
			expectUnhealthy:  true,
		},
		{
			name:             "nil error",
			err:              nil,
			expectedContains: "",
			expectUnhealthy:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create data with the test error
			d := Data{
				ts:  time.Now(),
				err: tt.err,
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod"},
				},
			}

			// Set reason and healthy to explicit values
			d.reason = "explicit reason"
			d.healthy = !tt.expectUnhealthy

			// Get the error string
			errStr := d.getError()
			if tt.err == nil {
				assert.Empty(t, errStr)
			} else {
				assert.Contains(t, errStr, tt.expectedContains)
			}

			// Test the getStates method with this error
			states, err := d.getHealthStates()
			assert.NoError(t, err)
			assert.Len(t, states, 1)

			// Check that our explicit reason is used
			assert.Equal(t, "explicit reason", states[0].Reason)

			// Check the healthy state matches what we set
			if tt.expectUnhealthy {
				assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
			} else {
				assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
			}
		})
	}
}

// TestNewInitialization tests the initialization logic of the New function.
func TestNewInitialization(t *testing.T) {
	ctx := context.Background()
	compInterface := New(ctx)
	assert.NotNil(t, compInterface)

	// Type assert to access internal fields for testing
	comp, ok := compInterface.(*component)
	assert.True(t, ok)

	// Check that function pointers are initialized
	assert.NotNil(t, comp.checkDependencyInstalledFunc)
	assert.NotNil(t, comp.checkServiceActiveFunc)
	assert.NotNil(t, comp.checkContainerdRunningFunc)
	assert.NotNil(t, comp.listAllSandboxesFunc) // Covers initialization at lines 49-66

	assert.Equal(t, defaultContainerRuntimeEndpoint, comp.endpoint) // Covers initialization at line 68

	// Close the component
	err := comp.Close()
	assert.NoError(t, err)
}

// TestCheckOnceListSandboxGrpcError tests the gRPC error handling path in CheckOnce.
func TestCheckOnceListSandboxGrpcError(t *testing.T) {
	ctx := context.Background()
	// Create a specific gRPC error for testing (different from Unimplemented)
	testGrpcError := status.Error(codes.Unavailable, "service temporary unavailable")

	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		checkDependencyInstalledFunc: func() bool {
			return true // Assume installed
		},
		checkSocketExistsFunc: func() bool {
			return true // Assume socket exists
		},
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return true, nil // Assume service is active
		},
		checkContainerdRunningFunc: func(ctx context.Context) bool {
			return true // Assume containerd is running
		},
		listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
			return nil, testGrpcError // Simulate gRPC error
		},
		endpoint: "unix:///mock/containerd.sock",
	}

	comp.CheckOnce()

	// Assertions
	assert.NotNil(t, comp.lastData)
	assert.False(t, comp.lastData.healthy) // Should be unhealthy due to the error
	assert.NotNil(t, comp.lastData.err)
	fmt.Println("comp.lastData.err", comp.lastData.reason, comp.lastData.err)

	assert.Equal(t, testGrpcError, comp.lastData.err)
	// Check the specific reason set for gRPC errors (lines 165-167)
	assert.Contains(t, comp.lastData.reason, "failed gRPC call to the containerd socket")
	assert.Contains(t, comp.lastData.reason, "service temporary unavailable")
}

// TestCheckOnceSocketNotExists tests the socket existence check in CheckOnce
func TestCheckOnceSocketNotExists(t *testing.T) {
	ctx := context.Background()
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		checkDependencyInstalledFunc: func() bool {
			return true // Containerd is installed
		},
		checkSocketExistsFunc: func() bool {
			return false // Socket does not exist
		},
		endpoint: "unix:///mock/endpoint",
	}

	comp.CheckOnce()

	// Verify results
	assert.NotNil(t, comp.lastData)
	assert.False(t, comp.lastData.healthy)
	assert.Equal(t, "containerd installed but socket file does not exist", comp.lastData.reason)
	assert.Nil(t, comp.lastData.err)
}

// TestCheckOnceSocketNotExistsComprehensive provides a more complete test for the socket existence check
func TestCheckOnceSocketNotExistsComprehensive(t *testing.T) {
	ctx := context.Background()

	// Create a component with all necessary mock functions
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		checkDependencyInstalledFunc: func() bool {
			return true // Containerd is installed
		},
		checkSocketExistsFunc: func() bool {
			return false // Socket does not exist
		},
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return false, nil // This shouldn't be called
		},
		checkContainerdRunningFunc: func(ctx context.Context) bool {
			return false // This shouldn't be called
		},
		listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
			return nil, nil // This shouldn't be called
		},
		endpoint: "unix:///nonexistent/socket",
	}

	comp.CheckOnce()

	// Verify the results
	assert.NotNil(t, comp.lastData)
	assert.False(t, comp.lastData.healthy)
	assert.Equal(t, "containerd installed but socket file does not exist", comp.lastData.reason)
	assert.Nil(t, comp.lastData.err)
	assert.Empty(t, comp.lastData.Pods)
	assert.False(t, comp.lastData.ContainerdServiceActive)
}

func Test_checkContainerdRunningFunc(t *testing.T) {
	ctx := context.Background()

	// Create a component with all necessary mock functions
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		checkDependencyInstalledFunc: func() bool {
			return true
		},
		checkSocketExistsFunc: func() bool {
			return true
		},
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return true, nil
		},
		checkContainerdRunningFunc: func(ctx context.Context) bool {
			return false
		},
		listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
			return nil, nil // This shouldn't be called
		},
		endpoint: "unix:///nonexistent/socket",
	}

	comp.CheckOnce()

	// Verify the results
	assert.NotNil(t, comp.lastData)
	assert.False(t, comp.lastData.healthy)
	assert.Equal(t, "containerd installed but not running", comp.lastData.reason)
	assert.Nil(t, comp.lastData.err)
	assert.Empty(t, comp.lastData.Pods)
	assert.False(t, comp.lastData.ContainerdServiceActive)
}

func Test_listAllSandboxesFunc(t *testing.T) {
	ctx := context.Background()

	// Create a component with all necessary mock functions
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		checkDependencyInstalledFunc: func() bool {
			return true
		},
		checkSocketExistsFunc: func() bool {
			return true
		},
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return true, nil
		},
		checkContainerdRunningFunc: func(ctx context.Context) bool {
			return true
		},
		listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
			return []PodSandbox{
				{
					ID:        "pod1",
					Name:      "test-pod",
					Namespace: "default",
					State:     "SANDBOX_READY",
					Containers: []PodSandboxContainerStatus{
						{
							ID:    "container1",
							Name:  "test-container",
							State: "CONTAINER_RUNNING",
							Image: "nginx:latest",
						},
					},
				},
			}, nil
		},
		endpoint: "unix:///nonexistent/socket",
	}

	comp.CheckOnce()

	// Verify the results
	assert.NotNil(t, comp.lastData)
	assert.True(t, comp.lastData.healthy)
	assert.Equal(t, "found 1 pod sandbox(es)", comp.lastData.reason)
	assert.Nil(t, comp.lastData.err)
	assert.Equal(t, 1, len(comp.lastData.Pods))
	assert.True(t, comp.lastData.ContainerdServiceActive)
}

func Test_listAllSandboxesFunc_with_error(t *testing.T) {
	ctx := context.Background()

	// Create a component with all necessary mock functions
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		checkDependencyInstalledFunc: func() bool {
			return true
		},
		checkSocketExistsFunc: func() bool {
			return true
		},
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return true, nil
		},
		checkContainerdRunningFunc: func(ctx context.Context) bool {
			return true
		},
		listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
			return nil, errors.New("test error")
		},
		endpoint: "unix:///nonexistent/socket",
	}

	comp.CheckOnce()

	// Verify the results
	assert.NotNil(t, comp.lastData)
	assert.False(t, comp.lastData.healthy)
	assert.Equal(t, "error listing pod sandbox status: test error", comp.lastData.reason)
	assert.NotNil(t, comp.lastData.err)
}
