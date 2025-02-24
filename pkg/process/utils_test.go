package process

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testProcess implements Process interface for testing
type testProcess struct {
	labels map[string]string
	cmd    *exec.Cmd
	waitCh chan error
	stdout io.ReadCloser
	stderr io.ReadCloser
	mu     sync.Mutex
}

func (p *testProcess) Labels() map[string]string {
	return p.labels
}

func (p *testProcess) ExitCode() int32 {
	return 0
}

func (p *testProcess) PID() int32 {
	return 0
}

func (p *testProcess) Start(context.Context) error {
	return nil
}

func (p *testProcess) Started() bool {
	return true
}

func (p *testProcess) StartAndWaitForCombinedOutput(_ context.Context) ([]byte, error) {
	return nil, nil
}

func (p *testProcess) StdoutReader() io.Reader {
	p.mu.Lock()
	defer p.mu.Unlock()
	return bufio.NewReader(p.stdout)
}

func (p *testProcess) StderrReader() io.Reader {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stderr == nil {
		return strings.NewReader("")
	}
	return bufio.NewReader(p.stderr)
}

func (p *testProcess) Wait() <-chan error {
	return p.waitCh
}

func (p *testProcess) Close(ctx context.Context) error {
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

func (p *testProcess) Closed() bool {
	return false
}

func newTestProcess(command string, args ...string) *testProcess {
	cmd := exec.Command(command, args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	waitCh := make(chan error, 1)
	p := &testProcess{cmd: cmd, waitCh: waitCh, stdout: stdout, stderr: stderr}

	go func() {
		waitCh <- cmd.Run()
		close(waitCh)
	}()

	return p
}

func TestReadAll(t *testing.T) {
	// Test reading both stdout and stderr
	t.Run("read stdout and stderr", func(t *testing.T) {
		// This command outputs to both stdout and stderr
		p := newTestProcess("sh", "-c", `echo "stdout line" && echo "stderr line" >&2`)
		lines := make([]string, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := Read(
			ctx,
			p,
			WithReadStdout(),
			WithReadStderr(),
			WithProcessLine(func(line string) {
				lines = append(lines, line)
			}),
		)
		cancel()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if len(lines) != 2 {
			t.Errorf("expected 2 lines, got %d", len(lines))
		}

		hasStdout := false
		hasStderr := false
		for _, line := range lines {
			if line == "stdout line" {
				hasStdout = true
			}
			if line == "stderr line" {
				hasStderr = true
			}
		}

		if !hasStdout {
			t.Error("missing stdout line")
		}
		if !hasStderr {
			t.Error("missing stderr line")
		}
	})

	// Test 1: Basic echo command
	t.Run("basic echo command", func(t *testing.T) {
		p := newTestProcess("echo", "hello world")
		output := ""

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := Read(ctx, p, WithReadStdout(), WithProcessLine(func(line string) {
			output = line
		}))
		cancel()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if output != "hello world" {
			t.Errorf("expected 'hello world', got '%s'", output)
		}
	})

	// Test 2: Multiple lines
	t.Run("multiple lines", func(t *testing.T) {
		p := newTestProcess("sh", "-c", "echo 'line1\nline2\nline3'")
		lines := []string{}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := Read(
			ctx,
			p,
			WithReadStdout(),
			WithProcessLine(func(line string) {
				lines = append(lines, line)
			}),
		)
		cancel()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(lines) != 3 {
			t.Errorf("expected 3 lines, got %d", len(lines))
		}
	})

	// Test 3: Wait for command
	t.Run("wait for command", func(t *testing.T) {
		p := newTestProcess("echo", "test")
		completed := false

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := Read(
			ctx,
			p,
			WithReadStdout(),
			WithProcessLine(func(line string) {}),
			WithWaitForCmd(),
		)
		cancel()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		select {
		case <-p.Wait():
			completed = true
		default:
		}

		if !completed {
			t.Error("command should have completed")
		}
	})
}

func TestNilReaders(t *testing.T) {
	// Test nil stdout reader
	t.Run("nil stdout reader", func(t *testing.T) {
		p := &nilReaderProcess{returnNilStdout: true}
		err := Read(context.Background(), p, WithReadStdout())
		if err == nil || err.Error() != "stdout reader is nil" {
			t.Errorf("expected 'stdout reader is nil' error, got %v", err)
		}
	})

	// Test nil stderr reader
	t.Run("nil stderr reader", func(t *testing.T) {
		p := &nilReaderProcess{returnNilStderr: true}
		err := Read(context.Background(), p, WithReadStderr())
		if err == nil || err.Error() != "stderr reader is nil" {
			t.Errorf("expected 'stderr reader is nil' error, got %v", err)
		}
	})

	// Test both nil readers
	t.Run("both nil readers", func(t *testing.T) {
		p := &nilReaderProcess{returnNilStdout: true, returnNilStderr: true}
		err := Read(context.Background(), p, WithReadStdout(), WithReadStderr())
		if err == nil || err.Error() != "stdout reader is nil" {
			t.Errorf("expected 'stdout reader is nil' error, got %v", err)
		}
	})
}

// nilReaderProcess implements Process interface for testing nil reader cases
type nilReaderProcess struct {
	returnNilStdout bool
	returnNilStderr bool
}

func (p *nilReaderProcess) Labels() map[string]string {
	return nil
}

func (p *nilReaderProcess) ExitCode() int32 {
	return 0
}

func (p *nilReaderProcess) PID() int32 {
	return 0
}

func (p *nilReaderProcess) Start(context.Context) error {
	return nil
}

func (p *nilReaderProcess) Started() bool {
	return true
}

func (p *nilReaderProcess) StartAndWaitForCombinedOutput(_ context.Context) ([]byte, error) {
	return nil, nil
}

func (p *nilReaderProcess) StdoutReader() io.Reader {
	if p.returnNilStdout {
		return nil
	}
	return strings.NewReader("")
}

func (p *nilReaderProcess) StderrReader() io.Reader {
	if p.returnNilStderr {
		return nil
	}
	return strings.NewReader("")
}

func (p *nilReaderProcess) Wait() <-chan error {
	ch := make(chan error, 1)
	close(ch)
	return ch
}

func (p *nilReaderProcess) Close(context.Context) error {
	return nil
}

func (p *nilReaderProcess) Closed() bool {
	return false
}

// stateProcess implements Process interface for testing process states
type stateProcess struct {
	isStarted bool
	isAborted bool
}

func (p *stateProcess) Labels() map[string]string {
	return nil
}

func (p *stateProcess) ExitCode() int32 {
	return 0
}

func (p *stateProcess) PID() int32 {
	return 0
}

func (p *stateProcess) Start(context.Context) error {
	return nil
}

func (p *stateProcess) Started() bool {
	return p.isStarted
}

func (p *stateProcess) StartAndWaitForCombinedOutput(_ context.Context) ([]byte, error) {
	return nil, nil
}

func (p *stateProcess) StdoutReader() io.Reader {
	return strings.NewReader("")
}

func (p *stateProcess) StderrReader() io.Reader {
	return strings.NewReader("")
}

func (p *stateProcess) Wait() <-chan error {
	ch := make(chan error, 1)
	close(ch)
	return ch
}

func (p *stateProcess) Close(context.Context) error {
	return nil
}

func (p *stateProcess) Closed() bool {
	return p.isAborted
}

func TestProcessStates(t *testing.T) {
	// Test not started process
	t.Run("not started process", func(t *testing.T) {
		p := &stateProcess{isStarted: false}
		err := Read(context.Background(), p, WithReadStdout())
		if err != ErrProcessNotStarted {
			t.Errorf("expected ErrProcessNotStarted, got %v", err)
		}
	})

	// Test started process
	t.Run("started process", func(t *testing.T) {
		p := &stateProcess{isStarted: true}
		err := Read(context.Background(), p, WithReadStdout())
		if err != nil {
			t.Errorf("expected no error for started process, got %v", err)
		}
	})

	// Test aborted process
	t.Run("aborted process", func(t *testing.T) {
		p := &stateProcess{isStarted: true, isAborted: true}
		err := Read(context.Background(), p, WithReadStdout())
		if err != ErrProcessAborted {
			t.Errorf("expected ErrProcessAborted, got %v", err)
		}
	})
}

func TestRemoveBashFiles(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "remove-bash-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	testFiles := []struct {
		name      string
		content   string
		remove    bool // should be removed by pattern
		preDelete bool // delete before running RemoveBashFiles
	}{
		{
			name:    "test-1.bash",
			content: "echo 1",
			remove:  true,
		},
		{
			name:      "test-2.bash",
			content:   "echo 2",
			remove:    true,
			preDelete: true, // This file will be deleted before running RemoveBashFiles
		},
		{
			name:    "other.txt",
			content: "not a bash file",
			remove:  false,
		},
		{
			name:    "test-3.bash",
			content: "echo 3",
			remove:  true,
		},
	}

	// Create the test files
	for _, tf := range testFiles {
		path := filepath.Join(tmpDir, tf.name)
		if err := os.WriteFile(path, []byte(tf.content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Pre-delete files marked for pre-deletion
	for _, tf := range testFiles {
		if tf.preDelete {
			path := filepath.Join(tmpDir, tf.name)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Run RemoveBashFiles
	RemoveBashFiles(tmpDir, "test-*.bash")

	// Verify the results
	for _, tf := range testFiles {
		path := filepath.Join(tmpDir, tf.name)
		_, err := os.Stat(path)

		if tf.remove && !tf.preDelete {
			// Should be removed
			if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("file %q should have been removed but still exists", tf.name)
			}
		} else if !tf.remove {
			// Should still exist
			if err != nil {
				t.Errorf("file %q should exist but was removed or not found: %v", tf.name, err)
			}
			// Verify content is unchanged
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != tf.content {
				t.Errorf("file %q content was modified, got %q, want %q", tf.name, string(content), tf.content)
			}
		}
	}
}
