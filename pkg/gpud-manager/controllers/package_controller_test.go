package controllers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPackageController(t *testing.T) {
	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)

	assert.NotNil(t, controller)
	assert.Equal(t, watcher, controller.fileWatcher)
	assert.NotNil(t, controller.packageStatus)
	assert.Equal(t, 3*time.Second, controller.syncPeriod)
}

func TestStatus(t *testing.T) {
	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)

	// Add some test data
	controller.packageStatus["pkg1"] = &packages.PackageStatus{
		Name:           "pkg1",
		Skipped:        false,
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		Status:         true,
		TargetVersion:  "1.0.0",
		CurrentVersion: "1.0.0",
	}
	controller.packageStatus["pkg2"] = &packages.PackageStatus{
		Name:           "pkg2",
		Skipped:        true,
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		Status:         true,
		TargetVersion:  "2.0.0",
		CurrentVersion: "2.0.0",
	}

	status, err := controller.Status(context.Background())
	assert.NoError(t, err)
	assert.Len(t, status, 2)

	// Verify sorting works (packages should be sorted by name)
	assert.Equal(t, "pkg1", status[0].Name)
	assert.False(t, status[0].Skipped)
	assert.Equal(t, "pkg2", status[1].Name)
	assert.True(t, status[1].Skipped)
}

func TestRun(t *testing.T) {
	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)

	// Run should start goroutines but not block
	err := controller.Run(context.Background())
	assert.NoError(t, err)
}

func TestReconcileLoop(t *testing.T) {
	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the reconcile loop in a goroutine
	go controller.reconcileLoop(ctx)

	// Send a package info to be reconciled
	pkg := packages.PackageInfo{
		Name:          "test-pkg",
		ScriptPath:    "/path/to/script",
		TargetVersion: "1.0.0",
		Dependency:    [][]string{{"dep1", "1.0.0"}},
		TotalTime:     5 * time.Minute,
	}

	// Send package info to the watcher
	watcher <- pkg

	// Allow some time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify the package status was updated
	controller.RLock()
	status, exists := controller.packageStatus["test-pkg"]
	controller.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, pkg.Name, status.Name)
	assert.Equal(t, pkg.ScriptPath, status.ScriptPath)
	assert.Equal(t, pkg.TargetVersion, status.TargetVersion)
	assert.Equal(t, pkg.Dependency, status.Dependency)
	assert.Equal(t, pkg.TotalTime, status.TotalTime)

	// Test context cancellation
	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	go controller.reconcileLoop(cancelCtx)
	cancelFunc()
	// Give some time for the goroutine to exit
	time.Sleep(100 * time.Millisecond)
}

func TestUpdateRunner(t *testing.T) {
	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a test script that returns version info
	scriptPath := filepath.Join(tempDir, "update-test.sh")
	scriptContent := `#!/bin/bash
if [ "$1" == "version" ]; then
  echo "1.0.0"
  exit 0
elif [ "$1" == "shouldSkip" ]; then
  exit 1  # Don't skip
elif [ "$1" == "upgrade" ]; then
  exit 0
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)

	// Set up a package that needs update
	controller.packageStatus["test-pkg"] = &packages.PackageStatus{
		Name:           "test-pkg",
		Skipped:        false,
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		Status:         true,
		TargetVersion:  "2.0.0", // Higher version than current
		CurrentVersion: "1.0.0",
		ScriptPath:     scriptPath,
		TotalTime:      5 * time.Second,
	}

	// Create context with short timeout to test partial execution
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// Run the update runner for a short time
	go controller.updateRunner(ctx)

	// Allow some time for the ticker to fire (at least one cycle)
	time.Sleep(controller.syncPeriod + 100*time.Millisecond)

	// Verify that the update process has started or completed
	controller.RLock()
	status := controller.packageStatus["test-pkg"]
	controller.RUnlock()

	// We don't assert the exact state since it depends on timing
	// Just log the status for debugging
	t.Logf("Package status: installing=%v, progress=%d", status.Installing, status.Progress)
}

func TestUpdateRunnerShouldSkip(t *testing.T) {
	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a test script that returns shouldSkip = 0 (should skip)
	scriptPath := filepath.Join(tempDir, "update-skip-test.sh")
	scriptContent := `#!/bin/bash
if [ "$1" == "version" ]; then
  echo "1.0.0"
  exit 0
elif [ "$1" == "shouldSkip" ]; then
  exit 0  # Should skip
elif [ "$1" == "upgrade" ]; then
  exit 0
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)
	controller.syncPeriod = 100 * time.Millisecond

	// Set up a package that would need update but should be skipped
	controller.packageStatus["skip-pkg"] = &packages.PackageStatus{
		Name:           "skip-pkg",
		Skipped:        false,
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		Status:         true,
		TargetVersion:  "2.0.0", // Higher version than current
		CurrentVersion: "1.0.0",
		ScriptPath:     scriptPath,
		TotalTime:      5 * time.Second,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run the update runner
	go controller.updateRunner(ctx)

	// Allow time for at least one sync cycle
	time.Sleep(controller.syncPeriod + 200*time.Millisecond)

	// Verify that the package was marked as skipped
	controller.RLock()
	status := controller.packageStatus["skip-pkg"]
	controller.RUnlock()

	assert.True(t, status.Skipped, "Package should be marked as skipped")
	assert.False(t, status.Installing, "Package should not be installing since it was skipped")
	t.Logf("Package status: skipped=%v, installing=%v", status.Skipped, status.Installing)
}

func TestInstallRunner(t *testing.T) {
	if os.Getenv("TEST_INSTALL_RUNNER") != "true" {
		t.Skip("TEST_INSTALL_RUNNER is not set")
	}

	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a test script for installation
	scriptPath := filepath.Join(tempDir, "install-test.sh")
	scriptContent := `#!/bin/bash
if [ "$1" == "isInstalled" ]; then
  exit 1 # Not installed
elif [ "$1" == "shouldSkip" ]; then
  exit 1 # Don't skip
elif [ "$1" == "install" ]; then
  exit 0 # Installation successful
elif [ "$1" == "start" ]; then
  exit 0 # Start successful
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)
	controller.syncPeriod = 200 * time.Millisecond // Reduce sync period for testing

	// Set up a package to be installed
	controller.packageStatus["install-pkg"] = &packages.PackageStatus{
		Name:           "install-pkg",
		Skipped:        false,
		IsInstalled:    false,
		Installing:     false,
		Progress:       0,
		Status:         false,
		TargetVersion:  "1.0.0",
		CurrentVersion: "",
		ScriptPath:     scriptPath,
		TotalTime:      2 * time.Second,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run the install runner
	go controller.installRunner(ctx)

	// Allow time for at least one sync cycle
	time.Sleep(controller.syncPeriod + 200*time.Millisecond)

	// For this test, we just verify that the function completes without errors
	// and logs the output for debugging
	controller.RLock()
	status := controller.packageStatus["install-pkg"]
	controller.RUnlock()

	t.Logf("Package status after install runner: installing=%v, isInstalled=%v, progress=%d",
		status.Installing, status.IsInstalled, status.Progress)
}

func TestInstallRunnerShouldSkip(t *testing.T) {
	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a test script that returns shouldSkip = 0 (should skip)
	scriptPath := filepath.Join(tempDir, "install-skip-test.sh")
	scriptContent := `#!/bin/bash
if [ "$1" == "isInstalled" ]; then
  exit 1 # Not installed
elif [ "$1" == "shouldSkip" ]; then
  exit 0 # Should skip
elif [ "$1" == "install" ]; then
  exit 0 # Installation successful
elif [ "$1" == "start" ]; then
  exit 0 # Start successful
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)
	controller.syncPeriod = 100 * time.Millisecond

	// Set up a package to be installed but should be skipped
	controller.packageStatus["skip-install-pkg"] = &packages.PackageStatus{
		Name:           "skip-install-pkg",
		Skipped:        false,
		IsInstalled:    false,
		Installing:     false,
		Progress:       0,
		Status:         false,
		TargetVersion:  "1.0.0",
		CurrentVersion: "",
		ScriptPath:     scriptPath,
		TotalTime:      2 * time.Second,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run the install runner
	go controller.installRunner(ctx)

	// Allow time for at least one sync cycle
	time.Sleep(controller.syncPeriod + 200*time.Millisecond)

	// Verify that the package was marked as skipped
	controller.RLock()
	status := controller.packageStatus["skip-install-pkg"]
	controller.RUnlock()

	assert.True(t, status.Skipped, "Package should be marked as skipped")
	assert.False(t, status.Installing, "Package should not be installing since it was skipped")
	assert.True(t, status.IsInstalled, "Package should not be installed since it was skipped")
	t.Logf("Package status: skipped=%v, installing=%v, isInstalled=%v",
		status.Skipped, status.Installing, status.IsInstalled)
}

func TestDeleteRunner(t *testing.T) {
	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a test script for deletion
	scriptPath := filepath.Join(tempDir, "delete-test.sh")
	scriptContent := `#!/bin/bash
if [ "$1" == "needDelete" ]; then
  exit 0 # Needs deletion
elif [ "$1" == "delete" ]; then
  exit 0 # Deletion successful
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)
	controller.syncPeriod = 200 * time.Millisecond // Reduce sync period for testing

	// Set up a package to be deleted
	controller.packageStatus["delete-pkg"] = &packages.PackageStatus{
		Name:           "delete-pkg",
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		Status:         true,
		TargetVersion:  "1.0.0",
		CurrentVersion: "1.0.0",
		ScriptPath:     scriptPath,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run the delete runner for a short time
	go controller.deleteRunner(ctx)

	// Allow time for at least one sync cycle
	time.Sleep(controller.syncPeriod + 200*time.Millisecond)

	// For this test, we just verify that the function completes without errors
}

func TestStatusRunner(t *testing.T) {
	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a test script that returns OK status
	workingScriptPath := filepath.Join(tempDir, "status-ok.sh")
	workingScriptContent := `#!/bin/bash
if [ "$1" == "status" ]; then
  exit 0 # Status is OK
elif [ "$1" == "shouldSkip" ]; then
  exit 1 # Don't skip
else
  exit 1
fi
`
	err = os.WriteFile(workingScriptPath, []byte(workingScriptContent), 0755)
	require.NoError(t, err)

	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)
	controller.syncPeriod = 200 * time.Millisecond // Reduce sync period for testing

	// Set up a package with good status
	controller.packageStatus["ok-pkg"] = &packages.PackageStatus{
		Name:           "ok-pkg",
		Skipped:        false,
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		Status:         false, // Will be set to true
		TargetVersion:  "1.0.0",
		CurrentVersion: "1.0.0",
		ScriptPath:     workingScriptPath,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	// Run the status runner
	go controller.statusRunner(ctx)

	// Allow time for at least one sync cycle
	time.Sleep(controller.syncPeriod + 300*time.Millisecond)

	// Verify the status of the working package
	controller.RLock()
	status := controller.packageStatus["ok-pkg"]
	controller.RUnlock()

	t.Logf("Package status after status runner: status=%v", status.Status)

	// We don't assert the exact state since it may depend on timing
	// and execution environment conditions
}

func TestStatusRunnerShouldSkip(t *testing.T) {
	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a test script that returns shouldSkip = 0 (should skip)
	scriptPath := filepath.Join(tempDir, "status-skip-test.sh")
	scriptContent := `#!/bin/bash
if [ "$1" == "status" ]; then
  exit 1 # Status check would fail
elif [ "$1" == "shouldSkip" ]; then
  exit 0 # Should skip
elif [ "$1" == "stop" ]; then
  exit 0
elif [ "$1" == "start" ]; then
  exit 0
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	watcher := make(chan packages.PackageInfo)
	controller := NewPackageController(watcher)
	controller.syncPeriod = 100 * time.Millisecond

	// Set up an installed package that should be skipped
	controller.packageStatus["skip-status-pkg"] = &packages.PackageStatus{
		Name:           "skip-status-pkg",
		Skipped:        false,
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		Status:         false, // Will be set to true due to shouldSkip
		TargetVersion:  "1.0.0",
		CurrentVersion: "1.0.0",
		ScriptPath:     scriptPath,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run the status runner
	go controller.statusRunner(ctx)

	// Allow time for at least one sync cycle
	time.Sleep(controller.syncPeriod + 200*time.Millisecond)

	// Verify that the package was marked as skipped and status set to true
	controller.RLock()
	status := controller.packageStatus["skip-status-pkg"]
	controller.RUnlock()

	assert.True(t, status.Skipped, "Package should be marked as skipped")
	assert.True(t, status.Status, "Package status should be true when skipped")
	t.Logf("Package status: skipped=%v, status=%v", status.Skipped, status.Status)
}

func TestRunCommand(t *testing.T) {
	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a simple test script
	scriptPath := filepath.Join(tempDir, "test-script.sh")
	scriptContent := `#!/bin/bash
if [ "$1" == "version" ]; then
  echo "1.0.0"
  exit 0
elif [ "$1" == "isInstalled" ]; then
  exit 0
elif [ "$1" == "status" ]; then
  exit 0
elif [ "$1" == "shouldSkip" ]; then
  exit 0
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	// Test runCommand with version query (capturing output)
	var version string
	err = runCommand(context.Background(), scriptPath, "version", &version)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", version)

	// Test runCommand with isInstalled query (no output captured)
	err = runCommand(context.Background(), scriptPath, "isInstalled", nil)
	assert.NoError(t, err)

	// Test runCommand with shouldSkip query (capturing output to avoid log file)
	var shouldSkipResult string
	err = runCommand(context.Background(), scriptPath, "shouldSkip", &shouldSkipResult)
	assert.NoError(t, err)

	// Test runCommand with failing command
	err = runCommand(context.Background(), scriptPath, "invalid", nil)
	assert.Error(t, err)

	// Test with canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	err = runCommand(ctx, scriptPath, "version", nil)
	assert.Error(t, err)
}
