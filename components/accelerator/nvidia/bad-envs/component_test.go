package badenvs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
)

func TestNew(t *testing.T) {
	ctx := context.Background()
	c := New(ctx)
	assert.NotNil(t, c)
	assert.Equal(t, Name, c.Name())
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := New(ctx)
	err := c.Start()
	assert.NoError(t, err)
}

func TestCheckOnce(t *testing.T) {
	// Save original env vars to restore later
	origVars := map[string]string{}
	for k := range BAD_CUDA_ENV_KEYS {
		origVars[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		// Restore original env vars
		for k, v := range origVars {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	ctx := context.Background()
	c := New(ctx).(*component)

	// Test with no bad env vars set
	c.CheckOnce()
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	assert.NotNil(t, lastData)
	assert.Empty(t, lastData.FoundBadEnvsForCUDA)

	// Test with a bad env var set
	badEnvVar := "CUDA_PROFILE"
	os.Setenv(badEnvVar, "1")
	c.CheckOnce()
	c.lastMu.RLock()
	lastData = c.lastData
	c.lastMu.RUnlock()
	assert.NotNil(t, lastData)
	assert.Len(t, lastData.FoundBadEnvsForCUDA, 1)
	assert.Contains(t, lastData.FoundBadEnvsForCUDA, badEnvVar)

	// Test with multiple bad env vars set
	secondBadEnvVar := "COMPUTE_PROFILE"
	os.Setenv(secondBadEnvVar, "1")
	c.CheckOnce()
	c.lastMu.RLock()
	lastData = c.lastData
	c.lastMu.RUnlock()
	assert.NotNil(t, lastData)
	assert.Len(t, lastData.FoundBadEnvsForCUDA, 2)
	assert.Contains(t, lastData.FoundBadEnvsForCUDA, badEnvVar)
	assert.Contains(t, lastData.FoundBadEnvsForCUDA, secondBadEnvVar)
}

func TestCustomCheckEnvFunc(t *testing.T) {
	ctx := context.Background()
	c := New(ctx).(*component)

	// Set custom environment check function that always returns true
	c.checkEnvFunc = func(key string) bool {
		return true
	}

	c.CheckOnce()
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()

	// All environment variables should be detected as "bad"
	assert.NotNil(t, lastData)
	assert.Equal(t, len(BAD_CUDA_ENV_KEYS), len(lastData.FoundBadEnvsForCUDA))

	// Set custom environment check function that always returns false
	c.checkEnvFunc = func(key string) bool {
		return false
	}

	c.CheckOnce()
	c.lastMu.RLock()
	lastData = c.lastData
	c.lastMu.RUnlock()

	// No environment variables should be detected as "bad"
	assert.NotNil(t, lastData)
	assert.Empty(t, lastData.FoundBadEnvsForCUDA)
}

func TestStates(t *testing.T) {
	ctx := context.Background()
	c := New(ctx).(*component)

	// Test with nil data
	states, err := c.States(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with empty data
	c.lastData = &Data{
		ts:      time.Now(),
		healthy: true,
		reason:  "no bad envs found",
	}
	states, err = c.States(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no bad envs found", states[0].Reason)

	// Test with bad env data
	c.lastData = &Data{
		ts: time.Now(),
		FoundBadEnvsForCUDA: map[string]string{
			"CUDA_PROFILE": BAD_CUDA_ENV_KEYS["CUDA_PROFILE"],
		},
		healthy: true,
		reason:  "CUDA_PROFILE: Enables CUDA profiling.",
	}
	states, err = c.States(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "CUDA_PROFILE")

	// Test with error
	c.lastData = &Data{
		ts:      time.Now(),
		err:     assert.AnError,
		healthy: false,
		reason:  "failed to get bad envs data",
	}
	states, err = c.States(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "failed to get bad envs data")
	assert.Equal(t, assert.AnError.Error(), states[0].Error)
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	c := New(ctx)
	events, err := c.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Just in case Close doesn't cancel the context

	c := New(ctx).(*component)
	err := c.Close()
	assert.NoError(t, err)

	// Verify that the context was canceled
	select {
	case <-c.ctx.Done():
		// Context was properly canceled
	default:
		t.Error("Context was not canceled by Close()")
	}
}

func TestDataGetError(t *testing.T) {
	// Test with nil Data
	var d *Data
	assert.Empty(t, d.getError())

	// Test with nil error
	d = &Data{}
	assert.Empty(t, d.getError())

	// Test with actual error
	d = &Data{err: assert.AnError}
	assert.Equal(t, assert.AnError.Error(), d.getError())
}

func TestPeriodicCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a component with a mocked check function
	c := New(ctx).(*component)

	checkCalled := make(chan struct{}, 5) // Buffer to avoid blocking
	c.checkEnvFunc = func(key string) bool {
		// Signal that check was called
		select {
		case checkCalled <- struct{}{}:
		default:
		}
		return false
	}

	// Start the component
	err := c.Start()
	assert.NoError(t, err)

	// Wait for the initial check
	select {
	case <-checkCalled:
		// Check was called
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Initial check was not called")
	}

	// Cancel context to stop the goroutine
	cancel()
}

func TestDataWithMultipleBadEnvs(t *testing.T) {
	// Create data with multiple bad environments and set a valid reason
	reason := "CUDA_PROFILE: Enables CUDA profiling.; COMPUTE_PROFILE: Enables compute profiling."
	d := &Data{
		FoundBadEnvsForCUDA: map[string]string{
			"CUDA_PROFILE":    "Enables CUDA profiling.",
			"COMPUTE_PROFILE": "Enables compute profiling.",
		},
		ts:      time.Now(),
		healthy: true,
		reason:  reason,
	}

	// Check the reason string contains both env vars
	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Contains(t, states[0].Reason, "CUDA_PROFILE")
	assert.Contains(t, states[0].Reason, "COMPUTE_PROFILE")

	// Verify JSON marshaling in ExtraInfo
	assert.Contains(t, states[0].ExtraInfo["data"], "CUDA_PROFILE")
	assert.Contains(t, states[0].ExtraInfo["data"], "COMPUTE_PROFILE")
}
