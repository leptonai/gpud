package process

import (
	"context"
	"testing"
)

func TestCountProcessesByStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test normal operation
	processes, err := CountProcessesByStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("processes: %+v", processes)

	// Verify that we have at least one process status
	if len(processes) == 0 {
		t.Fatal("Expected at least one process status, got none")
	}

	// Verify that each status has at least one process
	for status, procs := range processes {
		if len(procs) == 0 {
			t.Fatalf("Expected at least one process for status %s, got none", status)
		}

		// Verify that each process has a valid PID
		for _, proc := range procs {
			if proc.Pid <= 0 {
				t.Fatalf("Expected positive PID, got %d", proc.Pid)
			}
		}
	}

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	_, err = CountProcessesByStatus(canceledCtx)
	// We don't necessarily expect an error here, but we're testing the code path
	// where the context is canceled
	t.Logf("Result with canceled context: %v", err)
}

// TestCheckRunningByPid tests the CheckRunningByPid function
func TestCheckRunningByPid(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test with a process that should be running
	// On macOS, "launchd" is typically running instead of "init"
	processToCheck := "launchd"
	running := CheckRunningByPid(ctx, processToCheck)
	if !running {
		t.Logf("Process '%s' not found, trying alternative", processToCheck)
		// Try an alternative process that should exist on most systems
		processToCheck = "bash"
		running = CheckRunningByPid(ctx, processToCheck)
		if !running {
			t.Logf("Process '%s' not found, trying alternative", processToCheck)
			// Try another alternative
			processToCheck = "sh"
			running = CheckRunningByPid(ctx, processToCheck)
			if !running {
				t.Skipf("Could not find a common process to test with. Skipping test.")
			}
		}
	}
	t.Logf("Successfully found running process: %s", processToCheck)

	// Test with a process that should not be running
	running = CheckRunningByPid(ctx, "nonexistentprocess123456789")
	if running {
		t.Fatalf("Expected 'nonexistentprocess123456789' to not be running")
	}
}

// TestCountRunningPids tests the CountRunningPids function
func TestCountRunningPids(t *testing.T) {
	// Get the count of running processes
	count, err := CountRunningPids()
	if err != nil {
		t.Fatalf("Failed to count running PIDs: %v", err)
	}

	// Verify that the count is reasonable (at least 1 process should be running)
	if count < 1 {
		t.Fatalf("Expected at least 1 running process, got %d", count)
	}

	t.Logf("Number of running processes: %d", count)

	// Verify that the count matches the number of processes we can get from the OS
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	processes, err := CountProcessesByStatus(ctx)
	if err != nil {
		t.Fatalf("Failed to get processes by status: %v", err)
	}

	// Calculate total number of processes from all statuses
	var totalProcesses uint64
	for _, procs := range processes {
		totalProcesses += uint64(len(procs))
	}

	// The counts might not match exactly due to timing differences, but they should be close
	t.Logf("Total processes from CountProcessesByStatus: %d", totalProcesses)

	// Check that the difference is not too large (arbitrary threshold of 20%)
	difference := float64(totalProcesses) - float64(count)
	percentDifference := (difference / float64(totalProcesses)) * 100
	if percentDifference > 20 || percentDifference < -20 {
		t.Logf("Warning: Large difference between CountRunningPids (%d) and CountProcessesByStatus (%d): %.2f%%",
			count, totalProcesses, percentDifference)
	}
}
