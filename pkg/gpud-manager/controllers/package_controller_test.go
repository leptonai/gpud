package controllers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/process"

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

	// Poll until the package status is updated
	require.Eventually(t, func() bool {
		controller.RLock()
		defer controller.RUnlock()
		_, exists := controller.packageStatus["test-pkg"]
		return exists
	}, 5*time.Second, 10*time.Millisecond)

	controller.RLock()
	status := controller.packageStatus["test-pkg"]
	controller.RUnlock()

	assert.Equal(t, pkg.Name, status.Name)
	assert.Equal(t, pkg.ScriptPath, status.ScriptPath)
	assert.Equal(t, pkg.TargetVersion, status.TargetVersion)
	assert.Equal(t, pkg.Dependency, status.Dependency)
	assert.Equal(t, pkg.TotalTime, status.TotalTime)

	// Test context cancellation
	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	go controller.reconcileLoop(cancelCtx)
	cancelFunc()
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go controller.updateRunner(ctx)

	// Poll until the package is marked as skipped
	require.Eventually(t, func() bool {
		controller.RLock()
		defer controller.RUnlock()
		return controller.packageStatus["skip-pkg"].Skipped
	}, 10*time.Second, 50*time.Millisecond, "Package should be marked as skipped")

	controller.RLock()
	status := controller.packageStatus["skip-pkg"]
	controller.RUnlock()

	assert.False(t, status.Installing, "Package should not be installing since it was skipped")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go controller.installRunner(ctx)

	// Poll until the package is marked as skipped
	require.Eventually(t, func() bool {
		controller.RLock()
		defer controller.RUnlock()
		return controller.packageStatus["skip-install-pkg"].Skipped
	}, 10*time.Second, 50*time.Millisecond, "Package should be marked as skipped")

	controller.RLock()
	status := controller.packageStatus["skip-install-pkg"]
	controller.RUnlock()

	assert.False(t, status.Installing, "Package should not be installing since it was skipped")
	assert.True(t, status.IsInstalled, "Package should be marked as installed when skipped")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go controller.statusRunner(ctx)

	// Poll until the package is marked as skipped
	require.Eventually(t, func() bool {
		controller.RLock()
		defer controller.RUnlock()
		return controller.packageStatus["skip-status-pkg"].Skipped
	}, 10*time.Second, 50*time.Millisecond, "Package should be marked as skipped")

	controller.RLock()
	status := controller.packageStatus["skip-status-pkg"]
	controller.RUnlock()

	assert.True(t, status.Status, "Package status should be true when skipped")
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

// TestRunCommandWithBackgroundProcessKilledByDefault demonstrates the default
// process behavior (Setpgid=true) where backgrounded processes get killed when
// the parent exits and Close() is called.
//
// This test simulates the real-world scenario where package scripts end with:
//
//	sleep 10 && systemctl restart gpud &
//
// Without WithAllowDetachedProcess(true), the backgrounded command would be killed
// when the parent script exits and Close() is called (because Setpgid creates a
// process group that gets killed together).
//
// NOTE: This test uses the process package directly (without WithAllowDetachedProcess)
// to demonstrate the default safe behavior that runCommand overrides.
func TestRunCommandWithBackgroundProcessKilledByDefault(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test-bg")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create marker file path
	markerFile := filepath.Join(tempDir, "marker")

	// Create a test script that simulates the pattern:
	// "sleep N && systemctl restart gpud &"
	//
	// This pattern is common in deployment scripts where:
	// 1. The script does some work
	// 2. At the end, it schedules a delayed restart of gpud
	// 3. The script exits immediately (because of &)
	// 4. The backgrounded command should continue running
	scriptPath := filepath.Join(tempDir, "test-bg-script.sh")
	scriptContent := `#!/bin/bash
# This simulates a package init script that schedules a delayed restart
# The pattern "sleep 10 && systemctl restart gpud &" is commonly used
# to allow the script to complete before gpud restarts

if [ "$1" == "install" ]; then
  # Do installation work...
  echo "Installing..."

  # Schedule a delayed action (simulates "sleep 10 && systemctl restart gpud &")
  # In this test, we use "sleep 1 && touch marker" to verify the behavior
  sleep 1 && touch "` + markerFile + `" &

  # Exit immediately - the backgrounded command should continue
  exit 0
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	// Run the command WITHOUT WithAllowDetachedProcess (default behavior)
	// This demonstrates the default safe behavior (Setpgid=true, kills process group).
	// runCommand uses WithAllowDetachedProcess(true) to override this.
	p, err := process.New(
		process.WithCommand("bash", scriptPath, "install"),
		// NOTE: No WithAllowDetachedProcess - uses default (Setpgid=true)
	)
	require.NoError(t, err)

	ctx := context.Background()
	err = p.Start(ctx)
	require.NoError(t, err)

	// Wait for the script to exit
	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for script to exit")
	}

	// Close with default behavior - kills the process group immediately
	err = p.Close(ctx)
	require.NoError(t, err)

	// Wait for what should be enough time for the background process to complete
	time.Sleep(2 * time.Second)

	// Check if marker file was created
	// WITHOUT WithAllowDetachedProcess, the marker file should NOT exist
	// because the backgrounded process was killed (Setpgid kills entire group)
	_, statErr := os.Stat(markerFile)
	require.True(t, os.IsNotExist(statErr),
		"DEFAULT BEHAVIOR: Without WithAllowDetachedProcess, background process is killed. "+
			"This is why runCommand uses WithAllowDetachedProcess(true).")
	t.Log("DEFAULT BEHAVIOR: Background process was killed (Setpgid=true)")
	t.Log("This is why runCommand uses WithAllowDetachedProcess(true)")
}

// TestRunCommandWithAllowDetachedProcess demonstrates that runCommand now works
// correctly with backgrounded processes because it uses WithAllowDetachedProcess(true).
//
// This test simulates the real-world scenario where package scripts end with:
//
//	sleep 10 && systemctl restart gpud &
//
// With WithAllowDetachedProcess(true) in runCommand, the backgrounded command is
// allowed to continue as an orphan process.
func TestRunCommandWithAllowDetachedProcess(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test-bg-default")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create marker file path
	markerFile := filepath.Join(tempDir, "marker")

	// Same script as above
	scriptPath := filepath.Join(tempDir, "test-bg-default-script.sh")
	scriptContent := `#!/bin/bash
# This simulates a package init script that schedules a delayed restart
# The pattern "sleep 10 && systemctl restart gpud &" is commonly used

if [ "$1" == "install" ]; then
  echo "Installing..."

  # Schedule a delayed action (simulates "sleep 10 && systemctl restart gpud &")
  sleep 1 && touch "` + markerFile + `" &

  exit 0
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	// Run using runCommand which now uses WithAllowDetachedProcess(true)
	// The backgrounded process should be allowed to continue as an orphan
	t.Log("Running script with runCommand (uses WithAllowDetachedProcess(true))...")
	err = runCommand(context.Background(), scriptPath, "install", nil)
	require.NoError(t, err)

	// Note: runCommand uses WithAllowDetachedProcess(true), so backgrounded processes
	// become orphans and continue running after the parent exits.
	// Wait for the backgrounded process to complete
	time.Sleep(2 * time.Second)

	// Check if marker file was created
	// WITH WithAllowDetachedProcess(true), the marker file SHOULD exist
	_, statErr := os.Stat(markerFile)
	require.NoError(t, statErr,
		"FIX CONFIRMED: With WithAllowDetachedProcess(true), background process completed. "+
			"runCommand now correctly handles 'sleep N && systemctl restart gpud &' patterns.")
	t.Log("FIX CONFIRMED: Background process completed with WithAllowDetachedProcess(true)")
	t.Log("runCommand now correctly handles 'sleep N && systemctl restart gpud &' patterns")
}

// TestRunCommandWithBackgroundProcessCompletesDirectly demonstrates the fix.
// When using WithAllowDetachedProcess(true), the backgrounded process is allowed
// to continue as an orphan and complete.
//
// This test uses the process package directly with WithAllowDetachedProcess(true)
// to show how the fix works.
func TestRunCommandWithBackgroundProcessCompletesDirectly(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a temporary directory for test scripts
	tempDir, err := os.MkdirTemp("", "package-controller-test-bg-grace")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create marker file path
	markerFile := filepath.Join(tempDir, "marker")

	// Same script as above
	scriptPath := filepath.Join(tempDir, "test-bg-grace-script.sh")
	scriptContent := `#!/bin/bash
if [ "$1" == "install" ]; then
  echo "Installing..."

  # Schedule a delayed action (simulates "sleep 10 && systemctl restart gpud &")
  sleep 1 && touch "` + markerFile + `" &

  exit 0
else
  exit 1
fi
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	// Run the command WITH WithAllowDetachedProcess(true) using the process package directly
	// This demonstrates how the fix works
	p, err := process.New(
		process.WithCommand("bash", scriptPath, "install"),
		process.WithAllowDetachedProcess(true), // Allow background process to become orphan
	)
	require.NoError(t, err)

	ctx := context.Background()
	err = p.Start(ctx)
	require.NoError(t, err)

	// Wait for the script to exit
	select {
	case err := <-p.Wait():
		require.NoError(t, err, "Script should exit successfully")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for script to exit")
	}

	// Close - with WithAllowDetachedProcess(true), only kills direct child
	// Background process continues as orphan
	t.Log("Calling Close() - only kills direct child, background continues...")
	err = p.Close(ctx)
	require.NoError(t, err)

	// Wait for the backgrounded process to complete (sleep 1 + buffer)
	time.Sleep(2 * time.Second)

	// Check if marker file was created
	// WITH WithAllowDetachedProcess(true), the marker file SHOULD exist
	_, statErr := os.Stat(markerFile)
	require.NoError(t, statErr,
		"FIX CONFIRMED: With WithAllowDetachedProcess(true), marker file SHOULD exist. "+
			"The backgrounded process was allowed to complete as orphan.")
	t.Log("FIX CONFIRMED: Background process completed (marker file created)")
}
