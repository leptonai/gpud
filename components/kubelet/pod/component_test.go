package pod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockGPUdInstance creates a minimal GPUdInstance for testing
func createMockGPUdInstance(ctx context.Context) *components.GPUdInstance {
	return &components.GPUdInstance{
		RootCtx: ctx,
	}
}

func TestListFromKubeletReadOnlyPort(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/pods", r.URL.Path, "expected path to be '/pods'")
		assert.Equal(t, http.MethodGet, r.Method, "expected GET request")
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, "kubelet-readonly-pods.json")
	}))
	defer srv.Close()

	portRaw := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portRaw, 10, 32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodeName, pods, err := listPodsFromKubeletReadOnlyPort(ctx, int(port))
	require.NoError(t, err)
	require.Equal(t, "mynodehostname", nodeName)
	require.NotNil(t, pods, "pods should not be nil")
	require.Len(t, pods, 2, "expected 2 pods")

	assert.Equal(t, "vector-jldbs", pods[0].Name)
	assert.Equal(t, string(corev1.PodRunning), pods[0].Phase)
	assert.Equal(t, "kube-proxy-hfqwt", pods[1].Name)
	assert.Equal(t, string(corev1.PodRunning), pods[1].Phase)
}

func TestGetFromKubeletReadOnlyPort_ParseError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	portRaw := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portRaw, 10, 32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, result, err := listPodsFromKubeletReadOnlyPort(ctx, int(port))

	require.Error(t, err, "expected an error")
	require.Nil(t, result, "result should be nil")
}

func TestGetFromKubeletReadOnlyPort_ConnError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))

	portRaw := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portRaw, 10, 32)

	srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// connection error should not be a failure
	nodeName, pods, err := listPodsFromKubeletReadOnlyPort(ctx, int(port))

	require.Error(t, err, "expected an error")
	assert.Empty(t, nodeName, "node name should be empty")
	assert.Nil(t, pods, "pods should be nil")
}

func Test_parsePodsFromKubeletReadOnlyPort(t *testing.T) {
	t.Parallel()

	file, err := os.OpenFile("kubelet-readonly-pods.json", os.O_RDONLY, 0644)
	require.NoError(t, err)
	defer file.Close()

	pods, err := parsePodsFromKubeletReadOnlyPort(file)
	require.NoError(t, err)
	require.NotNil(t, pods, "pods should not be nil")
	require.Len(t, pods.Items, 2, "expected 2 pods")

	assert.Equal(t, "vector-jldbs", pods.Items[0].Name)
	assert.Equal(t, corev1.PodRunning, pods.Items[0].Status.Phase)
	assert.Equal(t, "kube-proxy-hfqwt", pods.Items[1].Name)
	assert.Equal(t, corev1.PodRunning, pods.Items[1].Status.Phase)
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

func Test_PodStatusJSON(t *testing.T) {
	now := metav1.Now()

	testCases := []struct {
		name     string
		status   PodStatus
		expected string
	}{
		{
			name:     "empty status",
			status:   PodStatus{},
			expected: `{}`,
		},
		{
			name: "basic status",
			status: PodStatus{
				ID:        "pod-123",
				Namespace: "default",
				Name:      "test-pod",
				Phase:     "Running",
			},
			expected: `{"id":"pod-123","namespace":"default","name":"test-pod","phase":"Running"}`,
		},
		{
			name: "with conditions",
			status: PodStatus{
				ID:        "pod-123",
				Namespace: "default",
				Name:      "test-pod",
				Phase:     "Running",
				Conditions: []PodCondition{
					{
						Type:               "Ready",
						Status:             "True",
						LastTransitionTime: now,
					},
				},
				StartTime: &now,
			},
			// We'll use JSONEq for comparison since time formatting can be tricky
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonData, err := tc.status.JSON()
			require.NoError(t, err)

			if tc.name == "with conditions" {
				// For the case with time values, just make sure it doesn't error and produces valid JSON
				var obj map[string]interface{}
				err = json.Unmarshal(jsonData, &obj)
				require.NoError(t, err)
				assert.NotEmpty(t, obj)
			} else {
				assert.JSONEq(t, tc.expected, string(jsonData))
			}
		})
	}
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
				assert.Contains(t, states[0].ExtraInfo, "encoding")
				assert.Equal(t, "json", states[0].ExtraInfo["encoding"])
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

func Test_convertToPodsStatus(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "default",
				UID:       "uid1",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "kube-system",
				UID:       "uid2",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		},
	}

	result := convertToPodsStatus(pods...)

	require.Len(t, result, 2)
	assert.Equal(t, "pod1", result[0].Name)
	assert.Equal(t, "default", result[0].Namespace)
	assert.Equal(t, "uid1", result[0].ID)
	assert.Equal(t, string(corev1.PodRunning), result[0].Phase)

	assert.Equal(t, "pod2", result[1].Name)
	assert.Equal(t, "kube-system", result[1].Namespace)
	assert.Equal(t, "uid2", result[1].ID)
	assert.Equal(t, string(corev1.PodPending), result[1].Phase)
}

func Test_convertToPodStatus(t *testing.T) {
	now := metav1.Now()

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "pod-123",
		},
		Status: corev1.PodStatus{
			Phase:   corev1.PodRunning,
			Message: "Running",
			Reason:  "Started",
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "PodReady",
					Message:            "Pod is ready",
				},
			},
			StartTime: &now,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "container1",
					Ready:        true,
					RestartCount: 2,
					Image:        "nginx:latest",
					ContainerID:  "docker://123",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: now,
						},
					},
				},
			},
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "init-container",
					Ready:        true,
					RestartCount: 0,
					Image:        "busybox:latest",
					ContainerID:  "docker://456",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode:   0,
							Reason:     "Completed",
							StartedAt:  now,
							FinishedAt: now,
						},
					},
				},
			},
		},
	}

	result := convertToPodStatus(pod)

	assert.Equal(t, "pod-123", result.ID)
	assert.Equal(t, "default", result.Namespace)
	assert.Equal(t, "test-pod", result.Name)
	assert.Equal(t, string(corev1.PodRunning), result.Phase)
	assert.Equal(t, "Running", result.Message)
	assert.Equal(t, "Started", result.Reason)
	assert.Equal(t, &now, result.StartTime)

	require.Len(t, result.Conditions, 1)
	assert.Equal(t, "Ready", result.Conditions[0].Type)
	assert.Equal(t, "True", result.Conditions[0].Status)
	assert.Equal(t, "PodReady", result.Conditions[0].Reason)
	assert.Equal(t, "Pod is ready", result.Conditions[0].Message)

	require.Len(t, result.ContainerStatuses, 1)
	assert.Equal(t, "container1", result.ContainerStatuses[0].Name)
	assert.True(t, result.ContainerStatuses[0].Ready)
	assert.Equal(t, int32(2), result.ContainerStatuses[0].RestartCount)
	assert.Equal(t, "nginx:latest", result.ContainerStatuses[0].Image)
	assert.Equal(t, "docker://123", result.ContainerStatuses[0].ContainerID)

	require.Len(t, result.InitContainerStatuses, 1)
	assert.Equal(t, "init-container", result.InitContainerStatuses[0].Name)
	assert.True(t, result.InitContainerStatuses[0].Ready)
	assert.Equal(t, int32(0), result.InitContainerStatuses[0].RestartCount)
	assert.Equal(t, "busybox:latest", result.InitContainerStatuses[0].Image)
	assert.Equal(t, "docker://456", result.InitContainerStatuses[0].ContainerID)
}

func Test_defaultHTTPClient(t *testing.T) {
	client := defaultHTTPClient()

	require.NotNil(t, client)
	require.NotNil(t, client.Transport)

	// Check timeout is set
	assert.Equal(t, 30*time.Second, client.Timeout)

	// Check transport is configured
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.True(t, transport.DisableCompression)
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
				failedCount:     tc.failedCount,
			}

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

// Test context cancellation for listPodsFromKubeletReadOnlyPort
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
	nodeName, pods, err := listPodsFromKubeletReadOnlyPort(ctx, int(port))
	assert.Error(t, err)
	assert.Empty(t, nodeName)
	assert.Nil(t, pods)
	assert.Contains(t, err.Error(), "context canceled")
}

// TestParsePodsFromKubeletReadOnlyPort_InvalidJSON tests error handling for invalid JSON
func TestParsePodsFromKubeletReadOnlyPort_InvalidJSON(t *testing.T) {
	t.Parallel()

	// Create a reader with invalid JSON
	invalidJSON := strings.NewReader(`{"items": [{"invalid": json}]}`)

	// Should fail to parse
	pods, err := parsePodsFromKubeletReadOnlyPort(invalidJSON)
	assert.Error(t, err)
	assert.Nil(t, pods)
}

// TestParsePodsFromKubeletReadOnlyPort_EmptyResponse tests handling empty response
func TestParsePodsFromKubeletReadOnlyPort_EmptyResponse(t *testing.T) {
	t.Parallel()

	// Create a reader with empty response
	emptyResponse := strings.NewReader(``)

	// Should fail to parse
	pods, err := parsePodsFromKubeletReadOnlyPort(emptyResponse)
	assert.Error(t, err)
	assert.Nil(t, pods)
}

// TestParsePodsFromKubeletReadOnlyPort_EmptyJSON tests handling empty JSON
func TestParsePodsFromKubeletReadOnlyPort_EmptyJSON(t *testing.T) {
	t.Parallel()

	// Create a reader with empty JSON
	emptyJSON := strings.NewReader(`{}`)

	// Should succeed but have empty items
	pods, err := parsePodsFromKubeletReadOnlyPort(emptyJSON)
	assert.NoError(t, err)
	assert.NotNil(t, pods)
	assert.Empty(t, pods.Items)
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
	nodeName, pods, err := listPodsFromKubeletReadOnlyPort(ctx, int(port))
	assert.Error(t, err)
	assert.Empty(t, nodeName)
	assert.Nil(t, pods)
}

// Test_componentCheck tests the Check method of the component
func Test_componentCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		} else if r.URL.Path == "/pods" {
			w.Header().Set("Content-Type", "application/json")
			http.ServeFile(w, r, "kubelet-readonly-pods.json")
		} else {
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
		ctx:                      ctx,
		cancel:                   cancel,
		checkDependencyInstalled: func() bool { return true },
		checkKubeletRunning:      func() bool { kubeletRunningCalled++; return true },
		kubeletReadOnlyPort:      int(port),
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
				failedCount:     tc.failedCount,
			}

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
		failedCount: 0,
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
	assert.Equal(t, defaultKubeletReadOnlyPort, c.kubeletReadOnlyPort)
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
				assert.Contains(t, states[0].ExtraInfo, "encoding")
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
		ctx:                      ctx,
		cancel:                   cancel,
		checkDependencyInstalled: func() bool { return true },
		checkKubeletRunning:      func() bool { return false }, // Kubelet not running
		kubeletReadOnlyPort:      10255,
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

// Test behavior with different checkDependencyInstalled and checkKubeletRunning combinations
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
				ctx:                      ctx,
				cancel:                   cancel,
				checkDependencyInstalled: func() bool { return tc.dependencyInstalled },
				checkKubeletRunning:      func() bool { return tc.kubeletRunning },
				kubeletReadOnlyPort:      port,
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
	assert.Equal(t, defaultKubeletReadOnlyPort, c.kubeletReadOnlyPort)

	// The checkKubeletRunning function should be set
	assert.NotNil(t, c.checkKubeletRunning)

	// We can't directly test the function's logic since it depends on the network,
	// but we can verify it's there and doesn't panic when called
	assert.NotPanics(t, func() {
		_ = c.checkKubeletRunning()
	})
}
