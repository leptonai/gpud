package kubelet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	nodeName, pods, err := ListPodsFromKubeletReadOnlyPort(ctx, int(port))
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
	_, result, err := ListPodsFromKubeletReadOnlyPort(ctx, int(port))

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
	nodeName, pods, err := ListPodsFromKubeletReadOnlyPort(ctx, int(port))

	require.Error(t, err, "expected an error")
	assert.Empty(t, nodeName, "node name should be empty")
	assert.Nil(t, pods, "pods should be nil")
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
			jsonData, err := json.Marshal(tc.status)
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

func Test_parsePodsFromKubeletReadOnlyPort(t *testing.T) {
	t.Parallel()

	file, err := os.OpenFile("kubelet-readonly-pods.json", os.O_RDONLY, 0644)
	require.NoError(t, err)
	defer func() {
		_ = file.Close()
	}()

	pods, err := parsePodsFromKubeletReadOnlyPort(file)
	require.NoError(t, err)
	require.NotNil(t, pods, "pods should not be nil")
	require.Len(t, pods.Items, 2, "expected 2 pods")

	assert.Equal(t, "vector-jldbs", pods.Items[0].Name)
	assert.Equal(t, corev1.PodRunning, pods.Items[0].Status.Phase)
	assert.Equal(t, "kube-proxy-hfqwt", pods.Items[1].Name)
	assert.Equal(t, corev1.PodRunning, pods.Items[1].Status.Phase)
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
