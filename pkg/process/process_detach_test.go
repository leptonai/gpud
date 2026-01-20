package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBackgroundProcessKilledWithoutGracePeriod demonstrates the bug where
// backgrounded processes like "sleep 2 && touch /tmp/marker &" get killed
// when Close() is called, even though they should continue running.
//
// This test shows that WITHOUT WithWaitForDetach, the backgrounded process
// is killed and the marker file is never created.
//
// USE CASE: Deployment scripts often end with:
//
//	sleep 10 && systemctl restart gpud &
//
// This pattern allows the script to exit immediately while scheduling a
// delayed restart. Without the grace period fix, the backgrounded command
// gets killed when the process group is terminated.
func TestBackgroundProcessKilledWithoutGracePeriod(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a unique marker file path
	markerFile := filepath.Join(os.TempDir(), "gpud-test-no-grace-"+time.Now().Format("20060102150405"))
	defer func() { _ = os.Remove(markerFile) }()

	// Remove marker file if it exists
	_ = os.Remove(markerFile)

	// This script backgrounds a command that:
	// 1. Sleeps for 1 second
	// 2. Creates a marker file
	// Without grace period, the backgrounded process will be killed before
	// it can create the marker file.
	script := `#!/bin/bash
# Background a delayed file creation
# This simulates "sleep 10 && systemctl restart gpud &"
sleep 1 && touch "` + markerFile + `" &

# Exit immediately (parent bash exits, background process should continue)
exit 0
`

	// Create process WITHOUT grace period
	p, err := New(
		WithBashScriptContentsToRun(script),
		// NOTE: No WithWaitForDetach - this demonstrates the bug
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))
	t.Logf("Process started with PID: %d", p.PID())

	// Wait for bash to exit (it exits immediately after backgrounding)
	select {
	case err := <-p.Wait():
		// Bash exits successfully (exit 0)
		require.NoError(t, err, "bash script should exit successfully")
		t.Log("Bash script exited successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for bash to exit")
	}

	// Close the process - this will kill the entire process group
	// including the backgrounded "sleep 1 && touch marker"
	require.NoError(t, p.Close(ctx))

	// Wait a bit longer than the sleep duration
	time.Sleep(2 * time.Second)

	// Check if marker file was created
	// WITHOUT grace period, it should NOT exist (background process was killed)
	_, err = os.Stat(markerFile)
	require.True(t, os.IsNotExist(err),
		"BUG CONFIRMED: Without grace period, marker file should NOT exist "+
			"because the backgrounded process was killed. If this test fails, "+
			"it means the bug has been inadvertently fixed or the test setup is wrong.")
	t.Log("Confirmed: Background process was killed (marker file not created)")
}

// TestBackgroundProcessCompletesWithGracePeriod demonstrates the fix.
// With WithWaitForDetach, the backgrounded process is allowed to complete
// before the process group is killed.
//
// This test shows that WITH WithWaitForDetach, the backgrounded process
// completes and creates the marker file.
func TestBackgroundProcessCompletesWithGracePeriod(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a unique marker file path
	markerFile := filepath.Join(os.TempDir(), "gpud-test-with-grace-"+time.Now().Format("20060102150405"))
	defer func() { _ = os.Remove(markerFile) }()

	// Remove marker file if it exists
	_ = os.Remove(markerFile)

	// Same script as above - backgrounds a delayed file creation
	script := `#!/bin/bash
# Background a delayed file creation
# This simulates "sleep 10 && systemctl restart gpud &"
sleep 1 && touch "` + markerFile + `" &

# Exit immediately (parent bash exits, background process should continue)
exit 0
`

	// Create process WITH grace period - this is the fix!
	p, err := New(
		WithBashScriptContentsToRun(script),
		WithWaitForDetach(3*time.Second), // Wait up to 3 seconds for background processes
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))
	t.Logf("Process started with PID: %d", p.PID())

	// Wait for bash to exit (it exits immediately after backgrounding)
	select {
	case err := <-p.Wait():
		// Bash exits successfully (exit 0)
		require.NoError(t, err, "bash script should exit successfully")
		t.Log("Bash script exited successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for bash to exit")
	}

	// Close the process - with grace period, it will wait for the
	// backgrounded process to complete before killing
	t.Log("Calling Close() - will wait for grace period...")
	start := time.Now()
	require.NoError(t, p.Close(ctx))
	elapsed := time.Since(start)
	t.Logf("Close() returned after %v", elapsed)

	// Check if marker file was created
	// WITH grace period, it SHOULD exist (background process was allowed to complete)
	_, err = os.Stat(markerFile)
	require.NoError(t, err,
		"FIX CONFIRMED: With grace period, marker file SHOULD exist "+
			"because the backgrounded process was allowed to complete. "+
			"If this test fails, the fix is not working correctly.")
	t.Log("Confirmed: Background process completed (marker file created)")
}

// TestGracePeriodExpiresIfProcessTakesTooLong tests that if the backgrounded
// process takes longer than the grace period, it gets killed after the timeout.
func TestGracePeriodExpiresIfProcessTakesTooLong(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a unique marker file path
	markerFile := filepath.Join(os.TempDir(), "gpud-test-expired-"+time.Now().Format("20060102150405"))
	defer func() { _ = os.Remove(markerFile) }()

	// Remove marker file if it exists
	_ = os.Remove(markerFile)

	// This script backgrounds a command that takes 5 seconds,
	// but we'll only wait 1 second
	script := `#!/bin/bash
# Background a long-running file creation
sleep 5 && touch "` + markerFile + `" &

# Exit immediately
exit 0
`

	// Create process with SHORT grace period (1 second)
	// The backgrounded process takes 5 seconds, so it will be killed
	p, err := New(
		WithBashScriptContentsToRun(script),
		WithWaitForDetach(1*time.Second), // Only wait 1 second
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))
	t.Logf("Process started with PID: %d", p.PID())

	// Wait for bash to exit
	select {
	case err := <-p.Wait():
		require.NoError(t, err)
		t.Log("Bash script exited successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for bash to exit")
	}

	// Close the process - will wait 1 second, then kill
	t.Log("Calling Close() - will wait 1 second grace period then kill...")
	start := time.Now()
	require.NoError(t, p.Close(ctx))
	elapsed := time.Since(start)
	t.Logf("Close() returned after %v", elapsed)

	// Grace period should have been approximately 1 second
	require.Greater(t, elapsed, 800*time.Millisecond, "should have waited for grace period")
	require.Less(t, elapsed, 2*time.Second, "should not wait longer than grace period + overhead")

	// Wait a bit more to ensure the background process would have completed if not killed
	time.Sleep(5 * time.Second)

	// Check if marker file was created
	// It should NOT exist because the process was killed after grace period expired
	_, err = os.Stat(markerFile)
	require.True(t, os.IsNotExist(err),
		"Grace period should have expired and killed the process before "+
			"it could create the marker file")
	t.Log("Confirmed: Background process was killed after grace period expired")
}

// TestGracePeriodWithQuickProcess tests that if the backgrounded process
// completes quickly (before grace period), Close() returns early without
// waiting for the full grace period.
func TestGracePeriodWithQuickProcess(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a unique marker file path
	markerFile := filepath.Join(os.TempDir(), "gpud-test-quick-"+time.Now().Format("20060102150405"))
	defer func() { _ = os.Remove(markerFile) }()

	// Remove marker file if it exists
	_ = os.Remove(markerFile)

	// This script backgrounds a command that completes in 500ms
	script := `#!/bin/bash
# Background a quick file creation
sleep 0.5 && touch "` + markerFile + `" &

# Exit immediately
exit 0
`

	// Create process with LONG grace period (10 seconds)
	// But the backgrounded process only takes 500ms
	p, err := New(
		WithBashScriptContentsToRun(script),
		WithWaitForDetach(10*time.Second), // Long grace period
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))
	t.Logf("Process started with PID: %d", p.PID())

	// Wait for bash to exit
	select {
	case err := <-p.Wait():
		require.NoError(t, err)
		t.Log("Bash script exited successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for bash to exit")
	}

	// Close the process - should return early when background process completes
	t.Log("Calling Close() - should return early when background process completes...")
	start := time.Now()
	require.NoError(t, p.Close(ctx))
	elapsed := time.Since(start)
	t.Logf("Close() returned after %v", elapsed)

	// Should return much faster than the 10 second grace period
	// (background process takes ~500ms + polling overhead)
	require.Less(t, elapsed, 3*time.Second,
		"Close() should return early when background process completes, "+
			"not wait for full grace period")

	// Check if marker file was created
	_, err = os.Stat(markerFile)
	require.NoError(t, err,
		"Background process should have completed and created marker file")
	t.Log("Confirmed: Close() returned early after background process completed")
}

// TestGracePeriodContextCancelation tests that if the caller's context is
// canceled during the grace period, Close() proceeds to kill immediately.
func TestGracePeriodContextCancelation(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a unique marker file path
	markerFile := filepath.Join(os.TempDir(), "gpud-test-ctx-cancel-"+time.Now().Format("20060102150405"))
	defer func() { _ = os.Remove(markerFile) }()

	// Remove marker file if it exists
	_ = os.Remove(markerFile)

	// This script backgrounds a long-running command
	script := `#!/bin/bash
# Background a long-running file creation
sleep 10 && touch "` + markerFile + `" &

# Exit immediately
exit 0
`

	// Create process with long grace period
	p, err := New(
		WithBashScriptContentsToRun(script),
		WithWaitForDetach(30*time.Second), // Very long grace period
	)
	require.NoError(t, err)

	// Create a context that we'll cancel during Close()
	ctx, cancel := context.WithCancel(context.Background())

	// Start the process
	require.NoError(t, p.Start(ctx))
	t.Logf("Process started with PID: %d", p.PID())

	// Wait for bash to exit
	select {
	case err := <-p.Wait():
		require.NoError(t, err)
		t.Log("Bash script exited successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for bash to exit")
	}

	// Cancel context after a short delay (in a goroutine)
	go func() {
		time.Sleep(500 * time.Millisecond)
		t.Log("Canceling context...")
		cancel()
	}()

	// Close the process - should respect context cancellation
	t.Log("Calling Close() - context will be canceled after 500ms...")
	start := time.Now()
	require.NoError(t, p.Close(ctx))
	elapsed := time.Since(start)
	t.Logf("Close() returned after %v", elapsed)

	// Should return quickly after context cancellation, not wait full grace period
	require.Less(t, elapsed, 2*time.Second,
		"Close() should respect context cancellation and return early")

	// Wait to ensure the background process would have created the file if not killed
	time.Sleep(2 * time.Second)

	// Check if marker file was created
	// It should NOT exist because context was canceled and process was killed
	_, err = os.Stat(markerFile)
	require.True(t, os.IsNotExist(err),
		"Context cancellation should have interrupted grace period and killed process")
	t.Log("Confirmed: Context cancellation interrupted grace period")
}

// TestNoGracePeriodByDefault ensures that without WithWaitForDetach,
// Close() kills the process group immediately (existing behavior preserved).
func TestNoGracePeriodByDefault(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a process WITHOUT grace period
	p, err := New(
		WithBashScriptContentsToRun(`#!/bin/bash
sleep 100 &
exit 0
`),
		// NOTE: No WithWaitForDetach
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))

	// Wait for bash to exit
	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}

	// Close should return immediately (no grace period)
	start := time.Now()
	require.NoError(t, p.Close(ctx))
	elapsed := time.Since(start)

	// Without grace period, Close() should return quickly
	require.Less(t, elapsed, 500*time.Millisecond,
		"Without grace period, Close() should return immediately")
	t.Logf("Close() returned after %v (no grace period)", elapsed)
}
