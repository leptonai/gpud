package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// TestKeepAliveHealthCheck403PersistsFailure verifies that when checkServerHealth
// returns an HTTP 403 error, the keepAlive loop persists the failure to session_states
// so that "gpud status" can surface it.
//
// This is the core fix for LEP-4748: previously, health-check 403s were only logged
// to stderr, leaving "gpud status" blind to auth failures after token invalidation.
func TestKeepAliveHealthCheck403PersistsFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the state file and session_states table up front,
	// because persistLoginStatus opens it by path.
	stateFile := tmpDir + "/gpud.state"
	dbSetup, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	require.NoError(t, sessionstates.CreateTable(context.Background(), dbSetup))
	require.NoError(t, dbSetup.Close())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:     ctx,
		dataDir: tmpDir,
		reader:  make(chan Body, 20),
		writer:  make(chan Body, 20),
		closer:  &closeOnce{closer: make(chan any)},
	}

	// Simulate health check returning 403 Forbidden
	var healthCheckCalls int32
	s.checkServerHealthFunc = func(ctx context.Context, jar *cookiejar.Jar, token string) error {
		count := atomic.AddInt32(&healthCheckCalls, 1)
		// Return 403 on first call, then cancel to stop the loop
		if count >= 2 {
			cancel()
		}
		return &healthCheckHTTPError{
			statusCode: http.StatusForbidden,
			body:       `{"error_code":"forbidden","error_summary":"Forbidden"}`,
		}
	}

	s.timeAfterFunc = func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	s.timeSleepFunc = func(d time.Duration) {}

	// These should never be called since health check fails before reader/writer start
	s.startReaderFunc = func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
		t.Fatal("startReader should not be called when health check fails")
	}
	s.startWriterFunc = func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
		t.Fatal("startWriter should not be called when health check fails")
	}

	// Run keepAlive and wait for it to process the 403
	done := make(chan struct{})
	go func() {
		s.keepAlive()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("keepAlive did not exit in time")
	}

	// Verify the failure was persisted to session_states
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	require.NoError(t, err)
	defer func() { _ = dbRO.Close() }()

	state, err := sessionstates.ReadLast(context.Background(), dbRO)
	require.NoError(t, err)
	require.NotNil(t, state, "Expected a session state entry to be persisted after 403 health check")

	assert.False(t, state.Success, "Session state should indicate failure")
	assert.Contains(t, state.Message, "HTTP 403")
	assert.Contains(t, state.Message, "Forbidden")
}

// TestKeepAliveHealthCheck500TokenValidationPersistsFailure verifies that when
// checkServerHealth returns an HTTP 500 error with "failed to validate token" body,
// the keepAlive loop persists the failure to session_states. The control plane may
// return 500 instead of 403 for auth failures (server-side bug).
func TestKeepAliveHealthCheck500TokenValidationPersistsFailure(t *testing.T) {
	tmpDir := t.TempDir()

	stateFile := tmpDir + "/gpud.state"
	dbSetup, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	require.NoError(t, sessionstates.CreateTable(context.Background(), dbSetup))
	require.NoError(t, dbSetup.Close())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:     ctx,
		dataDir: tmpDir,
		reader:  make(chan Body, 20),
		writer:  make(chan Body, 20),
		closer:  &closeOnce{closer: make(chan any)},
	}

	var healthCheckCalls int32
	s.checkServerHealthFunc = func(ctx context.Context, jar *cookiejar.Jar, token string) error {
		count := atomic.AddInt32(&healthCheckCalls, 1)
		if count >= 2 {
			cancel()
		}
		// Simulate: server returns 500 with "failed to validate token" instead of 403
		return &healthCheckHTTPError{
			statusCode: http.StatusInternalServerError,
			body:       `{"code":"InternalFailure","message":"failed to validate token"}`,
		}
	}

	s.timeAfterFunc = func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	s.timeSleepFunc = func(d time.Duration) {}
	s.startReaderFunc = func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
		t.Fatal("startReader should not be called when health check fails")
	}
	s.startWriterFunc = func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
		t.Fatal("startWriter should not be called when health check fails")
	}

	done := make(chan struct{})
	go func() {
		s.keepAlive()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("keepAlive did not exit in time")
	}

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	require.NoError(t, err)
	defer func() { _ = dbRO.Close() }()

	state, err := sessionstates.ReadLast(context.Background(), dbRO)
	require.NoError(t, err)
	require.NotNil(t, state, "Expected session state to be persisted for 500 with token validation failure")

	assert.False(t, state.Success, "Session state should indicate failure")
	assert.Contains(t, state.Message, "HTTP 500")
	assert.Contains(t, state.Message, "failed to validate token")
}

// TestKeepAliveHealthCheckNon403DoesNotPersist verifies that non-403 health check
// errors (e.g. generic 500s) do NOT persist to session_states. Only auth failures
// warrant a persistent record, since other errors are transient.
func TestKeepAliveHealthCheckNon403DoesNotPersist(t *testing.T) {
	tmpDir := t.TempDir()

	stateFile := tmpDir + "/gpud.state"
	dbSetup, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	require.NoError(t, sessionstates.CreateTable(context.Background(), dbSetup))
	require.NoError(t, dbSetup.Close())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:     ctx,
		dataDir: tmpDir,
		reader:  make(chan Body, 20),
		writer:  make(chan Body, 20),
		closer:  &closeOnce{closer: make(chan any)},
	}

	var healthCheckCalls int32
	s.checkServerHealthFunc = func(ctx context.Context, jar *cookiejar.Jar, token string) error {
		count := atomic.AddInt32(&healthCheckCalls, 1)
		if count >= 2 {
			cancel()
		}
		// Return a 500 error (not 403)
		return &healthCheckHTTPError{
			statusCode: http.StatusInternalServerError,
			body:       "internal server error",
		}
	}

	s.timeAfterFunc = func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	s.timeSleepFunc = func(d time.Duration) {}
	s.startReaderFunc = func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
		t.Fatal("startReader should not be called when health check fails")
	}
	s.startWriterFunc = func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
		t.Fatal("startWriter should not be called when health check fails")
	}

	done := make(chan struct{})
	go func() {
		s.keepAlive()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("keepAlive did not exit in time")
	}

	// Verify no failure was persisted (500 is transient, not auth failure)
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	require.NoError(t, err)
	defer func() { _ = dbRO.Close() }()

	state, err := sessionstates.ReadLast(context.Background(), dbRO)
	require.NoError(t, err)
	assert.Nil(t, state, "No session state should be persisted for non-403 errors")
}

// TestKeepAliveHealthCheckNetworkErrorDoesNotPersist verifies that plain network
// errors (not HTTP errors) don't persist to session_states.
func TestKeepAliveHealthCheckNetworkErrorDoesNotPersist(t *testing.T) {
	tmpDir := t.TempDir()

	stateFile := tmpDir + "/gpud.state"
	dbSetup, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	require.NoError(t, sessionstates.CreateTable(context.Background(), dbSetup))
	require.NoError(t, dbSetup.Close())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:     ctx,
		dataDir: tmpDir,
		reader:  make(chan Body, 20),
		writer:  make(chan Body, 20),
		closer:  &closeOnce{closer: make(chan any)},
	}

	var healthCheckCalls int32
	s.checkServerHealthFunc = func(ctx context.Context, jar *cookiejar.Jar, token string) error {
		count := atomic.AddInt32(&healthCheckCalls, 1)
		if count >= 2 {
			cancel()
		}
		// Plain network error (not healthCheckHTTPError)
		return fmt.Errorf("dial tcp: connection refused")
	}

	s.timeAfterFunc = func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	s.timeSleepFunc = func(d time.Duration) {}
	s.startReaderFunc = func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
		t.Fatal("startReader should not be called when health check fails")
	}
	s.startWriterFunc = func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
		t.Fatal("startWriter should not be called when health check fails")
	}

	done := make(chan struct{})
	go func() {
		s.keepAlive()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("keepAlive did not exit in time")
	}

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	require.NoError(t, err)
	defer func() { _ = dbRO.Close() }()

	state, err := sessionstates.ReadLast(context.Background(), dbRO)
	require.NoError(t, err)
	assert.Nil(t, state, "No session state should be persisted for network errors")
}

// TestHealthCheckHTTPErrorType verifies the healthCheckHTTPError type behavior.
func TestHealthCheckHTTPErrorType(t *testing.T) {
	t.Run("implements error interface", func(t *testing.T) {
		err := &healthCheckHTTPError{statusCode: 403, body: "forbidden"}
		var e error = err
		assert.Contains(t, e.Error(), "HTTP 403")
		assert.Contains(t, e.Error(), "forbidden")
	})

	t.Run("errors.As extracts the type", func(t *testing.T) {
		var err error = &healthCheckHTTPError{statusCode: 403, body: "test body"}
		var target *healthCheckHTTPError
		require.True(t, errors.As(err, &target))
		assert.Equal(t, 403, target.statusCode)
		assert.Equal(t, "test body", target.body)
	})

	t.Run("errors.As returns false for plain error", func(t *testing.T) {
		err := fmt.Errorf("plain error")
		var target *healthCheckHTTPError
		assert.False(t, errors.As(err, &target))
	})

	t.Run("errors.As works through wrapping", func(t *testing.T) {
		inner := &healthCheckHTTPError{statusCode: 500, body: "wrapped"}
		err := fmt.Errorf("outer: %w", inner)
		var target *healthCheckHTTPError
		require.True(t, errors.As(err, &target))
		assert.Equal(t, 500, target.statusCode)
	})
}
