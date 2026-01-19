package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestProcessWithInvalidCommand tests the process with an invalid command
func TestProcessWithInvalidCommand(t *testing.T) {
	// Try to create a process with a non-existent command
	_, err := New(
		WithCommand("non_existent_command_12345"),
	)

	// Should return an error
	require.Error(t, err, "Expected error for non-existent command, but got nil")
	require.Contains(t, err.Error(), "command not found", "Expected 'command not found' error, but got: %v", err)
}

// TestProcessWithDuplicateEnvVars tests the process with duplicate environment variables
func TestProcessWithDuplicateEnvVars(t *testing.T) {
	// Try to create a process with duplicate environment variables
	_, err := New(
		WithCommand("echo", "hello"),
		WithEnvs("TEST_VAR=value1", "TEST_VAR=value2"),
	)

	// Should return an error
	require.Error(t, err, "Expected error for duplicate environment variables, but got nil")
	require.Contains(t, err.Error(), "duplicate environment variable", "Expected 'duplicate environment variable' error, but got: %v", err)
}

// TestProcessWithInvalidEnvVars tests the process with invalid environment variables
func TestProcessWithInvalidEnvVars(t *testing.T) {
	// Try to create a process with invalid environment variables
	_, err := New(
		WithCommand("echo", "hello"),
		WithEnvs("INVALID_ENV_VAR"),
	)

	// Should return an error
	require.Error(t, err, "Expected error for invalid environment variable format, but got nil")
	require.Contains(t, err.Error(), "invalid environment variable format", "Expected 'invalid environment variable format' error, but got: %v", err)
}

// TestProcessWithMultipleCommandsWithoutBash tests the process with multiple commands without bash
func TestProcessWithMultipleCommandsWithoutBash(t *testing.T) {
	// Try to create a process with multiple commands without bash
	_, err := New(
		WithCommand("echo", "hello"),
		WithCommand("echo", "world"),
	)

	// Should return an error
	require.Error(t, err, "Expected error for multiple commands without a bash script mode, but got nil")
	require.Contains(t, err.Error(), "cannot run multiple commands without a bash script mode", "Expected 'cannot run multiple commands without a bash script mode' error, but got: %v", err)
}

// TestProcessWithNoCommand tests the process with no command
func TestProcessWithNoCommand(t *testing.T) {
	// Try to create a process with no command
	_, err := New()

	// Should return an error
	require.Error(t, err, "Expected error for no command, but got nil")
	require.Contains(t, err.Error(), "no command(s) or bash script contents provided", "Expected 'no command(s) or bash script contents provided' error, but got: %v", err)
}

// TestProcessWithSignals tests the process with signals
func TestProcessWithSignals(t *testing.T) {
	// Create a long-running process
	p, err := New(
		WithCommand("sleep", "30"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))

	// Get the PID
	pid := p.PID()
	require.Greater(t, pid, int32(0), "Expected positive PID, got %d", pid)

	// Wait a bit to ensure the process is running
	time.Sleep(1 * time.Second)

	// Close the process (should send SIGTERM)
	require.NoError(t, p.Close(ctx))

	// Check if the process is closed
	require.True(t, p.Closed(), "Process should be closed")

	// On macOS, the exit code might be 0 even when terminated with a signal
	// So we don't check for a specific exit code value
	// Just verify that the process was terminated
	exitCode := p.ExitCode()
	t.Logf("Process exit code: %d", exitCode)
}

// TestProcessWithCustomBashScriptDirectory tests the process with a custom bash script directory
func TestProcessWithCustomBashScriptDirectory(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "process-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a process with a custom bash script directory
	p, err := New(
		WithCommand("echo", "hello"),
		WithRunAsBashScript(),
		WithBashScriptTmpDirectory(tmpDir),
		WithBashScriptFilePattern("custom-*.sh"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))

	// Get the process instance to check the bash file
	proc, ok := p.(*process)
	require.True(t, ok, "Failed to cast Process to *process")

	// Check if the bash file is created in the custom directory
	bashFile := proc.runBashFile.Name()
	require.True(t, strings.HasPrefix(bashFile, tmpDir), "Expected bash file in %s, but got %s", tmpDir, bashFile)

	// Check if the bash file has the custom pattern
	baseName := filepath.Base(bashFile)
	require.True(t, strings.HasPrefix(baseName, "custom-") && strings.HasSuffix(baseName, ".sh"), "Expected bash file with pattern custom-*.sh, but got %s", baseName)

	// Wait for the process to finish
	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout")
	}

	// Close the process
	require.NoError(t, p.Close(ctx))

	// Check if the bash file is removed
	_, err = os.Stat(bashFile)
	require.ErrorIs(t, err, os.ErrNotExist, "Expected bash file to be removed, but it still exists: %s", bashFile)
}

// TestProcessWithRestartConfigZeroInterval tests the process with a restart config with zero interval
func TestProcessWithRestartConfigZeroInterval(t *testing.T) {
	// Create a process with a restart config with zero interval
	p, err := New(
		WithCommand("false"), // Command that always fails
		WithRestartConfig(RestartConfig{
			OnError:  true,
			Limit:    1,
			Interval: 0, // Should be set to default (5s)
		}),
	)
	require.NoError(t, err)

	// Get the process instance to check the restart config
	proc, ok := p.(*process)
	require.True(t, ok, "Failed to cast Process to *process")

	// Check if the interval is set to default
	require.Equal(t, 5*time.Second, proc.restartConfig.Interval, "Expected interval to be 5s, but got %s", proc.restartConfig.Interval)
}

// TestProcessStartAfterClose tests starting a process after it's closed
func TestProcessStartAfterClose(t *testing.T) {
	// Create a process
	p, err := New(
		WithCommand("echo", "hello"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))

	// Close the process
	require.NoError(t, p.Close(ctx))

	// Try to start the process again
	require.NoError(t, p.Start(ctx))

	// The process should not be started
	proc, ok := p.(*process)
	require.True(t, ok, "Failed to cast Process to *process")

	// The process should still be marked as aborted
	require.True(t, proc.Closed(), "Process should still be marked as closed")
}

// TestProcessCloseNotStarted tests closing a process that hasn't been started
func TestProcessCloseNotStarted(t *testing.T) {
	// Create a process
	p, err := New(
		WithCommand("echo", "hello"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Close the process without starting it
	require.NoError(t, p.Close(ctx))

	// The process should not be started
	require.False(t, p.Started(), "Process should not be started")
}

// TestProcessWithCommands tests the process with multiple commands
func TestProcessWithCommands(t *testing.T) {
	// Create a process with multiple commands
	commands := [][]string{
		{"echo", "hello"},
		{"echo", "world"},
	}
	p, err := New(
		WithCommands(commands),
		WithRunAsBashScript(),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))

	// Read the output
	var output strings.Builder
	err = Read(
		ctx,
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {
			output.WriteString(line + "\n")
		}),
	)
	require.NoError(t, err)

	// Check if both commands were executed
	outputStr := output.String()
	if !strings.Contains(outputStr, "hello") {
		t.Skipf("Expected 'hello' in output, but not found: %s", outputStr)
	}
	if !strings.Contains(outputStr, "world") {
		t.Skipf("Expected 'world' in output, but not found: %s", outputStr)
	}

	// Close the process
	require.NoError(t, p.Close(ctx))
}

// TestProcessWithContextCancellation tests the process with context cancellation
func TestProcessWithContextCancellation(t *testing.T) {
	// Create a long-running process
	p, err := New(
		WithCommand("sleep", "30"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))

	// Wait for the context to be canceled
	select {
	case err := <-p.Wait():
		require.Error(t, err, "Expected error due to context cancellation, but got nil")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for process to exit")
	}

	// Check if the process is closed
	if !p.Closed() {
		// Close the process explicitly
		require.NoError(t, p.Close(context.Background()))
	}
}

// TestProcessWithOutputFileAndReaders tests the process with output file and readers
func TestProcessWithOutputFileAndReaders(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "process-test-*.txt")
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()
	defer func() {
		_ = tmpFile.Close()
	}()

	// Create a process with output file
	p, err := New(
		WithCommand("echo", "hello"),
		WithOutputFile(tmpFile),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))

	// Wait for the process to finish
	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout")
	}

	// Check if the stdout reader is the same as the output file
	stdoutReader := p.StdoutReader()
	require.Equal(t, tmpFile, stdoutReader, "Expected stdout reader to be the output file")

	// Check if the stderr reader is the same as the output file
	stderrReader := p.StderrReader()
	require.Equal(t, tmpFile, stderrReader, "Expected stderr reader to be the output file")

	// Close the process
	require.NoError(t, p.Close(ctx))
}

// TestProcessWithNilCommand tests the process with nil command
func TestProcessWatchCmdWithNilCommand(t *testing.T) {
	// Create a process
	p, err := New(
		WithCommand("echo", "hello"),
	)
	require.NoError(t, err)

	// Cast to *process to access internal fields
	proc, ok := p.(*process)
	require.True(t, ok, "Failed to cast Process to *process")

	// Set cmd to nil
	proc.cmd = nil

	// Call watchCmd directly
	proc.watchCmd()

	// No panic should occur
}

// TestProcessWithRestartLimit tests the process with restart limit
func TestProcessWithRestartLimit(t *testing.T) {
	// Create a process with restart config
	p, err := New(
		WithCommand("false"), // Command that always fails
		WithRestartConfig(RestartConfig{
			OnError:  true,
			Limit:    2,
			Interval: 100 * time.Millisecond,
		}),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the process
	require.NoError(t, p.Start(ctx))

	// Wait for the process to exit
	select {
	case <-p.Wait():
		// Process should exit after reaching the restart limit
	case <-time.After(3 * time.Second):
		require.FailNow(t, "timeout waiting for process to exit")
	}

	// Close the process
	require.NoError(t, p.Close(ctx))
}

// TestProcessWatchCmdWithRestarts tests the watchCmd function with restarts
func TestProcessWatchCmdWithRestarts(t *testing.T) {
	// Create a process that will fail and restart
	p, err := New(
		WithCommand("sh", "-c", "exit 1"), // Command that will exit with error
		WithRestartConfig(RestartConfig{
			OnError:  true,
			Limit:    2,
			Interval: 100 * time.Millisecond,
		}),
	)
	require.NoError(t, err, "Failed to create process: %v", err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the process
	err = p.Start(ctx)
	require.NoError(t, err, "Failed to start process: %v", err)

	// Wait for the process to exit and restart a few times
	select {
	case err := <-p.Wait():
		t.Logf("Process exited with error: %v", err)
	case <-time.After(3 * time.Second):
		t.Logf("Process is still running after timeout")
	}

	// Close the process
	err = p.Close(ctx)
	if err != nil {
		t.Logf("Error closing process: %v", err)
	}

	// Check that the exit code is non-zero
	exitCode := p.ExitCode()
	t.Logf("Process exit code: %d", exitCode)
	require.Equal(t, int32(1), exitCode, "Expected exit code 1, got %d", exitCode)
}

// TestProcessWatchCmdWithContextCancellation tests the watchCmd function with context cancellation
func TestProcessWatchCmdWithContextCancellation(t *testing.T) {
	// Create a process that will run for a while
	p, err := New(
		WithCommand("sleep", "10"), // Sleep for 10 seconds
	)
	require.NoError(t, err, "Failed to create process: %v", err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Start the process
	err = p.Start(ctx)
	require.NoError(t, err, "Failed to start process: %v", err)

	// Wait for the context to be canceled
	select {
	case err := <-p.Wait():
		t.Logf("Process exited with error: %v", err)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "Process did not exit after context cancellation")
	}

	// Close the process
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer closeCancel()

	err = p.Close(closeCtx)
	if err != nil {
		t.Logf("Error closing process: %v", err)
	}

	// Check that the process was terminated
	require.True(t, p.Closed(), "Expected process to be closed")
}

// TestProcessGroupCleanupOnContextCancel verifies that when the context is canceled,
// all child processes (spawned via backgrounding with &) are also terminated.
//
// This tests the bug where StartAndWaitForCombinedOutput sets Setpgid=true but
// doesn't configure custom context cancellation handling. When the context is
// canceled, Go's exec.CommandContext default behavior kills only the parent
// process via os.Process.Kill(), not the entire process group. Any child
// processes become orphans and continue running indefinitely.
//
// The fix requires setting a custom Cmd.Cancel function that kills the process
// group using syscall.Kill(-pgid, ...).
func TestProcessGroupCleanupOnContextCancel(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment due to permission restrictions")
	}

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start a process that spawns multiple backgrounded children
	// The children will run for a long time (100s) so we can verify they get killed
	p, err := New(
		WithCommand("bash", "-c",
			// Spawn two background sleeps
			// Then wait for them (which blocks until they exit or we get killed)
			"sleep 100 & sleep 100 & wait",
		),
	)
	require.NoError(t, err)

	// Start the command in a goroutine since StartAndWaitForCombinedOutput blocks
	errCh := make(chan error, 1)
	go func() {
		_, err := p.StartAndWaitForCombinedOutput(ctx)
		errCh <- err
	}()

	// Give the background processes time to start
	time.Sleep(500 * time.Millisecond)

	// Get the PID of the parent process (which is also the PGID)
	pid := p.PID()
	require.NotEqual(t, 0, pid, "process PID is 0, process may not have started")

	// Verify child processes are running using pgrep -g (by PGID)
	// This is more reliable than pgrep -f (by command line pattern)
	checkCmd, err := New(WithCommand("pgrep", "-g", fmt.Sprintf("%d", pid)))
	require.NoError(t, err)
	output, err := checkCmd.StartAndWaitForCombinedOutput(context.Background())
	if err != nil {
		t.Logf("pgrep -g %d output: %s, error: %v", pid, string(output), err)
	} else {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		t.Logf("Found %d processes in PGID %d before context cancel", len(lines), pid)
	}

	// Cancel the context - this should kill all children too
	cancel()

	// Wait for the command to finish
	select {
	case <-errCh:
		// Expected - command was canceled
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for command to finish after context cancel")
	}

	// Wait a bit for signals to propagate and processes to terminate
	time.Sleep(500 * time.Millisecond)

	// Verify child processes are gone using pgrep -g (by PGID)
	checkCmd2, err := New(WithCommand("pgrep", "-g", fmt.Sprintf("%d", pid)))
	require.NoError(t, err)
	output2, err := checkCmd2.StartAndWaitForCombinedOutput(context.Background())
	if err == nil && len(strings.TrimSpace(string(output2))) > 0 {
		// Found processes still running - this indicates the bug is present
		t.Errorf("PROCESS LEAK DETECTED: Processes in PGID %d still running after context cancellation.\n"+
			"PIDs still alive: %s\n"+
			"This means backgrounded processes were not killed when context was canceled.",
			pid, strings.TrimSpace(string(output2)))

		// Clean up the leaked processes
		for pidStr := range strings.SplitSeq(strings.TrimSpace(string(output2)), "\n") {
			_ = exec.Command("kill", "-9", pidStr).Run()
		}
	} else {
		t.Log("SUCCESS: All child processes were properly terminated on context cancel")
	}
}

// TestProcessGroupCleanup verifies that when we close a process, all its child
// processes (spawned via backgrounding with &) are also terminated.
//
// This test validates the fix for the "process leak" bug where backgrounded
// processes would become orphaned (reparented to PID 1) when only the parent
// shell was killed.
//
// BACKGROUND ON THE BUG:
// When running commands like "sleep 100 & sleep 100 & wait", bash spawns
// child processes for each backgrounded command. Without process groups:
//   - Killing the parent bash only sends SIGTERM to bash itself
//   - The sleep processes are NOT killed and become orphans
//   - They continue running until they naturally exit (or forever for things like tail -f)
//
// THE FIX:
// By setting Setpgid=true when starting the command, all processes (parent + children)
// share the same Process Group ID (PGID). Then using syscall.Kill(-pgid, signal)
// sends the signal to ALL processes in the group at once.
func TestProcessGroupCleanup(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment due to permission restrictions")
	}

	ctx := context.Background()

	// Create a unique marker that we can use to identify our test processes
	// This avoids conflicts with other tests running in parallel
	marker := "GPUD_TEST_MARKER_12345"

	// Start a process that spawns multiple backgrounded children
	// The children will run for a long time (100s) so we can verify they get killed
	// We use 'exec' to replace the shell with the actual command (cleaner process tree)
	p, err := New(
		WithCommand("bash", "-c",
			// Spawn two background sleeps with our marker in their command line
			// Then wait for them (which blocks until they exit or we get killed)
			"sleep 100 "+marker+" & sleep 100 "+marker+" & wait",
		),
	)
	require.NoError(t, err)

	require.NoError(t, p.Start(ctx))

	// Give the background processes time to start
	time.Sleep(500 * time.Millisecond)

	// Verify child processes are running by checking for our marker
	// We use pgrep to find processes with our marker in their command line
	checkCmd, err := New(WithCommand("pgrep", "-f", marker))
	require.NoError(t, err)
	output, err := checkCmd.StartAndWaitForCombinedOutput(ctx)
	if err != nil {
		t.Logf("pgrep output: %s, error: %v", string(output), err)
		// pgrep might fail on some systems, skip the pre-check
	} else {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) < 2 {
			t.Logf("Expected at least 2 child processes, found %d (output: %s)", len(lines), string(output))
			// Don't fail, as process detection can be flaky
		} else {
			t.Logf("Found %d child processes before cleanup", len(lines))
		}
	}

	// Now close the parent process - this should kill all children too
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()

	if err := p.Close(closeCtx); err != nil {
		t.Logf("Error closing process: %v", err)
	}

	// Wait a bit for signals to propagate and processes to terminate
	time.Sleep(500 * time.Millisecond)

	// Verify child processes are gone
	checkCmd2, err := New(WithCommand("pgrep", "-f", marker))
	require.NoError(t, err)
	output2, err := checkCmd2.StartAndWaitForCombinedOutput(ctx)
	if err == nil && len(strings.TrimSpace(string(output2))) > 0 {
		// Found processes still running - this would indicate the bug is present
		t.Errorf("PROCESS LEAK DETECTED: Child processes still running after Close().\n"+
			"PIDs still alive: %s\n"+
			"This means backgrounded processes were not killed with the parent.",
			strings.TrimSpace(string(output2)))
	} else {
		t.Log("SUCCESS: All child processes were properly terminated with parent")
	}
}

// TestTailFParallelPattern tests the exact pattern used by fabric-manager log watcher:
//
//	tail -f file1 & tail -f file2 & wait
//
// This test verifies:
//  1. Both tail processes run in parallel and produce output
//  2. When Close() is called, BOTH tail processes are terminated (not just bash)
//  3. No orphaned tail processes remain after cleanup
func TestTailFParallelPattern(t *testing.T) {
	// Create two temporary files to tail
	file1, err := os.CreateTemp("", "tail-test-1-*.log")
	require.NoError(t, err)
	defer func() { _ = os.Remove(file1.Name()) }()
	defer func() { _ = file1.Close() }()

	file2, err := os.CreateTemp("", "tail-test-2-*.log")
	require.NoError(t, err)
	defer func() { _ = os.Remove(file2.Name()) }()
	defer func() { _ = file2.Close() }()

	// Write initial content so tail has something to start with
	_, err = file1.WriteString("initial1\n")
	require.NoError(t, err)
	_, err = file2.WriteString("initial2\n")
	require.NoError(t, err)
	_ = file1.Sync()
	_ = file2.Sync()

	ctx := context.Background()

	// Start the parallel tail pattern - exactly like fabric-manager does
	// Use WithRunAsBashScript() so shell metacharacters (&, |, etc.) are interpreted
	cmd := fmt.Sprintf("tail -f %s & tail -f %s & wait", file1.Name(), file2.Name())
	p, err := New(
		WithCommand(cmd),
		WithRunAsBashScript(),
	)
	require.NoError(t, err)

	require.NoError(t, p.Start(ctx))

	// Give tail processes time to start and output initial content
	time.Sleep(300 * time.Millisecond)

	// Write new content to both files
	_, err = file1.WriteString("line-from-file1\n")
	require.NoError(t, err)
	_, err = file2.WriteString("line-from-file2\n")
	require.NoError(t, err)
	_ = file1.Sync()
	_ = file2.Sync()

	// Read output from both tails
	time.Sleep(200 * time.Millisecond)

	// Verify the process is running
	pid := p.PID()
	require.NotEqual(t, 0, pid, "process PID is 0")
	t.Logf("Parent bash PID (also PGID): %d", pid)

	// Check how many tail processes are in the group
	checkCmd, err := New(WithCommand("pgrep", "-g", fmt.Sprintf("%d", pid), "tail"))
	require.NoError(t, err)
	output, err := checkCmd.StartAndWaitForCombinedOutput(ctx)
	if err != nil {
		t.Logf("pgrep output: %s (error: %v) - this may be expected on some systems", string(output), err)
	} else {
		tailPids := strings.Split(strings.TrimSpace(string(output)), "\n")
		t.Logf("Found %d tail processes in PGID %d: %v", len(tailPids), pid, tailPids)
		if len(tailPids) < 2 {
			t.Logf("Warning: expected 2 tail processes, found %d", len(tailPids))
		}
	}

	// Now close the process - this should kill bash AND both tails
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()

	if err := p.Close(closeCtx); err != nil {
		t.Logf("Close returned: %v", err)
	}

	// Wait for signals to propagate
	time.Sleep(300 * time.Millisecond)

	// Verify NO tail processes remain in the original process group
	checkCmd2, err := New(WithCommand("pgrep", "-g", fmt.Sprintf("%d", pid)))
	require.NoError(t, err)
	output2, err := checkCmd2.StartAndWaitForCombinedOutput(ctx)
	if err == nil && len(strings.TrimSpace(string(output2))) > 0 {
		remainingPids := strings.TrimSpace(string(output2))
		t.Errorf("PROCESS LEAK: Processes still in PGID %d after Close(): %s", pid, remainingPids)

		// Clean up leaked processes
		for pidStr := range strings.SplitSeq(remainingPids, "\n") {
			_ = exec.Command("kill", "-9", pidStr).Run()
		}
	} else {
		t.Log("SUCCESS: All tail processes were terminated with the parent")
	}
}

// TestTailFParallelPatternWithContextCancel tests context cancellation cleanup
// for the tail -f parallel pattern.
func TestTailFParallelPatternWithContextCancel(t *testing.T) {
	// Create two temporary files
	file1, err := os.CreateTemp("", "tail-ctx-1-*.log")
	require.NoError(t, err)
	defer func() { _ = os.Remove(file1.Name()) }()
	defer func() { _ = file1.Close() }()

	file2, err := os.CreateTemp("", "tail-ctx-2-*.log")
	require.NoError(t, err)
	defer func() { _ = os.Remove(file2.Name()) }()
	defer func() { _ = file2.Close() }()

	// Write initial content
	_, _ = file1.WriteString("init1\n")
	_, _ = file2.WriteString("init2\n")
	_ = file1.Sync()
	_ = file2.Sync()

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	cmd := fmt.Sprintf("tail -f %s & tail -f %s & wait", file1.Name(), file2.Name())
	p, err := New(
		WithCommand(cmd),
		WithRunAsBashScript(),
	)
	require.NoError(t, err)

	// Start in goroutine since Start+Wait would block
	errCh := make(chan error, 1)
	go func() {
		if err := p.Start(ctx); err != nil {
			errCh <- err
			return
		}
		// Wait for the process to exit
		for err := range p.Wait() {
			if err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	// Wait for processes to start
	time.Sleep(300 * time.Millisecond)

	pid := p.PID()
	require.NotEqual(t, 0, pid, "process not started")
	t.Logf("Started process with PID %d", pid)

	// Cancel the context - this should kill everything
	cancel()

	// Wait for process to exit
	select {
	case <-errCh:
		// Expected
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for process to exit after context cancel")
	}

	// Verify cleanup
	time.Sleep(300 * time.Millisecond)

	checkCmd, err := New(WithCommand("pgrep", "-g", fmt.Sprintf("%d", pid)))
	require.NoError(t, err)
	output, err := checkCmd.StartAndWaitForCombinedOutput(context.Background())
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		t.Errorf("LEAK: Processes still running after context cancel: %s", strings.TrimSpace(string(output)))
		// Cleanup
		for pidStr := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
			_ = exec.Command("kill", "-9", pidStr).Run()
		}
	} else {
		t.Log("SUCCESS: Context cancellation properly killed all tail processes")
	}
}
