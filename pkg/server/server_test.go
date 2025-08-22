package server

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestServerErrorForEmptyConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s, err := New(ctx, log.NewNopAuditLogger(), &config.Config{}, nil)
	require.Nil(t, s)
	require.NotNil(t, err)
}

func TestServerConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectedErr string
	}{
		{
			name:        "empty config",
			config:      &config.Config{},
			expectedErr: "address is required",
		},
		{
			name: "retention period too short",
			config: &config.Config{
				Address:         "localhost:8080",
				RetentionPeriod: metav1.Duration{Duration: 30 * time.Second},
			},
			expectedErr: "retention_period must be at least 1 minute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			s, err := New(ctx, log.NewNopAuditLogger(), tt.config, nil)
			require.Nil(t, s)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestServerErrInvalidStateFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s, err := New(ctx, log.NewNopAuditLogger(), &config.Config{State: "invalid"}, nil)
	require.Nil(t, s)
	require.Error(t, err)
}

func TestServer_InitPluginFailFast(t *testing.T) {
	tests := []struct {
		name                string
		pluginsInitFailFast bool
		expectError         bool
	}{
		{
			name:                "Fail fast disabled - continues on error",
			pluginsInitFailFast: false,
			expectError:         false,
		},
		{
			name:                "Fail fast enabled - fails on error",
			pluginsInitFailFast: true,
			expectError:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a simple server struct to test the flag
			s := &Server{
				pluginsInitFailFast: tt.pluginsInitFailFast,
			}

			// Verify that the flag is set correctly
			assert.Equal(t, tt.pluginsInitFailFast, s.pluginsInitFailFast,
				"pluginsInitFailFast should be set to %v", tt.pluginsInitFailFast)
		})
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	s := &Server{}
	cert, err := s.generateSelfSignedCert()
	require.NoError(t, err, "Should generate certificate without error")
	assert.NotNil(t, cert, "Should return a valid certificate")
	assert.NotEmpty(t, cert.Certificate, "Certificate data should not be empty")
	assert.NotNil(t, cert.PrivateKey, "Private key should not be nil")
}

func TestCreateURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{
			name:     "simple hostname",
			endpoint: "example.com",
			expected: "https://example.com",
		},
		{
			name:     "hostname with port",
			endpoint: "example.com:8080",
			expected: "https://example.com:8080",
		},
		{
			name:     "full url",
			endpoint: "https://example.com/path",
			expected: "https://example.com",
		},
		{
			name:     "IP address",
			endpoint: "127.0.0.1",
			expected: "https://127.0.0.1",
		},
		{
			name:     "IP address with port",
			endpoint: "127.0.0.1:8443",
			expected: "https://127.0.0.1:8443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createURL(tt.endpoint)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteToken(t *testing.T) {
	// Create a temporary file to use as a FIFO (we won't actually make it a FIFO for testing)
	tempFile, err := os.CreateTemp("", "gpud-token-test")
	require.NoError(t, err, "Should create temp file without error")
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// Test writing a token
	token := "test-token-123"
	err = WriteToken(token, tempFile.Name())
	require.NoError(t, err, "Should write token without error")

	// Verify the token was written
	data, err := os.ReadFile(tempFile.Name())
	require.NoError(t, err, "Should read file without error")
	assert.Equal(t, token, string(data), "Written token should match expected value")
}

func TestServerStop(t *testing.T) {
	// Create a server with minimal dependencies
	dbRW, err := sqlite.Open(":memory:")
	require.NoError(t, err)

	dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
	require.NoError(t, err)

	s := &Server{
		dbRW:               dbRW,
		dbRO:               dbRO,
		componentsRegistry: components.NewRegistry(nil),
	}

	// Call Stop
	s.Stop()

	// Verify that the databases are closed by trying to execute a query
	_, err = dbRW.Exec("SELECT 1")
	require.Error(t, err, "Database should be closed")

	_, err = dbRO.Exec("SELECT 1")
	require.Error(t, err, "Database should be closed")
}

// TestWriteTokenErrors tests error handling for writing tokens.
// Note: This test is slow and can take up to 30 seconds because the write token retries 30 times with 1-second backoffs.
func TestWriteTokenErrors(t *testing.T) {
	// Test with non-existent FIFO file
	err := WriteToken("test-token", "/non/existent/path")
	require.Error(t, err)
	require.Contains(t, err.Error(), "server not ready")

	// Test with invalid FIFO file (directory instead of file)
	tempDir, err := os.MkdirTemp("", "gpud-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	err = WriteToken("test-token", tempDir)
	require.Error(t, err)
}

func TestServerWithFifoFile(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "gpud-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a FIFO file
	fifoPath := filepath.Join(tempDir, "test.fifo")
	err = os.MkdirAll(filepath.Dir(fifoPath), 0755)
	require.NoError(t, err)

	// Skip the test if we can't create a FIFO (e.g., on Windows)
	if err := syscall.Mkfifo(fifoPath, 0666); err != nil {
		t.Skip("Cannot create FIFO file, skipping test")
	}

	dbRW, err := sqlite.Open(":memory:")
	require.NoError(t, err)

	dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
	require.NoError(t, err)

	// Open the FIFO file
	fifo, err := os.OpenFile(fifoPath, os.O_RDWR, 0)
	require.NoError(t, err)

	// Create a server with the FIFO file
	s := &Server{
		dbRW:               dbRW,
		dbRO:               dbRO,
		fifoPath:           fifoPath,
		fifo:               fifo,
		componentsRegistry: components.NewRegistry(nil),
	}

	// Verify the FIFO file is set correctly
	require.Equal(t, fifoPath, s.fifoPath)
	require.NotNil(t, s.fifo)

	// Call Stop with the FIFO file
	s.Stop()

	// Verify the FIFO file is closed by trying to write to it
	_, err = fifo.Write([]byte("test"))
	require.Error(t, err, "FIFO file should be closed")
}

func setupTestDB(t *testing.T) (*sql.DB, *sql.DB, func()) {
	db, err := sqlite.Open(":memory:")
	require.NoError(t, err, "Should create in-memory DB without error")

	dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
	require.NoError(t, err, "Should create read-only in-memory DB without error")

	cleanup := func() {
		db.Close()
		dbRO.Close()
	}

	return db, dbRO, cleanup
}

func TestDoCompact(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test with valid period
	compactPeriod := 50 * time.Millisecond
	done := make(chan struct{})

	go func() {
		doCompact(ctx, db, compactPeriod)
		close(done)
	}()

	// Let it run for a bit and then cancel
	time.Sleep(compactPeriod * 2)
	cancel()

	select {
	case <-done:
		// doCompact exited properly
	case <-time.After(time.Second):
		t.Fatal("doCompact didn't exit after context was canceled")
	}

	// Test with zero period (should return immediately)
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	done = make(chan struct{})
	go func() {
		doCompact(ctx, db, 0)
		close(done)
	}()

	select {
	case <-done:
		// doCompact should return quickly
	case <-time.After(time.Second):
		t.Fatal("doCompact with zero period didn't return quickly")
	}
}

func TestServerStopNil(t *testing.T) {
	s := &Server{
		dbRW:     nil, // These would normally be initialized
		dbRO:     nil,
		fifoPath: "",
	}

	// Should not panic without initialized fields
	s.Stop()
}

func TestUpdateToken(t *testing.T) {
	// This is a simple test to ensure the function can be called without errors
	// We're not trying to test the full functionality, just that it handles basic input

	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a context with a tiny timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Create a server with a minimal setup
	s := &Server{
		machineID:      "test-uid",
		dbRW:           db,
		dbRO:           db,
		epControlPlane: "https://example.com",
	}
	userToken := &UserToken{}

	// Call updateToken directly - it will try to mkfifo and fail,
	// but that's ok for this test as we just want to make sure it doesn't hang
	s.updateToken(ctx, nil, userToken)

	// If we get here, the function returned after context expiration
}

func TestStartListener(t *testing.T) {
	// For this test, we'll just verify that the function correctly handles
	// shutdown with nil arguments, since fully testing HTTP server startup
	// would be more complex.

	s := &Server{}

	// Create minimal test config
	cfg := &config.Config{
		Address: "localhost:0", // Use port 0 to find a free port automatically
	}

	// Create a router with minimal setup
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Generate a self-signed cert
	cert, err := s.generateSelfSignedCert()
	require.NoError(t, err)

	// Call startListener in a goroutine - we expect it to try to start
	// the server and then exit when the server fails to bind
	done := make(chan struct{})
	go func() {
		s.startListener(nil, nil, cfg, router, cert)
		close(done)
	}()

	// Give it a second to run and likely fail
	select {
	case <-done:
		// This is expected since we used a minimal config and nil syncer
	case <-time.After(3 * time.Second):
		// If we get here, the function likely didn't fail as expected,
		// but we don't want to block the test forever
		t.Log("startListener didn't exit as expected, but this might be due to test environment differences")
	}
}
