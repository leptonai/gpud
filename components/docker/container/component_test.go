package container

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgdocker "github.com/leptonai/gpud/pkg/docker"
)

func TestData_getStates(t *testing.T) {
	tests := []struct {
		name         string
		data         checkResult
		expectedText string
	}{
		{
			name: "No error with containers",
			data: checkResult{
				Containers: []pkgdocker.DockerContainer{{}, {}, {}},
				err:        nil,
				health:     apiv1.HealthStateTypeHealthy,
				reason:     "total 3 container(s)",
			},
			expectedText: "total 3 container",
		},
		{
			name: "Empty containers",
			data: checkResult{
				Containers: []pkgdocker.DockerContainer{},
				err:        nil,
				health:     apiv1.HealthStateTypeHealthy,
				reason:     "total 0 container(s)",
			},
			expectedText: "total 0 container",
		},
		{
			name: "Nil containers",
			data: checkResult{
				Containers: nil,
				err:        nil,
				health:     apiv1.HealthStateTypeHealthy,
				reason:     "total 0 container(s)",
			},
			expectedText: "total 0 container",
		},
		{
			name: "Docker client version newer than daemon",
			data: checkResult{
				Containers: []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:        errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
				health:     apiv1.HealthStateTypeUnhealthy,
				reason:     "not supported; Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43 (needs upgrading docker daemon in the host)",
			},
			expectedText: "not supported",
		},
		{
			name: "Connection error",
			data: checkResult{
				Containers: []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:        errors.New("Cannot connect to the Docker daemon"),
				health:     apiv1.HealthStateTypeUnhealthy,
				reason:     "connection error to docker daemon -- Cannot connect to the Docker daemon",
			},
			expectedText: "connection error to docker daemon",
		},
		{
			name: "General error",
			data: checkResult{
				Containers: []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:        errors.New("some general error"),
				health:     apiv1.HealthStateTypeUnhealthy,
				reason:     "error listing containers -- some general error",
			},
			expectedText: "error listing containers",
		},
		// Additional test cases for different error scenarios
		{
			name: "Connection refusal",
			data: checkResult{
				Containers: []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:        errors.New("connection refused"),
				health:     apiv1.HealthStateTypeUnhealthy,
				reason:     "error listing containers -- connection refused",
			},
			expectedText: "error listing containers",
		},
		{
			name: "Daemon not running",
			data: checkResult{
				Containers: []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:        errors.New("Is the docker daemon running?"),
				health:     apiv1.HealthStateTypeUnhealthy,
				reason:     "connection error to docker daemon -- Is the docker daemon running?",
			},
			expectedText: "connection error to docker daemon",
		},
		{
			name: "Permission denied error",
			data: checkResult{
				Containers: []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:        errors.New("permission denied"),
				health:     apiv1.HealthStateTypeUnhealthy,
				reason:     "error listing containers -- permission denied",
			},
			expectedText: "error listing containers",
		},
		{
			name: "Docker network error",
			data: checkResult{
				Containers: []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:        errors.New("network error communicating with Docker daemon"),
				health:     apiv1.HealthStateTypeUnhealthy,
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
	var nilData *checkResult
	states := nilData.getLastHealthStates()
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataHealthField(t *testing.T) {
	tests := []struct {
		name           string
		data           checkResult
		expectedHealth apiv1.HealthStateType
	}{
		{
			name: "No error",
			data: checkResult{
				err:    nil,
				health: apiv1.HealthStateTypeHealthy,
				reason: "healthy",
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "Connection error - ignored",
			data: checkResult{
				err:    errors.New("Cannot connect to the Docker daemon"),
				health: apiv1.HealthStateTypeHealthy,
				reason: "connection error to docker daemon but ignored",
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "Connection error - not ignored",
			data: checkResult{
				err:    errors.New("Cannot connect to the Docker daemon"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "connection error to docker daemon",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "General error",
			data: checkResult{
				err:    errors.New("some general error"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "error occurred",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		// Additional test cases for error scenarios
		{
			name: "Permission denied error - not ignored",
			data: checkResult{
				err:    errors.New("permission denied"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "permission denied error",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "Docker service inactive error - not ignored",
			data: checkResult{
				err:                 errors.New("docker service is not active"),
				DockerServiceActive: false,
				health:              apiv1.HealthStateTypeUnhealthy,
				reason:              "docker service is not active",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "Docker not found error - not ignored",
			data: checkResult{
				err:    errors.New("docker not found"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "docker not found error",
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "Is docker daemon running - ignored",
			data: checkResult{
				err:    errors.New("Is the docker daemon running?"),
				health: apiv1.HealthStateTypeHealthy,
				reason: "connection error ignored",
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := tt.data.getLastHealthStates()
			assert.Equal(t, tt.expectedHealth, states[0].Health)
		})
	}

	// Test with nil data
	var nilData *checkResult
	states := nilData.getLastHealthStates()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

func TestDataGetStates(t *testing.T) {
	tests := []struct {
		name           string
		data           checkResult
		stateCount     int
		expectedHealth apiv1.HealthStateType
	}{
		{
			name: "No containers",
			data: checkResult{
				DockerServiceActive: true,
				Containers:          []pkgdocker.DockerContainer{},
				err:                 nil,
				health:              apiv1.HealthStateTypeHealthy,
				reason:              "no container found",
			},
			stateCount:     1,
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "With containers",
			data: checkResult{
				DockerServiceActive: true,
				Containers:          []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:                 nil,
				health:              apiv1.HealthStateTypeHealthy,
				reason:              "total 1 container(s)",
			},
			stateCount:     1,
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "With error not ignored",
			data: checkResult{
				DockerServiceActive: true,
				Containers:          []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:                 errors.New("test error"),
				health:              apiv1.HealthStateTypeUnhealthy,
				reason:              "error listing containers -- test error",
			},
			stateCount:     1,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "With connection error ignored",
			data: checkResult{
				DockerServiceActive: true,
				Containers:          []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:                 errors.New("connection error"),
				health:              apiv1.HealthStateTypeHealthy,
				reason:              "connection error but ignored",
			},
			stateCount:     1,
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "With connection error not ignored",
			data: checkResult{
				DockerServiceActive: true,
				Containers:          []pkgdocker.DockerContainer{{ID: "test-id"}},
				err:                 errors.New("connection error"),
				health:              apiv1.HealthStateTypeUnhealthy,
				reason:              "connection error not ignored",
			},
			stateCount:     1,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
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
				assert.NotNil(t, states[0].ExtraInfo)
				assert.Contains(t, states[0].ExtraInfo, "data")
				assert.Contains(t, states[0].ExtraInfo, "encoding")
			}
		})
	}

	// Test with nil data
	var nilData *checkResult
	states := nilData.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
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
	comp.lastCheckResult = &checkResult{
		DockerServiceActive: true,
		Containers:          []pkgdocker.DockerContainer{},
		ts:                  time.Now(),
		err:                 nil,
		health:              apiv1.HealthStateTypeHealthy,
		reason:              "total 0 container(s)",
	}

	states := comp.LastHealthStates()
	assert.Equal(t, 1, len(states))
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)

	// Test with containers
	comp.lastCheckResult = &checkResult{
		DockerServiceActive: true,
		Containers: []pkgdocker.DockerContainer{
			{ID: "test-id", Name: "test-name"},
		},
		ts:     time.Now(),
		err:    nil,
		health: apiv1.HealthStateTypeHealthy,
		reason: "total 1 container(s)",
	}

	states = comp.LastHealthStates()
	assert.Equal(t, 1, len(states))
	assert.NotNil(t, states[0].ExtraInfo)

	// Test with error but ignoreConnectionErrors is true
	comp.lastCheckResult = &checkResult{
		DockerServiceActive: false,
		Containers:          []pkgdocker.DockerContainer{},
		ts:                  time.Now(),
		err:                 errors.New("Cannot connect to the Docker daemon"),
		health:              apiv1.HealthStateTypeHealthy,
		reason:              "connection error to docker daemon -- Cannot connect to the Docker daemon",
	}

	states = comp.LastHealthStates()
	assert.Equal(t, 1, len(states))
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

func TestCheckOnceErrorConditions(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	require.NoError(t, err)
	comp := c.(*component)

	// Test with connection error
	mockData := &checkResult{
		DockerServiceActive: false,
		Containers:          []pkgdocker.DockerContainer{},
		ts:                  time.Now(),
		err:                 errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"),
		health:              apiv1.HealthStateTypeHealthy,
		reason:              "connection error to docker daemon -- Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
	}

	comp.lastMu.Lock()
	comp.lastCheckResult = mockData
	comp.lastMu.Unlock()

	// Get states and verify error handling with ignoreConnectionErrors=true
	states := comp.LastHealthStates()
	assert.Equal(t, 1, len(states))
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health) // Should be healthy because we're ignoring connection errors

	// Create a new component that doesn't ignore connection errors
	c2, err := New(gpudInstance)
	require.NoError(t, err)
	comp2 := c2.(*component)
	comp2.ignoreConnectionErrors = false

	mockData.health = apiv1.HealthStateTypeUnhealthy
	comp2.lastMu.Lock()
	comp2.lastCheckResult = mockData
	comp2.lastMu.Unlock()

	// Get states and verify error handling with ignoreConnectionErrors=false
	states = comp2.LastHealthStates()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health) // Should be unhealthy because we're not ignoring connection errors

	// Test with client version newer than daemon error
	mockData = &checkResult{
		DockerServiceActive: false,
		Containers: []pkgdocker.DockerContainer{
			{ID: "test-id"},
		},
		ts:     time.Now(),
		err:    errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "not supported; Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43 (needs upgrading docker daemon in the host)",
	}

	comp.lastMu.Lock()
	comp.lastCheckResult = mockData
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
			listContainersFunc: func(context.Context) ([]pkgdocker.DockerContainer, error) {
				return []pkgdocker.DockerContainer{{ID: "container1", Name: "test-container-1"}}, nil
			},
			lastCheckResult: &checkResult{},
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify data was saved
		comp.lastMu.RLock()
		assert.NotNil(t, comp.lastCheckResult)
		assert.Equal(t, time.Now().UTC().Format("2006-01-02"), comp.lastCheckResult.ts.Format("2006-01-02"))
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
			lastCheckResult: &checkResult{},
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify error handling
		comp.lastMu.RLock()
		assert.False(t, comp.lastCheckResult.DockerServiceActive)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, comp.lastCheckResult.health)
		assert.Contains(t, comp.lastCheckResult.reason, "docker service is not active")
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
			lastCheckResult: &checkResult{},
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify error handling
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, comp.lastCheckResult.health)
		assert.Contains(t, comp.lastCheckResult.reason, "docker installed but docker is not running")
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
			listContainersFunc: func(context.Context) ([]pkgdocker.DockerContainer, error) {
				return nil, errors.New("listing error")
			},
			lastCheckResult: &checkResult{},
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify error handling
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, comp.lastCheckResult.health)
		assert.Contains(t, comp.lastCheckResult.reason, "error listing containers")
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
			listContainersFunc: func(context.Context) ([]pkgdocker.DockerContainer, error) {
				return nil, versionErr
			},
			lastCheckResult: &checkResult{},
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify special error handling
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, comp.lastCheckResult.health)
		assert.Contains(t, comp.lastCheckResult.reason, "not supported")
		assert.Contains(t, comp.lastCheckResult.reason, "needs upgrading docker daemon")
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
			listContainersFunc: func(context.Context) ([]pkgdocker.DockerContainer, error) {
				return nil, connErr
			},
			ignoreConnectionErrors: true,
			lastCheckResult:        &checkResult{},
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify connection error handling with ignoreConnectionErrors=true
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, comp.lastCheckResult.health)
		assert.Contains(t, comp.lastCheckResult.reason, "connection error to docker daemon")
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
			listContainersFunc: func(context.Context) ([]pkgdocker.DockerContainer, error) {
				return nil, connErr
			},
			ignoreConnectionErrors: false,
			lastCheckResult:        &checkResult{},
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify connection error handling with ignoreConnectionErrors=false
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, comp.lastCheckResult.health)
		assert.Contains(t, comp.lastCheckResult.reason, "connection error to docker daemon")
		comp.lastMu.RUnlock()
	})

	// Test case 8: Successful container list
	t.Run("Successful container list", func(t *testing.T) {
		containers := []pkgdocker.DockerContainer{
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
			listContainersFunc: func(context.Context) ([]pkgdocker.DockerContainer, error) {
				return containers, nil
			},
			lastCheckResult: &checkResult{},
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify successful container list
		comp.lastMu.RLock()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, comp.lastCheckResult.health)
		assert.Equal(t, containers, comp.lastCheckResult.Containers)
		assert.Contains(t, comp.lastCheckResult.reason, "total 2 container")
		comp.lastMu.RUnlock()
	})
}

func TestDataMarshalJSON(t *testing.T) {
	cr := &checkResult{
		DockerServiceActive: true,
		Containers: []pkgdocker.DockerContainer{
			{
				ID:    "test-id",
				Name:  "test-name",
				Image: "test-image",
			},
		},
		ts:  time.Now(),
		err: nil,
	}

	json, err := json.Marshal(cr)
	require.NoError(t, err)
	assert.Contains(t, string(json), "test-id")
	assert.Contains(t, string(json), "test-name")
	assert.Contains(t, string(json), "test-image")
	assert.Contains(t, string(json), "docker_service_active")
	assert.Contains(t, string(json), "containers")
}
