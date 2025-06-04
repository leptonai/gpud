package nfs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestNewComponent(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(&components.GPUdInstance{
		RootCtx:    ctx,
		EventStore: store,
		MachineID:  "test-machine",
	})
	require.NoError(t, err)
	defer comp.Close()

	assert.Equal(t, Name, comp.Name())
	assert.True(t, comp.IsSupported())

	err = comp.Close()
	require.NoError(t, err)
}

func TestComponentName(t *testing.T) {
	c := &component{}
	assert.Equal(t, Name, c.Name())
	assert.Equal(t, "nfs", c.Name())
}

func TestTags(t *testing.T) {
	c := &component{}

	expectedTags := []string{
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 1, "Component should return exactly 1 tag")
}

func TestIsSupported(t *testing.T) {
	c := &component{}
	assert.True(t, c.IsSupported())
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		},
	}

	err := c.Start()
	assert.NoError(t, err)

	// Allow some time for the goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Clean up
	c.Close()
}

func TestClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,
	}

	err := c.Close()
	assert.NoError(t, err)

	// Verify context is canceled
	require.Error(t, c.ctx.Err(), "Context should be canceled after Close()")
	assert.Equal(t, context.Canceled, c.ctx.Err())
}

func TestEvents(t *testing.T) {
	c := &component{}
	events, err := c.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestLastHealthStatesWithNoData(t *testing.T) {
	c := &component{}
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestLastHealthStatesWithData(t *testing.T) {
	testTime := time.Now().UTC()
	c := &component{
		lastCheckResult: &checkResult{
			ts:     testTime,
			health: apiv1.HealthStateTypeHealthy,
			reason: "test reason",
		},
	}

	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "test reason", states[0].Reason)
	assert.Equal(t, testTime, states[0].Time.Time)
}

func TestCheckWithNoConfigs(t *testing.T) {
	c := &component{
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		},
	}

	result := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Equal(t, "no nfs group configs found", result.Summary())
}

func TestCheckWithInvalidConfigs(t *testing.T) {
	c := &component{
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					Dir: "", // Invalid - empty dir
				},
			}
		},
	}

	result := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "invalid nfs group configs")
}

func TestCheckWithValidConfigs(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	c := &component{
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					Dir:              tmpDir,
					FileContents:     "test content",
					TTLToDelete:      metav1.Duration{Duration: time.Hour},
					NumExpectedFiles: 1,
				},
			}
		},
	}

	result := c.Check()
	cr := result.(*checkResult)

	// The check should succeed
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Len(t, cr.NFSCheckResults, 1)
	assert.Equal(t, tmpDir, cr.NFSCheckResults[0].Dir)
}

// Test checkResult methods

func TestCheckResultComponentName(t *testing.T) {
	cr := &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())
}

func TestCheckResultString(t *testing.T) {
	tests := []struct {
		name     string
		cr       *checkResult
		expected string
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name: "empty NFSCheckResults",
			cr: &checkResult{
				NFSCheckResults: []pkgnfschecker.CheckResult{},
			},
			expected: "",
		},
		{
			name: "with NFSCheckResults",
			cr: &checkResult{
				NFSCheckResults: []pkgnfschecker.CheckResult{
					{
						Dir:     "/test/dir1",
						Message: "success",
					},
					{
						Dir:     "/test/dir2",
						Message: "failed",
					},
				},
			},
			expected: "no devices with ACS enabled (ok)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cr.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckResultSummary(t *testing.T) {
	tests := []struct {
		name     string
		cr       *checkResult
		expected string
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name: "with reason",
			cr: &checkResult{
				reason: "test reason",
			},
			expected: "test reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cr.Summary())
		})
	}
}

func TestCheckResultHealthStateType(t *testing.T) {
	tests := []struct {
		name     string
		cr       *checkResult
		expected apiv1.HealthStateType
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name: "healthy",
			cr: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
			},
			expected: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy",
			cr: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expected: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cr.HealthStateType())
		})
	}
}

func TestCheckResultGetError(t *testing.T) {
	tests := []struct {
		name     string
		cr       *checkResult
		expected string
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name: "no error",
			cr: &checkResult{
				err: nil,
			},
			expected: "",
		},
		{
			name: "with error",
			cr: &checkResult{
				err: errors.New("test error"),
			},
			expected: "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cr.getError())
		})
	}
}

func TestCheckResultHealthStates(t *testing.T) {
	t.Run("nil checkResult", func(t *testing.T) {
		var cr *checkResult
		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})

	t.Run("with data", func(t *testing.T) {
		testTime := time.Now().UTC()
		testError := errors.New("test error")
		cr := &checkResult{
			ts:     testTime,
			err:    testError,
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "test reason",
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "test reason", states[0].Reason)
		assert.Equal(t, "test error", states[0].Error)
		assert.Equal(t, testTime, states[0].Time.Time)
	})
}

func TestCheckWithNFSCheckerError(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	c := &component{
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					Dir:              tmpDir,
					FileContents:     "test content",
					TTLToDelete:      metav1.Duration{Duration: time.Hour},
					NumExpectedFiles: 10, // Expect more files than we'll have
				},
			}
		},
	}

	result := c.Check()

	// The check should fail because we expect 10 files but only have 1
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "failed to check nfs checker")
}

func TestCheckResultInterface(t *testing.T) {
	// Verify that checkResult implements components.CheckResult interface
	var _ components.CheckResult = &checkResult{}
}

func TestComponentInterface(t *testing.T) {
	// Verify that component implements components.Component interface
	var _ components.Component = &component{}
}

func TestConcurrentAccess(t *testing.T) {
	c := &component{
		machineID: "test-machine",
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		},
	}

	// Test concurrent access to LastHealthStates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			states := c.LastHealthStates()
			assert.Len(t, states, 1)
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestComponentWithRealData(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(&components.GPUdInstance{
		RootCtx:    ctx,
		EventStore: store,
		MachineID:  "test-machine",
	})
	require.NoError(t, err)
	defer comp.Close()

	// Test the actual Check method
	result := comp.Check()
	assert.NotNil(t, result)

	// Should be healthy since no configs are set by default
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Equal(t, "no nfs group configs found", result.Summary())

	// Test that lastCheckResult is updated
	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}
