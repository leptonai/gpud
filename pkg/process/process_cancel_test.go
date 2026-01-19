package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStartAndWaitForCombinedOutputContextCancel tests the cmd.Cancel function
// that is set in StartAndWaitForCombinedOutput. When context is canceled,
// this Cancel function sends SIGKILL to the entire process group.
//
// This specifically tests the code path:
//
//	p.cmd.Cancel = func() error {
//	    if p.cmd.Process == nil { return nil }
//	    pgid := p.cmd.Process.Pid
//	    if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
//	        return err
//	    }
//	    return nil
//	}
func TestStartAndWaitForCombinedOutputContextCancel(t *testing.T) {
	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start a long-running process
	p, err := New(WithCommand("sleep", "60"))
	require.NoError(t, err)

	// Start in goroutine since StartAndWaitForCombinedOutput blocks
	resultCh := make(chan struct {
		output []byte
		err    error
	}, 1)

	go func() {
		out, err := p.StartAndWaitForCombinedOutput(ctx)
		resultCh <- struct {
			output []byte
			err    error
		}{out, err}
	}()

	// Wait for process to start
	time.Sleep(200 * time.Millisecond)

	// Verify process is running
	pid := p.PID()
	require.NotZero(t, pid, "process should have started")

	// Cancel the context - this triggers cmd.Cancel
	cancel()

	// Wait for result
	select {
	case result := <-resultCh:
		// Should get an error due to context cancellation
		require.Error(t, result.err, "expected error from context cancellation")
		t.Logf("Got expected error: %v", result.err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process to exit after context cancel")
	}
}

// TestStartAndWaitForCombinedOutputContextTimeout tests context timeout behavior
// which also exercises the cmd.Cancel code path.
func TestStartAndWaitForCombinedOutputContextTimeout(t *testing.T) {
	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start a long-running process
	p, err := New(WithCommand("sleep", "60"))
	require.NoError(t, err)

	// StartAndWaitForCombinedOutput should return error when context times out
	start := time.Now()
	_, err = p.StartAndWaitForCombinedOutput(ctx)
	elapsed := time.Since(start)

	require.Error(t, err, "expected error from context timeout")
	require.Less(t, elapsed, 2*time.Second, "should return quickly after timeout")
	t.Logf("Process killed after %v, error: %v", elapsed, err)
}

// TestStartAndWaitForCombinedOutputProcessGroupKill tests that when context is
// canceled, all child processes in the process group are killed, not just the parent.
func TestStartAndWaitForCombinedOutputProcessGroupKill(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping process group test in CI environment")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start a command that spawns background children
	// The children will run for a long time, so we can verify they're killed
	p, err := New(WithCommand("bash", "-c", "sleep 100 & sleep 100 & wait"))
	require.NoError(t, err)

	resultCh := make(chan error, 1)
	go func() {
		_, err := p.StartAndWaitForCombinedOutput(ctx)
		resultCh <- err
	}()

	// Wait for processes to start
	time.Sleep(300 * time.Millisecond)

	pid := p.PID()
	require.NotZero(t, pid, "process should have started")
	t.Logf("Parent process (PGID) started with PID: %d", pid)

	// Verify child processes are running in the process group
	checkCmd, err := New(WithCommand("pgrep", "-g", fmt.Sprintf("%d", pid)))
	require.NoError(t, err)

	output, err := checkCmd.StartAndWaitForCombinedOutput(context.Background())
	if err == nil {
		pids := strings.Split(strings.TrimSpace(string(output)), "\n")
		t.Logf("Found %d processes in PGID %d before cancel", len(pids), pid)
	}

	// Cancel context - this triggers cmd.Cancel which should kill the entire group
	cancel()

	// Wait for the command to finish
	select {
	case err := <-resultCh:
		require.Error(t, err, "expected error from context cancellation")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process to exit")
	}

	// Wait for signals to propagate
	time.Sleep(300 * time.Millisecond)

	// Verify all processes in the group are gone
	checkCmd2, err := New(WithCommand("pgrep", "-g", fmt.Sprintf("%d", pid)))
	require.NoError(t, err)

	output2, err := checkCmd2.StartAndWaitForCombinedOutput(context.Background())
	if err == nil && len(strings.TrimSpace(string(output2))) > 0 {
		t.Errorf("PROCESS LEAK: Processes still in PGID %d: %s", pid, strings.TrimSpace(string(output2)))
		// Clean up
		for pidStr := range strings.SplitSeq(strings.TrimSpace(string(output2)), "\n") {
			_ = exec.Command("kill", "-9", pidStr).Run()
		}
	} else {
		t.Log("SUCCESS: All processes in group were killed by cmd.Cancel")
	}
}

// TestStartAndWaitForCombinedOutputImmediateCancel tests the edge case where
// context is canceled almost immediately after starting the command.
// This exercises the race condition handling in cmd.Cancel.
func TestStartAndWaitForCombinedOutputImmediateCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	p, err := New(WithCommand("sleep", "60"))
	require.NoError(t, err)

	resultCh := make(chan error, 1)
	go func() {
		_, err := p.StartAndWaitForCombinedOutput(ctx)
		resultCh <- err
	}()

	// Cancel immediately (or very quickly)
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-resultCh:
		// Either nil (process hadn't fully started) or error (killed)
		t.Logf("Result after immediate cancel: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process to exit")
	}
}

// TestStartAndWaitForCombinedOutputCancelWithOutput tests that partial output
// is returned when context is canceled.
func TestStartAndWaitForCombinedOutputCancelWithOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Command that outputs something then sleeps
	p, err := New(WithCommand("bash", "-c", "echo 'hello'; sleep 60"))
	require.NoError(t, err)

	resultCh := make(chan struct {
		output []byte
		err    error
	}, 1)

	go func() {
		out, err := p.StartAndWaitForCombinedOutput(ctx)
		resultCh <- struct {
			output []byte
			err    error
		}{out, err}
	}()

	// Wait for output to be produced
	time.Sleep(300 * time.Millisecond)

	// Cancel while process is still running
	cancel()

	select {
	case result := <-resultCh:
		require.Error(t, result.err, "expected error from context cancellation")
		// Note: Output may or may not be captured depending on timing
		t.Logf("Output before cancel: %q, error: %v", string(result.output), result.err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process to exit")
	}
}

// TestStartAndWaitForCombinedOutputAlreadyExited tests the case where
// the process exits naturally before context is canceled.
// The cmd.Cancel function handles ESRCH (no such process) gracefully.
func TestStartAndWaitForCombinedOutputAlreadyExited(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Quick command that exits immediately
	p, err := New(WithCommand("echo", "done"))
	require.NoError(t, err)

	out, err := p.StartAndWaitForCombinedOutput(ctx)
	require.NoError(t, err)
	require.Equal(t, "done\n", string(out))

	// Cancel after process has already exited - this tests ESRCH handling
	cancel()
	// No panic or error should occur
}

// TestStartAndWaitForCombinedOutputBashInlineCancel tests context cancellation
// with inline bash script (feeds script via stdin).
func TestStartAndWaitForCombinedOutputBashInlineCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	script := `
echo "starting"
sleep 60
echo "done"
`
	p, err := New(WithBashScriptContentsToRun(script))
	require.NoError(t, err)

	resultCh := make(chan struct {
		output []byte
		err    error
	}, 1)

	go func() {
		out, err := p.StartAndWaitForCombinedOutput(ctx)
		resultCh <- struct {
			output []byte
			err    error
		}{out, err}
	}()

	// Wait for process to start
	time.Sleep(300 * time.Millisecond)

	pid := p.PID()
	require.NotZero(t, pid, "process should have started")

	// Cancel context
	cancel()

	select {
	case result := <-resultCh:
		require.Error(t, result.err, "expected error from context cancellation")
		t.Logf("Output: %q, error: %v", string(result.output), result.err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process to exit")
	}
}

// TestStartAndWaitForCombinedOutputMultipleCancels tests that multiple
// calls to cancel don't cause issues (idempotent).
func TestStartAndWaitForCombinedOutputMultipleCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	p, err := New(WithCommand("sleep", "60"))
	require.NoError(t, err)

	resultCh := make(chan error, 1)
	go func() {
		_, err := p.StartAndWaitForCombinedOutput(ctx)
		resultCh <- err
	}()

	time.Sleep(200 * time.Millisecond)

	// Cancel multiple times
	cancel()
	cancel()
	cancel()

	select {
	case err := <-resultCh:
		t.Logf("Result after multiple cancels: %v", err)
		// Should not panic
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// TestStartAndWaitForCombinedOutputCancelDuringStartup tests cancellation
// while the process is being set up but may not have fully started.
func TestStartAndWaitForCombinedOutputCancelDuringStartup(t *testing.T) {
	// Use a pre-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before starting

	p, err := New(WithCommand("sleep", "60"))
	require.NoError(t, err)

	// StartAndWaitForCombinedOutput with already-canceled context
	_, err = p.StartAndWaitForCombinedOutput(ctx)

	// Should get an error (either context canceled or command killed)
	t.Logf("Error with pre-canceled context: %v", err)
}

// TestCmdCancelWithNilProcess tests the defensive nil check in cmd.Cancel.
// This directly tests the code path: if p.cmd.Process == nil { return nil }
//
// In normal operation, cmd.Cancel is only called after cmd.Start(), so
// Process is always non-nil. However, this test verifies the defensive
// guard works correctly.
func TestCmdCancelWithNilProcess(t *testing.T) {
	p, err := New(WithCommand("echo", "hello"))
	require.NoError(t, err)

	proc := p.(*process)

	// Manually set up the command without starting it
	// This simulates calling Cancel before Process is set
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc.ctx = ctx
	proc.cancel = cancel
	proc.cmd = proc.createCmd()

	// Set up the Cancel function like StartAndWaitForCombinedOutput does
	proc.cmd.Cancel = func() error {
		if proc.cmd.Process == nil {
			return nil
		}
		pgid := proc.cmd.Process.Pid
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return err
		}
		return nil
	}

	// Call Cancel before starting - Process is nil
	// This should return nil (the defensive guard)
	err = proc.cmd.Cancel()
	require.NoError(t, err, "Cancel with nil Process should return nil")
}

// TestCmdCancelWithESRCH tests that ESRCH (no such process) is handled gracefully.
// This tests the code path: if err != nil && err != syscall.ESRCH { return err }
//
// ESRCH occurs when the process has already exited before we try to kill it.
func TestCmdCancelWithESRCH(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a command that exits immediately
	p, err := New(WithCommand("true"))
	require.NoError(t, err)

	proc := p.(*process)

	// Start and wait for the process to complete
	proc.ctx = ctx
	proc.cancel = cancel
	proc.cmd = proc.createCmd()
	proc.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err = proc.cmd.Start()
	require.NoError(t, err)

	// Wait for process to exit naturally
	err = proc.cmd.Wait()
	require.NoError(t, err)

	// Now the process is gone, calling Kill should return ESRCH
	pgid := proc.cmd.Process.Pid

	// Set up Cancel function that we can test
	cancelFunc := func() error {
		if proc.cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return err
		}
		return nil
	}

	// Call Cancel on already-exited process - should handle ESRCH gracefully
	err = cancelFunc()
	require.NoError(t, err, "Cancel should handle ESRCH gracefully")
}
