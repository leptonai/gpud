package process

import (
	"bufio"
	"context"
	"errors"
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
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %d", p.PID())

	// redunant start is ok
	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}

	if err := Read(
		ctx,
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {
			t.Logf("stdout: %q", line)
		}),
	); err != nil {
		t.Fatal(err)
	}

	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if !p.Closed() {
		t.Fatal("process is not aborted")
	}
}

func TestProcessRunBashScriptContents(t *testing.T) {
	p, err := New(
		WithBashScriptContentsToRun(`#!/bin/bash

# do not mask errors in a pipeline
set -o pipefail

echo "hello"
`),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %d", p.PID())

	b, err := io.ReadAll(p.StderrReader())
	if err != nil {
		if !strings.Contains(err.Error(), "file already closed") {
			t.Fatal(err)
		}
	}
	t.Logf("stderr: %q", string(b))

	b, err = io.ReadAll(p.StdoutReader())
	if err != nil {
		if !strings.Contains(err.Error(), "file already closed") {
			t.Fatal(err)
		}
	}
	t.Logf("stdout: %q", string(b))

	select {
	case err := <-p.Wait():
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	proc, _ := p.(*process)
	if proc.Closed() {
		t.Fatal("process is closed")
	}
	bashFile := proc.runBashFile.Name()
	if bashFile == "" {
		t.Fatal("bash file is not created")
	}

	if _, err := os.Stat(bashFile); err != nil {
		t.Fatal(err)
	}

	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// redunant abort is ok
	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}

	if !proc.Closed() {
		t.Fatal("process is not closed")
	}
	if _, err := os.Stat(bashFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestProcessWithBash(t *testing.T) {
	p, err := New(
		WithCommand("echo", "hello"),
		WithCommand("echo hello && echo 111 | grep 1"),
		WithRunAsBashScript(),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %d", p.PID())

	select {
	case err := <-p.Wait():
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestProcessWithTempFile(t *testing.T) {
	// create a temporary file
	tmpFile, err := os.CreateTemp("", "process-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	p, err := New(
		WithCommand("echo", "hello"),
		WithOutputFile(tmpFile),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %d", p.PID())

	select {
	case err := <-p.Wait():
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify the content of the temporary file
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	expectedContent := "hello\n"
	if string(content) != expectedContent {
		t.Fatalf("Expected content %q, but got %q", expectedContent, string(content))
	}
}

func TestProcessWithStdoutReader(t *testing.T) {
	p, err := New(
		WithCommand("echo hello && sleep 1000"),
		WithRunAsBashScript(),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %d", p.PID())

	select {
	case err := <-p.Wait():
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
	}

	rd := p.StdoutReader()
	buf := make([]byte, 1024)
	n, err := rd.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	output := string(buf[:n])
	expectedOutput := "hello\n"
	if output != expectedOutput {
		t.Fatalf("expected output %q, but got %q", expectedOutput, output)
	}
	t.Logf("stdout: %q", output)

	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestProcessWithStdoutReaderUntilEOF(t *testing.T) {
	p, err := New(
		WithCommand("echo hello 1 && sleep 1"),
		WithCommand("echo hello 2 && sleep 1"),
		WithCommand("echo hello 3 && sleep 1"),
		WithRunAsBashScript(),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %d", p.PID())

	rd := p.StdoutReader()
	scanner := bufio.NewScanner(rd)
	var output string
	for scanner.Scan() {
		output += scanner.Text() + "\n"
	}
	expectedOutput := "hello 1\nhello 2\nhello 3\n"
	if output != expectedOutput {
		t.Fatalf("expected output %q, but got %q", expectedOutput, output)
	}
	t.Logf("stdout: %q", output)

	select {
	case err := <-p.Wait():
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
	}

	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if scanner.Err() != nil {
		t.Fatal(scanner.Err())
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
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %d", p.PID())

	for i := 0; i < 3; i++ {
		select {
		case err := <-p.Wait():
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), "exit status 1") {
				t.Log(err)
				continue
			}
			t.Fatal(err)

		case <-time.After(2 * time.Second):
			t.Fatal("timeout")
		}
	}

	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestProcessSleep(t *testing.T) {
	p, err := New(
		WithCommand("sleep", "99999"),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %d", p.PID())

	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-p.Wait():
		if err == nil {
			t.Fatal("expected error")
		}
		t.Log(err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestProcessStream(t *testing.T) {
	opts := []OpOption{
		WithRunAsBashScript(),
	}
	for i := 0; i < 100; i++ {
		opts = append(opts, WithCommand(fmt.Sprintf("echo hello %d && sleep 1", i)))
	}

	p, err := New(opts...)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %d", p.PID())

	rd := p.StdoutReader()
	buf := make([]byte, 1024)
	for i := 0; i < 3; i++ {
		n, err := rd.Read(buf)
		if err != nil {
			t.Fatal(err)
		}

		output := string(buf[:n])
		expectedOutput := fmt.Sprintf("hello %d\n", i)
		if output != expectedOutput {
			t.Fatalf("expected output %q, but got %q", expectedOutput, output)
		}
		t.Logf("stdout: %q", output)
	}

	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestProcessExitCode(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedError  bool
		expectedOutput string
		expectedCode   int32
	}{
		{
			name:           "command with non-zero exit",
			args:           []string{"sh", "-c", "exit 42"},
			expectedError:  true,
			expectedOutput: "",
			expectedCode:   42,
		},
		{
			name:           "successful command",
			args:           []string{"echo", "hello"},
			expectedError:  false,
			expectedOutput: "hello\n",
			expectedCode:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := New(WithCommand(tt.args...))
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := p.Start(ctx); err != nil {
				t.Fatal(err)
			}

			var output string
			if err := Read(
				ctx,
				p,
				WithReadStdout(),
				WithProcessLine(func(line string) {
					output += line + "\n"
				}),
			); err != nil && !tt.expectedError {
				t.Fatal(err)
			}

			select {
			case err := <-p.Wait():
				if tt.expectedError && err == nil {
					t.Error("expected error but got none")
				}
				if !tt.expectedError && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timeout waiting for process to finish")
			}

			if output != tt.expectedOutput {
				t.Errorf("expected output %q, got %q", tt.expectedOutput, output)
			}

			if p.ExitCode() != tt.expectedCode {
				t.Errorf("expected exit code %d, got %d", tt.expectedCode, p.ExitCode())
			}

			if err := p.Close(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
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
			if err != nil {
				t.Fatalf("failed to create process: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			out, err := p.StartAndWaitForCombinedOutput(ctx)
			cancel()
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q but got %q", tt.errorContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if string(out) != tt.expectedOut {
				t.Errorf("expected output %q but got %q", tt.expectedOut, string(out))
			}
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
	if err != nil {
		t.Fatalf("failed to create process: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := p.StartAndWaitForCombinedOutput(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Line 1\nLine 2\nLine 3\n"
	if string(out) != expected {
		t.Errorf("expected output %q but got %q", expected, string(out))
	}
}

func TestStartAndWaitForCombinedOutputAlreadyStarted(t *testing.T) {
	p, err := New(WithCommand("echo", "hello"))
	if err != nil {
		t.Fatalf("failed to create process: %v", err)
	}

	// Start the process first
	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	// Try to call StartAndWaitForCombinedOutput after process is already started
	_, err = p.StartAndWaitForCombinedOutput(ctx)
	if err != ErrProcessAlreadyStarted {
		t.Errorf("expected ErrProcessAlreadyStarted but got %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to create process: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	out, err := p.StartAndWaitForCombinedOutput(ctx)
	cancel()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output contains expected number of lines
	lines := strings.Split(string(out), "\n")
	// -1 because Split includes an empty string after the last newline
	if len(lines)-1 != 1000 {
		t.Errorf("expected 1000 lines but got %d", len(lines)-1)
	}

	// Verify some random lines
	if !strings.Contains(string(out), "Line 1") {
		t.Error("output missing Line 1")
	}
	if !strings.Contains(string(out), "Line 1000") {
		t.Error("output missing Line 1000")
	}
}

func TestProcessWithBashScriptTmpDirAndPattern(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "process-test-bash-*")
	if err != nil {
		t.Fatal(err)
	}
	// Only remove if the directory still exists
	defer func() {
		if _, err := os.Stat(tmpDir); err == nil {
			os.RemoveAll(tmpDir)
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
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := p.Start(ctx); err != nil {
				t.Fatal(err)
			}

			// Get the process instance to access the bash file
			proc, ok := p.(*process)
			if !ok {
				t.Fatal("failed to cast to *process")
			}

			// Verify the bash file exists and matches the pattern
			bashFile := proc.runBashFile.Name()
			if bashFile == "" {
				t.Fatal("bash file is not created")
			}

			// Check if the file is in the specified directory
			if !strings.HasPrefix(bashFile, tmpDir) {
				t.Errorf("expected bash file to be in directory %q, got %q", tmpDir, bashFile)
			}

			// Check if the file matches the pattern
			base := filepath.Base(bashFile)
			matched, err := filepath.Match(strings.ReplaceAll(tt.expectedMatch, "*", "*"), base)
			if err != nil {
				t.Fatal(err)
			}
			if !matched {
				t.Errorf("expected bash file to match pattern %q, got %q", tt.expectedMatch, base)
			}

			// Verify the file exists on disk
			if _, err := os.Stat(bashFile); err != nil {
				t.Fatal(err)
			}

			// Clean up
			if err := p.Close(ctx); err != nil {
				t.Fatal(err)
			}

			// Verify the file is removed after Close
			if _, err := os.Stat(bashFile); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("bash file was not removed after Close")
			}
		})
	}
}
