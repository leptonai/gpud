package container

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	docker_types "github.com/docker/docker/api/types"
	"github.com/leptonai/gpud/components"
	docker_container_id "github.com/leptonai/gpud/components/docker/container/id"
)

func TestIsDockerRunning(t *testing.T) {
	t.Logf("%v", isDockerRunning())
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
			result := isErrDockerClientVersionNewerThanDaemon(tt.err)
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

	json, err := container.JSON()
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
		input    docker_types.Container
		expected DockerContainer
	}{
		{
			name: "Basic container without Kubernetes labels",
			input: docker_types.Container{
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
			input: docker_types.Container{
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
			input: docker_types.Container{
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

func TestDataMarshalJSON(t *testing.T) {
	d := Data{
		DockerPidFound: true,
		Containers: []DockerContainer{
			{
				ID:    "test-id",
				Name:  "test-name",
				Image: "test-image",
			},
		},
		ts:      time.Now(),
		err:     nil,
		connErr: false,
	}

	json, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(json), "test-id")
	assert.Contains(t, string(json), "test-name")
	assert.Contains(t, string(json), "test-image")
	assert.Contains(t, string(json), "docker_pid_found")
	assert.Contains(t, string(json), "containers")
}

func TestDataReason(t *testing.T) {
	tests := []struct {
		name         string
		data         Data
		expectedText string
	}{
		{
			name: "No error",
			data: Data{
				Containers: []DockerContainer{{}, {}, {}},
				err:        nil,
			},
			expectedText: "total 3 containers",
		},
		{
			name: "Docker client version newer than daemon",
			data: Data{
				err: errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
			},
			expectedText: "not supported",
		},
		{
			name: "Connection error",
			data: Data{
				err:     errors.New("Cannot connect to the Docker daemon"),
				connErr: true,
			},
			expectedText: "connection error to docker daemon",
		},
		{
			name: "General error",
			data: Data{
				err: errors.New("some general error"),
			},
			expectedText: "failed to list containers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.Reason()
			assert.Contains(t, result, tt.expectedText)
		})
	}
}

func TestDataGetHealth(t *testing.T) {
	tests := []struct {
		name            string
		data            Data
		ignoreConnErr   bool
		expectedHealth  string
		expectedHealthy bool
	}{
		{
			name: "No error",
			data: Data{
				err: nil,
			},
			ignoreConnErr:   false,
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
		},
		{
			name: "Connection error - ignored",
			data: Data{
				err:     errors.New("Cannot connect to the Docker daemon"),
				connErr: true,
			},
			ignoreConnErr:   true,
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
		},
		{
			name: "Connection error - not ignored",
			data: Data{
				err:     errors.New("Cannot connect to the Docker daemon"),
				connErr: true,
			},
			ignoreConnErr:   false,
			expectedHealth:  components.StateUnhealthy,
			expectedHealthy: false,
		},
		{
			name: "General error",
			data: Data{
				err: errors.New("some general error"),
			},
			ignoreConnErr:   true,
			expectedHealth:  components.StateUnhealthy,
			expectedHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health, healthy := tt.data.getHealth(tt.ignoreConnErr)
			assert.Equal(t, tt.expectedHealth, health)
			assert.Equal(t, tt.expectedHealthy, healthy)
		})
	}
}

func TestDataGetStates(t *testing.T) {
	tests := []struct {
		name          string
		data          Data
		ignoreConnErr bool
		expectError   bool
		stateCount    int
	}{
		{
			name: "No containers",
			data: Data{
				DockerPidFound: true,
				Containers:     []DockerContainer{},
				err:            nil,
			},
			ignoreConnErr: false,
			expectError:   false,
			stateCount:    1,
		},
		{
			name: "With containers",
			data: Data{
				DockerPidFound: true,
				Containers:     []DockerContainer{{ID: "test-id"}},
				err:            nil,
			},
			ignoreConnErr: false,
			expectError:   false,
			stateCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.data.getStates(tt.ignoreConnErr)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.stateCount, len(states))
			assert.Equal(t, docker_container_id.Name, states[0].Name)
		})
	}
}

func TestComponentName(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, true)
	assert.Equal(t, docker_container_id.Name, c.Name())
}

func TestComponentStart(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, true)
	err := c.Start()
	assert.NoError(t, err)
}

func Test_componentStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{ctx: ctx, cancel: cancel}
	err := c.Start()
	assert.NoError(t, err)
}

func TestComponentEvents(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, true)
	events, err := c.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestComponentMetrics(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, true)
	metrics, err := c.Metrics(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestComponentClose(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, true)
	err := c.Close()
	assert.NoError(t, err)
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, true)
	assert.NotNil(t, c)
	assert.Equal(t, docker_container_id.Name, c.Name())

	// With different ignoreConnectionErrors value
	c2 := New(ctx, false)
	assert.NotNil(t, c2)
	assert.Equal(t, docker_container_id.Name, c2.Name())
}

func TestComponentStates(t *testing.T) {
	ctx := context.Background()
	comp := New(ctx, true).(*component)

	// Test with empty data
	comp.lastData = Data{
		DockerPidFound: true,
		Containers:     []DockerContainer{},
		ts:             time.Now(),
		err:            nil,
		connErr:        false,
	}

	states, err := comp.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Equal(t, docker_container_id.Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.Equal(t, true, states[0].Healthy)

	// Test with containers
	comp.lastData = Data{
		DockerPidFound: true,
		Containers: []DockerContainer{
			{ID: "test-id", Name: "test-name"},
		},
		ts:      time.Now(),
		err:     nil,
		connErr: false,
	}

	states, err = comp.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.NotNil(t, states[0].ExtraInfo)

	// Test with error but ignoreConnectionErrors is true
	comp.lastData = Data{
		DockerPidFound: false,
		Containers:     []DockerContainer{},
		ts:             time.Now(),
		err:            errors.New("Cannot connect to the Docker daemon"),
		connErr:        true,
	}

	states, err = comp.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.Equal(t, true, states[0].Healthy)
}

// TestMockCheckOnce tests a part of the checkOnce method's behavior
func TestMockCheckOnce(t *testing.T) {
	ctx := context.Background()
	comp := New(ctx, false).(*component)

	// Initial state should be empty
	assert.Empty(t, comp.lastData.Containers)

	// Manually update the lastData to simulate checkOnce behavior
	mockData := Data{
		DockerPidFound: true,
		Containers: []DockerContainer{
			{
				ID:    "mock-id",
				Name:  "mock-name",
				Image: "mock-image",
				State: "running",
			},
		},
		ts:      time.Now(),
		err:     nil,
		connErr: false,
	}

	comp.lastMu.Lock()
	comp.lastData = mockData
	comp.lastMu.Unlock()

	// Verify States now returns our mocked data
	states, err := comp.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))

	// State should contain our mock data in ExtraInfo
	jsonData, ok := states[0].ExtraInfo["data"]
	assert.True(t, ok)
	assert.Contains(t, jsonData, "mock-id")
	assert.Contains(t, jsonData, "mock-name")
	assert.Contains(t, jsonData, "mock-image")
	assert.Contains(t, jsonData, "running")
}

// mockDockerClient is a helper function to test Docker client related functions without actual Docker
// This is a more advanced test that uses monkey patching to override the Docker client functions
func TestMockListContainers(t *testing.T) {
	// We can't directly test the Docker client functions without actual Docker.
	// This test is more to demonstrate how you would test such functions with a proper mocking framework.
	t.Skip("This is a demonstration test that would require Docker mocking.")

	// In a real test with proper mocking, you would do something like:
	// 1. Replace the docker_client.NewClientWithOpts with a mock function that returns a mock client
	// 2. Have the mock client return predetermined container list
	// 3. Verify the conversion works correctly

	// Example pseudo-code (not runnable):
	/*
		mockContainers := []docker_types.Container{
			{
				ID:      "mock-id-1",
				Names:   []string{"mock-name-1"},
				Image:   "mock-image-1",
				Created: 12345,
				State:   "running",
				Labels: map[string]string{
					"io.kubernetes.pod.name":      "mock-pod",
					"io.kubernetes.pod.namespace": "mock-namespace",
				},
			},
		}

		// Setup mock environment
		// ...

		// Call the function
		containers, err := listContainers(context.Background())

		// Assertions
		assert.NoError(t, err)
		assert.Equal(t, 1, len(containers))
		assert.Equal(t, "mock-id-1", containers[0].ID)
		assert.Equal(t, "mock-name-1", containers[0].Name)
		assert.Equal(t, "mock-image-1", containers[0].Image)
		assert.Equal(t, "running", containers[0].State)
		assert.Equal(t, "mock-pod", containers[0].PodName)
		assert.Equal(t, "mock-namespace", containers[0].PodNamespace)
	*/
}

// Improve checkOnce test coverage
func TestCheckOnceErrorConditions(t *testing.T) {
	// We're not initializing logger in tests to avoid dependencies
	// If the real implementation relies on the logger, consider using a mocking framework

	ctx := context.Background()
	comp := New(ctx, true).(*component)

	// Test with connection error
	mockData := Data{
		DockerPidFound: false,
		Containers:     []DockerContainer{},
		ts:             time.Now(),
		err:            errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"),
		connErr:        true,
	}

	comp.lastMu.Lock()
	comp.lastData = mockData
	comp.lastMu.Unlock()

	// Get states and verify error handling with ignoreConnectionErrors=true
	states, err := comp.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Equal(t, components.StateHealthy, states[0].Health) // Should be healthy because we're ignoring connection errors

	// Create a new component that doesn't ignore connection errors
	comp2 := New(ctx, false).(*component)
	comp2.lastMu.Lock()
	comp2.lastData = mockData
	comp2.lastMu.Unlock()

	// Get states and verify error handling with ignoreConnectionErrors=false
	states, err = comp2.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Equal(t, components.StateUnhealthy, states[0].Health) // Should be unhealthy because we're not ignoring connection errors

	// Test with client version newer than daemon error
	mockData = Data{
		DockerPidFound: false,
		Containers:     []DockerContainer{},
		ts:             time.Now(),
		err:            errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
		connErr:        false,
	}

	comp.lastMu.Lock()
	comp.lastData = mockData
	comp.lastMu.Unlock()

	// Get states and verify special error message
	states, err = comp.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Contains(t, states[0].Reason, "not supported")
	assert.Contains(t, states[0].Reason, "needs upgrading docker daemon")
}
