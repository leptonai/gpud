package process

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"sync"
	"testing"
)

// testProcess implements Process interface for testing
type testProcess struct {
	cmd    *exec.Cmd
	waitCh chan error
	stdout io.ReadCloser
	mu     sync.Mutex
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
	return nil
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
	waitCh := make(chan error, 1)
	p := &testProcess{cmd: cmd, waitCh: waitCh, stdout: stdout}

	go func() {
		waitCh <- cmd.Run()
		close(waitCh)
	}()

	return p
}

func TestReadAllStdout(t *testing.T) {
	// Test 1: Basic echo command
	t.Run("basic echo command", func(t *testing.T) {
		p := newTestProcess("echo", "hello world")
		output := ""
		err := ReadAllStdout(context.Background(), p, WithProcessLine(func(line string) {
			output = line
		}))

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
		err := ReadAllStdout(context.Background(), p, WithProcessLine(func(line string) {
			lines = append(lines, line)
		}))

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

		err := ReadAllStdout(context.Background(), p,
			WithProcessLine(func(line string) {}),
			WithWaitForCmd(),
		)

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
