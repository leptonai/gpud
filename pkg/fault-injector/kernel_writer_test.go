package faultinjector

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultDevKmsg(t *testing.T) {
	assert.Equal(t, "/dev/kmsg", DefaultDevKmsg)
}

func TestOpenKmsgForWrite(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping test on non-Linux platform")
	}

	// Test with non-existent file
	_, err := openKmsgForWrite("/non/existent/file")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open")

	// Test with /dev/kmsg (may fail if not running as root)
	if os.Geteuid() == 0 {
		writer, err := openKmsgForWrite(DefaultDevKmsg)
		if err == nil {
			defer func() {
				_ = writer.(*os.File).Close()
			}()
			assert.NotNil(t, writer)
		}
	}
}

func TestOpenKmsgForWrite_WithTempFile(t *testing.T) {
	// Create a temporary file to simulate kmsg device
	tmpFile, err := os.CreateTemp("", "test-kmsg-*")
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()
	require.NoError(t, tmpFile.Close())

	// Test opening the temporary file
	writer, err := openKmsgForWrite(tmpFile.Name())
	if err == nil {
		defer func() {
			_ = writer.(*os.File).Close()
		}()
		assert.NotNil(t, writer)

		// Test writing to the file
		_, writeErr := writer.Write([]byte("test"))
		assert.NoError(t, writeErr)
	}
}

func TestNewWriter(t *testing.T) {
	tests := []struct {
		name     string
		devFile  string
		setup    func()
		teardown func()
		check    func(t *testing.T, writer KmsgWriter)
	}{
		{
			name:    "non-Linux returns noOpWriter",
			devFile: "",
			setup: func() {
				// No need to skip - the test will verify behavior on current platform
			},
			teardown: func() {},
			check: func(t *testing.T, writer KmsgWriter) {
				if runtime.GOOS != "linux" {
					_, ok := writer.(*noOpWriter)
					assert.True(t, ok, "Expected noOpWriter on non-Linux platform")
				}
			},
		},
		{
			name:    "non-root returns noOpWriter",
			devFile: "",
			setup: func() {
				// No need to skip - the test will verify behavior on current platform
			},
			teardown: func() {},
			check: func(t *testing.T, writer KmsgWriter) {
				if runtime.GOOS == "linux" && os.Geteuid() != 0 {
					_, ok := writer.(*noOpWriter)
					assert.True(t, ok, "Expected noOpWriter when not running as root")
				}
			},
		},
		{
			name:    "empty devFile uses default",
			devFile: "",
			setup: func() {
				// This test will return noOpWriter on non-Linux or non-root
			},
			teardown: func() {},
			check: func(t *testing.T, writer KmsgWriter) {
				assert.NotNil(t, writer)
			},
		},
		{
			name:    "invalid devFile returns noOpWriter",
			devFile: "/invalid/path/to/kmsg",
			setup: func() {
				// No need to skip - the test will verify behavior on current platform
			},
			teardown: func() {},
			check: func(t *testing.T, writer KmsgWriter) {
				// On any platform, invalid device file should return noOpWriter
				_, ok := writer.(*noOpWriter)
				assert.True(t, ok, "Expected noOpWriter for invalid device file")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer tt.teardown()

			writer := NewWriter(tt.devFile)
			tt.check(t, writer)
		})
	}
}

func TestNoOpWriter_Write(t *testing.T) {
	writer := &noOpWriter{}

	msg := &KernelMessage{
		Priority: KernelMessagePriorityInfo,
		Message:  "test message",
	}

	err := writer.Write(msg)
	assert.NoError(t, err)

	// Test with nil message
	err = writer.Write(nil)
	assert.NoError(t, err)
}

// mockWriter is a mock implementation of io.Writer for testing
type mockWriter struct {
	written []byte
	err     error
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	m.written = append(m.written, p...)
	return len(p), nil
}

func TestKmsgWriter_Write(t *testing.T) {
	tests := []struct {
		name        string
		msg         *KernelMessage
		expectedErr error
		checkOutput func(t *testing.T, output string)
	}{
		{
			name: "simple message",
			msg: &KernelMessage{
				Priority: KernelMessagePriorityInfo,
				Message:  "test message",
			},
			expectedErr: nil,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "<46>test message\n")
			},
		},
		{
			name: "message with newline",
			msg: &KernelMessage{
				Priority: KernelMessagePriorityError,
				Message:  "line1\nline2\n",
			},
			expectedErr: nil,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "<43>line1\n")
				assert.Contains(t, output, "<43>line2\n")
			},
		},
		{
			name: "message with tabs",
			msg: &KernelMessage{
				Priority: KernelMessagePriorityWarning,
				Message:  "message\twith\ttabs",
			},
			expectedErr: nil,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "<44>message with tabs\n")
			},
		},
		{
			name: "empty message",
			msg: &KernelMessage{
				Priority: KernelMessagePriorityDebug,
				Message:  "",
			},
			expectedErr: nil,
			checkOutput: func(t *testing.T, output string) {
				assert.Equal(t, "", output)
			},
		},
		{
			name: "very long message",
			msg: &KernelMessage{
				Priority: KernelMessagePriorityInfo,
				Message:  strings.Repeat("a", MaxPrintkRecordLength+100),
			},
			expectedErr: nil,
			checkOutput: func(t *testing.T, output string) {
				// Should be truncated with "..." at the end
				assert.Contains(t, output, "...\n")
				assert.LessOrEqual(t, len(output), MaxPrintkRecordLength)
			},
		},
		{
			name: "message with multiple lines and tabs",
			msg: &KernelMessage{
				Priority: KernelMessagePriorityCrit,
				Message:  "error:\n\tdetail1\n\tdetail2\n",
			},
			expectedErr: nil,
			checkOutput: func(t *testing.T, output string) {
				lines := strings.Split(output, "\n")
				assert.Contains(t, lines[0], "<42>error:")
				assert.Contains(t, lines[1], "<42> detail1")
				assert.Contains(t, lines[2], "<42> detail2")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockWriter{}
			writer := &kmsgWriter{wr: mock}

			err := writer.Write(tt.msg)
			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
				tt.checkOutput(t, string(mock.written))
			}
		})
	}
}

func TestKmsgWriter_WriteError(t *testing.T) {
	expectedErr := errors.New("write error")
	mock := &mockWriter{err: expectedErr}
	writer := &kmsgWriter{wr: mock}

	msg := &KernelMessage{
		Priority: KernelMessagePriorityInfo,
		Message:  "test message",
	}

	err := writer.Write(msg)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestKmsgWriter_InterfaceCompliance(t *testing.T) {
	// Ensure both implementations satisfy the interface
	var _ KmsgWriter = &noOpWriter{}
	var _ KmsgWriter = &kmsgWriter{}
}

func TestKmsgWriter_MultilineMessageSplitting(t *testing.T) {
	mock := &mockWriter{}
	writer := &kmsgWriter{wr: mock}

	// Test message with multiple newlines
	msg := &KernelMessage{
		Priority: KernelMessagePriorityInfo,
		Message:  "line1\nline2\nline3",
	}

	err := writer.Write(msg)
	require.NoError(t, err)

	output := string(mock.written)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	assert.Len(t, lines, 3)
	assert.Contains(t, lines[0], "<46>line1")
	assert.Contains(t, lines[1], "<46>line2")
	assert.Contains(t, lines[2], "<46>line3")
}

func TestKmsgWriter_MessageWithOnlyNewlines(t *testing.T) {
	mock := &mockWriter{}
	writer := &kmsgWriter{wr: mock}

	msg := &KernelMessage{
		Priority: KernelMessagePriorityInfo,
		Message:  "\n\n\n",
	}

	err := writer.Write(msg)
	require.NoError(t, err)

	output := string(mock.written)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Each newline should produce an empty message line
	for _, line := range lines {
		if line != "" {
			assert.Equal(t, "<46>", line)
		}
	}
}

func TestKmsgWriter_LongLineTruncation(t *testing.T) {
	mock := &mockWriter{}
	writer := &kmsgWriter{wr: mock}

	// Create a message that will exceed MaxPrintkRecordLength after adding priority
	longMessage := strings.Repeat("a", MaxPrintkRecordLength)
	msg := &KernelMessage{
		Priority: KernelMessagePriorityInfo,
		Message:  longMessage,
	}

	err := writer.Write(msg)
	require.NoError(t, err)

	output := string(mock.written)

	// Verify the output was truncated
	assert.LessOrEqual(t, len(output), MaxPrintkRecordLength)
	assert.Contains(t, output, "...")
	assert.True(t, strings.HasSuffix(output, "...\n"), "Expected output to end with '...\\n'")
}

// Benchmark tests

// Example test
func ExampleNewWriter() {
	// Create a new writer (will be noOpWriter on non-Linux or non-root)
	writer := NewWriter("")

	// Write a kernel message
	msg := &KernelMessage{
		Priority: KernelMessagePriorityInfo,
		Message:  "System initialized successfully",
	}

	_ = writer.Write(msg)
	// Output:
}

func ExampleKernelMessage() {
	msg := &KernelMessage{
		Priority: KernelMessagePriorityWarning,
		Message:  "Temperature threshold exceeded",
	}

	// Validate the message
	if err := msg.Validate(); err != nil {
		fmt.Printf("Invalid message: %v\n", err)
	}

	// Get syslog priority
	priority := msg.Priority.SyslogPriority()
	fmt.Printf("Syslog priority: %d\n", priority)
	// Output: Syslog priority: 44
}
