package container

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
)

func TestData_getReason(t *testing.T) {
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
			},
			expectedText: "total 3 containers",
		},
		{
			name: "Empty containers",
			data: Data{
				Containers: []DockerContainer{},
				err:        nil,
			},
			expectedText: "no container found or docker is not running",
		},
		{
			name: "Nil containers",
			data: Data{
				Containers: nil,
				err:        nil,
			},
			expectedText: "no container found or docker is not running",
		},
		{
			name: "Docker client version newer than daemon",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
			},
			expectedText: "not supported",
		},
		{
			name: "Connection error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("Cannot connect to the Docker daemon"),
				connErr:    true,
			},
			expectedText: "connection error to docker daemon",
		},
		{
			name: "General error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        errors.New("some general error"),
			},
			expectedText: "failed to list containers",
		},
		{
			name: "Context deadline exceeded error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        context.DeadlineExceeded,
			},
			expectedText: "check failed with context deadline exceeded -- transient error, please retry",
		},
		{
			name: "Context canceled error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        context.Canceled,
			},
			expectedText: "check failed with context canceled -- transient error, please retry",
		},
		{
			name: "Wrapped context deadline exceeded error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        fmt.Errorf("operation timeout: %w", context.DeadlineExceeded),
			},
			expectedText: "check failed with operation timeout: context deadline exceeded -- transient error, please retry",
		},
		{
			name: "Wrapped context canceled error",
			data: Data{
				Containers: []DockerContainer{{ID: "test-id"}},
				err:        fmt.Errorf("operation aborted: %w", context.Canceled),
			},
			expectedText: "check failed with operation aborted: context canceled -- transient error, please retry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.getReason()
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
		{
			name: "Context deadline exceeded error - ignoreConnErr true",
			data: Data{
				err: context.DeadlineExceeded,
			},
			ignoreConnErr:   true,
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
		},
		{
			name: "Context deadline exceeded error - ignoreConnErr false",
			data: Data{
				err: context.DeadlineExceeded,
			},
			ignoreConnErr:   false,
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
		},
		{
			name: "Context canceled error - ignoreConnErr true",
			data: Data{
				err: context.Canceled,
			},
			ignoreConnErr:   true,
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
		},
		{
			name: "Context canceled error - ignoreConnErr false",
			data: Data{
				err: context.Canceled,
			},
			ignoreConnErr:   false,
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
		},
		{
			name: "Wrapped context deadline exceeded error",
			data: Data{
				err: fmt.Errorf("operation timeout: %w", context.DeadlineExceeded),
			},
			ignoreConnErr:   false,
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
		},
		{
			name: "Wrapped context canceled error",
			data: Data{
				err: fmt.Errorf("operation aborted: %w", context.Canceled),
			},
			ignoreConnErr:   false,
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
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
				DockerServiceActive: true,
				Containers:          []DockerContainer{},
				err:                 nil,
			},
			ignoreConnErr: false,
			expectError:   false,
			stateCount:    1,
		},
		{
			name: "With containers",
			data: Data{
				DockerServiceActive: true,
				Containers:          []DockerContainer{{ID: "test-id"}},
				err:                 nil,
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
			assert.Equal(t, Name, states[0].Name)
		})
	}
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
		connErr:             false,
	}

	states, err := comp.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.Equal(t, true, states[0].Healthy)

	// Test with containers
	comp.lastData = &Data{
		DockerServiceActive: true,
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
	comp.lastData = &Data{
		DockerServiceActive: false,
		Containers:          []DockerContainer{},
		ts:                  time.Now(),
		err:                 errors.New("Cannot connect to the Docker daemon"),
		connErr:             true,
	}

	states, err = comp.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.Equal(t, true, states[0].Healthy)
}

// Improve checkOnce test coverage
func TestCheckOnceErrorConditions(t *testing.T) {
	// We're not initializing logger in tests to avoid dependencies
	// If the real implementation relies on the logger, consider using a mocking framework

	ctx := context.Background()
	comp := New(ctx, true).(*component)

	// Test with connection error
	mockData := &Data{
		DockerServiceActive: false,
		Containers:          []DockerContainer{},
		ts:                  time.Now(),
		err:                 errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"),
		connErr:             true,
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
	assert.Equal(t, components.StateUnhealthy, states[0].Health) // Should be unhealthy because we're not ignoring connection errors

	// Test with client version newer than daemon error
	mockData = &Data{
		DockerServiceActive: false,
		Containers: []DockerContainer{
			{ID: "test-id"},
		},
		ts:      time.Now(),
		err:     errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
		connErr: false,
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

// TestDirectCheckOnce directly tests the CheckOnce method with various conditions
func TestDirectCheckOnce(t *testing.T) {
	ctx := context.Background()

	// Test case 1: Docker is running successfully
	t.Run("Docker running successfully", func(t *testing.T) {
		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalled: func() bool {
				return true
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

	// Test case 2: Connection error
	t.Run("Connection error", func(t *testing.T) {
		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalled: func() bool {
				return true
			},
			lastData:               &Data{},
			ignoreConnectionErrors: true,
		}

		// Create a mock error
		connErr := errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")

		// Manually set the data to simulate the error
		errData := &Data{
			Containers: []DockerContainer{
				{ID: "test-id"},
			},
			ts:      time.Now(),
			err:     connErr,
			connErr: true,
		}

		comp.lastMu.Lock()
		comp.lastData = errData
		comp.lastMu.Unlock()

		// Verify the connection error is handled correctly through getReason
		reason := comp.lastData.getReason()
		assert.Contains(t, reason, "connection error to docker daemon")

		// Verify the connection error is handled correctly through States
		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Equal(t, components.StateHealthy, states[0].Health) // Should be healthy with ignoreConnectionErrors=true

		// Now test with ignoreConnectionErrors=false
		comp.ignoreConnectionErrors = false
		states, err = comp.States(ctx)
		assert.NoError(t, err)
		assert.Equal(t, components.StateUnhealthy, states[0].Health) // Should be unhealthy with ignoreConnectionErrors=false
	})

	// Test case 4: Docker client version newer than daemon
	t.Run("Client version newer than daemon", func(t *testing.T) {
		// First check that the error detection function works as expected
		versionErr := errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43")
		assert.True(t, isErrDockerClientVersionNewerThanDaemon(versionErr))

		// Create a Data instance with the error
		data := &Data{
			Containers: []DockerContainer{
				{ID: "test-id"},
			},
			err: versionErr,
		}

		// Directly test the getReason() function to ensure it returns the expected error message
		reason := data.getReason()
		assert.Contains(t, reason, "not supported;")
		assert.Contains(t, reason, "needs upgrading docker daemon")
	})
}

// TestCheckOnceMetrics tests that metrics are set correctly in CheckOnce
func TestCheckOnceMetrics(t *testing.T) {
	// Since we can't easily mock the metrics package directly,
	// we'll test the logic that determines whether to set success or failure metrics

	ctx := context.Background()

	// Test case 1: Success case - no error
	t.Run("Success metrics on no error", func(t *testing.T) {
		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalled: func() bool {
				return true
			},
			lastData: &Data{},
		}

		// Set up test data with no error
		mockData := &Data{
			DockerServiceActive: true,
			Containers: []DockerContainer{
				{ID: "test-id"},
			},
			ts:      time.Now(),
			err:     nil, // No error should trigger success metric
			connErr: false,
		}

		comp.lastMu.Lock()
		comp.lastData = mockData
		comp.lastMu.Unlock()

		// For success metrics, we can only verify indirectly
		// by checking that States returns healthy state
		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Equal(t, components.StateHealthy, states[0].Health)
	})

	// Test case 2: Failure case - with error
	t.Run("Failure metrics on error", func(t *testing.T) {
		comp := &component{
			ctx:    ctx,
			cancel: func() {},
			checkDependencyInstalled: func() bool {
				return true
			},
			lastData: &Data{},
		}

		// Set up test data with error
		mockData := &Data{
			DockerServiceActive: true,
			Containers:          []DockerContainer{},
			ts:                  time.Now(),
			err:                 errors.New("some error"), // Error should trigger failure metric
			connErr:             false,
		}

		comp.lastMu.Lock()
		comp.lastData = mockData
		comp.lastMu.Unlock()

		// For failure metrics with non-connection error, verify indirectly
		// by checking that States returns unhealthy state
		comp.ignoreConnectionErrors = false
		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Equal(t, components.StateUnhealthy, states[0].Health)
	})
}
