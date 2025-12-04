package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// mockServer implements the ServerStopper interface for testing
type mockServer struct {
	stopCalled bool
}

func (s *mockServer) Stop() {
	s.stopCalled = true
}

func TestHandleSignalsSIGPIPE(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	serverC := make(chan ServerStopper, 1)

	notifyStoppingCalled := false
	notifyStopping := func(ctx context.Context) error {
		notifyStoppingCalled = true
		return nil
	}

	done := HandleSignals(ctx, cancel, signals, serverC, notifyStopping)

	// Set up the server
	mockSrv := &mockServer{}
	serverC <- mockSrv

	// Send SIGPIPE which should be ignored
	signals <- unix.SIGPIPE

	// Give some time for the goroutine to process the signal
	time.Sleep(100 * time.Millisecond)

	// Check server was not stopped and context was not canceled
	select {
	case <-ctx.Done():
		t.Fatal("Context was canceled but should not have been")
	default:
		// This is the expected path
	}

	assert.False(t, mockSrv.stopCalled, "Server.Stop() should not have been called")
	assert.False(t, notifyStoppingCalled, "notifyStopping should not have been called")

	// Clean up: send a SIGTERM to allow the HandleSignals goroutine to exit
	signals <- syscall.SIGTERM
	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for done channel to close during cleanup")
	}
}

func TestHandleSignalsSIGUSR1(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	serverC := make(chan ServerStopper, 1)

	notifyStoppingCalled := false
	notifyStopping := func(ctx context.Context) error {
		notifyStoppingCalled = true
		return nil
	}

	done := HandleSignals(ctx, cancel, signals, serverC, notifyStopping)

	// Set up the server
	mockSrv := &mockServer{}
	serverC <- mockSrv

	// Send SIGUSR1 to dump stacks
	signals <- unix.SIGUSR1

	// Give some time for the goroutine to process the signal
	time.Sleep(100 * time.Millisecond)

	// Check server was not stopped and context was not canceled
	select {
	case <-ctx.Done():
		t.Fatal("Context was canceled but should not have been")
	default:
		// This is the expected path
	}

	// Check that stack dump file was created
	stackFile := filepath.Join(os.TempDir(), fmt.Sprintf("gpud.%d.stacks.log", os.Getpid()))
	_, err := os.Stat(stackFile)
	require.NoError(t, err, "Stack dump file should exist")

	// Clean up the file
	_ = os.Remove(stackFile)

	assert.False(t, mockSrv.stopCalled, "Server.Stop() should not have been called")
	assert.False(t, notifyStoppingCalled, "notifyStopping should not have been called")

	// Clean up: send a SIGTERM to allow the HandleSignals goroutine to exit
	signals <- syscall.SIGTERM
	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for done channel to close during cleanup")
	}
}

func TestHandleSignalsSIGTERM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// No defer cancel() here, as the signal handler should call it.

	signals := make(chan os.Signal, 1)
	serverC := make(chan ServerStopper, 1)

	notifyStoppingCalled := false
	notifyStopping := func(ctx context.Context) error {
		notifyStoppingCalled = true
		return nil
	}

	done := HandleSignals(ctx, cancel, signals, serverC, notifyStopping)

	// Set up the server
	mockSrv := &mockServer{}
	serverC <- mockSrv
	time.Sleep(50 * time.Millisecond) // Allow HandleSignals to receive the server

	// Send SIGTERM which should trigger shutdown
	signals <- syscall.SIGTERM

	// Wait for the done channel to be closed
	select {
	case <-done:
		// This is expected
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for done channel to close")
	}

	// Verify context was canceled
	select {
	case <-ctx.Done():
		// This is expected
	default:
		t.Fatal("Context should have been canceled")
	}

	// Verify server was stopped
	assert.True(t, mockSrv.stopCalled, "Server.Stop() should have been called")

	// Verify notifyStopping was called
	assert.True(t, notifyStoppingCalled, "notifyStopping should have been called")
}

func TestDumpStacks(t *testing.T) {
	// Create a temporary file to dump stacks to
	tmpDir, err := os.MkdirTemp("", "gpud-test")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	stackFile := filepath.Join(tmpDir, "stacks.log")

	// Call the function
	dumpStacks(stackFile)

	// Verify file was created
	_, err = os.Stat(stackFile)
	require.NoError(t, err, "Stack dump file should exist")

	// Verify file has content
	content, err := os.ReadFile(stackFile)
	require.NoError(t, err)
	assert.NotEmpty(t, content, "Stack dump file should not be empty")

	// Test with an invalid path
	invalidPath := filepath.Join(tmpDir, "non-existent-dir", "stacks.log")
	dumpStacks(invalidPath)
	// This should return silently without error, even though file creation fails
}
