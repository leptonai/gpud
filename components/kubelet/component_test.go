package kubelet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
)

// MockGPUdInstance creates a minimal GPUdInstance for testing
func createMockGPUdInstance(ctx context.Context) *components.GPUdInstance {
	return &components.GPUdInstance{
		RootCtx: ctx,
	}
}

func Test_marshalJSON(t *testing.T) {
	testCases := []struct {
		name     string
		data     checkResult
		expected string
	}{
		{
			name:     "empty data",
			data:     checkResult{},
			expected: `{"kubelet_service_active":false}`,
		},
		{
			name: "with node name",
			data: checkResult{
				KubeletServiceActive: true,
				NodeName:             "test-node",
			},
			expected: `{"kubelet_service_active":true,"node_name":"test-node"}`,
		},
		{
			name: "with pods",
			data: checkResult{
				KubeletServiceActive: true,
				NodeName:             "test-node",
				Pods: []PodStatus{
					{
						Name:      "test-pod",
						Namespace: "default",
						Phase:     "Running",
					},
				},
			},
			expected: `{"kubelet_service_active":true,"node_name":"test-node","pods":[{"name":"test-pod","namespace":"default","phase":"Running"}]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tc.data)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(jsonData))
		})
	}
}

func Test_componentName(t *testing.T) {
	c := &component{}
	assert.Equal(t, "kubelet", c.Name())
}

func TestTags(t *testing.T) {
	c := &component{}

	expectedTags := []string{
		"container",
		"kubelet",
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 2, "Component should return exactly 2 tags")
}

func Test_getLastHealthStates(t *testing.T) {
	testCases := []struct {
		name           string
		data           checkResult
		expectedLen    int
		expectedHealth apiv1.HealthStateType
	}{
		{
			name: "no pods",
			data: checkResult{
				health: apiv1.HealthStateTypeHealthy,
				reason: "test reason",
			},
			expectedLen:    1,
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "with pods",
			data: checkResult{
				Pods: []PodStatus{
					{Name: "pod1"},
					{Name: "pod2"},
				},
				health: apiv1.HealthStateTypeHealthy,
				reason: "test reason",
			},
			expectedLen:    1,
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "with error - unhealthy",
			data: checkResult{
				err:    errors.New("some error"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "test reason",
			},
			expectedLen:    1,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "with connection error - healthy",
			data: checkResult{
				err:    errors.New("connection refused"),
				health: apiv1.HealthStateTypeHealthy,
				reason: "test reason",
			},
			expectedLen:    1,
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "with failed count above threshold - unhealthy",
			data: checkResult{
				reason: "test reason",
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expectedLen:    1,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			states := tc.data.HealthStates()
			require.Len(t, states, tc.expectedLen)
			assert.Equal(t, tc.expectedHealth, states[0].Health)

			if len(tc.data.Pods) > 0 {
				require.NotNil(t, states[0].ExtraInfo)
				assert.Contains(t, states[0].ExtraInfo, "data")
			}
		})
	}
}

func Test_componentEvents(t *testing.T) {
	c := &component{}
	events, err := c.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func Test_componentStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{ctx: ctx, cancel: cancel}
	err := c.Start()
	assert.NoError(t, err)
}

func Test_componentLastHealthStates(t *testing.T) {
	testCases := []struct {
		name           string
		data           checkResult
		failedCount    int
		expectedHealth apiv1.HealthStateType
	}{
		{
			name: "healthy state",
			data: checkResult{
				health: apiv1.HealthStateTypeHealthy,
				reason: "test reason",
			},
			failedCount:    0,
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy state",
			data: checkResult{
				err:    errors.New("test error"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "test reason",
			},
			failedCount:    0,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "connection error - ignored",
			data: checkResult{
				err:    errors.New("connection refused"),
				health: apiv1.HealthStateTypeHealthy,
				reason: "test reason",
			},
			failedCount:    0,
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "failed count above threshold",
			data: checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "test reason",
			},
			failedCount:    6,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &component{
				lastCheckResult: &tc.data,
			}
			c.failedCount.Store(int32(tc.failedCount))

			states := c.LastHealthStates()
			require.Len(t, states, 1)
			assert.Equal(t, tc.expectedHealth, states[0].Health)
		})
	}
}

func Test_componentClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,
	}

	err := c.Close()
	assert.NoError(t, err)

	// Verify that the context was canceled
	select {
	case <-ctx.Done():
		// This is expected, the context should be canceled
	default:
		t.Error("Expected context to be canceled after Close()")
	}
}

// Test context cancellation for ListPodsFromKubeletReadOnlyPort
func TestListPodsFromKubeletReadOnlyPort_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create a server that hangs/delays to test context cancellation
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep for a while to simulate a slow server
		time.Sleep(500 * time.Millisecond)
		http.ServeFile(w, r, "kubelet-readonly-pods.json")
	}))
	defer srv.Close()

	portStr := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portStr, 10, 32)

	// Create a context that's immediately canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should fail due to canceled context
	nodeName, pods, err := ListPodsFromKubeletReadOnlyPort(ctx, int(port))
	assert.Error(t, err)
	assert.Empty(t, nodeName)
	assert.Nil(t, pods)
	assert.Contains(t, err.Error(), "context canceled")
}

// TestListPodsFromKubeletReadOnlyPort_HTTPError tests handling HTTP error responses
func TestListPodsFromKubeletReadOnlyPort_HTTPError(t *testing.T) {
	t.Parallel()

	// Setup server that returns different HTTP errors
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Server Error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	portStr := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portStr, 10, 32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should fail due to HTTP error
	nodeName, pods, err := ListPodsFromKubeletReadOnlyPort(ctx, int(port))
	assert.Error(t, err)
	assert.Empty(t, nodeName)
	assert.Nil(t, pods)
}

// Test_componentCheck tests the Check method of the component
func Test_componentCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		case "/pods":
			w.Header().Set("Content-Type", "application/json")
			http.ServeFile(w, r, "kubelet-readonly-pods.json")
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	portStr := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portStr, 10, 32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a counter to verify checkKubeletRunning is called
	kubeletRunningCalled := 0

	// Create component with our test port
	c := &component{
		ctx:                   ctx,
		cancel:                cancel,
		checkKubeletInstalled: func() bool { return true },
		checkKubeletRunning:   func() bool { kubeletRunningCalled++; return true },
		kubeletReadOnlyPort:   int(port),
	}

	// Run the check
	data := c.Check()

	// Verify checkKubeletRunning was called
	assert.Equal(t, 1, kubeletRunningCalled, "checkKubeletRunning should be called once")

	// Verify the returned data
	dataResult, ok := data.(*checkResult)
	require.True(t, ok, "data should be of type *checkResult")

	// We can't reliably check KubeletPidFound as it depends on the test environment
	assert.Equal(t, "mynodehostname", dataResult.NodeName)
	assert.Len(t, dataResult.Pods, 2)
}

// Test_componentLastHealthStates_ConnectionErrors tests the LastHealthStates method
func Test_componentLastHealthStates_ConnectionErrors(t *testing.T) {
	testCases := []struct {
		name           string
		data           checkResult
		failedCount    int
		expectedHealth apiv1.HealthStateType
	}{
		{
			name: "connection error - marked healthy",
			data: checkResult{
				err:    errors.New("connection refused"),
				health: apiv1.HealthStateTypeHealthy,
			},
			failedCount:    0,
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "connection error - marked unhealthy",
			data: checkResult{
				err:    errors.New("connection refused"),
				health: apiv1.HealthStateTypeUnhealthy,
			},
			failedCount:    0,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &component{
				lastCheckResult: &tc.data,
			}
			c.failedCount.Store(int32(tc.failedCount))

			states := c.LastHealthStates()
			require.Len(t, states, 1)
			assert.Equal(t, tc.expectedHealth, states[0].Health)
		})
	}
}

// Test that canceled context in LastHealthStates works correctly
func Test_componentLastHealthStates_ContextCancellation(t *testing.T) {
	c := &component{
		lastCheckResult: &checkResult{
			Pods:   []PodStatus{{Name: "test-pod"}},
			health: apiv1.HealthStateTypeHealthy,
		},
	}

	// Create a canceled context - we don't need to use it, just testing component behavior
	_, cancel := context.WithCancel(context.Background())
	cancel()

	// Should still return states using cached data
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

// Add test for component constructor
func Test_componentConstructor(t *testing.T) {
	ctx := context.Background()
	mockInstance := createMockGPUdInstance(ctx)
	comp, err := New(mockInstance)
	require.NoError(t, err)

	// Type assertion
	c, ok := comp.(*component)
	require.True(t, ok, "Component should be of type *component")

	// Check fields
	assert.Equal(t, DefaultKubeletReadOnlyPort, c.kubeletReadOnlyPort)
	assert.NotNil(t, c.ctx)
	assert.NotNil(t, c.cancel)
	assert.Equal(t, defaultFailedCountThreshold, c.failedCountThreshold)
}

func TestDataGetLastHealthStatesNil(t *testing.T) {
	// Test with nil data
	var cr *checkResult
	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataGetLastHealthStatesErrorReturn(t *testing.T) {
	testCases := []struct {
		name           string
		data           checkResult
		expectedHealth apiv1.HealthStateType
	}{
		{
			name: "standard error returned",
			data: checkResult{
				err:    errors.New("standard error"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "standard error",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "empty pods with error",
			data: checkResult{
				Pods:   []PodStatus{},
				err:    errors.New("no pods error"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "no pods error",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "connection error - healthy",
			data: checkResult{
				NodeName: "test-node",
				err:      errors.New("connection refused"),
				health:   apiv1.HealthStateTypeHealthy,
				reason:   "connection refused",
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "connection error - unhealthy",
			data: checkResult{
				NodeName: "test-node",
				err:      errors.New("connection refused"),
				health:   apiv1.HealthStateTypeUnhealthy,
				reason:   "connection refused",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "no error with pods",
			data: checkResult{
				NodeName: "test-node",
				Pods:     []PodStatus{{Name: "pod1"}},
				err:      nil,
				health:   apiv1.HealthStateTypeHealthy,
				reason:   "success",
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "kubelet service error",
			data: checkResult{
				NodeName:             "test-node",
				KubeletServiceActive: false,
				err:                  errors.New("kubelet service not active"),
				health:               apiv1.HealthStateTypeUnhealthy,
				reason:               "kubelet service not active",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "failed count above threshold",
			data: checkResult{
				NodeName: "test-node",
				health:   apiv1.HealthStateTypeUnhealthy,
				reason:   "failed threshold exceeded",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			states := tc.data.HealthStates()

			// Verify state properties
			require.Len(t, states, 1)
			assert.Equal(t, tc.expectedHealth, states[0].Health)
			assert.Equal(t, Name, states[0].Name)

			// Check for extra info if we have pods
			if len(tc.data.Pods) > 0 {
				assert.NotNil(t, states[0].ExtraInfo)
				assert.Contains(t, states[0].ExtraInfo, "data")
			}
		})
	}
}

func TestDataGetLastHealthStatesWithSpecificErrors(t *testing.T) {
	// Test with context deadline exceeded error
	deadlineErr := context.DeadlineExceeded
	deadlineData := checkResult{
		NodeName: "test-node",
		err:      deadlineErr,
		health:   apiv1.HealthStateTypeUnhealthy,
		reason:   "deadline exceeded",
	}
	states := deadlineData.HealthStates()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "deadline exceeded")

	// Test with context canceled error
	canceledErr := context.Canceled
	canceledData := checkResult{
		NodeName: "test-node",
		err:      canceledErr,
		health:   apiv1.HealthStateTypeUnhealthy,
		reason:   "context canceled",
	}
	states = canceledData.HealthStates()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "context canceled")

	// Test with formatted error message
	customErr := fmt.Errorf("custom error: %v", "details")
	customData := checkResult{
		NodeName: "test-node",
		err:      customErr,
		health:   apiv1.HealthStateTypeUnhealthy,
		reason:   "custom error",
	}
	states = customData.HealthStates()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "custom error")
}

// Test_componentCheck_KubeletNotRunning tests the Check method of the component when kubelet is not running
func Test_componentCheck_KubeletNotRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create component with checkKubeletRunning that returns false
	c := &component{
		ctx:                   ctx,
		cancel:                cancel,
		checkKubeletInstalled: func() bool { return true },
		checkKubeletRunning:   func() bool { return false }, // Kubelet not running
		kubeletReadOnlyPort:   10255,
	}

	// Run the check
	result := c.Check()
	dataResult, ok := result.(*checkResult)
	require.True(t, ok, "result should be of type *checkResult")

	// Should return early so empty pods and no errors, but healthy status
	assert.Empty(t, dataResult.NodeName)
	assert.Empty(t, dataResult.Pods)
	assert.Nil(t, dataResult.err)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, dataResult.health)
}

// Test behavior with different checkKubeletInstalled and checkKubeletRunning combinations
func Test_componentCheck_Dependencies(t *testing.T) {
	testCases := []struct {
		name                    string
		dependencyInstalled     bool
		kubeletRunning          bool
		expectDataToBeCollected bool
	}{
		{
			name:                    "dependency installed and kubelet running",
			dependencyInstalled:     true,
			kubeletRunning:          true,
			expectDataToBeCollected: true,
		},
		{
			name:                    "dependency installed but kubelet not running",
			dependencyInstalled:     true,
			kubeletRunning:          false,
			expectDataToBeCollected: false,
		},
		{
			name:                    "dependency not installed",
			dependencyInstalled:     false,
			kubeletRunning:          true, // Doesn't matter since dependency check comes first
			expectDataToBeCollected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server if needed for this test case
			var srv *httptest.Server
			var port int

			if tc.expectDataToBeCollected {
				// Only create server if we expect to actually query it
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					http.ServeFile(w, r, "kubelet-readonly-pods.json")
				}))
				defer srv.Close()

				portStr := srv.URL[len("http://127.0.0.1:"):]
				portVal, _ := strconv.ParseInt(portStr, 10, 32)
				port = int(portVal)
			} else {
				// Use a dummy port for tests that won't reach the HTTP call
				port = 10255
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create component with the test conditions
			c := &component{
				ctx:                   ctx,
				cancel:                cancel,
				checkKubeletInstalled: func() bool { return tc.dependencyInstalled },
				checkKubeletRunning:   func() bool { return tc.kubeletRunning },
				kubeletReadOnlyPort:   port,
			}

			// Run the check
			result := c.Check()
			dataResult, ok := result.(*checkResult)
			require.True(t, ok, "result should be of type *checkResult")

			if !tc.expectDataToBeCollected {
				// Should return early with default data
				assert.Empty(t, dataResult.NodeName)
				assert.Empty(t, dataResult.Pods)
				assert.Nil(t, dataResult.err)
				assert.Equal(t, apiv1.HealthStateTypeHealthy, dataResult.health)
			} else {
				// Should attempt to collect data
				assert.NotEmpty(t, dataResult.NodeName)
				assert.NotEmpty(t, dataResult.Pods)
			}
		})
	}
}

// Test that the component constructor sets checkKubeletRunning correctly
func Test_componentConstructor_CheckKubeletRunning(t *testing.T) {
	ctx := context.Background()
	mockInstance := createMockGPUdInstance(ctx)
	comp, err := New(mockInstance)
	require.NoError(t, err)

	// Type assertion
	c, ok := comp.(*component)
	require.True(t, ok, "Component should be of type *component")

	// Check fields
	assert.Equal(t, DefaultKubeletReadOnlyPort, c.kubeletReadOnlyPort)

	// The checkKubeletRunning function should be set
	assert.NotNil(t, c.checkKubeletRunning)

	// We can't directly test the function's logic since it depends on the network,
	// but we can verify it's there and doesn't panic when called
	assert.NotPanics(t, func() {
		_ = c.checkKubeletRunning()
	})
}
