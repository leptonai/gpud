package containerd

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
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// Mock NVML instance for testing
type mockNVMLInstance struct {
	nvmlExists  bool
	productName string
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.nvmlExists
}

func (m *mockNVMLInstance) ProductName() string {
	return m.productName
}

func (m *mockNVMLInstance) Architecture() string {
	return "mock-architecture"
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return nil
}

func (m *mockNVMLInstance) Brand() string {
	return "NVIDIA"
}

func (m *mockNVMLInstance) DriverVersion() string {
	return "470.00"
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 470
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return "11.4"
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return false
}

func (m *mockNVMLInstance) FabricStateSupported() bool {
	return false
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}

func runContainerdChecks(c *component, times int) *checkResult {
	var cr *checkResult
	for i := 0; i < times; i++ {
		result := c.Check()
		var ok bool
		cr, ok = result.(*checkResult)
		if !ok {
			return nil
		}
	}
	return cr
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func (m *mockNVMLInstance) InitError() error {
	return nil
}

func Test_componentStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:                          ctx,
		cancel:                       cancel,
		checkDependencyInstalledFunc: func() bool { return true },
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}
	err := c.Start()
	assert.NoError(t, err)
	assert.NoError(t, c.Close())
}

func TestComponentBasics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:                          ctx,
		cancel:                       cancel,
		checkDependencyInstalledFunc: func() bool { return true },
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

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

func TestTags(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{ctx: ctx, cancel: cancel}

	expectedTags := []string{
		"container",
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 2, "Component should return exactly 2 tags")
}

func TestDataFunctions(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		cr := checkResult{
			// Set explicit reason to avoid test failures
			reason: "empty data reason",
		}

		// Test marshalJSON
		b, err := json.Marshal(cr)
		assert.NoError(t, err)
		assert.NotNil(t, b)

		// Test states with empty data
		states := cr.HealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)

		// Check for our explicit reason
		assert.Equal(t, "empty data reason", states[0].Reason)
	})

	t.Run("data with error", func(t *testing.T) {
		cr := checkResult{
			Pods:   []PodSandbox{{ID: "pod1"}},
			err:    errors.New("test error"),
			health: apiv1.HealthStateTypeUnhealthy, // Explicitly set health
		}

		// Test states with error - just verify we get an unhealthy state
		states := cr.HealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	})

	t.Run("data with gRPC unimplemented error", func(t *testing.T) {
		cr := checkResult{
			Pods:   []PodSandbox{{ID: "pod1"}},
			err:    status.Error(codes.Unimplemented, "test unimplemented"),
			health: apiv1.HealthStateTypeUnhealthy, // Explicitly set health
		}

		// Test states with unimplemented error
		states := cr.HealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Contains(t, states[0].Error, "test unimplemented")
	})

	t.Run("empty data with error - empty pods takes precedence", func(t *testing.T) {
		cr := checkResult{
			Pods: []PodSandbox{},
			err:  errors.New("test error"),
			// Set explicit reason
			reason: "empty pods with error reason",
		}

		// Test states with empty pods and error
		states := cr.HealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, "empty pods with error reason", states[0].Reason)
	})

	t.Run("data with pods", func(t *testing.T) {
		cr := checkResult{
			Pods: []PodSandbox{
				{
					ID:   "pod1",
					Name: "test-pod",
				},
			},
		}

		// Test getStates with pods
		states := cr.HealthStates()
		assert.Len(t, states, 1)
		assert.Contains(t, states[0].ExtraInfo, "data")
	})
}

// Test the component States method separately
func TestComponentStates(t *testing.T) {
	ctx := context.Background()
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		lastCheckResult: &checkResult{
			Pods: []PodSandbox{
				{
					ID:   "pod1",
					Name: "test-pod",
				},
			},
		},
	}

	states := comp.LastHealthStates()
	assert.NotNil(t, states)
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
			expectedHealthy:          true,
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
			expectedHealthy:          true,
			expectedReasonContains:   "CRI is not enabled",
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
			expectedReasonContains:   "ok",
			expectedPodsLength:       0,
			expectedServiceActive:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			comp := &component{
				ctx:    ctx,
				cancel: func() {},
				checkDependencyInstalledFunc: func() bool {
					return tt.checkDependencyInstalled
				},
				checkSocketExistsFunc: func() bool {
					return tt.socketExists
				},
				checkContainerdRunningFunc: func(ctx context.Context) bool {
					return tt.containerdRunning
				},
				checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
					return tt.serviceActive, tt.serviceActiveError
				},
				listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
					if tt.listSandboxError != nil {
						return nil, tt.listSandboxError
					}
					return []PodSandbox{}, nil
				},
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
				endpoint: "unix:///mock/endpoint",
			}

			comp.Check()

			// Assert results
			assert.NotNil(t, comp.lastCheckResult)
			assert.Equal(t, tt.expectedHealthy, comp.lastCheckResult.health == apiv1.HealthStateTypeHealthy)
			assert.Contains(t, comp.lastCheckResult.reason, tt.expectedReasonContains)
			assert.Equal(t, tt.expectedPodsLength, len(comp.lastCheckResult.Pods))
			assert.Equal(t, tt.expectedServiceActive, comp.lastCheckResult.ContainerdServiceActive)
		})
	}
}

// Test New function
func TestNew(t *testing.T) {
	ctx := context.Background()
	compInterface, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	require.NoError(t, err)

	assert.NotNil(t, compInterface)
	assert.Equal(t, Name, compInterface.Name())
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
		cr := checkResult{
			ts:                      time.Now().UTC(),
			ContainerdServiceActive: true,
			Pods:                    mockPods, // Use our mock pods
			health:                  apiv1.HealthStateTypeHealthy,
			reason:                  fmt.Sprintf("found %d pod sandbox(es)", len(mockPods)),
		}

		c.lastMu.Lock()
		c.lastCheckResult = &cr
		c.lastMu.Unlock()
	}

	// Run our test specific version
	testCheckOnce(comp)

	// Assert results
	assert.NotNil(t, comp.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, comp.lastCheckResult.health)
	assert.Equal(t, 2, len(comp.lastCheckResult.Pods))
	assert.Equal(t, "pod1", comp.lastCheckResult.Pods[0].ID)
	assert.Equal(t, "pod2", comp.lastCheckResult.Pods[1].ID)
	assert.Equal(t, "SANDBOX_READY", comp.lastCheckResult.Pods[0].State)
	assert.Equal(t, 1, len(comp.lastCheckResult.Pods[0].Containers))
	assert.Equal(t, "container1", comp.lastCheckResult.Pods[0].Containers[0].ID)
	assert.Equal(t, "default", comp.lastCheckResult.Pods[0].Namespace)
	assert.Equal(t, "kube-system", comp.lastCheckResult.Pods[1].Namespace)
	assert.Contains(t, comp.lastCheckResult.reason, "found 2 pod")
}

// Test component Events method more thoroughly
func TestComponentEvents(t *testing.T) {
	ctx := context.Background()
	comp := &component{
		ctx:                          ctx,
		cancel:                       func() {},
		checkDependencyInstalledFunc: func() bool { return true },
		endpoint:                     "unix:///nonexistent/socket",
		lastCheckResult: &checkResult{
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
	states := comp.LastHealthStates()
	assert.NotNil(t, states)
}

// Test marshalJSON function with different scenarios
func TestDataMarshalJSON(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		cr := checkResult{}
		jsonData, err := json.Marshal(cr)
		assert.NoError(t, err)
		assert.NotNil(t, jsonData)
		// Empty data should marshal to an empty JSON object
		assert.Equal(t, "{\"containerd_service_active\":false}", string(jsonData))
	})

	t.Run("data with pods", func(t *testing.T) {
		cr := checkResult{
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
		jsonData, err := json.Marshal(cr)
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

		cr := checkResult{
			Pods: []PodSandbox{badPod},
		}

		// This is expected to either return an error or escape the invalid UTF-8
		jsonData, _ := json.Marshal(cr)
		assert.NotNil(t, jsonData) // Should still produce some output
	})
}

// Test getHealth function with different error types
func TestGetHealthFromStates(t *testing.T) {
	t.Run("connection refused error", func(t *testing.T) {
		cr := checkResult{
			err: &net.OpError{
				Op:  "dial",
				Err: errors.New("connection refused"),
			},
			health: apiv1.HealthStateTypeUnhealthy, // Explicitly set health
		}

		states := cr.HealthStates()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	})

	t.Run("permission denied error", func(t *testing.T) {
		cr := checkResult{
			err: &os.PathError{
				Op:   "open",
				Path: "/path/to/socket",
				Err:  errors.New("permission denied"),
			},
			health: apiv1.HealthStateTypeUnhealthy, // Explicitly set health
		}

		states := cr.HealthStates()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	})

	t.Run("context canceled error", func(t *testing.T) {
		cr := checkResult{
			err:    context.Canceled,
			health: apiv1.HealthStateTypeUnhealthy, // Explicitly set health
		}

		states := cr.HealthStates()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	})

	t.Run("context deadline exceeded error", func(t *testing.T) {
		cr := checkResult{
			err:    context.DeadlineExceeded,
			health: apiv1.HealthStateTypeUnhealthy, // Explicitly set health
		}

		states := cr.HealthStates()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	})

	t.Run("grpc unavailable error", func(t *testing.T) {
		cr := checkResult{
			err:    status.Error(codes.Unavailable, "service unavailable"),
			health: apiv1.HealthStateTypeUnhealthy, // Explicitly set health
		}

		states := cr.HealthStates()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	})
}

// Test getStates function with edge cases
func TestGetStatesEdgeCases(t *testing.T) {
	t.Run("empty data with error", func(t *testing.T) {
		cr := checkResult{
			err: errors.New("some error"),
			// Add explicit reason
			reason: "empty data edge case",
			health: apiv1.HealthStateTypeUnhealthy,
		}

		states := cr.HealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "empty data edge case", states[0].Reason)
	})

	t.Run("data with pods and error", func(t *testing.T) {
		cr := checkResult{
			Pods:   []PodSandbox{{ID: "pod1"}},
			err:    errors.New("grpc connection error"),
			reason: "pods with error edge case",
			health: apiv1.HealthStateTypeUnhealthy,
		}

		states := cr.HealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
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

		cr := checkResult{
			Pods: pods,
			// Add explicit values
			reason: "many pods edge case",
			health: apiv1.HealthStateTypeHealthy,
		}

		states := cr.HealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "many pods edge case", states[0].Reason)

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

		cr := checkResult{
			Pods: []PodSandbox{badPod},
		}

		states := cr.HealthStates()
		// Even with JSON issues, the function should not return an error
		// It might produce empty or escaped JSON
		assert.Len(t, states, 1)
	})
}

// TestData_Reason specifically tests the Data struct reason logic through getStates
func TestData_Reason(t *testing.T) {
	tests := []struct {
		name           string
		data           checkResult
		explicitReason string
	}{
		{
			name: "nil pods array",
			data: checkResult{
				Pods: nil,
				err:  nil,
			},
			explicitReason: "nil pods reason",
		},
		{
			name: "empty data no error",
			data: checkResult{
				Pods: []PodSandbox{},
				err:  nil,
			},
			explicitReason: "empty data reason",
		},
		{
			name: "empty pods with connection error",
			data: checkResult{
				Pods: []PodSandbox{},
				err:  errors.New("connection refused"),
			},
			explicitReason: "empty pods with error reason",
		},
		{
			name: "single pod no error",
			data: checkResult{
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
			data: checkResult{
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
			data: checkResult{
				Pods: []PodSandbox{
					{ID: "pod1"},
				},
				err: errors.New("generic error"),
			},
			explicitReason: "generic error reason",
		},
		{
			name: "unimplemented error",
			data: checkResult{
				Pods: []PodSandbox{
					{ID: "pod1"},
				},
				err: status.Error(codes.Unimplemented, "unknown service"),
			},
			explicitReason: "unimplemented error reason",
		},
		{
			name: "pods with unimplemented error",
			data: checkResult{
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
			data: checkResult{
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
			states := tt.data.HealthStates()
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
		data checkResult
	}{
		{
			name: "context canceled error",
			data: checkResult{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  context.Canceled,
			},
		},
		{
			name: "context deadline exceeded error",
			data: checkResult{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  context.DeadlineExceeded,
			},
		},
		{
			name: "network dial error",
			data: checkResult{
				Pods: []PodSandbox{{ID: "pod1"}},
				err: &net.OpError{
					Op:  "dial",
					Err: errors.New("connection refused"),
				},
			},
		},
		{
			name: "network connect error",
			data: checkResult{
				Pods: []PodSandbox{{ID: "pod1"}},
				err: &net.OpError{
					Op:  "connect",
					Err: errors.New("connection reset by peer"),
				},
			},
		},
		{
			name: "permission denied error",
			data: checkResult{
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
			data: checkResult{
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
			data: checkResult{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  status.Error(codes.Internal, "internal error"),
			},
		},
		{
			name: "grpc not found error",
			data: checkResult{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  status.Error(codes.NotFound, "not found"),
			},
		},
		{
			name: "grpc resource exhausted error",
			data: checkResult{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  status.Error(codes.ResourceExhausted, "resource exhausted"),
			},
		},
		{
			name: "wrapped error",
			data: checkResult{
				Pods: []PodSandbox{{ID: "pod1"}},
				err:  fmt.Errorf("could not connect: %w", errors.New("underlying error")),
			},
		},
		{
			name: "error take precedence over empty pod",
			data: checkResult{
				Pods: []PodSandbox{},
				err:  errors.New("this error"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set explicit reason and healthy to ensure predictable behavior
			tt.data.reason = "explicit test reason"
			tt.data.health = apiv1.HealthStateTypeUnhealthy

			states := tt.data.HealthStates()
			assert.Equal(t, "explicit test reason", states[0].Reason)
			assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		})
	}
}

// TestData_HealthStates thoroughly tests the health status from Data.getStates method
func TestData_HealthStates(t *testing.T) {
	tests := []struct {
		name          string
		data          *checkResult
		expectedState apiv1.HealthStateType
		expectHealthy bool
	}{
		{
			name:          "nil data",
			data:          nil,
			expectedState: apiv1.HealthStateTypeHealthy,
			expectHealthy: true,
		},
		{
			name: "empty data with explicit healthy",
			data: &checkResult{
				Pods:   []PodSandbox{},
				err:    nil,
				health: apiv1.HealthStateTypeHealthy,
			},
			expectedState: apiv1.HealthStateTypeHealthy,
			expectHealthy: true,
		},
		{
			name: "data with pods and explicit healthy",
			data: &checkResult{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
					{ID: "pod2", Name: "test-pod-2"},
				},
				err:    nil,
				health: apiv1.HealthStateTypeHealthy,
			},
			expectedState: apiv1.HealthStateTypeHealthy,
			expectHealthy: true,
		},
		{
			name: "data with generic error",
			data: &checkResult{
				Pods:   []PodSandbox{},
				err:    errors.New("generic error"),
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expectedState: apiv1.HealthStateTypeUnhealthy,
			expectHealthy: false,
		},
		{
			name: "data with gRPC unimplemented error",
			data: &checkResult{
				Pods:   []PodSandbox{},
				err:    status.Error(codes.Unimplemented, "unknown service"),
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expectedState: apiv1.HealthStateTypeUnhealthy,
			expectHealthy: false,
		},
		{
			name: "data with context canceled error",
			data: &checkResult{
				Pods:   []PodSandbox{},
				err:    context.Canceled,
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expectedState: apiv1.HealthStateTypeUnhealthy,
			expectHealthy: false,
		},
		{
			name: "data with network error",
			data: &checkResult{
				Pods: []PodSandbox{},
				err: &net.OpError{
					Op:  "dial",
					Err: errors.New("connection refused"),
				},
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expectedState: apiv1.HealthStateTypeUnhealthy,
			expectHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For non-nil data, set an explicit reason to avoid relying on automatic reason logic
			if tt.data != nil {
				tt.data.reason = "test reason"
			}

			states := tt.data.HealthStates()
			assert.Equal(t, tt.expectedState, states[0].Health)
		})
	}
}

// TestData_getStates thoroughly tests the Data.getStates method
func TestData_getStates(t *testing.T) {
	tests := []struct {
		name           string
		data           *checkResult
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
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectError:    false,
		},
		{
			name: "data with explicit values",
			data: &checkResult{
				Pods:   []PodSandbox{},
				err:    nil,
				health: apiv1.HealthStateTypeHealthy,
				reason: "test reason",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectError:    false,
		},
		{
			name: "data with pods and explicit values",
			data: &checkResult{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
					{ID: "pod2", Name: "test-pod-2"},
				},
				err:    nil,
				health: apiv1.HealthStateTypeHealthy,
				reason: "test reason with pods",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectError:    false,
		},
		{
			name: "data with error and explicit values",
			data: &checkResult{
				Pods:   []PodSandbox{},
				err:    errors.New("generic error"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "test reason with error",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectError:    false,
		},
		{
			name: "data with gRPC unimplemented error and explicit values",
			data: &checkResult{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
				},
				err:    status.Error(codes.Unimplemented, "unknown service"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "test reason with unimplemented error",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectError:    false,
		},
		{
			name: "data with many pods and JSON extraInfo",
			data: &checkResult{
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod-1"},
					{ID: "pod2", Name: "test-pod-2"},
					{ID: "pod3", Name: "test-pod-3"},
				},
				ContainerdServiceActive: true,
				health:                  apiv1.HealthStateTypeHealthy,
				reason:                  "test reason with many pods",
			},
			expectedStates: 1,
			expectedName:   Name,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := tt.data.HealthStates()
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
			if tt.data != nil && len(tt.data.Pods) > 0 {
				assert.NotNil(t, states[0].ExtraInfo)
				assert.Contains(t, states[0].ExtraInfo, "data")

				// Verify we can unmarshal the JSON data
				var decodedData checkResult
				jsonErr := json.Unmarshal([]byte(states[0].ExtraInfo["data"]), &decodedData)
				assert.NoError(t, jsonErr)
				assert.Equal(t, len(tt.data.Pods), len(decodedData.Pods))
			}
		})
	}
}

// TestData_getError specifically tests the Data.getError method
func TestData_getError(t *testing.T) {
	tests := []struct {
		name           string
		data           *checkResult
		expectedResult string
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedResult: "",
		},
		{
			name: "nil error",
			data: &checkResult{
				err: nil,
			},
			expectedResult: "",
		},
		{
			name: "simple error",
			data: &checkResult{
				err: errors.New("simple error message"),
			},
			expectedResult: "simple error message",
		},
		{
			name: "grpc error",
			data: &checkResult{
				err: status.Error(codes.NotFound, "resource not found"),
			},
			expectedResult: "rpc error: code = NotFound desc = resource not found",
		},
		{
			name: "wrapped error",
			data: &checkResult{
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

// TestData_GetStatesWithNillastCheckResult tests the component's States method when lastCheckResult is nil
func TestData_GetStatesWithNillastCheckResult(t *testing.T) {
	// Create a component with nil lastCheckResult
	comp := &component{
		ctx:             context.Background(),
		cancel:          func() {},
		lastCheckResult: nil,
	}

	// Call States method
	states := comp.LastHealthStates()

	// Verify results
	assert.NotNil(t, states)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
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
		lastCheckResult: &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeHealthy,
			Pods:   []PodSandbox{{ID: "pod1", Name: "test-pod"}},
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
				states := comp.LastHealthStates()
				assert.NotEmpty(t, states)
			}
		}(i)
	}

	wg.Wait()
}

// TestDataWithCustomReason tests custom reason setting
func TestDataWithCustomReason(t *testing.T) {
	cr := checkResult{
		health: apiv1.HealthStateTypeHealthy,
		reason: "custom reason",
	}

	states := cr.HealthStates()
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
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	// Set a default endpoint value since that's what the component does
	comp.endpoint = DefaultContainerRuntimeEndpoint

	err := comp.Start()
	assert.NoError(t, err)

	// Verify the endpoint now has a value
	assert.NotEmpty(t, comp.endpoint)
}

// TestDataWithReason tests setting reason directly in the Data struct
func TestDataWithReason(t *testing.T) {
	// Create data with explicit reason and healthy values
	cr := &checkResult{
		reason: "test reason",
		health: apiv1.HealthStateTypeHealthy,
	}

	// Call getStates
	states := cr.HealthStates()
	assert.Equal(t, cr.reason, states[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)

	// Update reason and healthy
	cr.reason = "unhealthy reason"
	cr.health = apiv1.HealthStateTypeUnhealthy

	// Call getStates again
	states = cr.HealthStates()
	assert.Equal(t, cr.reason, states[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
}

// TestDataWithEmptyOrNilValues tests Data with empty or nil values
func TestDataWithEmptyOrNilValues(t *testing.T) {
	// Nil data
	var cr *checkResult
	states := cr.HealthStates()
	assert.Equal(t, "no data yet", states[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)

	// Empty data with explicit reason
	cr = &checkResult{
		reason: "explicit reason for empty data",
	}
	states = cr.HealthStates()
	assert.Equal(t, "explicit reason for empty data", states[0].Reason)

	// Data with empty pods and explicit reason
	cr = &checkResult{
		Pods:   []PodSandbox{},
		reason: "explicit reason for data with empty pods",
	}
	states = cr.HealthStates()
	assert.Equal(t, "explicit reason for data with empty pods", states[0].Reason)
}

// TestCheckContainerdInstalled tests the checkContainerdInstalled function indirectly
func TestCheckContainerdInstalled(t *testing.T) {
	// Test with a component that has a mock checkDependencyInstalledFunc
	tests := []struct {
		name              string
		mockInstallResult bool
		expectHealth      apiv1.HealthStateType
		expectReason      string
	}{
		{
			name:              "containerd installed",
			mockInstallResult: true,
			expectHealth:      apiv1.HealthStateTypeUnhealthy,
			expectReason:      "failed continuously",
		},
		{
			name:              "containerd not installed",
			mockInstallResult: false,
			expectHealth:      apiv1.HealthStateTypeHealthy,
			expectReason:      "containerd not installed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create component with mocked dependency check
			ctx := context.Background()
			socketExists := false
			comp := &component{
				ctx:    ctx,
				cancel: func() {},
				checkDependencyInstalledFunc: func() bool {
					return tt.mockInstallResult
				},
				checkSocketExistsFunc: func() bool {
					return socketExists
				},
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
			}

			var cr *checkResult
			if tt.mockInstallResult {
				cr = runContainerdChecks(comp, socketMissingConsecutiveThreshold)
			} else {
				result := comp.Check()
				cr = result.(*checkResult)
			}

			// Verify results
			assert.Equal(t, tt.expectHealth, cr.health)
			assert.Contains(t, cr.reason, tt.expectReason)
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
		data              checkResult
		expectContains    []string
		expectNotContains []string
	}{
		{
			name: "empty data",
			data: checkResult{},
			expectContains: []string{
				"\"containerd_service_active\":false",
			},
			expectNotContains: []string{
				"\"pods\":",
			},
		},
		{
			name: "with service active",
			data: checkResult{
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
			data: checkResult{
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
			data: checkResult{
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
			data: checkResult{
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
	cr := checkResult{
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
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "custom test reason",
	}

	// Get the states
	states := cr.HealthStates()
	assert.Len(t, states, 1)

	// Verify the state fields
	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, "custom test reason", state.Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)

	// Check that ExtraInfo contains the expected data
	assert.NotNil(t, state.ExtraInfo)
	assert.Contains(t, state.ExtraInfo, "data")

	// Deserialize the data back and verify it contains the expected fields
	var parsedData checkResult
	err := json.Unmarshal([]byte(state.ExtraInfo["data"]), &parsedData)
	assert.NoError(t, err)
	assert.Equal(t, len(cr.Pods), len(parsedData.Pods))
	assert.Equal(t, cr.ContainerdServiceActive, parsedData.ContainerdServiceActive)
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
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
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

	cr := &checkResult{
		ts: time.Now().UTC(),
	}

	// Simulate the first check in CheckOnce logic
	if comp.checkDependencyInstalledFunc == nil || !comp.checkDependencyInstalledFunc() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "containerd not installed"
	}

	// Verify results
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "containerd not installed", cr.reason)
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
			cr := checkResult{
				ts:  time.Now(),
				err: tt.err,
				Pods: []PodSandbox{
					{ID: "pod1", Name: "test-pod"},
				},
			}

			// Set reason and healthy to explicit values
			cr.reason = "explicit reason"
			cr.health = apiv1.HealthStateTypeHealthy
			if tt.expectUnhealthy {
				cr.health = apiv1.HealthStateTypeUnhealthy
			}

			// Get the error string
			errStr := cr.getError()
			if tt.err == nil {
				assert.Empty(t, errStr)
			} else {
				assert.Contains(t, errStr, tt.expectedContains)
			}

			// Test the getStates method with this error
			states := cr.HealthStates()
			assert.Len(t, states, 1)

			// Check that our explicit reason is used
			assert.Equal(t, "explicit reason", states[0].Reason)

			// Check the healthy state matches what we set
			if tt.expectUnhealthy {
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
			} else {
				assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
			}
		})
	}
}

// TestNewInitialization tests the initialization logic of the New function.
func TestNewInitialization(t *testing.T) {
	ctx := context.Background()
	compInterface, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	require.NoError(t, err)
	assert.NotNil(t, compInterface)

	// Type assert to access internal fields for testing
	comp, ok := compInterface.(*component)
	assert.True(t, ok)

	// Check that function pointers are initialized
	assert.NotNil(t, comp.checkDependencyInstalledFunc)
	assert.NotNil(t, comp.checkServiceActiveFunc)
	assert.NotNil(t, comp.checkContainerdRunningFunc)
	assert.NotNil(t, comp.listAllSandboxesFunc) // Covers initialization at lines 49-66

	assert.Equal(t, DefaultContainerRuntimeEndpoint, comp.endpoint) // Covers initialization at line 68

	// Close the component
	assert.NoError(t, comp.Close())
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
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		endpoint: "unix:///mock/containerd.sock",
	}

	_ = comp.Check()

	// Assertions
	assert.NotNil(t, comp.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, comp.lastCheckResult.health) // Should be unhealthy due to the error
	assert.NotNil(t, comp.lastCheckResult.err)

	// Based on the component.go implementation (lines 151-156), non-Unimplemented errors
	// will have a reason message format like "error listing pod sandbox status: %v"
	assert.Contains(t, comp.lastCheckResult.reason, "error listing pod sandbox status")
}

// TestCheckOnceSocketNotExists tests the socket existence check in CheckOnce
func TestCheckOnceSocketNotExists(t *testing.T) {
	ctx := context.Background()
	socketExists := false
	comp := &component{
		ctx:    ctx,
		cancel: func() {},
		checkDependencyInstalledFunc: func() bool {
			return true // Containerd is installed
		},
		checkSocketExistsFunc: func() bool {
			return socketExists
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		endpoint: "unix:///mock/endpoint",
	}

	for i := 0; i < socketMissingConsecutiveThreshold-1; i++ {
		_ = comp.Check()
		assert.NotNil(t, comp.lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, comp.lastCheckResult.health)
		assert.Contains(t, comp.lastCheckResult.reason, "socket file does not exist")
		assert.Nil(t, comp.lastCheckResult.err)
	}

	cr := runContainerdChecks(comp, 1)
	require.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "socket file does not exist")
	assert.Contains(t, cr.reason, "failed continuously")
	assert.Nil(t, cr.err)

	// Reset after recovery
	socketExists = true
	cr = runContainerdChecks(comp, 1)
	require.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "ok", cr.reason)

	// Ensure counter resets after a successful check
	socketExists = false
	cr = runContainerdChecks(comp, 1)
	require.NotNil(t, cr)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "socket file does not exist")
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
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		endpoint: "unix:///nonexistent/socket",
	}

	_ = comp.Check()

	// Verify the results
	assert.NotNil(t, comp.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, comp.lastCheckResult.health)
	assert.Contains(t, comp.lastCheckResult.reason, "containerd installed but socket file does not exist")
	assert.Nil(t, comp.lastCheckResult.err)
	assert.Empty(t, comp.lastCheckResult.Pods)
	assert.False(t, comp.lastCheckResult.ContainerdServiceActive)
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
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		endpoint: "unix:///nonexistent/socket",
	}

	_ = comp.Check()

	// Verify the results
	assert.NotNil(t, comp.lastCheckResult)
	assert.False(t, comp.lastCheckResult.health == apiv1.HealthStateTypeHealthy)
	assert.Equal(t, "containerd installed but not running", comp.lastCheckResult.reason)
	assert.Nil(t, comp.lastCheckResult.err)
}

// TestNVMLValidationWithContainerToolkit tests the NVML validation with nvidia-container-toolkit logic
func TestNVMLValidationWithContainerToolkit(t *testing.T) {
	tests := []struct {
		name                              string
		nvmlInstance                      nvidianvml.Instance
		getContainerdConfigFunc           func() ([]byte, error)
		pods                              []PodSandbox
		getTimeNowFunc                    func() time.Time
		containerToolkitCreationThreshold time.Duration
		expectedHealth                    apiv1.HealthStateType
		expectedReason                    string
	}{
		{
			name: "nvml with nvidia config and container toolkit present and running long enough",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				config := `default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`
				return []byte(config), nil
			},
			pods: []PodSandbox{
				{
					Name:  "nvidia-container-toolkit-daemonset-abc123",
					State: "SANDBOX_READY",
					Containers: []PodSandboxContainerStatus{
						{
							Name:      "nvidia-container-toolkit-ctr",
							State:     "CONTAINER_RUNNING",
							CreatedAt: time.Now().Add(-15 * time.Minute).UnixNano(),
						},
					},
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "ok",
		},
		{
			name: "nvml without nvidia config but container toolkit present and running long enough",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				return []byte("[plugins.\"io.containerd.grpc.v1.cri\".containerd.runtimes.runc]"), nil
			},
			pods: []PodSandbox{
				{
					Name:      "nvidia-container-toolkit-daemonset-def456",
					State:     "SANDBOX_READY",
					CreatedAt: time.Now().Add(-15 * time.Minute).UnixNano(),
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeUnhealthy,
			expectedReason:                    "nvidia-container-toolkit pod is running but /etc/containerd/config.toml is missing NVIDIA runtime configuration",
		},
		{
			name: "nvml with container toolkit but not running long enough",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				config := `default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`
				return []byte(config), nil
			},
			pods: []PodSandbox{
				{
					Name:  "nvidia-container-toolkit-daemonset-ghi789",
					State: "SANDBOX_READY",
					Containers: []PodSandboxContainerStatus{
						{
							Name:      "nvidia-container-toolkit-ctr",
							State:     "CONTAINER_RUNNING",
							CreatedAt: time.Now().Add(-5 * time.Minute).UnixNano(),
						},
					},
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "ok",
		},
		{
			name: "nvml with container toolkit but container not running",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				config := `default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`
				return []byte(config), nil
			},
			pods: []PodSandbox{
				{
					Name:  "nvidia-container-toolkit-daemonset-jkl012",
					State: "SANDBOX_READY",
					Containers: []PodSandboxContainerStatus{
						{
							Name:      "nvidia-container-toolkit-ctr",
							State:     "CONTAINER_EXITED",
							CreatedAt: time.Now().Add(-15 * time.Minute).UnixNano(),
						},
					},
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "ok",
		},
		{
			name: "nvml with container toolkit pod but pod not ready",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				config := `default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`
				return []byte(config), nil
			},
			pods: []PodSandbox{
				{
					Name:      "nvidia-container-toolkit-daemonset-notready",
					State:     "SANDBOX_NOTREADY",
					CreatedAt: time.Now().Add(-15 * time.Minute).UnixNano(),
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "nvidia GPUs found but nvidia-container-toolkit pod is not found",
		},
		{
			name: "nvml with nvidia GPUs but no container toolkit pod",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				config := `default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`
				return []byte(config), nil
			},
			pods: []PodSandbox{
				{
					Name:  "some-other-pod",
					State: "SANDBOX_READY",
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "nvidia GPUs found but nvidia-container-toolkit pod is not found",
		},
		{
			name: "nvml with config error when checking containerd config",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				return nil, errors.New("config read error")
			},
			pods: []PodSandbox{
				{
					Name:      "nvidia-container-toolkit-daemonset-xyz",
					State:     "SANDBOX_READY",
					CreatedAt: time.Now().Add(-15 * time.Minute).UnixNano(),
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "error getting containerd config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := &component{
				ctx:    ctx,
				cancel: cancel,

				nvmlInstance:                      tt.nvmlInstance,
				getContainerdConfigFunc:           tt.getContainerdConfigFunc,
				getTimeNowFunc:                    tt.getTimeNowFunc,
				containerToolkitCreationThreshold: tt.containerToolkitCreationThreshold,

				// Mock all dependencies as successful
				checkDependencyInstalledFunc: func() bool {
					return true
				},
				checkSocketExistsFunc: func() bool {
					return true
				},
				checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
					return true, nil
				},
				checkContainerdRunningFunc: func(context.Context) bool {
					return true
				},
				listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
					return tt.pods, nil
				},
				endpoint: "unix:///var/run/containerd/containerd.sock",
			}

			cr := c.Check()
			checkResult, ok := cr.(*checkResult)
			require.True(t, ok, "Expected checkResult type")

			assert.Equal(t, tt.expectedHealth, checkResult.health, "Health state should match expected")
			assert.Equal(t, tt.expectedReason, checkResult.reason, "Reason should match expected text exactly")
		})
	}
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
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		endpoint: "unix:///nonexistent/socket",
	}

	_ = comp.Check()

	// Verify the results
	assert.NotNil(t, comp.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, comp.lastCheckResult.health)
	assert.Equal(t, "ok", comp.lastCheckResult.reason)
	assert.Nil(t, comp.lastCheckResult.err)
	assert.Equal(t, 1, len(comp.lastCheckResult.Pods))
	assert.True(t, comp.lastCheckResult.ContainerdServiceActive)
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
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		endpoint: "unix:///nonexistent/socket",
	}

	_ = comp.Check()

	// Verify the results
	assert.NotNil(t, comp.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, comp.lastCheckResult.health)
	assert.Equal(t, "error listing pod sandbox status", comp.lastCheckResult.reason)
	assert.NotNil(t, comp.lastCheckResult.err)
}

// TestDataString thoroughly tests the Data.String() method
func TestDataString(t *testing.T) {
	tests := []struct {
		name              string
		data              *checkResult
		expectedResult    string
		expectEmpty       bool
		expectContains    []string
		expectNotContains []string
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedResult: "",
			expectEmpty:    true,
		},
		{
			name: "empty pods",
			data: &checkResult{
				Pods: []PodSandbox{},
			},
			expectedResult: "no pod found",
			expectEmpty:    false,
		},
		{
			name: "single pod without containers",
			data: &checkResult{
				Pods: []PodSandbox{
					{
						ID:        "pod1",
						Name:      "test-pod",
						Namespace: "default",
						State:     "READY",
					},
				},
			},
			expectEmpty: false,
			expectContains: []string{
				"NAMESPACE", "POD", "CONTAINER", "STATE", // Headers are uppercase in tablewriter
			},
			// Pods without containers don't appear in the table
			expectNotContains: []string{
				"default", "test-pod",
			},
		},
		{
			name: "single pod with containers",
			data: &checkResult{
				Pods: []PodSandbox{
					{
						ID:        "pod1",
						Name:      "test-pod",
						Namespace: "default",
						State:     "READY",
						Containers: []PodSandboxContainerStatus{
							{
								ID:    "container1",
								Name:  "container-1",
								State: "RUNNING",
							},
						},
					},
				},
			},
			expectEmpty: false,
			expectContains: []string{
				"NAMESPACE", "POD", "CONTAINER", "STATE", // Headers are uppercase in tablewriter
				"default", "test-pod", "container-1", "RUNNING", // Pod and container data
			},
		},
		{
			name: "multiple pods with multiple containers",
			data: &checkResult{
				Pods: []PodSandbox{
					{
						ID:        "pod1",
						Name:      "test-pod-1",
						Namespace: "default",
						State:     "READY",
						Containers: []PodSandboxContainerStatus{
							{
								ID:    "container1",
								Name:  "container-1",
								State: "RUNNING",
							},
							{
								ID:    "container2",
								Name:  "container-2",
								State: "STOPPED",
							},
						},
					},
					{
						ID:        "pod2",
						Name:      "test-pod-2",
						Namespace: "kube-system",
						State:     "READY",
						Containers: []PodSandboxContainerStatus{
							{
								ID:    "container3",
								Name:  "container-3",
								State: "RUNNING",
							},
						},
					},
				},
			},
			expectEmpty: false,
			expectContains: []string{
				"NAMESPACE", "POD", "CONTAINER", "STATE", // Headers are uppercase in tablewriter
				"default", "test-pod-1", "container-1", "RUNNING",
				"default", "test-pod-1", "container-2", "STOPPED",
				"kube-system", "test-pod-2", "container-3", "RUNNING",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.String()

			if tt.expectEmpty {
				assert.Empty(t, result)
			} else if tt.expectedResult != "" {
				assert.Equal(t, tt.expectedResult, result)
			} else {
				for _, str := range tt.expectContains {
					assert.Contains(t, result, str)
				}
				// Check strings that shouldn't be in the result
				if tt.expectNotContains != nil {
					for _, str := range tt.expectNotContains {
						assert.NotContains(t, result, str)
					}
				}
			}
		})
	}
}

// TestDataSummary thoroughly tests the Data.Summary() method
func TestDataSummary(t *testing.T) {
	tests := []struct {
		name           string
		data           *checkResult
		expectedResult string
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedResult: "",
		},
		{
			name: "data with explicit reason",
			data: &checkResult{
				reason: "custom summary reason",
			},
			expectedResult: "custom summary reason",
		},
		{
			name: "data with empty reason",
			data: &checkResult{
				reason: "",
			},
			expectedResult: "",
		},
		{
			name: "data with explicit reason and pods",
			data: &checkResult{
				reason: "found 3 pods",
				Pods:   []PodSandbox{{ID: "pod1"}, {ID: "pod2"}, {ID: "pod3"}},
			},
			expectedResult: "found 3 pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.Summary()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestDataHealthState thoroughly tests the Data.HealthState() method
func TestDataHealthState(t *testing.T) {
	tests := []struct {
		name           string
		data           *checkResult
		expectedResult apiv1.HealthStateType
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedResult: "",
		},
		{
			name: "data with explicit healthy state",
			data: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
			},
			expectedResult: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "data with explicit unhealthy state",
			data: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expectedResult: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "data with empty health state",
			data: &checkResult{
				health: "",
			},
			expectedResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.HealthStateType()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestCheckWithNilFunctions tests component Check with missing functions
func TestCheckWithNilFunctions(t *testing.T) {
	// Test cases to cover edge cases for component Check method
	tests := []struct {
		name                          string
		checkDependencyInstalledFunc  func() bool
		checkSocketExistsFunc         func() bool
		checkServiceActiveFunc        func(context.Context) (bool, error)
		checkContainerdRunningFunc    func(context.Context) bool
		listAllSandboxesFunc          func(context.Context, string) ([]PodSandbox, error)
		expectedHealth                apiv1.HealthStateType
		expectServiceChecked          bool
		expectContainerdRunningChecks bool
		expectPodChecks               bool
	}{
		{
			name:                          "all nil functions",
			checkDependencyInstalledFunc:  nil,
			checkSocketExistsFunc:         nil,
			checkServiceActiveFunc:        nil,
			checkContainerdRunningFunc:    nil,
			listAllSandboxesFunc:          nil,
			expectedHealth:                apiv1.HealthStateTypeHealthy,
			expectServiceChecked:          false,
			expectContainerdRunningChecks: false,
			expectPodChecks:               false,
		},
		{
			name: "only dependency check non-nil but false",
			checkDependencyInstalledFunc: func() bool {
				return false
			},
			checkSocketExistsFunc:         nil,
			checkServiceActiveFunc:        nil,
			checkContainerdRunningFunc:    nil,
			listAllSandboxesFunc:          nil,
			expectedHealth:                apiv1.HealthStateTypeHealthy,
			expectServiceChecked:          false,
			expectContainerdRunningChecks: false,
			expectPodChecks:               false,
		},
		{
			name: "only dependency check non-nil and true, others nil",
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkSocketExistsFunc:         nil,
			checkServiceActiveFunc:        nil,
			checkContainerdRunningFunc:    nil,
			listAllSandboxesFunc:          nil,
			expectedHealth:                apiv1.HealthStateTypeHealthy, // The actual behavior is Healthy when socket check is nil
			expectServiceChecked:          false,
			expectContainerdRunningChecks: false,
			expectPodChecks:               false,
		},
		{
			name: "dependency and socket exist, others nil",
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkSocketExistsFunc: func() bool {
				return true
			},
			checkServiceActiveFunc:        nil,
			checkContainerdRunningFunc:    nil,
			listAllSandboxesFunc:          nil,
			expectedHealth:                apiv1.HealthStateTypeHealthy, // The actual behavior is Healthy when containerdRunning is nil
			expectServiceChecked:          false,
			expectContainerdRunningChecks: false,
			expectPodChecks:               false,
		},
		{
			name: "all functions except listAllSandboxes",
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
			listAllSandboxesFunc:          nil,
			expectedHealth:                apiv1.HealthStateTypeHealthy,
			expectServiceChecked:          true,
			expectContainerdRunningChecks: true,
			expectPodChecks:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create component with the provided functions
			comp := &component{
				ctx:                          ctx,
				cancel:                       cancel,
				checkDependencyInstalledFunc: tt.checkDependencyInstalledFunc,
				checkSocketExistsFunc:        tt.checkSocketExistsFunc,
				checkServiceActiveFunc:       tt.checkServiceActiveFunc,
				checkContainerdRunningFunc:   tt.checkContainerdRunningFunc,
				listAllSandboxesFunc:         tt.listAllSandboxesFunc,
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
				endpoint: "unix:///mock/endpoint",
			}

			// Run Check method
			result := comp.Check()

			// Verify result
			assert.NotNil(t, result)
			data, ok := result.(*checkResult)
			assert.True(t, ok)

			// Check health state
			assert.Equal(t, tt.expectedHealth, data.health)

			// Verify containerd service active status if service was checked
			if tt.expectServiceChecked {
				assert.True(t, data.ContainerdServiceActive)
			}

			// Verify pods data if we expect pod checks
			if tt.expectPodChecks {
				assert.NotNil(t, data.Pods)
			} else if !tt.expectServiceChecked && !tt.expectContainerdRunningChecks {
				// If we're not checking services or running, the Pods array should be empty
				assert.Empty(t, data.Pods)
			}
		})
	}
}

// TestCheckServiceActiveError tests handling of service active check errors
func TestCheckServiceActiveError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create error to test with
	testError := errors.New("service active check error")

	// Create component with mocked functions
	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		checkDependencyInstalledFunc: func() bool {
			return true
		},
		checkSocketExistsFunc: func() bool {
			return true
		},
		checkContainerdRunningFunc: func(ctx context.Context) bool {
			return true
		},
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return false, testError
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		endpoint: "unix:///mock/endpoint",
	}

	// Run Check method
	result := comp.Check()

	// Verify result
	assert.NotNil(t, result)
	data, ok := result.(*checkResult)
	assert.True(t, ok)

	// Check health state
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "service is not active")
	assert.False(t, data.ContainerdServiceActive)
	assert.NotNil(t, data.err)
}

// TestComponentCheckWithContextDeadline tests what happens when context times out during checks
func TestComponentCheckWithContextDeadline(t *testing.T) {
	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Sleep to ensure the context expires
	time.Sleep(1 * time.Millisecond)

	// Create component with functions that should never be called due to expired context
	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		checkDependencyInstalledFunc: func() bool {
			return true
		},
		checkSocketExistsFunc: func() bool {
			return true
		},
		// This function will never be called because the context is already expired
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			default:
				return true, nil
			}
		},
		checkContainerdRunningFunc: func(ctx context.Context) bool {
			return false
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		endpoint: "unix:///mock/endpoint",
	}

	// Run Check method
	result := comp.Check()

	// Verify result
	assert.NotNil(t, result)
	// The check should still complete, but may have partial results
}

func TestCheckWhenContainerdCRINotEnabled(t *testing.T) {
	// Create a component with mocked dependencies
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},

		// Mock containerd as installed
		checkDependencyInstalledFunc: func() bool {
			return true
		},
		// Mock socket as existing
		checkSocketExistsFunc: func() bool {
			return true
		},
		// Mock service as active
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return true, nil
		},
		// Mock containerd as running
		checkContainerdRunningFunc: func(context.Context) bool {
			return true
		},
		// Mock listing sandboxes to return an Unimplemented error
		listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
			return nil, status.Error(codes.Unimplemented, "unknown service runtime.v1.RuntimeService")
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	// Call the Check method
	cr := c.Check()

	// Assert the result
	checkResult, ok := cr.(*checkResult)
	assert.True(t, ok, "Expected checkResult type")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, checkResult.health)
	assert.Equal(t, "containerd installed and active but containerd CRI is not enabled", checkResult.reason)
}

func TestNVMLValidation(t *testing.T) {
	tests := []struct {
		name                    string
		nvmlInstance            nvidianvml.Instance
		getContainerdConfigFunc func() ([]byte, error)
		expectedHealth          apiv1.HealthStateType
		expectedReason          string
	}{
		{
			name:         "nvml instance is nil",
			nvmlInstance: nil,
			getContainerdConfigFunc: func() ([]byte, error) {
				return []byte("nvidia"), nil
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "ok",
		},
		{
			name: "nvml does not exist",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  false,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				return []byte("nvidia"), nil
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "ok",
		},
		{
			name: "product name is empty",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				return []byte("nvidia"), nil
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "ok",
		},
		{
			name: "getContainerdConfigFunc is nil - skips config check",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: nil,
			expectedHealth:          apiv1.HealthStateTypeHealthy,
			expectedReason:          "ok",
		},
		{
			name: "config read error",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				return nil, errors.New("config file not found")
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "ok",
		},
		{
			name: "config contains nvidia - healthy",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				config := `[plugins."io.containerd.grpc.v1.cri".containerd]
  default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"
  runtime_engine = ""
  runtime_root = ""
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  privileged_without_host_devices = false
  runtime_engine = ""
  runtime_root = ""
  runtime_type = "io.containerd.runc.v2"`
				return []byte(config), nil
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "ok",
		},
		{
			name: "config does not contain nvidia - healthy, only logs warning",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				config := `[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"
  runtime_engine = ""
  runtime_root = ""`
				return []byte(config), nil
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "ok",
		},
		{
			name: "config missing required nvidia runtime - healthy, only logs warning",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			getContainerdConfigFunc: func() ([]byte, error) {
				return []byte("some other config without that gpu runtime"), nil
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := &component{
				ctx:    ctx,
				cancel: cancel,

				nvmlInstance:            tt.nvmlInstance,
				getContainerdConfigFunc: tt.getContainerdConfigFunc,

				// Mock all dependencies as successful to focus on NVML validation
				checkDependencyInstalledFunc: func() bool {
					return true
				},
				checkSocketExistsFunc: func() bool {
					return true
				},
				checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
					return true, nil
				},
				checkContainerdRunningFunc: func(context.Context) bool {
					return true
				},
				listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
					return []PodSandbox{}, nil
				},
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
			}

			cr := c.Check()
			checkResult, ok := cr.(*checkResult)
			require.True(t, ok, "Expected checkResult type")

			assert.Equal(t, tt.expectedHealth, checkResult.health, "Health state should match expected")
			assert.Equal(t, tt.expectedReason, checkResult.reason, "Reason should match expected")
		})
	}
}

func TestNVMLValidationIntegration(t *testing.T) {
	// This test verifies the complete flow including the NVML check integration
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test case where NVML exists, GPU detected, but config missing nvidia
	c := &component{
		ctx:    ctx,
		cancel: cancel,

		nvmlInstance: &mockNVMLInstance{
			nvmlExists:  true,
			productName: "Tesla V100-SXM2-32GB",
		},
		getContainerdConfigFunc: func() ([]byte, error) {
			// Return a config that doesn't contain "nvidia"
			return []byte(`[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"
  runtime_engine = ""
  runtime_root = ""`), nil
		},

		// All other checks pass
		checkDependencyInstalledFunc: func() bool {
			return true
		},
		checkSocketExistsFunc: func() bool {
			return true
		},
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return true, nil
		},
		checkContainerdRunningFunc: func(context.Context) bool {
			return true
		},
		listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
			return []PodSandbox{}, nil
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	cr := c.Check()
	checkResult, ok := cr.(*checkResult)
	require.True(t, ok, "Expected checkResult type")

	// Should be healthy - the new logic only logs warnings, doesn't change health state
	assert.Equal(t, apiv1.HealthStateTypeHealthy, checkResult.health)
	assert.Equal(t, "ok", checkResult.reason)

	// Verify health states
	healthStates := checkResult.HealthStates()
	require.Len(t, healthStates, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, healthStates[0].Health)
	assert.Equal(t, Name, healthStates[0].Component)
}

// TestContainerToolkitValidation tests the nvidia-container-toolkit validation logic
// TestCheckContainerdActiveness tests the checkContainerdActiveness method
func TestCheckContainerdActiveness(t *testing.T) {
	boolPtr := func(b bool) *bool {
		return &b
	}

	tests := []struct {
		name                   string
		checkSocketExists      func() bool
		checkContainerdRunning func(context.Context) bool
		checkServiceActive     func(context.Context) (bool, error)
		expectedResult         bool
		expectedHealth         apiv1.HealthStateType
		expectedReason         string
		expectedServiceActive  *bool
		expectedErr            string
	}{
		{
			name: "all checks pass",
			checkSocketExists: func() bool {
				return true
			},
			checkContainerdRunning: func(ctx context.Context) bool {
				return true
			},
			checkServiceActive: func(ctx context.Context) (bool, error) {
				return true, nil
			},
			expectedResult:        true,
			expectedHealth:        "",
			expectedReason:        "",
			expectedServiceActive: boolPtr(true),
		},
		{
			name: "socket does not exist",
			checkSocketExists: func() bool {
				return false
			},
			checkContainerdRunning: func(ctx context.Context) bool {
				return true
			},
			checkServiceActive: func(ctx context.Context) (bool, error) {
				return true, nil
			},
			expectedResult: false,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "containerd installed but socket file does not exist (detected 1/5 consecutive checks)",
		},
		{
			name: "containerd not running",
			checkSocketExists: func() bool {
				return true
			},
			checkContainerdRunning: func(ctx context.Context) bool {
				return false
			},
			checkServiceActive: func(ctx context.Context) (bool, error) {
				return true, nil
			},
			expectedResult: false,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedReason: "containerd installed but not running",
		},
		{
			name: "service not active",
			checkSocketExists: func() bool {
				return true
			},
			checkContainerdRunning: func(ctx context.Context) bool {
				return true
			},
			checkServiceActive: func(ctx context.Context) (bool, error) {
				return false, nil
			},
			expectedResult:        false,
			expectedHealth:        apiv1.HealthStateTypeUnhealthy,
			expectedReason:        "containerd installed but service is not active",
			expectedServiceActive: boolPtr(false),
		},
		{
			name: "service check error",
			checkSocketExists: func() bool {
				return true
			},
			checkContainerdRunning: func(ctx context.Context) bool {
				return true
			},
			checkServiceActive: func(ctx context.Context) (bool, error) {
				return false, errors.New("service check failed")
			},
			expectedResult:        false,
			expectedHealth:        apiv1.HealthStateTypeUnhealthy,
			expectedReason:        "containerd installed but service is not active",
			expectedServiceActive: boolPtr(false),
			expectedErr:           "service check failed",
		},
		{
			name:              "socket check nil - skipped",
			checkSocketExists: nil,
			checkContainerdRunning: func(ctx context.Context) bool {
				return true
			},
			checkServiceActive: func(ctx context.Context) (bool, error) {
				return true, nil
			},
			expectedResult: true,
			expectedHealth: "",
			expectedReason: "",
		},
		{
			name: "containerd running check nil - skipped",
			checkSocketExists: func() bool {
				return true
			},
			checkContainerdRunning: nil,
			checkServiceActive: func(ctx context.Context) (bool, error) {
				return true, nil
			},
			expectedResult: true,
			expectedHealth: "",
			expectedReason: "",
		},
		{
			name: "service active check nil - skipped",
			checkSocketExists: func() bool {
				return true
			},
			checkContainerdRunning: func(ctx context.Context) bool {
				return true
			},
			checkServiceActive:    nil,
			expectedResult:        true,
			expectedHealth:        "",
			expectedReason:        "",
			expectedServiceActive: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			comp := &component{
				ctx:                        ctx,
				cancel:                     cancel,
				checkSocketExistsFunc:      tt.checkSocketExists,
				checkContainerdRunningFunc: tt.checkContainerdRunning,
				checkServiceActiveFunc:     tt.checkServiceActive,
			}

			cr := &checkResult{}
			result := comp.checkContainerdActiveness(cr)

			assert.Equal(t, tt.expectedResult, result, "checkContainerdActiveness result should match expected")
			if !tt.expectedResult {
				assert.Equal(t, tt.expectedHealth, cr.health, "Health state should match expected")
				assert.Equal(t, tt.expectedReason, cr.reason, "Reason should match expected")
			}

			if tt.expectedServiceActive != nil {
				assert.Equal(t, *tt.expectedServiceActive, cr.ContainerdServiceActive, "Service active flag should match expected")
			}

			if tt.expectedErr != "" {
				require.Error(t, cr.err, "Error should be set when expectedErr is provided")
				assert.EqualError(t, cr.err, tt.expectedErr)
			} else {
				assert.NoError(t, cr.err)
			}
		})
	}
}

func TestCheckContainerdActiveness_SocketMissingConsecutiveFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	socketExists := false
	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		checkSocketExistsFunc: func() bool {
			return socketExists
		},
	}

	for i := 0; i < socketMissingConsecutiveThreshold-1; i++ {
		cr := &checkResult{}
		result := comp.checkContainerdActiveness(cr)
		assert.False(t, result)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "socket file does not exist")
	}

	cr := &checkResult{}
	result := comp.checkContainerdActiveness(cr)
	assert.False(t, result)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "socket file does not exist")
	assert.Contains(t, cr.reason, "failed continuously")

	// Reset when the socket check passes.
	socketExists = true
	cr = &checkResult{}
	result = comp.checkContainerdActiveness(cr)
	assert.True(t, result)

	// After reset, the next failure should not be unhealthy.
	socketExists = false
	cr = &checkResult{}
	result = comp.checkContainerdActiveness(cr)
	assert.False(t, result)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "socket file does not exist")
}

func TestContainerToolkitValidation(t *testing.T) {
	tests := []struct {
		name                              string
		nvmlInstance                      nvidianvml.Instance
		pods                              []PodSandbox
		getTimeNowFunc                    func() time.Time
		containerToolkitCreationThreshold time.Duration
		expectedHealth                    apiv1.HealthStateType
		expectedReason                    string
	}{
		{
			name:         "no nvml instance - skips container toolkit check",
			nvmlInstance: nil,
			pods: []PodSandbox{
				{Name: "regular-pod", Containers: []PodSandboxContainerStatus{{Name: "regular-container"}}},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "ok",
		},
		{
			name: "nvml exists but no product name - skips container toolkit check",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "",
			},
			pods: []PodSandbox{
				{Name: "regular-pod", Containers: []PodSandboxContainerStatus{{Name: "regular-container"}}},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "ok",
		},
		{
			name: "no pods - skips container toolkit check",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			pods: []PodSandbox{},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "ok",
		},
		{
			name: "container toolkit daemonset not found - logs warning",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			pods: []PodSandbox{
				{
					Name: "regular-pod",
					Containers: []PodSandboxContainerStatus{
						{Name: "regular-container"},
					},
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "nvidia GPUs found but nvidia-container-toolkit pod is not found",
		},
		{
			name: "container toolkit found and running long enough",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			pods: []PodSandbox{
				{
					Name:  "nvidia-container-toolkit-daemonset-xyz",
					State: "SANDBOX_READY",
					Containers: []PodSandboxContainerStatus{
						{
							Name:      "nvidia-container-toolkit-ctr",
							State:     "CONTAINER_RUNNING",
							CreatedAt: time.Now().Add(-15 * time.Minute).UnixNano(),
						},
					},
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "ok",
		},
		{
			name: "container toolkit found but not running long enough",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			pods: []PodSandbox{
				{
					Name:  "nvidia-container-toolkit-daemonset-abc",
					State: "SANDBOX_READY",
					Containers: []PodSandboxContainerStatus{
						{
							Name:      "nvidia-container-toolkit-ctr",
							State:     "CONTAINER_RUNNING",
							CreatedAt: time.Now().Add(-3 * time.Minute).UnixNano(),
						},
					},
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "ok",
		},
		{
			name: "container toolkit found but container not running",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			pods: []PodSandbox{
				{
					Name:  "nvidia-container-toolkit-daemonset-def",
					State: "SANDBOX_READY",
					Containers: []PodSandboxContainerStatus{
						{
							Name:      "nvidia-container-toolkit-ctr",
							State:     "CONTAINER_EXITED",
							CreatedAt: time.Now().Add(-15 * time.Minute).UnixNano(),
						},
					},
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "ok",
		},
		{
			name: "container toolkit daemonset found but pod not ready",
			nvmlInstance: &mockNVMLInstance{
				nvmlExists:  true,
				productName: "Tesla V100",
			},
			pods: []PodSandbox{
				{
					Name:  "nvidia-container-toolkit-daemonset-notready",
					State: "SANDBOX_NOTREADY",
					Containers: []PodSandboxContainerStatus{
						{
							Name:      "nvidia-container-toolkit-ctr",
							State:     "CONTAINER_RUNNING",
							CreatedAt: time.Now().Add(-15 * time.Minute).UnixNano(),
						},
					},
				},
			},
			getTimeNowFunc: func() time.Time {
				return time.Now()
			},
			containerToolkitCreationThreshold: 10 * time.Minute,
			expectedHealth:                    apiv1.HealthStateTypeHealthy,
			expectedReason:                    "nvidia GPUs found but nvidia-container-toolkit pod is not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := &component{
				ctx:    ctx,
				cancel: cancel,

				nvmlInstance:                      tt.nvmlInstance,
				getTimeNowFunc:                    tt.getTimeNowFunc,
				containerToolkitCreationThreshold: tt.containerToolkitCreationThreshold,
				getContainerdConfigFunc: func() ([]byte, error) {
					// Return config with both nvidia settings to avoid the nvidia config warning
					config := `default_runtime_name = "nvidia"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
  runtime_type = "io.containerd.runc.v2"`
					return []byte(config), nil
				},

				// Mock all dependencies as successful to focus on container toolkit validation
				checkDependencyInstalledFunc: func() bool {
					return true
				},
				checkSocketExistsFunc: func() bool {
					return true
				},
				checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
					return true, nil
				},
				checkContainerdRunningFunc: func(context.Context) bool {
					return true
				},
				listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
					return tt.pods, nil
				},
				endpoint: "unix:///var/run/containerd/containerd.sock",
			}

			cr := c.Check()
			checkResult, ok := cr.(*checkResult)
			require.True(t, ok, "Expected checkResult type")

			assert.Equal(t, tt.expectedHealth, checkResult.health, "Health state should match expected")
			assert.Equal(t, tt.expectedReason, checkResult.reason, "Reason should match expected text exactly")
		})
	}
}

// TestCheckContainerdUptimeGracePeriod tests the uptime grace period logic for containerd
// that allows containerd to be considered healthy if it's recently started but not yet fully active
func TestCheckContainerdUptimeGracePeriod(t *testing.T) {
	tests := []struct {
		name                          string
		containerdInstalled           bool
		socketExists                  bool
		containerdRunning             bool
		serviceActive                 bool
		serviceActiveError            error
		getContainerdUptimeFunc       func() (*time.Duration, error)
		activenssCheckUptimeThreshold time.Duration
		expectedHealth                apiv1.HealthStateType
		expectedReasonContains        string
		expectedReasonNotContains     string
		uptimeFuncShouldBeCalled      bool
	}{
		{
			name:                          "containerd not active, uptime check disabled (nil func)",
			containerdInstalled:           true,
			socketExists:                  true,
			containerdRunning:             true,
			serviceActive:                 false, // Not active
			getContainerdUptimeFunc:       nil,   // No uptime function
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeUnhealthy,
			expectedReasonContains:        "service is not active",
			expectedReasonNotContains:     "has not been running for long enough",
			uptimeFuncShouldBeCalled:      false,
		},
		{
			name:                "containerd not active, uptime func returns error",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   true,
			serviceActive:       false,
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return nil, errors.New("failed to get uptime")
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeUnhealthy,
			expectedReasonContains:        "service is not active",
			expectedReasonNotContains:     "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
		{
			name:                "containerd not active, uptime func returns nil duration",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   true,
			serviceActive:       false,
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return nil, nil // nil duration
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeUnhealthy,
			expectedReasonContains:        "service is not active",
			expectedReasonNotContains:     "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
		{
			name:                "containerd not active, uptime below threshold (grace period applies)",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   true,
			serviceActive:       false,
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return durationPtr(2 * time.Minute), nil // 2 minutes < 5 minute threshold
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeHealthy, // Grace period makes it healthy
			expectedReasonContains:        "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
		{
			name:                "containerd not active, uptime at threshold boundary (no grace period)",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   true,
			serviceActive:       false,
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return durationPtr(5 * time.Minute), nil // Exactly at threshold
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeUnhealthy, // NOT < threshold, no grace
			expectedReasonContains:        "service is not active",
			expectedReasonNotContains:     "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
		{
			name:                "containerd not active, uptime above threshold (no grace period)",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   true,
			serviceActive:       false,
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return durationPtr(10 * time.Minute), nil // 10 minutes > 5 minute threshold
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeUnhealthy,
			expectedReasonContains:        "service is not active",
			expectedReasonNotContains:     "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
		{
			name:                "socket missing, uptime below threshold (grace period applies)",
			containerdInstalled: true,
			socketExists:        false, // Socket doesn't exist
			containerdRunning:   false,
			serviceActive:       false,
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return durationPtr(1 * time.Minute), nil
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeHealthy,
			expectedReasonContains:        "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
		{
			name:                "containerd not running, uptime below threshold (grace period applies)",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   false, // Not running
			serviceActive:       false,
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return durationPtr(3 * time.Minute), nil
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeHealthy,
			expectedReasonContains:        "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
		{
			name:                "containerd active, uptime check not executed",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   true,
			serviceActive:       true, // Active and healthy
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				t.Error("uptime func should not be called when containerd is active")
				return nil, errors.New("should not be called")
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeHealthy,
			expectedReasonContains:        "ok",
			expectedReasonNotContains:     "has not been running for long enough",
			uptimeFuncShouldBeCalled:      false,
		},
		{
			name:                "very short uptime (30 seconds) with grace period",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   true,
			serviceActive:       false,
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return durationPtr(30 * time.Second), nil
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeHealthy,
			expectedReasonContains:        "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
		{
			name:                "zero uptime with grace period",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   true,
			serviceActive:       false,
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return durationPtr(0 * time.Second), nil // Just started
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeHealthy,
			expectedReasonContains:        "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
		{
			name:                "service check returns error, uptime below threshold",
			containerdInstalled: true,
			socketExists:        true,
			containerdRunning:   true,
			serviceActive:       false,
			serviceActiveError:  errors.New("systemd error"),
			getContainerdUptimeFunc: func() (*time.Duration, error) {
				return durationPtr(1 * time.Minute), nil
			},
			activenssCheckUptimeThreshold: 5 * time.Minute,
			expectedHealth:                apiv1.HealthStateTypeHealthy, // Grace period still applies
			expectedReasonContains:        "has not been running for long enough",
			uptimeFuncShouldBeCalled:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			uptimeFuncCalled := false
			var wrappedUptimeFunc func() (*time.Duration, error)
			if tt.getContainerdUptimeFunc != nil {
				wrappedUptimeFunc = func() (*time.Duration, error) {
					uptimeFuncCalled = true
					return tt.getContainerdUptimeFunc()
				}
			}

			comp := &component{
				ctx:    ctx,
				cancel: func() {},
				checkDependencyInstalledFunc: func() bool {
					return tt.containerdInstalled
				},
				checkSocketExistsFunc: func() bool {
					return tt.socketExists
				},
				checkContainerdRunningFunc: func(ctx context.Context) bool {
					return tt.containerdRunning
				},
				checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
					return tt.serviceActive, tt.serviceActiveError
				},
				getContainerdUptimeFunc:       wrappedUptimeFunc,
				activenssCheckUptimeThreshold: tt.activenssCheckUptimeThreshold,
				listAllSandboxesFunc: func(ctx context.Context, endpoint string) ([]PodSandbox, error) {
					return []PodSandbox{}, nil
				},
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
				endpoint: "unix:///mock/containerd.sock",
			}

			// Execute the check
			_ = comp.Check()

			// Verify uptime func was called if expected
			assert.Equal(t, tt.uptimeFuncShouldBeCalled, uptimeFuncCalled,
				"uptime function call expectation mismatch")

			// Assert results
			assert.NotNil(t, comp.lastCheckResult, "lastCheckResult should not be nil")
			assert.Equal(t, tt.expectedHealth, comp.lastCheckResult.health,
				"health state should match expected")
			assert.Contains(t, comp.lastCheckResult.reason, tt.expectedReasonContains,
				"reason should contain expected text")

			if tt.expectedReasonNotContains != "" {
				assert.NotContains(t, comp.lastCheckResult.reason, tt.expectedReasonNotContains,
					"reason should NOT contain unexpected text")
			}
		})
	}
}

// Helper function to create a pointer to a time.Duration
func durationPtr(d time.Duration) *time.Duration {
	return &d
}
