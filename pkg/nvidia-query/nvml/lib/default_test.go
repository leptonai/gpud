package lib

import (
	"os"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewDefaultNoEnvVars tests the NewDefault function when no environment variables are set
func TestNewDefaultNoEnvVars(t *testing.T) {
	// Make sure environment variables are not set
	os.Unsetenv(EnvMockAllSuccess)
	os.Unsetenv(EnvInjectRemapedRowsPending)
	os.Unsetenv(EnvInjectClockEventsHwSlowdown)

	// Create a new library instance
	lib := NewDefault()

	// Verify the library instance is created with default options
	assert.NotNil(t, lib)
	assert.NotNil(t, lib.NVML())
}

// TestNewDefaultMultipleEnvVars tests the NewDefault function when multiple environment variables are set
func TestNewDefaultMultipleEnvVars(t *testing.T) {
	// Clean up environment variables first
	cleanupEnvVars()
	defer cleanupEnvVars()

	// Set multiple environment variables
	os.Setenv(EnvMockAllSuccess, "true")
	os.Setenv(EnvInjectRemapedRowsPending, "true")
	os.Setenv(EnvInjectClockEventsHwSlowdown, "true")

	// Create a new library instance
	lib := NewDefault()

	// Verify the library instance is created correctly
	assert.NotNil(t, lib)

	// Test that NVML functions succeed
	ret := lib.NVML().Init()
	assert.Equal(t, nvml.SUCCESS, ret)

	// Get devices to test modified functions
	devices, err := lib.GetDevices()
	require.NoError(t, err)
	require.NotEmpty(t, devices)

	// Test the injected function to get remapped rows
	corrRows, uncRows, isPending, failureOccurred, retRemapped := devices[0].GetRemappedRows()
	assert.Equal(t, 0, corrRows)
	assert.Equal(t, 10, uncRows)
	assert.True(t, isPending)
	assert.False(t, failureOccurred)
	assert.Equal(t, nvml.SUCCESS, retRemapped)

	// Test the injected function to get clock events
	reasons, retClock := devices[0].GetCurrentClocksEventReasons()
	expectedReasons := reasonHWSlowdown | reasonSwThermalSlowdown | reasonHWSlowdownThermal | reasonHWSlowdownPowerBrake
	assert.Equal(t, expectedReasons, reasons)
	assert.Equal(t, nvml.SUCCESS, retClock)
}

// Utility function to clean up environment variables
func cleanupEnvVars() {
	os.Unsetenv(EnvMockAllSuccess)
	os.Unsetenv(EnvInjectRemapedRowsPending)
	os.Unsetenv(EnvInjectClockEventsHwSlowdown)
}
