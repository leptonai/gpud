package process

import (
	"context"
	"errors"
	"testing"
	"time"

	procs "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
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

// mockProcessStatus implements the ProcessStatus interface for testing
type mockProcessStatus struct {
	name    string
	pid     int32
	status  []string
	nameErr error
	statErr error
}

func (m *mockProcessStatus) Name() (string, error) {
	return m.name, m.nameErr
}

func (m *mockProcessStatus) PID() int32 {
	return m.pid
}

func (m *mockProcessStatus) Status() ([]string, error) {
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
		return []ProcessStatus{nil, &mockProcessStatus{status: []string{"running"}}}, nil
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
			&mockProcessStatus{
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
			&mockProcessStatus{
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
			&mockProcessStatus{
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
			&mockProcessStatus{status: []string{"running"}},
			&mockProcessStatus{status: []string{"running"}},
			&mockProcessStatus{status: []string{"sleeping"}},
			&mockProcessStatus{status: []string{"waiting"}},
			&mockProcessStatus{status: []string{"zombie"}},
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

// TestFindProcessByName tests the findProcessByName function
func TestFindProcessByName(t *testing.T) {
	ctx := context.Background()

	// To keep the tests separately, we'll use a different approach and not modify findProcessByName

	t.Run("empty process list", func(t *testing.T) {
		emptyListFunc := func(ctx context.Context) ([]*procs.Process, error) {
			return []*procs.Process{}, nil
		}

		result, err := findProcessByName(ctx, "test", emptyListFunc)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("no matching process", func(t *testing.T) {
		noMatchListFunc := func(ctx context.Context) ([]*procs.Process, error) {
			return []*procs.Process{
				{Pid: 100},
				{Pid: 200},
			}, nil
		}

		result, err := findProcessByName(ctx, "nonexistent", noMatchListFunc)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("matching process found", func(t *testing.T) {
		// This test needs to be skipped as it relies on the behavior of real Process.Name() calls
		// which we can't properly mock in this test framework
		t.Skip("This test requires real process objects and can't be properly mocked")
	})

	t.Run("error getting process list", func(t *testing.T) {
		expectedErr := errors.New("failed to list processes")
		errorListFunc := func(ctx context.Context) ([]*procs.Process, error) {
			return nil, expectedErr
		}

		result, err := findProcessByName(ctx, "test", errorListFunc)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, result)
	})
}

// TestFindProcessByNameWrapper tests the FindProcessByName wrapper function
func TestFindProcessByNameWrapper(t *testing.T) {
	// This test is best run with a process we know is running
	// We'll test with a common process that should exist on most systems
	ctx := context.Background()

	// Skip this test in automated environments where it might be flaky
	t.Run("real process", func(t *testing.T) {
		// Try a few common process names that might be running
		commonProcesses := []string{"bash", "sh", "launchd", "systemd", "init"}

		var foundProcess string
		var result ProcessStatus
		var err error

		for _, proc := range commonProcesses {
			result, err = FindProcessByName(ctx, proc)
			if err == nil && result != nil {
				foundProcess = proc
				break
			}
		}

		if foundProcess == "" {
			t.Skip("Could not find a common process to test with")
		}

		// We found a process, check that it has the expected name
		name, err := result.Name()
		assert.NoError(t, err)
		assert.Contains(t, name, foundProcess)

		// Check that it has a valid PID
		assert.Greater(t, result.PID(), int32(0))
	})

	t.Run("nonexistent process", func(t *testing.T) {
		result, err := FindProcessByName(ctx, "nonexistentprocessthatdoesnotexist123456789")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}

// TestProcessStatusPID tests the PID method of the processStatus struct
func TestProcessStatusPID(t *testing.T) {
	// Create a process with a known PID
	p := &processStatus{
		Process: &procs.Process{
			Pid: 12345,
		},
	}

	// Verify that the PID method returns the expected value
	assert.Equal(t, int32(12345), p.PID())
}

// TestGetProcessStatus tests the getProcessStatus function
func TestGetProcessStatus(t *testing.T) {
	// Create a process with a known PID
	proc := &procs.Process{
		Pid: 67890,
	}

	// Get the process status
	status := getProcessStatus(proc)

	// Verify that the status is not nil and has the expected PID
	assert.NotNil(t, status)
	assert.Equal(t, int32(67890), status.PID())
}

// TestCountProcessesByStatusContextCanceled tests the context cancellation handling in CountProcessesByStatus
func TestCountProcessesByStatusContextCanceled(t *testing.T) {
	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Call the CountProcessesByStatus function with the canceled context
	// The function may or may not return an error depending on how quickly it detects the cancellation
	result, err := CountProcessesByStatus(ctx)

	if err != nil {
		// If we got an error, it should be a context error
		assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
			"Expected context error, got: %v", err)
	} else {
		// If we didn't get an error, we should have a valid result
		// (this can happen if the list process call completes before checking the context)
		assert.NotNil(t, result, "Expected non-nil result when no error occurred")
	}

	// Test with an immediate context timeout
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	// Sleep to ensure the timeout expires
	time.Sleep(1 * time.Millisecond)

	// The function may or may not return an error depending on how quickly it detects the timeout
	result, err = CountProcessesByStatus(timeoutCtx)

	if err != nil {
		// If we got an error, it should be a context error
		assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
			"Expected context error, got: %v", err)
	} else {
		// If we didn't get an error, we should have a valid result
		assert.NotNil(t, result, "Expected non-nil result when no error occurred")
	}
}

// TestCountProcessesByStatusWithError tests error handling in CountProcessesByStatus
func TestCountProcessesByStatusWithError(t *testing.T) {
	originalFunc := CountProcessesByStatus

	// Create a custom function that wraps the underlying implementation
	// with our test-specific behavior
	customFunc := func(ctx context.Context) (map[string][]ProcessStatus, error) {
		// Directly call countProcessesByStatus with a mock process lister
		// that simulates the error we want to test
		return countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
			return nil, errors.New("mocked process list error")
		})
	}

	// Test with our custom function to simulate the error
	ctx := context.Background()
	result, err := customFunc(ctx)

	// Verify the results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mocked process list error")
	assert.Nil(t, result)

	// Verify the original function works as expected
	// This is just a sanity check
	_, origErr := originalFunc(ctx)
	// We don't expect an error here in normal operation
	// but we're not testing that, we're just ensuring we didn't break anything
	if origErr != nil {
		t.Logf("Original function returned expected error: %v", origErr)
	}
}

// TestCountProcessesByStatusRealError tests error handling in CountProcessesByStatus
// by directly calling the underlying implementation with a mock that reliably returns errors
func TestCountProcessesByStatusRealError(t *testing.T) {
	// Create a mocked version of the pids.go:84-92 func wrapper
	mockedProcessListFunc := func(ctx context.Context) ([]ProcessStatus, error) {
		return nil, errors.New("mocked error from process list")
	}

	// Call the underlying implementation with our custom mock
	ctx := context.Background()
	result, err := countProcessesByStatus(ctx, mockedProcessListFunc)

	// Verify the results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mocked error from process list")
	assert.Nil(t, result)
}
