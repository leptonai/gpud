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
	c.lastData = &Data{ts: time.Now()}
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
		ts:  time.Now(),
		err: assert.AnError,
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

func TestMetrics(t *testing.T) {
	ctx := context.Background()
	c := New(ctx)
	metrics, err := c.Metrics(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, metrics)
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

func TestDataMethods(t *testing.T) {
	// Test nil data
	var d *Data
	reason := d.getReason()
	assert.Equal(t, "no bad envs data", reason)

	health, healthy := d.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	errStr := d.getError()
	assert.Empty(t, errStr)

	// Test data with error
	d = &Data{err: assert.AnError}
	reason = d.getReason()
	assert.Equal(t, "failed to get bad envs data -- assert.AnError general error for testing", reason)

	health, healthy = d.getHealth()
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)

	errStr = d.getError()
	assert.Equal(t, assert.AnError.Error(), errStr)

	// Test data with found bad envs
	d = &Data{
		FoundBadEnvsForCUDA: map[string]string{
			"CUDA_PROFILE": "Enables CUDA profiling.",
		},
	}
	reason = d.getReason()
	assert.Contains(t, reason, "CUDA_PROFILE")
	assert.Contains(t, reason, "Enables CUDA profiling.")
}
