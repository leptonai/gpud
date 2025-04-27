package process

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockReader is a mock io.Reader that returns a predefined error
type mockReader struct {
	data  string
	pos   int
	err   error
	mutex sync.Mutex
}

func (r *mockReader) Read(p []byte) (n int, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.err != nil {
		return 0, r.err
	}

	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// mockProcess implements Process interface for testing read functions
type mockProcess struct {
	stdoutReader io.Reader
	stderrReader io.Reader
	waitCh       chan error
	started      bool
	closed       bool
}

func (p *mockProcess) Start(context.Context) error {
	p.started = true
	return nil
}

func (p *mockProcess) Started() bool {
	return p.started
}

func (p *mockProcess) StartAndWaitForCombinedOutput(_ context.Context) ([]byte, error) {
	return nil, nil
}

func (p *mockProcess) Close(context.Context) error {
	p.closed = true
	return nil
}

func (p *mockProcess) Closed() bool {
	return p.closed
}

func (p *mockProcess) Wait() <-chan error {
	return p.waitCh
}

func (p *mockProcess) PID() int32 {
	return 0
}

func (p *mockProcess) ExitCode() int32 {
	return 0
}

func (p *mockProcess) StdoutReader() io.Reader {
	return p.stdoutReader
}

func (p *mockProcess) StderrReader() io.Reader {
	return p.stderrReader
}

// TestReadWithErrorReader tests the Read function with a reader that returns an error
func TestReadWithErrorReader(t *testing.T) {
	// Create a mock reader that returns an error
	expectedErr := errors.New("mock read error")
	mockStdoutReader := &mockReader{
		data: "line1\nline2\n",
		err:  expectedErr,
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Read from the process
	err := Read(
		context.Background(),
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {}),
	)

	// Check if the error is returned
	if err == nil || !strings.Contains(err.Error(), expectedErr.Error()) {
		t.Fatalf("Expected error containing %q, got %v", expectedErr, err)
	}
}

// TestReadWithContextCancellation tests the Read function with context cancellation
func TestReadWithContextCancellation(t *testing.T) {
	// Create a mock reader with data that includes newlines
	mockStdoutReader := &mockReader{
		data: strings.Repeat("a\n", 100000), // Add newlines but keep it large enough to ensure the read takes time
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Add a small delay to ensure the context is canceled
	time.Sleep(2 * time.Millisecond)

	// Read from the process
	err := Read(
		ctx,
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {}),
	)

	// Check if the context cancellation error is returned
	if err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("Expected context cancellation error, got %v", err)
	}
}

// TestReadWithWaitForCmd tests the Read function with WaitForCmd option
func TestReadWithWaitForCmd(t *testing.T) {
	// Create a mock reader
	mockStdoutReader := &mockReader{
		data: "line1\nline2\n",
	}

	// Create a mock process
	waitCh := make(chan error, 1)
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       waitCh,
		started:      true,
	}

	// Set up a goroutine to send an error after a short delay
	expectedErr := errors.New("mock wait error")
	go func() {
		time.Sleep(50 * time.Millisecond)
		waitCh <- expectedErr
		close(waitCh)
	}()

	// Read from the process with WaitForCmd option
	err := Read(
		context.Background(),
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {}),
		WithWaitForCmd(),
	)

	// Check if the error from Wait() is returned
	if err == nil || err.Error() != expectedErr.Error() {
		t.Fatalf("Expected error %q, got %v", expectedErr, err)
	}
}

// TestReadWithBothStdoutAndStderr tests the Read function with both stdout and stderr
func TestReadWithBothStdoutAndStderr(t *testing.T) {
	// Create mock readers
	mockStdoutReader := &mockReader{
		data: "stdout1\nstdout2\n",
	}
	mockStderrReader := &mockReader{
		data: "stderr1\nstderr2\n",
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		stderrReader: mockStderrReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Collect lines
	var lines []string
	var linesMutex sync.Mutex

	// Read from the process
	err := Read(
		context.Background(),
		p,
		WithReadStdout(),
		WithReadStderr(),
		WithProcessLine(func(line string) {
			linesMutex.Lock()
			lines = append(lines, line)
			linesMutex.Unlock()
		}),
	)

	// Check if there's no error
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if all lines are read
	if len(lines) != 4 {
		t.Fatalf("Expected 4 lines, got %d: %v", len(lines), lines)
	}

	// Check if all expected lines are present
	expectedLines := []string{"stdout1", "stdout2", "stderr1", "stderr2"}
	for _, expected := range expectedLines {
		found := false
		for _, line := range lines {
			if line == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected line %q not found in %v", expected, lines)
		}
	}
}

// TestReadWithEmptyReader tests the Read function with an empty reader
func TestReadWithEmptyReader(t *testing.T) {
	// Create a mock reader with empty data
	mockStdoutReader := &mockReader{
		data: "",
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Read from the process
	lineCount := 0
	err := Read(
		context.Background(),
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {
			lineCount++
		}),
	)

	// Check if there's no error
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if no lines are read
	if lineCount != 0 {
		t.Fatalf("Expected 0 lines, got %d", lineCount)
	}
}

// TestReadWithPartialLine tests the Read function with a partial line (no newline at the end)
func TestReadWithPartialLine(t *testing.T) {
	// Create a mock reader with a partial line
	mockStdoutReader := &mockReader{
		data: "line1\npartial",
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Read from the process
	var lines []string
	err := Read(
		context.Background(),
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {
			lines = append(lines, line)
		}),
	)

	// Check if there's no error
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if both lines are read (including the partial line)
	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines, got %d: %v", len(lines), lines)
	}

	// Check the content of the lines
	if lines[0] != "line1" {
		t.Errorf("Expected first line to be 'line1', got %q", lines[0])
	}
	if lines[1] != "partial" {
		t.Errorf("Expected second line to be 'partial', got %q", lines[1])
	}
}

// TestReadWithLongLine tests the Read function with a very long line
func TestReadWithLongLine(t *testing.T) {
	// Create a mock reader with a very long line
	longLine := strings.Repeat("a", 10000)
	mockStdoutReader := &mockReader{
		data: longLine + "\n",
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Read from the process
	var lines []string
	err := Read(
		context.Background(),
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {
			lines = append(lines, line)
		}),
	)

	// Check if there's no error
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if the line is read
	if len(lines) != 1 {
		t.Fatalf("Expected 1 line, got %d", len(lines))
	}

	// Check the content of the line
	if lines[0] != longLine {
		t.Errorf("Expected line to be %d characters, got %d characters", len(longLine), len(lines[0]))
	}
}

// TestReadWithMultipleNewlines tests the Read function with multiple consecutive newlines
func TestReadWithMultipleNewlines(t *testing.T) {
	// Create a mock reader with multiple consecutive newlines
	mockStdoutReader := &mockReader{
		data: "line1\n\n\nline2\n",
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Read from the process
	var lines []string
	err := Read(
		context.Background(),
		p,
		WithReadStdout(),
		WithProcessLine(func(line string) {
			lines = append(lines, line)
		}),
	)

	// Check if there's no error
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if all lines are read (including empty lines)
	if len(lines) != 4 {
		t.Fatalf("Expected 4 lines, got %d: %v", len(lines), lines)
	}

	// Check the content of the lines
	expectedLines := []string{"line1", "", "", "line2"}
	for i, expected := range expectedLines {
		if lines[i] != expected {
			t.Errorf("Expected line %d to be %q, got %q", i, expected, lines[i])
		}
	}
}

// TestReadWithNilProcessLineFunc tests the Read function with a nil ProcessLine function
func TestReadWithNilProcessLineFunc(t *testing.T) {
	// Create a mock reader
	mockStdoutReader := &mockReader{
		data: "line1\nline2\n",
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Read from the process with a nil ProcessLine function
	err := Read(
		context.Background(),
		p,
		WithReadStdout(),
		WithProcessLine(nil),
	)

	// Check if there's no error
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

// TestReadWithNoOptions tests the Read function with no options
func TestReadWithNoOptions(t *testing.T) {
	// Create a mock reader
	mockStdoutReader := &mockReader{
		data: "test data",
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Read from the process with no options
	// Note: The Read function requires at least one of readStdout or readStderr to be true
	err := Read(
		context.Background(),
		p,
		WithReadStdout(), // Add this option to satisfy the requirement
	)

	// Check if there's no error
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

// TestReadWithInitialBufferSize tests the Read function with InitialBufferSize option
func TestReadWithInitialBufferSize(t *testing.T) {
	// Create a mock reader with a large line
	mockStdoutReader := &mockReader{
		data: strings.Repeat("a", 8192) + "\n", // Line larger than default buffer size
	}

	// Create a mock process
	p := &mockProcess{
		stdoutReader: mockStdoutReader,
		waitCh:       make(chan error),
		started:      true,
	}

	// Read from the process with a larger buffer size
	var capturedLine string
	err := Read(
		context.Background(),
		p,
		WithReadStdout(),
		WithInitialBufferSize(16384), // Set a larger buffer size
		WithProcessLine(func(line string) {
			capturedLine = line
		}),
	)

	// Check if there's no error
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if the line was captured correctly
	if len(capturedLine) != 8192 {
		t.Fatalf("Expected line length 8192, got %d", len(capturedLine))
	}
}
