package badenvs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
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

func TestTags(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 4, "Component should return exactly 4 tags")
}

func TestIsSupported(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Test when nvmlInstance is nil
	comp.nvmlInstance = nil
	assert.False(t, comp.IsSupported())

	// Test when NVMLExists returns false
	comp.nvmlInstance = &mockNVMLInstance{exists: false, pname: ""}
	assert.False(t, comp.IsSupported())

	// Test when ProductName returns empty string
	comp.nvmlInstance = &mockNVMLInstance{exists: true, pname: ""}
	assert.False(t, comp.IsSupported())

	// Test when all conditions are met
	comp.nvmlInstance = &mockNVMLInstance{exists: true, pname: "Tesla V100"}
	assert.True(t, comp.IsSupported())
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

	// Create a mock NVML instance that returns true for NVMLExists and has a product name
	mockNVMLInstance := &mockNVMLInstance{exists: true, pname: "Tesla V100"}
	comp.nvmlInstance = mockNVMLInstance

	// Test with no bad env vars set
	result := comp.Check()
	assert.NotNil(t, result)
	assert.Empty(t, result.(*checkResult).FoundBadEnvsForCUDA)

	// Test with a bad env var set
	badEnvVar := "CUDA_PROFILE"
	os.Setenv(badEnvVar, "1")
	result = comp.Check()
	assert.NotNil(t, result)
	assert.Len(t, result.(*checkResult).FoundBadEnvsForCUDA, 1)
	assert.Contains(t, result.(*checkResult).FoundBadEnvsForCUDA, badEnvVar)

	// Test with multiple bad env vars set
	secondBadEnvVar := "COMPUTE_PROFILE"
	os.Setenv(secondBadEnvVar, "1")
	result = comp.Check()
	assert.NotNil(t, result)
	assert.Len(t, result.(*checkResult).FoundBadEnvsForCUDA, 2)
	assert.Contains(t, result.(*checkResult).FoundBadEnvsForCUDA, badEnvVar)
	assert.Contains(t, result.(*checkResult).FoundBadEnvsForCUDA, secondBadEnvVar)
}

// mockNVMLInstance is a mock implementation of nvidianvml.Instance
type mockNVMLInstance struct {
	exists bool
	pname  string
}

// NewMockNVMLInstance creates a new mockNVMLInstance with default settings
func NewMockNVMLInstance(exists bool, pname string) *mockNVMLInstance {
	return &mockNVMLInstance{
		exists: exists,
		pname:  pname,
	}
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
	return m.pname
}

func (m *mockNVMLInstance) Architecture() string {
	return ""
}

func (m *mockNVMLInstance) Brand() string {
	return ""
}

func (m *mockNVMLInstance) DriverVersion() string {
	return ""
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 0
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return ""
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return true
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

	// Create a mock NVML instance that returns true for NVMLExists and has a product name
	mockNVMLInstance := &mockNVMLInstance{exists: true, pname: "Tesla V100"}
	comp.nvmlInstance = mockNVMLInstance

	// Set custom environment check function that always returns true
	comp.checkEnvFunc = func(key string) bool {
		return true
	}

	result := comp.Check()

	// All environment variables should be detected as "bad"
	assert.NotNil(t, result)
	assert.Equal(t, len(BAD_CUDA_ENV_KEYS), len(result.(*checkResult).FoundBadEnvsForCUDA))

	// Set custom environment check function that always returns false
	comp.checkEnvFunc = func(key string) bool {
		return false
	}

	result = comp.Check()

	// No environment variables should be detected as "bad"
	assert.NotNil(t, result)
	assert.Empty(t, result.(*checkResult).FoundBadEnvsForCUDA)
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
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with empty data
	comp.lastCheckResult = &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "no bad envs found",
	}
	states = comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no bad envs found", states[0].Reason)

	// Test with bad env data
	comp.lastCheckResult = &checkResult{
		ts: time.Now(),
		FoundBadEnvsForCUDA: map[string]string{
			"CUDA_PROFILE": BAD_CUDA_ENV_KEYS["CUDA_PROFILE"],
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "CUDA_PROFILE: Enables CUDA profiling.",
	}
	states = comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "CUDA_PROFILE")

	// Test with error
	comp.lastCheckResult = &checkResult{
		ts:     time.Now(),
		err:    assert.AnError,
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "failed to get bad envs data",
	}
	states = comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
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
	var cr *checkResult
	assert.Empty(t, cr.getError())

	// Test with nil error
	cr = &checkResult{}
	assert.Empty(t, cr.getError())

	// Test with actual error
	cr = &checkResult{err: assert.AnError}
	assert.Equal(t, assert.AnError.Error(), cr.getError())
}

func TestPeriodicCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a component with a mocked check function
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Create a mock NVML instance that returns true for NVMLExists and has a product name
	mockNVMLInstance := &mockNVMLInstance{exists: true, pname: "Tesla V100"}
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
	cr := &checkResult{
		FoundBadEnvsForCUDA: map[string]string{
			"CUDA_PROFILE":    "Enables CUDA profiling.",
			"COMPUTE_PROFILE": "Enables compute profiling.",
		},
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: reason,
	}

	// Check the reason string contains both env vars
	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Contains(t, states[0].Reason, "CUDA_PROFILE")
	assert.Contains(t, states[0].Reason, "COMPUTE_PROFILE")

	// Verify JSON marshaling in ExtraInfo
	assert.Contains(t, states[0].ExtraInfo["data"], "CUDA_PROFILE")
	assert.Contains(t, states[0].ExtraInfo["data"], "COMPUTE_PROFILE")
}

// Additional test cases to increase coverage

func TestCheckResultString(t *testing.T) {
	// Test with nil checkResult
	var cr *checkResult
	assert.Empty(t, cr.String())

	// Test with empty bad envs
	cr = &checkResult{}
	assert.Equal(t, "no bad envs found", cr.String())

	// Test with some bad envs
	cr = &checkResult{
		FoundBadEnvsForCUDA: map[string]string{
			"CUDA_PROFILE": "Enables CUDA profiling.",
		},
	}
	result := cr.String()
	assert.Contains(t, result, "CUDA_PROFILE")
	assert.Contains(t, result, "Enables CUDA profiling")
}

func TestCheckResultSummary(t *testing.T) {
	// Test with nil checkResult
	var cr *checkResult
	assert.Empty(t, cr.Summary())

	// Test with reason set
	cr = &checkResult{
		reason: "test reason",
	}
	assert.Equal(t, "test reason", cr.Summary())
}

func TestCheckResultHealthState(t *testing.T) {
	// Test with nil checkResult
	var cr *checkResult
	assert.Empty(t, cr.HealthStateType())

	// Test with health state set
	cr = &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
	}
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
}

func TestCheckWithNilNVML(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Set NVML instance to nil
	comp.nvmlInstance = nil

	// Test check with nil NVML
	result := comp.Check()
	checkResult := result.(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, checkResult.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", checkResult.reason)
}

func TestCheckWithNVMLNotExisting(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Set NVML instance that returns false for NVMLExists
	mockNVMLInstance := &mockNVMLInstance{exists: false, pname: "Tesla V100"}
	comp.nvmlInstance = mockNVMLInstance

	// Test check with NVML not existing
	result := comp.Check()
	checkResult := result.(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, checkResult.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", checkResult.reason)
}

func TestCheckAllEnvVarsForCoverage(t *testing.T) {
	// Save original env vars to restore later
	origVars := map[string]string{}
	for k := range BAD_CUDA_ENV_KEYS {
		origVars[k] = os.Getenv(k)
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

	mockNVMLInstance := &mockNVMLInstance{exists: true, pname: "Tesla V100"}
	comp.nvmlInstance = mockNVMLInstance

	// Set all env vars to "1" one by one to test each case
	for key := range BAD_CUDA_ENV_KEYS {
		// Reset all env vars
		for k := range BAD_CUDA_ENV_KEYS {
			os.Unsetenv(k)
		}

		// Set this specific env var
		os.Setenv(key, "1")

		// Run the check
		result := comp.Check()
		checkResult := result.(*checkResult)

		// Verify only this env var is detected
		assert.Len(t, checkResult.FoundBadEnvsForCUDA, 1)
		assert.Contains(t, checkResult.FoundBadEnvsForCUDA, key)
		assert.Contains(t, checkResult.reason, key)
	}
}

func TestDefaultCheckEnvFunc(t *testing.T) {
	// Save original env var to restore later
	origVar := os.Getenv("TEST_ENV_VAR")
	defer func() {
		if origVar != "" {
			os.Setenv("TEST_ENV_VAR", origVar)
		} else {
			os.Unsetenv("TEST_ENV_VAR")
		}
	}()

	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{RootCtx: ctx}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Test with env var not set
	os.Unsetenv("TEST_ENV_VAR")
	assert.False(t, comp.checkEnvFunc("TEST_ENV_VAR"))

	// Test with env var set to "1"
	os.Setenv("TEST_ENV_VAR", "1")
	assert.True(t, comp.checkEnvFunc("TEST_ENV_VAR"))

	// Test with env var set to something else
	os.Setenv("TEST_ENV_VAR", "true")
	assert.False(t, comp.checkEnvFunc("TEST_ENV_VAR"))
}
