package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	docker_types "github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerClientNegotiatesOlderAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_ping":
			w.Header().Set("API-Version", "1.42")
		case "/v1.42/containers/json":
			_, _ = w.Write([]byte(`[{"Id":"test-id","Names":["/test"],"Image":"test","Created":1,"State":"running","Labels":{}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", srv.URL)

	assert.True(t, CheckDockerRunning(context.Background()))
	containers, err := ListContainers(context.Background())
	require.NoError(t, err)
	require.Len(t, containers, 1)
	assert.Equal(t, "test-id", containers[0].ID)
}

func Test_checkDockerInstalled(t *testing.T) {
	t.Logf("%v", CheckDockerInstalled())
}

func Test_checkDockerRunning(t *testing.T) {
	t.Logf("%v", CheckDockerRunning(context.Background()))
}

func TestIsErrDockerClientVersionNewerThanDaemon(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Correct error message",
			err:      errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
			expected: true,
		},
		{
			name:     "Partial match - missing 'is too new'",
			err:      errors.New("Error response from daemon: client version 1.44. Maximum supported API version is 1.43"),
			expected: false,
		},
		{
			name:     "Partial match - missing 'client version'",
			err:      errors.New("Error response from daemon: Docker 1.44 is too new. Maximum supported API version is 1.43"),
			expected: false,
		},
		{
			name:     "Unrelated error message",
			err:      errors.New("Connection refused"),
			expected: false,
		},
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsErrDockerClientVersionNewerThanDaemon(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDockerContainer_JSON(t *testing.T) {
	container := DockerContainer{
		ID:           "test-id",
		Name:         "test-name",
		Image:        "test-image",
		CreatedAt:    123456789,
		State:        "running",
		PodName:      "test-pod",
		PodNamespace: "test-namespace",
	}

	json, err := json.Marshal(container)
	require.NoError(t, err)
	assert.Contains(t, string(json), "test-id")
	assert.Contains(t, string(json), "test-name")
	assert.Contains(t, string(json), "test-image")
	assert.Contains(t, string(json), "running")
	assert.Contains(t, string(json), "test-pod")
	assert.Contains(t, string(json), "test-namespace")
}

func TestConvertToDockerContainer(t *testing.T) {
	tests := []struct {
		name     string
		input    docker_types.Summary
		expected DockerContainer
	}{
		{
			name: "Basic container without Kubernetes labels",
			input: docker_types.Summary{
				ID:      "test-id",
				Names:   []string{"test-name"},
				Image:   "test-image",
				Created: 123456789,
				State:   "running",
				Labels:  map[string]string{},
			},
			expected: DockerContainer{
				ID:           "test-id",
				Name:         "test-name",
				Image:        "test-image",
				CreatedAt:    123456789,
				State:        "running",
				PodName:      "",
				PodNamespace: "",
			},
		},
		{
			name: "Container with Kubernetes labels",
			input: docker_types.Summary{
				ID:      "k8s-id",
				Names:   []string{"k8s-name"},
				Image:   "k8s-image",
				Created: 987654321,
				State:   "running",
				Labels: map[string]string{
					"io.kubernetes.pod.name":      "k8s-pod",
					"io.kubernetes.pod.namespace": "k8s-namespace",
				},
			},
			expected: DockerContainer{
				ID:           "k8s-id",
				Name:         "k8s-name",
				Image:        "k8s-image",
				CreatedAt:    987654321,
				State:        "running",
				PodName:      "k8s-pod",
				PodNamespace: "k8s-namespace",
			},
		},
		{
			name: "Container with multiple names",
			input: docker_types.Summary{
				ID:      "multi-id",
				Names:   []string{"name1", "name2", "name3"},
				Image:   "multi-image",
				Created: 123123123,
				State:   "exited",
				Labels:  map[string]string{},
			},
			expected: DockerContainer{
				ID:           "multi-id",
				Name:         "name1,name2,name3",
				Image:        "multi-image",
				CreatedAt:    123123123,
				State:        "exited",
				PodName:      "",
				PodNamespace: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToDockerContainer(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
