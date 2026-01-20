package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBackgroundProcessKilledByDefault demonstrates the default behavior where
// backgrounded processes like "sleep 2 && touch /tmp/marker &" get killed
// when Close() is called because the entire process group is terminated.
//
// This is the SAFE default behavior that prevents orphaned/leaked processes.
//
// USE CASE FOR OVERRIDING:
// Deployment scripts often end with:
//
//	sleep 10 && systemctl restart gpud &
//
// This pattern allows the script to exit immediately while scheduling a
// delayed restart. For such scripts, use WithAllowDetachedProcess(true).
func TestBackgroundProcessKilledByDefault(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a unique marker file path
	markerFile := filepath.Join(os.TempDir(), "gpud-test-default-"+time.Now().Format("20060102150405"))
	defer func() { _ = os.Remove(markerFile) }()

	// Remove marker file if it exists
	_ = os.Remove(markerFile)

	// This script backgrounds a command that:
	// 1. Sleeps for 1 second
	// 2. Creates a marker file
	// With default behavior (Setpgid=true), the backgrounded process will be killed
	// when Close() terminates the process group.
	script := `#!/bin/bash
# Background a delayed file creation
# This simulates "sleep 10 && systemctl restart gpud &"
sleep 1 && touch "` + markerFile + `" &

# Exit immediately (parent bash exits, background process should continue)
exit 0
`

	// Create process WITHOUT WithAllowDetachedProcess (default behavior)
	p, err := New(
		WithBashScriptContentsToRun(script),
		// NOTE: No WithAllowDetachedProcess - uses safe default (Setpgid=true)
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
	// With default behavior (Setpgid=true), it should NOT exist (background process was killed)
	_, err = os.Stat(markerFile)
	require.True(t, os.IsNotExist(err),
		"DEFAULT BEHAVIOR CONFIRMED: Without WithAllowDetachedProcess, marker file should NOT exist "+
			"because the entire process group (including backgrounded processes) was killed.")
	t.Log("Confirmed: Background process was killed by process group termination (safe default)")
}

// TestBackgroundProcessCompletesWithAllowDetachedProcess demonstrates that
// with WithAllowDetachedProcess(true), backgrounded processes are allowed to
// continue running after the parent shell exits.
//
// This test shows that WITH WithAllowDetachedProcess(true), the backgrounded
// process becomes an orphan and continues to run.
func TestBackgroundProcessCompletesWithAllowDetachedProcess(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a unique marker file path
	markerFile := filepath.Join(os.TempDir(), "gpud-test-detached-"+time.Now().Format("20060102150405"))
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

	// Create process WITH WithAllowDetachedProcess(true) - allows orphaned processes
	p, err := New(
		WithBashScriptContentsToRun(script),
		WithAllowDetachedProcess(true), // Allow background processes to become orphans
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

	// Close the process - with WithAllowDetachedProcess(true), only the direct
	// child process is killed, allowing backgrounded processes to continue
	t.Log("Calling Close() - only kills direct child, not backgrounded processes")
	require.NoError(t, p.Close(ctx))

	// Wait for the backgrounded process to complete (sleep 1 + buffer)
	time.Sleep(2 * time.Second)

	// Check if marker file was created
	// WITH WithAllowDetachedProcess(true), it SHOULD exist
	_, err = os.Stat(markerFile)
	require.NoError(t, err,
		"FIX CONFIRMED: With WithAllowDetachedProcess(true), marker file SHOULD exist "+
			"because the backgrounded process was allowed to become an orphan and continue running.")
	t.Log("Confirmed: Background process completed (marker file created)")
}

// TestCloseReturnsQuicklyByDefault tests that without WithAllowDetachedProcess,
// Close() kills the process group immediately (fast cleanup).
func TestCloseReturnsQuicklyByDefault(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	// Create a process WITHOUT WithAllowDetachedProcess (default behavior)
	p, err := New(
		WithBashScriptContentsToRun(`#!/bin/bash
sleep 100 &
exit 0
`),
		// NOTE: No WithAllowDetachedProcess - uses safe default (Setpgid=true)
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

	// Close should return quickly because it kills the process group immediately
	start := time.Now()
	require.NoError(t, p.Close(ctx))
	elapsed := time.Since(start)

	// Without WithAllowDetachedProcess (default), Close() should return quickly
	require.Less(t, elapsed, 500*time.Millisecond,
		"Default behavior: Close() should return quickly after killing process group")
	t.Logf("Close() returned after %v (fast process group termination)", elapsed)
}
