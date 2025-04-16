package container

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
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
				health:     apiv1.StateTypeHealthy,
				reason:     "total 3 container(s)",
			},
			expectedText: "total 3 container",
		},
		{
			name: "Empty containers",
			data: Data{
				Containers: []DockerContainer{},
				err:        nil,
				health:     apiv1.StateTypeHealthy,
				reason:     "total 0 container(s)",
			},
			expectedText: "total 0 container",
		},
		{
			name: "Nil containers",
			data: Data{
				Containers: nil,
				err:        nil,
				health:     apiv1.StateTypeHealthy,
				reason:     "total 0 container(s)",
			},
			expectedText: "total 0 container",
		},
		{
			name: "Docker client version newer than daemon",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
				health:     apiv1.StateTypeUnhealthy,
				reason:     "not supported; Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43 (needs upgrading docker daemon in the host)",
			},
			expectedText: "not supported",
		},
		{
			name: "Connection error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("Cannot connect to the Docker daemon"),
				health:     apiv1.StateTypeUnhealthy,
				reason:     "connection error to docker daemon -- Cannot connect to the Docker daemon",
			},
			expectedText: "connection error to docker daemon",
		},
		{
			name: "General error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("some general error"),
				health:     apiv1.StateTypeUnhealthy,
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
				health:     apiv1.StateTypeUnhealthy,
				reason:     "error listing containers -- connection refused",
			},
			expectedText: "error listing containers",
		},
		{
			name: "Daemon not running",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("Is the docker daemon running?"),
				health:     apiv1.StateTypeUnhealthy,
				reason:     "connection error to docker daemon -- Is the docker daemon running?",
			},
			expectedText: "connection error to docker daemon",
		},
		{
			name: "Permission denied error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("permission denied"),
				health:     apiv1.StateTypeUnhealthy,
				reason:     "error listing containers -- permission denied",
			},
			expectedText: "error listing containers",
		},
		{
			name: "Docker network error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("network error communicating with Docker daemon"),
				health:     apiv1.StateTypeUnhealthy,
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
	states := nilData.getLastHealthStates()
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataHealthField(t *testing.T) {
	tests := []struct {
		name           string
		data           Data
		expectedHealth apiv1.HealthStateType
	}{
		{
			name: "No error",
			data: Data{
				err:    nil,
				health: apiv1.StateTypeHealthy,
				reason: "healthy",
			},
			expectedHealth: apiv1.StateTypeHealthy,
		},
		{
			name: "Connection error - ignored",
			data: Data{
				err:    errors.New("Cannot connect to the Docker daemon"),
				health: apiv1.StateTypeHealthy,
				reason: "connection error to docker daemon but ignored",
			},
			expectedHealth: apiv1.StateTypeHealthy,
		},
		{
			name: "Connection error - not ignored",
			data: Data{
				err:    errors.New("Cannot connect to the Docker daemon"),
				health: apiv1.StateTypeUnhealthy,
				reason: "connection error to docker daemon",
			},
			expectedHealth: apiv1.StateTypeUnhealthy,
		},
		{
			name: "General error",
			data: Data{
				err:    errors.New("some general error"),
				health: apiv1.StateTypeUnhealthy,
				reason: "error occurred",
			},
			expectedHealth: apiv1.StateTypeUnhealthy,
		},
		// Additional test cases for error scenarios
		{
			name: "Permission denied error - not ignored",
			data: Data{
				err:    errors.New("permission denied"),
				health: apiv1.StateTypeUnhealthy,
				reason: "permission denied error",
			},
			expectedHealth: apiv1.StateTypeUnhealthy,
		},
		{
			name: "Docker service inactive error - not ignored",
			data: Data{
				err:                 errors.New("docker service is not active"),
				DockerServiceActive: false,
				health:              apiv1.StateTypeUnhealthy,
				reason:              "docker service is not active",
			},
			expectedHealth: apiv1.StateTypeUnhealthy,
		},
		{
			name: "Docker not found error - not ignored",
			data: Data{
				err:    errors.New("docker not found"),
				health: apiv1.StateTypeUnhealthy,
				reason: "docker not found error",
			},
			expectedHealth: apiv1.StateTypeUnhealthy,
		},
		{
			name: "Is docker daemon running - ignored",
			data: Data{
				err:    errors.New("Is the docker daemon running?"),
				health: apiv1.StateTypeHealthy,
				reason: "connection error ignored",
			},
			expectedHealth: apiv1.StateTypeHealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := tt.data.getLastHealthStates()
			assert.Equal(t, tt.expectedHealth, states[0].Health)
		})
	}

	// Test with nil data
	var nilData *Data
	states := nilData.getLastHealthStates()
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
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
				health:              apiv1.StateTypeHealthy,
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
				health:              apiv1.StateTypeHealthy,
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
				health:              apiv1.StateTypeUnhealthy,
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
				health:              apiv1.StateTypeHealthy,
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
				health:              apiv1.StateTypeUnhealthy,
				reason:              "connection error not ignored",
			},
			stateCount:     1,
			expectedHealth: apiv1.StateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := tt.data.getLastHealthStates()

			assert.Equal(t, tt.stateCount, len(states))
			assert.Equal(t, Name, states[0].Name)
			assert.Equal(t, tt.expectedHealth, states[0].Health)
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
	states := nilData.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestComponentName(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	require.NoError(t, err)
	assert.Equal(t, Name, c.Name())
}

func TestComponentStart(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	require.NoError(t, err)
	err = c.Start()
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
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	require.NoError(t, err)
	events, err := c.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestComponentClose(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	require.NoError(t, err)
	err = c.Close()
	assert.NoError(t, err)
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}

	c, err := New(gpudInstance)
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.Equal(t, Name, c.Name())

	// Create a new instance with different ignoreConnectionErrors
	comp := c.(*component)
	comp.ignoreConnectionErrors = false
	assert.False(t, comp.ignoreConnectionErrors)
}

func TestComponentStates(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	require.NoError(t, err)
	comp := c.(*component)

	// Test with empty data
	comp.lastData = &Data{
		DockerServiceActive: true,
		Containers:          []DockerContainer{},
		ts:                  time.Now(),
		err:                 nil,
		health:              apiv1.StateTypeHealthy,
		reason:              "total 0 container(s)",
	}

	states := comp.LastHealthStates()
	assert.Equal(t, 1, len(states))
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)

	// Test with containers
	comp.lastData = &Data{
		DockerServiceActive: true,
		Containers: []DockerContainer{
			{ID: "test-id", Name: "test-name"},
		},
		ts:     time.Now(),
		err:    nil,
		health: apiv1.StateTypeHealthy,
		reason: "total 1 container(s)",
	}

	states = comp.LastHealthStates()
	assert.Equal(t, 1, len(states))
	assert.NotNil(t, states[0].DeprecatedExtraInfo)

	// Test with error but ignoreConnectionErrors is true
	comp.lastData = &Data{
		DockerServiceActive: false,
		Containers:          []DockerContainer{},
		ts:                  time.Now(),
		err:                 errors.New("Cannot connect to the Docker daemon"),
		health:              apiv1.StateTypeHealthy,
		reason:              "connection error to docker daemon -- Cannot connect to the Docker daemon",
	}

	states = comp.LastHealthStates()
	assert.Equal(t, 1, len(states))
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
}

func TestCheckOnceErrorConditions(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	require.NoError(t, err)
	comp := c.(*component)

	// Test with connection error
	mockData := &Data{
		DockerServiceActive: false,
		Containers:          []DockerContainer{},
		ts:                  time.Now(),
		err:                 errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"),
		health:              apiv1.StateTypeHealthy,
		reason:              "connection error to docker daemon -- Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
	}

	comp.lastMu.Lock()
	comp.lastData = mockData
	comp.lastMu.Unlock()

	// Get states and verify error handling with ignoreConnectionErrors=true
	states := comp.LastHealthStates()
	assert.Equal(t, 1, len(states))
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health) // Should be healthy because we're ignoring connection errors

	// Create a new component that doesn't ignore connection errors
	c2, err := New(gpudInstance)
	require.NoError(t, err)
	comp2 := c2.(*component)
	comp2.ignoreConnectionErrors = false

	mockData.health = apiv1.StateTypeUnhealthy
	comp2.lastMu.Lock()
	comp2.lastData = mockData
	comp2.lastMu.Unlock()

	// Get states and verify error handling with ignoreConnectionErrors=false
	states = comp2.LastHealthStates()
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health) // Should be unhealthy because we're not ignoring connection errors

	// Test with client version newer than daemon error
	mockData = &Data{
		DockerServiceActive: false,
		Containers: []DockerContainer{
			{ID: "test-id"},
		},
		ts:     time.Now(),
		err:    errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
		health: apiv1.StateTypeUnhealthy,
		reason: "not supported; Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43 (needs upgrading docker daemon in the host)",
	}

	comp.lastMu.Lock()
	comp.lastData = mockData
	comp.lastMu.Unlock()

	// Get states and verify special error message
	states = comp.LastHealthStates()
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
		_ = comp.Check()

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
		_ = comp.Check()

		// Verify error handling
		comp.lastMu.RLock()
		assert.False(t, comp.lastData.DockerServiceActive)
		assert.Equal(t, apiv1.StateTypeUnhealthy, comp.lastData.health)
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
		_ = comp.Check()

		// Verify error handling
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.StateTypeUnhealthy, comp.lastData.health)
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
		_ = comp.Check()

		// Verify error handling
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.StateTypeUnhealthy, comp.lastData.health)
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
		_ = comp.Check()

		// Verify special error handling
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.StateTypeUnhealthy, comp.lastData.health)
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
		_ = comp.Check()

		// Verify connection error handling with ignoreConnectionErrors=true
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.StateTypeHealthy, comp.lastData.health)
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
		_ = comp.Check()

		// Verify connection error handling with ignoreConnectionErrors=false
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.StateTypeUnhealthy, comp.lastData.health)
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
		_ = comp.Check()

		// Verify successful container list
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.StateTypeHealthy, comp.lastData.health)
		assert.Equal(t, containers, comp.lastData.Containers)
		assert.Contains(t, comp.lastData.reason, "total 2 container")
		comp.lastMu.RUnlock()
	})
}
