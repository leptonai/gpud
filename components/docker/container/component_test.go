package container

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestData_getStates(t *testing.T) {
	tests := []struct {
		name         string
		data         Data
		expectedText string
	}{
		{
			name: "No error with containers",
			data: Data{
				Containers: []DockerContainer{{}, {}, {}},
				err:        nil,
				healthy:    true,
				reason:     "total 3 container(s)",
			},
			expectedText: "total 3 container",
		},
		{
			name: "Empty containers",
			data: Data{
				Containers: []DockerContainer{},
				err:        nil,
				healthy:    true,
				reason:     "total 0 container(s)",
			},
			expectedText: "total 0 container",
		},
		{
			name: "Nil containers",
			data: Data{
				Containers: nil,
				err:        nil,
				healthy:    true,
				reason:     "total 0 container(s)",
			},
			expectedText: "total 0 container",
		},
		{
			name: "Docker client version newer than daemon",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
				healthy:    false,
				reason:     "not supported; Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43 (needs upgrading docker daemon in the host)",
			},
			expectedText: "not supported",
		},
		{
			name: "Connection error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("Cannot connect to the Docker daemon"),
				healthy:    false,
				reason:     "connection error to docker daemon -- Cannot connect to the Docker daemon",
			},
			expectedText: "connection error to docker daemon",
		},
		{
			name: "General error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("some general error"),
				healthy:    false,
				reason:     "error listing containers -- some general error",
			},
			expectedText: "error listing containers",
		},
		// Additional test cases for different error scenarios
		{
			name: "Connection refusal",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("connection refused"),
				healthy:    false,
				reason:     "error listing containers -- connection refused",
			},
			expectedText: "error listing containers",
		},
		{
			name: "Daemon not running",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("Is the docker daemon running?"),
				healthy:    false,
				reason:     "connection error to docker daemon -- Is the docker daemon running?",
			},
			expectedText: "connection error to docker daemon",
		},
		{
			name: "Permission denied error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("permission denied"),
				healthy:    false,
				reason:     "error listing containers -- permission denied",
			},
			expectedText: "error listing containers",
		},
		{
			name: "Docker network error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("network error communicating with Docker daemon"),
				healthy:    false,
				reason:     "error listing containers -- network error communicating with Docker daemon",
			},
			expectedText: "error listing containers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, tt.data.reason, tt.expectedText)
		})
	}

	// Test with nil data
	var nilData *Data
	states, err := nilData.getHealthStates()
	assert.NoError(t, err)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataHealthField(t *testing.T) {
	tests := []struct {
		name            string
		data            Data
		expectedHealth  apiv1.HealthStateType
		expectedHealthy bool
	}{
		{
			name: "No error",
			data: Data{
				err:     nil,
				healthy: true,
				reason:  "healthy",
			},
			expectedHealth:  apiv1.StateTypeHealthy,
			expectedHealthy: true,
		},
		{
			name: "Connection error - ignored",
			data: Data{
				err:     errors.New("Cannot connect to the Docker daemon"),
				healthy: true,
				reason:  "connection error to docker daemon but ignored",
			},
			expectedHealth:  apiv1.StateTypeHealthy,
			expectedHealthy: true,
		},
		{
			name: "Connection error - not ignored",
			data: Data{
				err:     errors.New("Cannot connect to the Docker daemon"),
				healthy: false,
				reason:  "connection error to docker daemon",
			},
			expectedHealth:  apiv1.StateTypeUnhealthy,
			expectedHealthy: false,
		},
		{
			name: "General error",
			data: Data{
				err:     errors.New("some general error"),
				healthy: false,
				reason:  "error occurred",
			},
			expectedHealth:  apiv1.StateTypeUnhealthy,
			expectedHealthy: false,
		},
		// Additional test cases for error scenarios
		{
			name: "Permission denied error - not ignored",
			data: Data{
				err:     errors.New("permission denied"),
				healthy: false,
				reason:  "permission denied error",
			},
			expectedHealth:  apiv1.StateTypeUnhealthy,
			expectedHealthy: false,
		},
		{
			name: "Docker service inactive error - not ignored",
			data: Data{
				err:                 errors.New("docker service is not active"),
				DockerServiceActive: false,
				healthy:             false,
				reason:              "docker service is not active",
			},
			expectedHealth:  apiv1.StateTypeUnhealthy,
			expectedHealthy: false,
		},
		{
			name: "Docker not found error - not ignored",
			data: Data{
				err:     errors.New("docker not found"),
				healthy: false,
				reason:  "docker not found error",
			},
			expectedHealth:  apiv1.StateTypeUnhealthy,
			expectedHealthy: false,
		},
		{
			name: "Is docker daemon running - ignored",
			data: Data{
				err:     errors.New("Is the docker daemon running?"),
				healthy: true,
				reason:  "connection error ignored",
			},
			expectedHealth:  apiv1.StateTypeHealthy,
			expectedHealthy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.data.getHealthStates()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedHealth, states[0].Health)
			assert.Equal(t, tt.expectedHealthy, states[0].DeprecatedHealthy)
		})
	}

	// Test with nil data
	var nilData *Data
	states, err := nilData.getHealthStates()
	assert.NoError(t, err)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)
}

func TestDataGetStates(t *testing.T) {
	tests := []struct {
		name           string
		data           Data
		stateCount     int
		expectedHealth apiv1.HealthStateType
	}{
		{
			name: "No containers",
			data: Data{
				DockerServiceActive: true,
				Containers:          []DockerContainer{},
				err:                 nil,
				healthy:             true,
				reason:              "no container found",
			},
			stateCount:     1,
			expectedHealth: apiv1.StateTypeHealthy,
		},
		{
			name: "With containers",
			data: Data{
				DockerServiceActive: true,
				Containers:          []DockerContainer{{ID: "test-id"}},
				err:                 nil,
				healthy:             true,
				reason:              "total 1 container(s)",
			},
			stateCount:     1,
			expectedHealth: apiv1.StateTypeHealthy,
		},
		{
			name: "With error not ignored",
			data: Data{
				DockerServiceActive: true,
				Containers:          []DockerContainer{{ID: "test-id"}},
				err:                 errors.New("test error"),
				healthy:             false,
				reason:              "error listing containers -- test error",
			},
			stateCount:     1,
			expectedHealth: apiv1.StateTypeUnhealthy,
		},
		{
			name: "With connection error ignored",
			data: Data{
				DockerServiceActive: true,
				Containers:          []DockerContainer{{ID: "test-id"}},
				err:                 errors.New("connection error"),
				healthy:             true,
				reason:              "connection error but ignored",
			},
			stateCount:     1,
			expectedHealth: apiv1.StateTypeHealthy,
		},
		{
			name: "With connection error not ignored",
			data: Data{
				DockerServiceActive: true,
				Containers:          []DockerContainer{{ID: "test-id"}},
				err:                 errors.New("connection error"),
				healthy:             false,
				reason:              "connection error not ignored",
			},
			stateCount:     1,
			expectedHealth: apiv1.StateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.data.getHealthStates()
			assert.NoError(t, err)

			assert.Equal(t, tt.stateCount, len(states))
			assert.Equal(t, Name, states[0].Name)
			assert.Equal(t, tt.expectedHealth, states[0].Health)
			assert.Equal(t, tt.data.healthy, states[0].DeprecatedHealthy)
			assert.Equal(t, tt.data.reason, states[0].Reason)

			// For cases with containers, check ExtraInfo
			if len(tt.data.Containers) > 0 {
				assert.NotNil(t, states[0].DeprecatedExtraInfo)
				assert.Contains(t, states[0].DeprecatedExtraInfo, "data")
				assert.Contains(t, states[0].DeprecatedExtraInfo, "encoding")
			}
		})
	}

	// Test with nil data
	var nilData *Data
	states, err := nilData.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestComponentName(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, true)
	assert.Equal(t, Name, c.Name())
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
	assert.Equal(t, Name, c.Name())

	// With different ignoreConnectionErrors value
	c2 := New(ctx, false)
	assert.NotNil(t, c2)
	assert.Equal(t, Name, c2.Name())
}

func TestComponentStates(t *testing.T) {
	ctx := context.Background()
	comp := New(ctx, true).(*component)

	// Test with empty data
	comp.lastData = &Data{
		DockerServiceActive: true,
		Containers:          []DockerContainer{},
		ts:                  time.Now(),
		err:                 nil,
		healthy:             true,
		reason:              "total 0 container(s)",
	}

	states, err := comp.HealthStates(ctx)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(states))
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, true, states[0].DeprecatedHealthy)

	// Test with containers
	comp.lastData = &Data{
		DockerServiceActive: true,
		Containers: []DockerContainer{
			{ID: "test-id", Name: "test-name"},
		},
		ts:      time.Now(),
		err:     nil,
		healthy: true,
		reason:  "total 1 container(s)",
	}

	states, err = comp.HealthStates(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.NotNil(t, states[0].DeprecatedExtraInfo)

	// Test with error but ignoreConnectionErrors is true
	comp.lastData = &Data{
		DockerServiceActive: false,
		Containers:          []DockerContainer{},
		ts:                  time.Now(),
		err:                 errors.New("Cannot connect to the Docker daemon"),
		healthy:             true,
		reason:              "connection error to docker daemon -- Cannot connect to the Docker daemon",
	}

	states, err = comp.HealthStates(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, true, states[0].DeprecatedHealthy)
}

func TestCheckOnceErrorConditions(t *testing.T) {
	ctx := context.Background()
	comp := New(ctx, true).(*component)

	// Test with connection error
	mockData := &Data{
		DockerServiceActive: false,
		Containers:          []DockerContainer{},
		ts:                  time.Now(),
		err:                 errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"),
		healthy:             true,
		reason:              "connection error to docker daemon -- Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
	}

	comp.lastMu.Lock()
	comp.lastData = mockData
	comp.lastMu.Unlock()

	// Get states and verify error handling with ignoreConnectionErrors=true
	states, err := comp.HealthStates(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health) // Should be healthy because we're ignoring connection errors

	// Create a new component that doesn't ignore connection errors
	comp2 := New(ctx, false).(*component)
	mockData.healthy = false // this would be set by CheckOnce() when ignoreConnectionErrors=false
	comp2.lastMu.Lock()
	comp2.lastData = mockData
	comp2.lastMu.Unlock()

	// Get states and verify error handling with ignoreConnectionErrors=false
	states, err = comp2.HealthStates(ctx)
	assert.NoError(t, err)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health) // Should be unhealthy because we're not ignoring connection errors

	// Test with client version newer than daemon error
	mockData = &Data{
		DockerServiceActive: false,
		Containers: []DockerContainer{
			{ID: "test-id"},
		},
		ts:      time.Now(),
		err:     errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
		healthy: false,
		reason:  "not supported; Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43 (needs upgrading docker daemon in the host)",
	}

	comp.lastMu.Lock()
	comp.lastData = mockData
	comp.lastMu.Unlock()

	// Get states and verify special error message
	states, err = comp.HealthStates(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Contains(t, states[0].Reason, "not supported")
	assert.Contains(t, states[0].Reason, "needs upgrading docker daemon")
}

// TestDirectCheckOnce directly tests the CheckOnce method with various conditions
func TestDirectCheckOnce(t *testing.T) {
	ctx := context.Background()

	// Test case 1: Docker is running successfully
	t.Run("Docker running successfully", func(t *testing.T) {
		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkDockerRunningFunc: func(context.Context) bool {
				return true
			},
			listContainersFunc: func(context.Context) ([]DockerContainer, error) {
				return []DockerContainer{{ID: "container1", Name: "test-container-1"}}, nil
			},
			lastData: &Data{},
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify data was saved
		comp.lastMu.RLock()
		assert.NotNil(t, comp.lastData)
		assert.Equal(t, time.Now().UTC().Format("2006-01-02"), comp.lastData.ts.Format("2006-01-02"))
		comp.lastMu.RUnlock()
	})

	// Test case 2: Docker service is not active
	t.Run("Docker service not active", func(t *testing.T) {
		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkServiceActiveFunc: func() (bool, error) {
				return false, nil
			},
			checkDockerRunningFunc: func(context.Context) bool {
				return true
			},
			lastData: &Data{},
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify error handling
		comp.lastMu.RLock()
		assert.False(t, comp.lastData.DockerServiceActive)
		assert.False(t, comp.lastData.healthy)
		assert.Contains(t, comp.lastData.reason, "docker service is not active")
		comp.lastMu.RUnlock()
	})

	// Test case 3: Docker is not running
	t.Run("Docker not running", func(t *testing.T) {
		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkDockerRunningFunc: func(context.Context) bool {
				return false
			},
			lastData: &Data{},
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify error handling
		comp.lastMu.RLock()
		assert.False(t, comp.lastData.healthy)
		assert.Contains(t, comp.lastData.reason, "docker installed but docker is not running")
		comp.lastMu.RUnlock()
	})

	// Test case 4: Error listing containers
	t.Run("Error listing containers", func(t *testing.T) {
		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkDockerRunningFunc: func(context.Context) bool {
				return true
			},
			checkServiceActiveFunc: func() (bool, error) {
				return true, nil
			},
			listContainersFunc: func(context.Context) ([]DockerContainer, error) {
				return nil, errors.New("listing error")
			},
			lastData: &Data{},
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify error handling
		comp.lastMu.RLock()
		assert.False(t, comp.lastData.healthy)
		assert.Contains(t, comp.lastData.reason, "error listing containers")
		comp.lastMu.RUnlock()
	})

	// Test case 5: Client version newer than daemon
	t.Run("Client version newer than daemon", func(t *testing.T) {
		versionErr := errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43")

		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkDockerRunningFunc: func(context.Context) bool {
				return true
			},
			checkServiceActiveFunc: func() (bool, error) {
				return true, nil
			},
			listContainersFunc: func(context.Context) ([]DockerContainer, error) {
				return nil, versionErr
			},
			lastData: &Data{},
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify special error handling
		comp.lastMu.RLock()
		assert.False(t, comp.lastData.healthy)
		assert.Contains(t, comp.lastData.reason, "not supported")
		assert.Contains(t, comp.lastData.reason, "needs upgrading docker daemon")
		comp.lastMu.RUnlock()
	})

	// Test case 6: Connection error with ignoreConnectionErrors=true
	t.Run("Connection error ignored", func(t *testing.T) {
		connErr := errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")

		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkDockerRunningFunc: func(context.Context) bool {
				return true
			},
			checkServiceActiveFunc: func() (bool, error) {
				return true, nil
			},
			listContainersFunc: func(context.Context) ([]DockerContainer, error) {
				return nil, connErr
			},
			ignoreConnectionErrors: true,
			lastData:               &Data{},
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify connection error handling with ignoreConnectionErrors=true
		comp.lastMu.RLock()
		assert.True(t, comp.lastData.healthy)
		assert.Contains(t, comp.lastData.reason, "connection error to docker daemon")
		comp.lastMu.RUnlock()
	})

	// Test case 7: Connection error with ignoreConnectionErrors=false
	t.Run("Connection error not ignored", func(t *testing.T) {
		connErr := errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")

		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkDockerRunningFunc: func(context.Context) bool {
				return true
			},
			checkServiceActiveFunc: func() (bool, error) {
				return true, nil
			},
			listContainersFunc: func(context.Context) ([]DockerContainer, error) {
				return nil, connErr
			},
			ignoreConnectionErrors: false,
			lastData:               &Data{},
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify connection error handling with ignoreConnectionErrors=false
		comp.lastMu.RLock()
		assert.False(t, comp.lastData.healthy)
		assert.Contains(t, comp.lastData.reason, "connection error to docker daemon")
		comp.lastMu.RUnlock()
	})

	// Test case 8: Successful container list
	t.Run("Successful container list", func(t *testing.T) {
		containers := []DockerContainer{
			{ID: "container1", Name: "test-container-1"},
			{ID: "container2", Name: "test-container-2"},
		}

		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalledFunc: func() bool {
				return true
			},
			checkDockerRunningFunc: func(context.Context) bool {
				return true
			},
			checkServiceActiveFunc: func() (bool, error) {
				return true, nil
			},
			listContainersFunc: func(context.Context) ([]DockerContainer, error) {
				return containers, nil
			},
			lastData: &Data{},
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify successful container list
		comp.lastMu.RLock()
		assert.True(t, comp.lastData.healthy)
		assert.Equal(t, containers, comp.lastData.Containers)
		assert.Contains(t, comp.lastData.reason, "total 2 container")
		comp.lastMu.RUnlock()
	})
}
