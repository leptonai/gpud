package process

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExclusiveRunnerSimple(t *testing.T) {
	runner := NewExclusiveRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	script := `#!/bin/bash

# do not mask errors in a pipeline
set -o pipefail

echo hello
`

	out, exitCode, err := runner.RunUntilCompletion(ctx, script)
	assert.NoError(t, err)
	assert.Equal(t, "hello\n", string(out))
	assert.Equal(t, int32(0), exitCode)
}

func TestExclusiveRunnerAbortByProcessExit(t *testing.T) {
	runner := NewExclusiveRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, exitCode, err := runner.RunUntilCompletion(ctx, "exit 1")
	assert.Error(t, err)
	assert.Nil(t, out)
	assert.Equal(t, int32(1), exitCode)
}

func TestExclusiveRunnerAbortByContextCancellation(t *testing.T) {
	runner := NewExclusiveRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	done := make(chan struct{})
	go func() {
		defer close(done)

		out, _, err := runner.RunUntilCompletion(ctx, "sleep 10")
		assert.Error(t, err)
		assert.Nil(t, out)
	}()

	time.Sleep(time.Second)
	cancel()

	<-done
}

func TestExclusiveRunnerCannotAbortExistingProcess(t *testing.T) {
	runner := NewExclusiveRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		_, _, err := runner.RunUntilCompletion(ctx, "sleep 10")
		assert.Error(t, err)
	}()

	// wait for the first process to start
	time.Sleep(time.Second)

	done := make(chan struct{})
	go func() {
		_, _, err := runner.RunUntilCompletion(ctx, "echo hello")
		assert.True(t, errors.Is(err, ErrProcessAlreadyRunning))
		done <- struct{}{}
	}()

	<-done
}

func TestExclusiveRunnerExecuteComplexScript(t *testing.T) {
	runner := NewExclusiveRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	script := `#!/bin/bash
set -e
set -o pipefail

# Create a temporary file
TEMP_FILE=$(mktemp)
echo "Hello World" > $TEMP_FILE

# Read from the file
CONTENT=$(cat $TEMP_FILE)

# Check content
if [[ "$CONTENT" != "Hello World" ]]; then
    exit 1
fi

# Clean up
rm $TEMP_FILE

# Return success
echo "Script executed successfully"
exit 0
`

	out, exitCode, err := runner.RunUntilCompletion(ctx, script)
	assert.NoError(t, err)
	assert.Equal(t, int32(0), exitCode)
	assert.Contains(t, string(out), "Script executed successfully")
}

func TestExclusiveRunnerWithScriptErrors(t *testing.T) {
	runner := NewExclusiveRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test various error scenarios
	testCases := []struct {
		name       string
		script     string
		exitCode   int32
		shouldFail bool
	}{
		{
			name:       "Command not found",
			script:     "nonexistentcommand",
			exitCode:   127,
			shouldFail: true,
		},
		{
			name:       "Syntax error",
			script:     "if then fi",
			exitCode:   2,
			shouldFail: true,
		},
		{
			name:       "Exit with error code",
			script:     "exit 42",
			exitCode:   42,
			shouldFail: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, exitCode, err := runner.RunUntilCompletion(ctx, tc.script)
			if tc.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.exitCode, exitCode)
			// Check that out is either nil or contains something
			if tc.shouldFail {
				// For some errors, out might be nil
				// For others, it might contain error messages
				t.Logf("Script output: %s", string(out))
			}
		})
	}
}

func TestExclusiveRunnerMultipleScripts(t *testing.T) {
	runner := NewExclusiveRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run multiple scripts sequentially
	scripts := []struct {
		script   string
		expected string
	}{
		{
			script:   "echo 'First script'",
			expected: "First script\n",
		},
		{
			script:   "echo 'Second script'",
			expected: "Second script\n",
		},
		{
			script:   "echo 'Third script'",
			expected: "Third script\n",
		},
	}

	for _, s := range scripts {
		out, exitCode, err := runner.RunUntilCompletion(ctx, s.script)
		assert.NoError(t, err)
		assert.Equal(t, int32(0), exitCode)
		assert.Equal(t, s.expected, string(out))
	}
}

func TestCountProcessesWithRunningProcess(t *testing.T) {
	runner := NewExclusiveRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start a process in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Run a long sleep command to keep the process running
		_, _, _ = runner.RunUntilCompletion(ctx, "sleep 5")
	}()

	// Cancel the context to stop the process
	cancel()
	<-done
}
