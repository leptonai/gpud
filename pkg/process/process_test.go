package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProcess(t *testing.T) {
	p, err := New(
		WithCommand("echo", "hello"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	t.Logf("pid: %d", p.PID())

	// redunant start is ok
	require.NoError(t, p.Start(ctx))

	require.NoError(t, Read(
		ctx,
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {
			t.Logf("stdout: %q", line)
		}),
	))

	require.NoError(t, p.Close(ctx))
	require.NoError(t, p.Close(ctx))
	require.True(t, p.Closed(), "process is not aborted")
}

func TestProcessRunBashScriptContents(t *testing.T) {
	p, err := New(
		WithBashScriptContentsToRun(`#!/bin/bash

# do not mask errors in a pipeline
set -o pipefail

echo "hello"
`),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	t.Logf("pid: %d", p.PID())

	b, err := io.ReadAll(p.StderrReader())
	if err != nil {
		if !strings.Contains(err.Error(), "file already closed") {
			require.NoError(t, err)
		}
	}
	t.Logf("stderr: %q", string(b))

	b, err = io.ReadAll(p.StdoutReader())
	if err != nil {
		if !strings.Contains(err.Error(), "file already closed") {
			require.NoError(t, err)
		}
	}
	t.Logf("stdout: %q", string(b))

	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout")
	}

	proc, _ := p.(*process)
	require.False(t, proc.Closed(), "process is closed")
	bashFile := proc.runBashFile.Name()
	require.NotEmpty(t, bashFile, "bash file is not created")

	_, err = os.Stat(bashFile)
	require.NoError(t, err)

	require.NoError(t, p.Close(ctx))
	// redunant abort is ok
	require.NoError(t, p.Close(ctx))

	require.True(t, proc.Closed(), "process is not closed")
	_, err = os.Stat(bashFile)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestProcessWithBash(t *testing.T) {
	p, err := New(
		WithCommand("echo", "hello"),
		WithCommand("echo hello && echo 111 | grep 1"),
		WithRunAsBashScript(),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	t.Logf("pid: %d", p.PID())

	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout")
	}

	require.NoError(t, p.Close(ctx))
}

func TestProcessWithTempFile(t *testing.T) {
	// create a temporary file
	tmpFile, err := os.CreateTemp("", "process-test-*.txt")
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()
	defer func() {
		_ = tmpFile.Close()
	}()

	p, err := New(
		WithCommand("echo", "hello"),
		WithOutputFile(tmpFile),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	t.Logf("pid: %d", p.PID())

	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout")
	}

	require.NoError(t, p.Close(ctx))

	// Verify the content of the temporary file
	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)

	expectedContent := "hello\n"
	require.Equal(t, expectedContent, string(content))
}

func TestProcessWithStdoutReader(t *testing.T) {
	p, err := New(
		WithCommand("echo hello && sleep 1000"),
		WithRunAsBashScript(),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	t.Logf("pid: %d", p.PID())

	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-time.After(time.Second):
	}

	rd := p.StdoutReader()
	buf := make([]byte, 1024)
	n, err := rd.Read(buf)
	require.NoError(t, err)
	output := string(buf[:n])
	expectedOutput := "hello\n"
	require.Equal(t, expectedOutput, output)
	t.Logf("stdout: %q", output)

	require.NoError(t, p.Close(ctx))
}

func TestProcessWithStdoutReaderUntilEOF(t *testing.T) {
	p, err := New(
		WithCommand("echo hello 1 && sleep 1"),
		WithCommand("echo hello 2 && sleep 1"),
		WithCommand("echo hello 3 && sleep 1"),
		WithRunAsBashScript(),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	t.Logf("pid: %d", p.PID())

	rd := p.StdoutReader()
	scanner := bufio.NewScanner(rd)
	var output string
	for scanner.Scan() {
		output += scanner.Text() + "\n"
	}
	expectedOutput := "hello 1\nhello 2\nhello 3\n"
	require.Equal(t, expectedOutput, output)
	t.Logf("stdout: %q", output)

	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-time.After(time.Second):
	}

	require.NoError(t, p.Close(ctx))
	if scanner.Err() != nil && !strings.Contains(scanner.Err().Error(), "file already closed") {
		require.NoError(t, scanner.Err())
	}
}

func TestProcessWithRestarts(t *testing.T) {
	p, err := New(
		WithCommand("echo hello"),
		WithCommand("echo 111 && exit 1"),
		WithRunAsBashScript(),
		WithRestartConfig(RestartConfig{
			OnError:  true,
			Limit:    3,
			Interval: 100 * time.Millisecond,
		}),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	t.Logf("pid: %d", p.PID())

	for range 3 {
		select {
		case err := <-p.Wait():
			require.Error(t, err, "expected error")
			if strings.Contains(err.Error(), "exit status 1") {
				t.Log(err)
				continue
			}
			require.NoError(t, err)

		case <-time.After(2 * time.Second):
			require.FailNow(t, "timeout")
		}
	}

	require.NoError(t, p.Close(ctx))
}

func TestProcessSleep(t *testing.T) {
	p, err := New(
		WithCommand("sleep", "99999"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	t.Logf("pid: %d", p.PID())

	require.NoError(t, p.Close(ctx))

	select {
	case err := <-p.Wait():
		require.Error(t, err, "expected error")
		t.Log(err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "timeout")
	}
}

func TestProcessStream(t *testing.T) {
	opts := []OpOption{
		WithRunAsBashScript(),
	}
	for i := range 100 {
		opts = append(opts, WithCommand(fmt.Sprintf("echo hello %d && sleep 1", i)))
	}

	p, err := New(opts...)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	t.Logf("pid: %d", p.PID())

	rd := p.StdoutReader()
	buf := make([]byte, 1024)
	for i := range 3 {
		n, err := rd.Read(buf)
		require.NoError(t, err)

		output := string(buf[:n])
		expectedOutput := fmt.Sprintf("hello %d\n", i)
		require.Equal(t, expectedOutput, output)
		t.Logf("stdout: %q", output)
	}

	require.NoError(t, p.Close(ctx))
}

func TestStartAndWaitForCombinedOutput(t *testing.T) {
	tests := []struct {
		name          string
		command       []string
		expectedOut   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "simple echo command",
			command:     []string{"echo", "hello world"},
			expectedOut: "hello world\n",
			expectError: false,
		},
		{
			name:          "failing command",
			command:       []string{"sh", "-c", "exit 1"},
			expectError:   true,
			errorContains: "command exited with error",
		},
		{
			name:        "command with both stdout and stderr",
			command:     []string{"sh", "-c", "echo stdout message; echo stderr message >&2"},
			expectedOut: "stdout message\nstderr message\n",
			expectError: false,
		},
		{
			name:        "empty output command",
			command:     []string{"true"},
			expectedOut: "",
			expectError: false,
		},
		{
			name:        "command with spaces in arguments",
			command:     []string{"echo", "  spaces  in  between  "},
			expectedOut: "  spaces  in  between  \n",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{tt.command[0]}, tt.command[1:]...)
			p, err := New(WithCommand(args...))
			require.NoError(t, err, "failed to create process")

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			out, err := p.StartAndWaitForCombinedOutput(ctx)
			cancel()
			if tt.expectError {
				require.Error(t, err, "expected error but got nil")
				require.Contains(t, err.Error(), tt.errorContains)
				return
			}
			require.NoError(t, err, "unexpected error")

			require.Equal(t, tt.expectedOut, string(out))
		})
	}
}

// TestStartAndWaitForCombinedOutputLongRunning tests handling of longer running processes
func TestStartAndWaitForCombinedOutputLongRunning(t *testing.T) {
	script := `
		for i in $(seq 1 3); do
			echo "Line $i"
			sleep 0.1
		done
	`
	p, err := New(WithBashScriptContentsToRun(script))
	require.NoError(t, err, "failed to create process")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := p.StartAndWaitForCombinedOutput(ctx)
	require.NoError(t, err, "unexpected error")

	expected := "Line 1\nLine 2\nLine 3\n"
	require.Equal(t, expected, string(out))
}

func TestStartAndWaitForCombinedOutputAlreadyStarted(t *testing.T) {
	p, err := New(WithCommand("echo", "hello"))
	require.NoError(t, err, "failed to create process")

	// Start the process first
	ctx := context.Background()
	require.NoError(t, p.Start(ctx), "failed to start process")

	// Try to call StartAndWaitForCombinedOutput after process is already started
	_, err = p.StartAndWaitForCombinedOutput(ctx)
	require.Equal(t, ErrProcessAlreadyStarted, err)
}

func TestStartAndWaitForCombinedOutputWithStartNonZeroExitCode(t *testing.T) {
	p, err := New(
		WithCommand("sh", "-c", "echo 1 && exit 255"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	_, err = p.StartAndWaitForCombinedOutput(ctx)
	cancel()
	require.Error(t, err)
	require.Contains(t, err.Error(), "command exited with error: exit status 255")
}

func TestStartAndWaitForCombinedOutputWithNonZeroExitCode(t *testing.T) {
	command := `
echo hello 1
echo hello 2
echo hello 3
exit 255
`
	p, err := New(
		WithBashScriptContentsToRun(command),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	out, err := p.StartAndWaitForCombinedOutput(ctx)
	cancel()
	require.Equal(t, out, []byte("hello 1\nhello 2\nhello 3\n"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "command exited with error: exit status 255")
}

func TestStartAndWaitForCombinedOutputWithLongOutput(t *testing.T) {
	// Generate a command that produces a lot of output
	command := "for i in $(seq 1 1000); do echo \"Line $i\"; done"
	p, err := New(
		WithBashScriptContentsToRun(command),
	)
	require.NoError(t, err, "failed to create process")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	out, err := p.StartAndWaitForCombinedOutput(ctx)
	cancel()
	require.NoError(t, err, "unexpected error")

	// Verify output contains expected number of lines
	lines := strings.Split(string(out), "\n")
	// -1 because Split includes an empty string after the last newline
	require.Equal(t, 1000, len(lines)-1, "expected 1000 lines")

	// Verify some random lines
	require.Contains(t, string(out), "Line 1", "output missing Line 1")
	require.Contains(t, string(out), "Line 1000", "output missing Line 1000")
}

func TestProcessWithBashScriptTmpDirAndPattern(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "process-test-bash-*")
	require.NoError(t, err)
	// Only remove if the directory still exists
	defer func() {
		if _, err := os.Stat(tmpDir); err == nil {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	tests := []struct {
		name           string
		pattern        string
		expectedMatch  string
		scriptContents string
	}{
		{
			name:           "custom pattern with prefix",
			pattern:        "test-script-*.sh",
			expectedMatch:  "test-script-*.sh",
			scriptContents: "echo hello",
		},
		{
			name:           "custom pattern with suffix",
			pattern:        "script-*.bash",
			expectedMatch:  "script-*.bash",
			scriptContents: "echo world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := New(
				WithCommand(tt.scriptContents),
				WithRunAsBashScript(),
				WithBashScriptTmpDirectory(tmpDir),
				WithBashScriptFilePattern(tt.pattern),
			)
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			require.NoError(t, p.Start(ctx))

			// Get the process instance to access the bash file
			proc, ok := p.(*process)
			require.True(t, ok, "failed to cast to *process")

			// Verify the bash file exists and matches the pattern
			bashFile := proc.runBashFile.Name()
			require.NotEmpty(t, bashFile, "bash file is not created")

			// Check if the file is in the specified directory
			require.True(t, strings.HasPrefix(bashFile, tmpDir), "expected bash file to be in directory %q, got %q", tmpDir, bashFile)

			// Check if the file matches the pattern
			base := filepath.Base(bashFile)
			matched, err := filepath.Match(strings.ReplaceAll(tt.expectedMatch, "*", "*"), base)
			require.NoError(t, err)
			require.True(t, matched, "expected bash file to match pattern %q, got %q", tt.expectedMatch, base)

			// Verify the file exists on disk
			_, err = os.Stat(bashFile)
			require.NoError(t, err)

			// Clean up
			require.NoError(t, p.Close(ctx))

			// Verify the file is removed after Close
			_, err = os.Stat(bashFile)
			require.ErrorIs(t, err, os.ErrNotExist, "bash file was not removed after Close")
		})
	}
}
