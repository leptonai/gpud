package lib

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

// TestApplyOptsDefault tests the default behavior of applyOpts when no options are provided
func TestApplyOptsDefault(t *testing.T) {
	// Create an empty Op
	op := &Op{}

	// Apply no options
	op.applyOpts([]OpOption{})

	// Verify that nvmlLib is set to a non-nil value (default nvml.New())
	assert.NotNil(t, op.nvmlLib, "nvmlLib should be set to a default value when no options are provided")
}

func TestResolveNVMLLibraryPath(t *testing.T) {
	cleanupEnvVars()
	t.Cleanup(cleanupEnvVars)

	// With no explicit override, resolution probes the well-known driver
	// roots. Neither "/host" nor "/run/nvidia/driver" exists on a test
	// machine, so the probes find nothing and go-nvml's default system
	// lookup is preserved -- the same outcome bare-metal/systemd runs get.
	assert.Empty(t, resolveNVMLLibraryPath())

	// An explicit library path always wins over the driver-root probes.
	explicit := "/custom/libnvidia-ml.so.1"
	t.Setenv(EnvNVMLLibraryPath, explicit)
	assert.Equal(t, explicit, resolveNVMLLibraryPath())
}

func TestResolveFromDriverRootsFallbacks(t *testing.T) {
	driverRoot := t.TempDir()

	// A mounted but empty driver root (e.g. the GPU Operator has not
	// finished installing the driver yet) must retain the system-library
	// fallback.
	assert.Empty(t, resolveFromDriverRoots(t.TempDir(), driverRoot))

	// Exercise the common non-multiarch layout after the architecture-specific
	// candidate is absent.
	libraryPath := filepath.Join(driverRoot, "usr", "lib64", "libnvidia-ml.so.1")
	require.NoError(t, os.MkdirAll(filepath.Dir(libraryPath), 0o755))
	require.NoError(t, os.WriteFile(libraryPath, nil, 0o644))
	assert.Equal(t, libraryPath, resolveFromDriverRoots(t.TempDir(), driverRoot))
}

// TestResolveFromDriverRootsHostRootPrecedence verifies that a pre-installed
// host driver (host root mount) takes precedence over a GPU Operator-managed
// driver root when both contain an NVML library, mirroring the GPU
// Operator's driver-validation order.
func TestResolveFromDriverRootsHostRootPrecedence(t *testing.T) {
	cleanupEnvVars()
	t.Cleanup(cleanupEnvVars)

	writeLibrary := func(root string) string {
		libraryPath := filepath.Join(root, "usr", "lib", nvmlArchLibraryDir(runtime.GOARCH), "libnvidia-ml.so.1")
		require.NoError(t, os.MkdirAll(filepath.Dir(libraryPath), 0o755))
		require.NoError(t, os.WriteFile(libraryPath, nil, 0o644))
		return libraryPath
	}

	hostRoot := t.TempDir()
	hostLibrary := writeLibrary(hostRoot)
	driverRoot := t.TempDir()
	driverLibrary := writeLibrary(driverRoot)

	// Both present: the pre-installed host driver wins.
	assert.Equal(t, hostLibrary, resolveFromDriverRoots(hostRoot, driverRoot))

	// Host root without libraries falls through to the driver root.
	require.NoError(t, os.Remove(hostLibrary))
	assert.Equal(t, driverLibrary, resolveFromDriverRoots(hostRoot, driverRoot))

	// Neither root provides a library: preserve the system-library fallback.
	require.NoError(t, os.Remove(driverLibrary))
	assert.Empty(t, resolveFromDriverRoots(hostRoot, driverRoot))

	// An explicit library path still wins over both roots.
	explicit := "/custom/libnvidia-ml.so.1"
	t.Setenv(EnvNVMLLibraryPath, explicit)
	assert.Equal(t, explicit, resolveNVMLLibraryPath())
}

// TestResolveFromDriverRootsHostRootOnly verifies host-root resolution when
// the driver root mount is absent/empty.
func TestResolveFromDriverRootsHostRootOnly(t *testing.T) {
	hostRoot := t.TempDir()
	libraryPath := filepath.Join(hostRoot, "usr", "lib64", "libnvidia-ml.so.1")
	require.NoError(t, os.MkdirAll(filepath.Dir(libraryPath), 0o755))
	require.NoError(t, os.WriteFile(libraryPath, nil, 0o644))

	// The lib64 layout is found via the host root even with an empty driver root.
	assert.Equal(t, libraryPath, resolveFromDriverRoots(hostRoot, t.TempDir()))
}

// TestProbeDriverReadyContract covers the GPU Operator driver-ready contract
// discovery: missing contract, host-driver selection ("/"), rejected relative
// paths, quoted values, and a custom driver install directory.
func TestProbeDriverReadyContract(t *testing.T) {
	// Missing contract file.
	assert.Empty(t, probeDriverReadyContract(t.TempDir()))

	writeContract := func(hostRoot, contents string) {
		contractPath := filepath.Join(hostRoot, driverReadyContractPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(contractPath), 0o755))
		require.NoError(t, os.WriteFile(contractPath, []byte(contents), 0o644))
	}

	hostRoot := t.TempDir()

	// The contract selecting the host driver ("/") is covered by the
	// standard host-root probes, so the contract probe returns empty.
	writeContract(hostRoot, "NVIDIA_DRIVER_ROOT=/\n")
	assert.Empty(t, probeDriverReadyContract(hostRoot))

	// Relative paths are rejected.
	writeContract(hostRoot, "NVIDIA_DRIVER_ROOT=run/nvidia/driver\n")
	assert.Empty(t, probeDriverReadyContract(hostRoot))

	// A custom install directory without the NVML library yields empty.
	writeContract(hostRoot, "NVIDIA_DRIVER_ROOT=\"/opt/nvidia/driver\"\n")
	assert.Empty(t, probeDriverReadyContract(hostRoot))

	// A custom install directory with the NVML library resolves under the
	// host root.
	libraryPath := filepath.Join(hostRoot, "opt", "nvidia", "driver", "usr", "lib", nvmlArchLibraryDir(runtime.GOARCH), "libnvidia-ml.so.1")
	require.NoError(t, os.MkdirAll(filepath.Dir(libraryPath), 0o755))
	require.NoError(t, os.WriteFile(libraryPath, nil, 0o644))
	assert.Equal(t, libraryPath, probeDriverReadyContract(hostRoot))
}

// TestResolveFromDriverRootsDriverReadyContract verifies the GPU Operator's
// driver-ready contract is honored between the host-root and driver-root
// probes: a pre-installed host driver still wins, the Operator-validated
// custom install directory wins over the static driver root, and without the
// contract the static driver root is used.
func TestResolveFromDriverRootsDriverReadyContract(t *testing.T) {
	hostRoot := t.TempDir()
	contractPath := filepath.Join(hostRoot, driverReadyContractPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(contractPath), 0o755))
	require.NoError(t, os.WriteFile(contractPath, []byte("NVIDIA_DRIVER_ROOT=/opt/nvidia/driver\nDRIVER_ROOT_CTR_PATH=/driver-root\n"), 0o644))
	contractLibrary := filepath.Join(hostRoot, "opt", "nvidia", "driver", "usr", "lib64", "libnvidia-ml.so.1")
	require.NoError(t, os.MkdirAll(filepath.Dir(contractLibrary), 0o755))
	require.NoError(t, os.WriteFile(contractLibrary, nil, 0o644))

	driverRoot := t.TempDir()
	staticLibrary := filepath.Join(driverRoot, "usr", "lib", nvmlArchLibraryDir(runtime.GOARCH), "libnvidia-ml.so.1")
	require.NoError(t, os.MkdirAll(filepath.Dir(staticLibrary), 0o755))
	require.NoError(t, os.WriteFile(staticLibrary, nil, 0o644))

	// A pre-installed host driver takes precedence over the contract.
	hostLibrary := filepath.Join(hostRoot, "usr", "lib", nvmlArchLibraryDir(runtime.GOARCH), "libnvidia-ml.so.1")
	require.NoError(t, os.MkdirAll(filepath.Dir(hostLibrary), 0o755))
	require.NoError(t, os.WriteFile(hostLibrary, nil, 0o644))
	assert.Equal(t, hostLibrary, resolveFromDriverRoots(hostRoot, driverRoot))
	require.NoError(t, os.Remove(hostLibrary))

	// The Operator-validated custom install directory wins over the static
	// driver root.
	assert.Equal(t, contractLibrary, resolveFromDriverRoots(hostRoot, driverRoot))

	// Without the contract file, the static driver root is used.
	require.NoError(t, os.Remove(contractPath))
	assert.Equal(t, staticLibrary, resolveFromDriverRoots(hostRoot, driverRoot))
}

func TestNVMLArchLibraryDir(t *testing.T) {
	tests := map[string]string{
		"amd64":   "x86_64-linux-gnu",
		"arm64":   "aarch64-linux-gnu",
		"ppc64le": "powerpc64le-linux-gnu",
		"unknown": "x86_64-linux-gnu",
	}
	for goarch, expected := range tests {
		t.Run(goarch, func(t *testing.T) {
			assert.Equal(t, expected, nvmlArchLibraryDir(goarch))
		})
	}
}

func TestApplyOptsWithResolvedNVMLLibrary(t *testing.T) {
	cleanupEnvVars()
	t.Cleanup(cleanupEnvVars)

	libraryPath := filepath.Join(t.TempDir(), "libnvidia-ml.so.1")
	require.NoError(t, os.WriteFile(libraryPath, nil, 0o644))
	t.Setenv(EnvNVMLLibraryPath, libraryPath)

	op := &Op{}
	op.applyOpts(nil)
	assert.NotNil(t, op.nvmlLib)
}

// TestWithNVML tests that WithNVML correctly sets the nvmlLib field
func TestWithNVML(t *testing.T) {
	// Create a mock NVML interface
	mockNVML := &mock.Interface{}

	// Create an empty Op
	op := &Op{}

	// Apply the WithNVML option
	op.applyOpts([]OpOption{WithNVML(mockNVML)})

	// Verify that nvmlLib is set to our mock
	assert.Equal(t, mockNVML, op.nvmlLib, "nvmlLib should be set to the provided mock")
}

// TestWithInitReturn tests that WithInitReturn correctly sets the initReturn field
func TestWithInitReturn(t *testing.T) {
	// Create an empty Op
	op := &Op{}

	// Test value
	testReturn := nvml.ERROR_UNKNOWN

	// Apply the WithInitReturn option
	op.applyOpts([]OpOption{WithInitReturn(testReturn)})

	// Verify that initReturn is set and points to our test value
	assert.NotNil(t, op.initReturn, "initReturn should not be nil")
	assert.Equal(t, testReturn, *op.initReturn, "initReturn should be set to the provided value")
}

// TestWithPropertyExtractor tests that WithPropertyExtractor correctly sets the propertyExtractor field
func TestWithPropertyExtractor(t *testing.T) {
	// Create a mock PropertyExtractor
	mockExtractor := &nvinfo.PropertyExtractorMock{}

	// Create an empty Op
	op := &Op{}

	// Apply the WithPropertyExtractor option
	op.applyOpts([]OpOption{WithPropertyExtractor(mockExtractor)})

	// Verify that propertyExtractor is set to our mock
	assert.Equal(t, mockExtractor, op.propertyExtractor, "propertyExtractor should be set to the provided mock")
}

// TestWithDevice tests that WithDevice correctly adds to the devicesToReturn slice
func TestWithDevice(t *testing.T) {
	// Create a mock Device using testutil
	mockDevice := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "test-pci")

	// Create an empty Op
	op := &Op{}

	// Apply the WithDevice option
	op.applyOpts([]OpOption{WithDevice(mockDevice)})

	// Verify that devicesToReturn contains our mock device
	assert.Len(t, op.devicesToReturn, 1, "devicesToReturn should have one device")
	assert.Equal(t, mockDevice, op.devicesToReturn[0], "devicesToReturn should contain the provided device")

	// Add another device and verify both are present
	mockDevice2 := testutil.NewMockDevice(&mock.Device{}, "test-arch2", "test-brand2", "test-cuda2", "test-pci2")
	op.applyOpts([]OpOption{WithDevice(mockDevice2)})

	assert.Len(t, op.devicesToReturn, 2, "devicesToReturn should have two devices")
	assert.Equal(t, mockDevice, op.devicesToReturn[0], "First device should still be present")
	assert.Equal(t, mockDevice2, op.devicesToReturn[1], "Second device should be added")
}

// TestWithDeviceGetRemappedRowsForAllDevs tests that WithDeviceGetRemappedRowsForAllDevs correctly sets the function
func TestWithDeviceGetRemappedRowsForAllDevs(t *testing.T) {
	// Create a test function
	testFunc := func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return) {
		return 1, 2, true, false, nvml.SUCCESS
	}

	// Create an empty Op
	op := &Op{}

	// Apply the WithDeviceGetRemappedRowsForAllDevs option
	op.applyOpts([]OpOption{WithDeviceGetRemappedRowsForAllDevs(testFunc)})

	// Verify that the function is set
	assert.NotNil(t, op.devGetRemappedRowsForAllDevs, "devGetRemappedRowsForAllDevs should be set")

	// Call the function and verify it returns the expected values
	corrRows, uncRows, isPending, failureOccurred, ret := op.devGetRemappedRowsForAllDevs()
	assert.Equal(t, 1, corrRows, "corrRows should match")
	assert.Equal(t, 2, uncRows, "uncRows should match")
	assert.True(t, isPending, "isPending should match")
	assert.False(t, failureOccurred, "failureOccurred should match")
	assert.Equal(t, nvml.SUCCESS, ret, "ret should match")
}

// TestWithDeviceGetCurrentClocksEventReasonsForAllDevs tests that WithDeviceGetCurrentClocksEventReasonsForAllDevs correctly sets the function
func TestWithDeviceGetCurrentClocksEventReasonsForAllDevs(t *testing.T) {
	// Create a test function
	testFunc := func() (uint64, nvml.Return) {
		return 42, nvml.SUCCESS
	}

	// Create an empty Op
	op := &Op{}

	// Apply the WithDeviceGetCurrentClocksEventReasonsForAllDevs option
	op.applyOpts([]OpOption{WithDeviceGetCurrentClocksEventReasonsForAllDevs(testFunc)})

	// Verify that the function is set
	assert.NotNil(t, op.devGetCurrentClocksEventReasonsForAllDevs, "devGetCurrentClocksEventReasonsForAllDevs should be set")

	// Call the function and verify it returns the expected values
	reasons, ret := op.devGetCurrentClocksEventReasonsForAllDevs()
	assert.Equal(t, uint64(42), reasons, "reasons should match")
	assert.Equal(t, nvml.SUCCESS, ret, "ret should match")
}

// TestMultipleOptions tests that multiple options can be applied correctly
func TestMultipleOptions(t *testing.T) {
	// Create mocks and test values
	mockNVML := &mock.Interface{}
	mockExtractor := &nvinfo.PropertyExtractorMock{}
	mockDevice := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "test-pci")
	testReturn := nvml.ERROR_UNKNOWN

	// Create test functions
	remappedRowsFunc := func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return) {
		return 1, 2, true, false, nvml.SUCCESS
	}

	clockEventsFunc := func() (uint64, nvml.Return) {
		return 42, nvml.SUCCESS
	}

	// Create an empty Op
	op := &Op{}

	// Apply all options at once
	op.applyOpts([]OpOption{
		WithNVML(mockNVML),
		WithInitReturn(testReturn),
		WithPropertyExtractor(mockExtractor),
		WithDevice(mockDevice),
		WithDeviceGetRemappedRowsForAllDevs(remappedRowsFunc),
		WithDeviceGetCurrentClocksEventReasonsForAllDevs(clockEventsFunc),
	})

	// Verify all fields are set correctly
	assert.Equal(t, mockNVML, op.nvmlLib, "nvmlLib should be set correctly")
	assert.NotNil(t, op.initReturn, "initReturn should not be nil")
	assert.Equal(t, testReturn, *op.initReturn, "initReturn should be set correctly")
	assert.Equal(t, mockExtractor, op.propertyExtractor, "propertyExtractor should be set correctly")
	assert.Len(t, op.devicesToReturn, 1, "devicesToReturn should have one device")
	assert.Equal(t, mockDevice, op.devicesToReturn[0], "devicesToReturn should contain the provided device")
	assert.NotNil(t, op.devGetRemappedRowsForAllDevs, "devGetRemappedRowsForAllDevs should be set")
	assert.NotNil(t, op.devGetCurrentClocksEventReasonsForAllDevs, "devGetCurrentClocksEventReasonsForAllDevs should be set")

	// Call the functions and verify they return the expected values
	corrRows, uncRows, isPending, failureOccurred, retRows := op.devGetRemappedRowsForAllDevs()
	assert.Equal(t, 1, corrRows, "corrRows should match")
	assert.Equal(t, 2, uncRows, "uncRows should match")
	assert.True(t, isPending, "isPending should match")
	assert.False(t, failureOccurred, "failureOccurred should match")
	assert.Equal(t, nvml.SUCCESS, retRows, "retRows should match")

	reasons, retClock := op.devGetCurrentClocksEventReasonsForAllDevs()
	assert.Equal(t, uint64(42), reasons, "reasons should match")
	assert.Equal(t, nvml.SUCCESS, retClock, "retClock should match")
}
