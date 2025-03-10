package pod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func Test_isConnectionRefusedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "non-connection refused error",
			err:  errors.New("some other error"),
			want: false,
		},
		{
			name: "connection refused error with localhost IPv4",
			err:  errors.New(`Get "http://localhost:10255/pods": dial tcp 127.0.0.1:10255: connect: connection refused`),
			want: true,
		},
		{
			name: "connection refused error with localhost IPv6",
			err:  errors.New(`Get "http://localhost:10255/pods": dial tcp [::1]:10255: connect: connection refused`),
			want: true,
		},
		{
			name: "connection refused with different format",
			err:  fmt.Errorf("failed to connect: connection refused"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isConnectionRefusedError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
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
	assert.True(t, isConnectionRefusedError(err), "expected connection refused error")
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
		data     Data
		expected string
	}{
		{
			name:     "empty data",
			data:     Data{},
			expected: `{"kubelet_service_active":false}`,
		},
		{
			name: "with node name",
			data: Data{
				KubeletServiceActive: true,
				NodeName:             "test-node",
			},
			expected: `{"kubelet_service_active":true,"node_name":"test-node"}`,
		},
		{
			name: "with pods",
			data: Data{
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

func Test_describeReason(t *testing.T) {
	testCases := []struct {
		name     string
		data     Data
		expected string
	}{
		{
			name: "no error",
			data: Data{
				NodeName: "test-node",
				Pods:     []PodStatus{{}, {}},
			},
			expected: "total 2 pods (node test-node)",
		},
		{
			name: "empty pods",
			data: Data{
				NodeName: "test-node",
				Pods:     []PodStatus{},
			},
			expected: "no pod found or kubelet is not running",
		},
		{
			name: "nil pods",
			data: Data{
				NodeName: "test-node",
			},
			expected: "no pod found or kubelet is not running",
		},
		{
			name: "connection error",
			data: Data{
				NodeName: "test-node",
				Pods:     []PodStatus{{ID: "test-pod"}},
				err:      errors.New("connection refused"),
				connErr:  true,
			},
			expected: `connection error to node "test-node" -- connection refused`,
		},
		{
			name: "other error",
			data: Data{
				Pods: []PodStatus{{ID: "test-pod"}},
				err:  errors.New("some error"),
			},
			expected: "failed to list pods from kubelet read-only port -- some error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reason := tc.data.getReason()
			assert.Equal(t, tc.expected, reason)
		})
	}
}

func Test_getHealth(t *testing.T) {
	testCases := []struct {
		name            string
		data            Data
		ignoreConnErr   bool
		expectedHealth  string
		expectedHealthy bool
	}{
		{
			name:            "no error",
			data:            Data{},
			expectedHealth:  "Healthy",
			expectedHealthy: true,
		},
		{
			name: "connection error - ignored",
			data: Data{
				err:     errors.New("connection refused"),
				connErr: true,
			},
			ignoreConnErr:   true,
			expectedHealth:  "Healthy",
			expectedHealthy: true,
		},
		{
			name: "connection error - not ignored",
			data: Data{
				err:     errors.New("connection refused"),
				connErr: true,
			},
			ignoreConnErr:   false,
			expectedHealth:  "Unhealthy",
			expectedHealthy: false,
		},
		{
			name: "other error",
			data: Data{
				err: errors.New("some error"),
			},
			ignoreConnErr:   true,
			expectedHealth:  "Unhealthy",
			expectedHealthy: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			health, healthy := tc.data.getHealth(tc.ignoreConnErr)
			assert.Equal(t, tc.expectedHealth, health)
			assert.Equal(t, tc.expectedHealthy, healthy)
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

func Test_getStates(t *testing.T) {
	testCases := []struct {
		name           string
		data           Data
		ignoreConnErr  bool
		expectedLen    int
		expectedHealth string
	}{
		{
			name:           "no pods",
			data:           Data{},
			ignoreConnErr:  false,
			expectedLen:    1,
			expectedHealth: "Healthy",
		},
		{
			name: "with pods",
			data: Data{
				Pods: []PodStatus{
					{Name: "pod1"},
					{Name: "pod2"},
				},
			},
			ignoreConnErr:  false,
			expectedLen:    1,
			expectedHealth: "Healthy",
		},
		{
			name: "with error - not ignored",
			data: Data{
				err:     errors.New("some error"),
				connErr: false,
			},
			ignoreConnErr:  false,
			expectedLen:    1,
			expectedHealth: "Unhealthy",
		},
		{
			name: "with connection error - ignored",
			data: Data{
				err:     errors.New("connection refused"),
				connErr: true,
			},
			ignoreConnErr:  true,
			expectedLen:    1,
			expectedHealth: "Healthy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			states, err := tc.data.getStates(tc.ignoreConnErr)
			require.NoError(t, err)
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

func Test_componentMetrics(t *testing.T) {
	c := &component{}
	metrics, err := c.Metrics(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Nil(t, metrics)
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

func TestCheckKubeletReadOnlyPort(t *testing.T) {
	// Setup a test server that returns "ok" for /healthz endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Extract port from test server URL
	portRaw := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portRaw, 10, 32)

	// Test successful check
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := checkKubeletReadOnlyPortHealthz(ctx, int(port))
	assert.NoError(t, err)

	// Test with invalid response
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not ok"))
		}
	}))
	defer badSrv.Close()

	portRaw = badSrv.URL[len("http://127.0.0.1:"):]
	port, _ = strconv.ParseInt(portRaw, 10, 32)

	err = checkKubeletReadOnlyPortHealthz(ctx, int(port))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected 'ok'")

	// Test with non-200 response
	badSrv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer badSrv2.Close()

	portRaw = badSrv2.URL[len("http://127.0.0.1:"):]
	port, _ = strconv.ParseInt(portRaw, 10, 32)

	err = checkKubeletReadOnlyPortHealthz(ctx, int(port))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
}

// Test with a closed server to check connection error handling
func TestCheckKubeletReadOnlyPort_ConnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}
	}))

	portRaw := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portRaw, 10, 32)

	// Close the server before testing
	srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := checkKubeletReadOnlyPortHealthz(ctx, int(port))
	assert.Error(t, err)
}

func Test_componentStates(t *testing.T) {
	testCases := []struct {
		name             string
		lastData         Data
		ignoreConnErr    bool
		expectedLen      int
		expectedHealth   string
		expectStateError bool
	}{
		{
			name: "healthy state",
			lastData: Data{
				NodeName: "test-node",
				Pods: []PodStatus{
					{Name: "pod1"},
				},
			},
			ignoreConnErr:    false,
			expectedLen:      1,
			expectedHealth:   "Healthy",
			expectStateError: false,
		},
		{
			name: "unhealthy state",
			lastData: Data{
				NodeName: "test-node",
				err:      errors.New("some error"),
			},
			ignoreConnErr:    false,
			expectedLen:      1,
			expectedHealth:   "Unhealthy",
			expectStateError: false,
		},
		{
			name: "connection error - ignored",
			lastData: Data{
				NodeName: "test-node",
				err:      errors.New("connection refused"),
				connErr:  true,
			},
			ignoreConnErr:    true,
			expectedLen:      1,
			expectedHealth:   "Healthy",
			expectStateError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &component{
				ignoreConnectionErrors: tc.ignoreConnErr,
				lastData:               &tc.lastData,
			}

			states, err := c.States(context.Background())

			if tc.expectStateError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				require.Len(t, states, tc.expectedLen)
				assert.Equal(t, tc.expectedHealth, states[0].Health)

				if len(tc.lastData.Pods) > 0 {
					assert.Contains(t, states[0].ExtraInfo, "data")
				}
			}
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

// TestCheckKubeletReadOnlyPortListening tests the CheckKubeletReadOnlyPortListening function
func TestCheckKubeletReadOnlyPortListening(t *testing.T) {
	// Test the case where a healthy kubelet server is running
	t.Run("healthy kubelet server", func(t *testing.T) {
		// Setup a test server that mocks both the TCP connection and healthz endpoint
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			} else {
				http.Error(w, "Not found", http.StatusNotFound)
			}
		}))
		defer srv.Close()

		// Extract port from test server URL
		portStr := srv.URL[len("http://127.0.0.1:"):]
		port, _ := strconv.ParseInt(portStr, 10, 32)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result := checkKubeletReadOnlyPortListening(ctx, int(port))
		if runtime.GOOS == "linux" {
			t.Logf("On Linux system, result is %v", result)
		} else {
			assert.True(t, result)
		}
	})

	// Test with a closed server to check connection error handling
	t.Run("connection refused", func(t *testing.T) {
		if runtime.GOOS == "linux" {
			// Skip if actually running on Linux, can't reliably test
			t.Skip("Skipping test on actual Linux system")
		}

		// Use a closed server to force connection refused error
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		portStr := srv.URL[len("http://127.0.0.1:"):]
		port, _ := strconv.ParseInt(portStr, 10, 32)
		srv.Close() // Close immediately to force connection error

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result := checkKubeletReadOnlyPortListening(ctx, int(port))
		assert.False(t, result)
	})
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

// Test_componentCheckOnce tests the checkOnce method of the component
func Test_componentCheckOnce(t *testing.T) {
	// We can't easily mock process.CheckRunningByPid since it's in an external package
	// So we'll only test the pod retrieval part and check the fields we can control

	// Setup a test server that returns pod data
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

	// Create component with our test port
	c := &component{
		ctx:                      ctx,
		cancel:                   cancel,
		checkDependencyInstalled: func() bool { return true },
		kubeletReadOnlyPort:      int(port),
		checkServiceActive:       func(ctx context.Context) (bool, error) { return true, nil },
		ignoreConnectionErrors:   false,
	}

	// Run the check
	c.CheckOnce()

	// Verify the last data
	c.lastMu.RLock()
	defer c.lastMu.RUnlock()

	// We can't reliably check KubeletPidFound as it depends on the test environment
	assert.Equal(t, "mynodehostname", c.lastData.NodeName)
	assert.Len(t, c.lastData.Pods, 2)
	assert.Nil(t, c.lastData.err)
	assert.False(t, c.lastData.connErr)
}

// Test_componentStates_IgnoreConnectionErrors tests the States method with ignoreConnectionErrors=true
func Test_componentStates_IgnoreConnectionErrors(t *testing.T) {
	testCases := []struct {
		name           string
		lastData       Data
		ignoreConnErr  bool
		expectedHealth string
	}{
		{
			name: "connection error - ignored",
			lastData: Data{
				NodeName: "test-node",
				err:      errors.New("connection refused"),
				connErr:  true,
			},
			ignoreConnErr:  true,
			expectedHealth: "Healthy",
		},
		{
			name: "connection error - not ignored",
			lastData: Data{
				NodeName: "test-node",
				err:      errors.New("connection refused"),
				connErr:  true,
			},
			ignoreConnErr:  false,
			expectedHealth: "Unhealthy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &component{
				ignoreConnectionErrors: tc.ignoreConnErr,
				lastData:               &tc.lastData,
			}

			states, err := c.States(context.Background())
			assert.NoError(t, err)
			require.Len(t, states, 1)
			assert.Equal(t, tc.expectedHealth, states[0].Health)
		})
	}
}

// Test that canceling context in States works correctly
func Test_componentStates_ContextCancellation(t *testing.T) {
	c := &component{
		lastData: &Data{
			NodeName: "test-node",
			Pods:     []PodStatus{{Name: "pod1"}},
		},
	}

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// States should still work with canceled context since it uses already cached data
	states, err := c.States(ctx)
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "Healthy", states[0].Health)
}

// Add test for component constructor
func Test_componentConstructor(t *testing.T) {
	ctx := context.Background()
	comp := New(ctx, DefaultKubeletReadOnlyPort, true)

	// Type assertion
	c, ok := comp.(*component)
	require.True(t, ok, "Component should be of type *component")

	// Check fields
	assert.Equal(t, DefaultKubeletReadOnlyPort, c.kubeletReadOnlyPort)
	assert.True(t, c.ignoreConnectionErrors)
	assert.NotNil(t, c.ctx)
	assert.NotNil(t, c.cancel)
}

// TestCheckKubeletReadOnlyPort_ReadError tests error handling when reading response body fails
func TestCheckKubeletReadOnlyPort_ReadError(t *testing.T) {
	// Create a server that returns a problematic reader
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			// Set the content length but don't actually write the body
			// This will cause the io.ReadAll to fail
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			// Only write a small part of the promised data
			_, _ = w.Write([]byte("o"))
			// Then close the connection to force a read error
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("ResponseWriter does not support hijacking")
			}
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	defer srv.Close()

	portStr := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portStr, 10, 32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// This should result in a read error
	err := checkKubeletReadOnlyPortHealthz(ctx, int(port))
	assert.Error(t, err)
}
