package process

import (
	"bufio"
	"context"
	"io"
	"os/exec"
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

func (p *testProcess) PID() int32 {
	return 0
}

func (p *testProcess) Start(context.Context) error {
	return nil
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

func (p *testProcess) Abort(ctx context.Context) error {
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
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
