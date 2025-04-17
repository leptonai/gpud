package badenvs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

func TestNew(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.Equal(t, Name, c.Name())
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	err = c.Start()
	assert.NoError(t, err)
}

func TestCheck(t *testing.T) {
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
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Create a mock NVML instance that returns true for NVMLExists
	mockNVMLInstance := &mockNVMLInstance{exists: true}
	comp.nvmlInstance = mockNVMLInstance

	// Test with no bad env vars set
	result := comp.Check()
	assert.NotNil(t, result)
	assert.Empty(t, result.(*Data).FoundBadEnvsForCUDA)

	// Test with a bad env var set
	badEnvVar := "CUDA_PROFILE"
	os.Setenv(badEnvVar, "1")
	result = comp.Check()
	assert.NotNil(t, result)
	assert.Len(t, result.(*Data).FoundBadEnvsForCUDA, 1)
	assert.Contains(t, result.(*Data).FoundBadEnvsForCUDA, badEnvVar)

	// Test with multiple bad env vars set
	secondBadEnvVar := "COMPUTE_PROFILE"
	os.Setenv(secondBadEnvVar, "1")
	result = comp.Check()
	assert.NotNil(t, result)
	assert.Len(t, result.(*Data).FoundBadEnvsForCUDA, 2)
	assert.Contains(t, result.(*Data).FoundBadEnvsForCUDA, badEnvVar)
	assert.Contains(t, result.(*Data).FoundBadEnvsForCUDA, secondBadEnvVar)
}

// mockNVMLInstance is a mock implementation of nvidianvml.InstanceV2
type mockNVMLInstance struct {
	exists bool
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.exists
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return nil
}

func (m *mockNVMLInstance) ProductName() string {
	return ""
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func TestCustomCheckEnvFunc(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Create a mock NVML instance that returns true for NVMLExists
	mockNVMLInstance := &mockNVMLInstance{exists: true}
	comp.nvmlInstance = mockNVMLInstance

	// Set custom environment check function that always returns true
	comp.checkEnvFunc = func(key string) bool {
		return true
	}

	result := comp.Check()

	// All environment variables should be detected as "bad"
	assert.NotNil(t, result)
	assert.Equal(t, len(BAD_CUDA_ENV_KEYS), len(result.(*Data).FoundBadEnvsForCUDA))

	// Set custom environment check function that always returns false
	comp.checkEnvFunc = func(key string) bool {
		return false
	}

	result = comp.Check()

	// No environment variables should be detected as "bad"
	assert.NotNil(t, result)
	assert.Empty(t, result.(*Data).FoundBadEnvsForCUDA)
}

func TestLastHealthStates(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Test with nil data
	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with empty data
	comp.lastData = &Data{
		ts:     time.Now(),
		health: apiv1.StateTypeHealthy,
		reason: "no bad envs found",
	}
	states = comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "no bad envs found", states[0].Reason)

	// Test with bad env data
	comp.lastData = &Data{
		ts: time.Now(),
		FoundBadEnvsForCUDA: map[string]string{
			"CUDA_PROFILE": BAD_CUDA_ENV_KEYS["CUDA_PROFILE"],
		},
		health: apiv1.StateTypeHealthy,
		reason: "CUDA_PROFILE: Enables CUDA profiling.",
	}
	states = comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "CUDA_PROFILE")

	// Test with error
	comp.lastData = &Data{
		ts:     time.Now(),
		err:    assert.AnError,
		health: apiv1.StateTypeUnhealthy,
		reason: "failed to get bad envs data",
	}
	states = comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "failed to get bad envs data")
	assert.Equal(t, assert.AnError.Error(), states[0].Error)
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	events, err := c.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Just in case Close doesn't cancel the context

	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)
	err = comp.Close()
	assert.NoError(t, err)

	// Verify that the context was canceled
	select {
	case <-comp.ctx.Done():
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
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Create a mock NVML instance that returns true for NVMLExists
	mockNVMLInstance := &mockNVMLInstance{exists: true}
	comp.nvmlInstance = mockNVMLInstance

	// Create a flag to track if the check function was called
	called := false
	comp.checkEnvFunc = func(key string) bool {
		called = true
		return false
	}

	// Call Check directly
	_ = comp.Check()

	// Verify check was called
	assert.True(t, called, "Check function was not called")
}

func TestDataWithMultipleBadEnvs(t *testing.T) {
	// Create data with multiple bad environments and set a valid reason
	reason := "CUDA_PROFILE: Enables CUDA profiling.; COMPUTE_PROFILE: Enables compute profiling."
	d := &Data{
		FoundBadEnvsForCUDA: map[string]string{
			"CUDA_PROFILE":    "Enables CUDA profiling.",
			"COMPUTE_PROFILE": "Enables compute profiling.",
		},
		ts:     time.Now(),
		health: apiv1.StateTypeHealthy,
		reason: reason,
	}

	// Check the reason string contains both env vars
	states := d.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Contains(t, states[0].Reason, "CUDA_PROFILE")
	assert.Contains(t, states[0].Reason, "COMPUTE_PROFILE")

	// Verify JSON marshaling in ExtraInfo
	assert.Contains(t, states[0].DeprecatedExtraInfo["data"], "CUDA_PROFILE")
	assert.Contains(t, states[0].DeprecatedExtraInfo["data"], "COMPUTE_PROFILE")
}
