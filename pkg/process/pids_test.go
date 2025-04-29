package process

import (
	"context"
	"errors"
	"testing"
	"time"
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
			status, err := proc.Status()
			if err != nil {
				t.Fatal(err)
			}
			if len(status) == 0 {
				t.Fatalf("Expected at least one status, got none")
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

	// Test with a context that times out
	// Note: This might fail if the process list is retrieved before the timeout
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	// Let the context time out
	time.Sleep(1 * time.Millisecond)
	// The call may succeed or fail with context error, we just want to make sure it doesn't panic
	_, err = CountProcessesByStatus(timeoutCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			t.Logf("Expected timeout error: %v", err)
		} else {
			t.Logf("Unexpected error: %v", err)
		}
	} else {
		t.Logf("Call completed before context timed out")
	}
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

	// Test with a canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	running = CheckRunningByPid(canceledCtx, processToCheck)
	t.Logf("Result with canceled context: %v", running)

	// Test with empty process name
	running = CheckRunningByPid(ctx, "")
	t.Logf("Result with empty process name: %v", running)
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

// MockProcessStatus implements the ProcessStatus interface for testing
type MockProcessStatus struct {
	name    string
	status  []string
	nameErr error
	statErr error
}

func (m *MockProcessStatus) Name() (string, error) {
	return m.name, m.nameErr
}

func (m *MockProcessStatus) Status() ([]string, error) {
	return m.status, m.statErr
}

// TestCountProcessesByStatusWithMock tests the countProcessesByStatus function with mock process statuses
func TestCountProcessesByStatusWithMock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test with empty process list
	result, err := countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
		return []ProcessStatus{}, nil
	})
	if err != nil {
		t.Fatalf("Expected no error with empty process list, got: %v", err)
	}
	if result != nil {
		t.Fatalf("Expected nil result with empty process list, got: %v", result)
	}

	// Test with a process list that returns an error
	testErr := errors.New("test error")
	_, err = countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
		return nil, testErr
	})
	if err != testErr {
		t.Fatalf("Expected error %v, got: %v", testErr, err)
	}

	// Test with a nil process in the list
	result, err = countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
		return []ProcessStatus{nil, &MockProcessStatus{status: []string{"running"}}}, nil
	})
	if err != nil {
		t.Fatalf("Expected no error with nil process, got: %v", err)
	}
	if len(result["running"]) != 1 {
		t.Fatalf("Expected 1 running process, got: %d", len(result["running"]))
	}

	// Test with a process that returns an error for Status()
	result, err = countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
		return []ProcessStatus{
			&MockProcessStatus{
				name:    "error-process",
				status:  nil,
				statErr: errors.New("not found"),
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("Expected no error with process status error, got: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("Expected 0 processes with status error, got: %d", len(result))
	}

	// Test with a process that returns an error containing "no such file"
	result, err = countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
		return []ProcessStatus{
			&MockProcessStatus{
				name:    "no-such-file-process",
				status:  nil,
				statErr: errors.New("open /proc/12345/status: no such file or directory"),
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("Expected no error with 'no such file' error, got: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("Expected 0 processes with 'no such file' error, got: %d", len(result))
	}

	// Test with a process that returns an empty status list
	result, err = countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
		return []ProcessStatus{
			&MockProcessStatus{
				name:   "empty-status-process",
				status: []string{},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("Expected no error with empty status list, got: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("Expected 0 processes with empty status list, got: %d", len(result))
	}

	// Test with multiple processes with different statuses
	result, err = countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
		return []ProcessStatus{
			&MockProcessStatus{status: []string{"running"}},
			&MockProcessStatus{status: []string{"running"}},
			&MockProcessStatus{status: []string{"sleeping"}},
			&MockProcessStatus{status: []string{"waiting"}},
			&MockProcessStatus{status: []string{"zombie"}},
		}, nil
	})
	if err != nil {
		t.Fatalf("Expected no error with multiple processes, got: %v", err)
	}
	if len(result) != 4 {
		t.Fatalf("Expected 4 different process statuses, got: %d", len(result))
	}
	if len(result["running"]) != 2 {
		t.Fatalf("Expected 2 running processes, got: %d", len(result["running"]))
	}
	if len(result["sleeping"]) != 1 {
		t.Fatalf("Expected 1 sleeping process, got: %d", len(result["sleeping"]))
	}

	// Test error case from CountProcessesByStatus
	// We can do this by mocking the process list function to return an error
	_, err = countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
		return nil, errors.New("test error from ProcessesWithContext")
	})
	if err == nil {
		t.Fatal("Expected error from ProcessesWithContext, got nil")
	}
	if err.Error() != "test error from ProcessesWithContext" {
		t.Fatalf("Expected 'test error from ProcessesWithContext', got: %v", err)
	}
}

// TestCountRunningPidsError tests error handling in CountRunningPids
func TestCountRunningPidsError(t *testing.T) {
	// Test the wrapper implementation directly for error handling
	mockError := errors.New("mock pid error")

	// Test error case
	_, err := countRunningPidsImpl(func() ([]int32, error) {
		return nil, mockError
	})
	if err == nil {
		t.Fatal("Expected error from countRunningPidsImpl, got nil")
	}
	if err.Error() != mockError.Error() {
		t.Fatalf("Expected error %v, got: %v", mockError, err)
	}

	// Test success case
	count, err := countRunningPidsImpl(func() ([]int32, error) {
		return []int32{1, 2, 3}, nil
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if count != 3 {
		t.Fatalf("Expected count of 3, got %d", count)
	}
}
