package server

import (
	"context"
	"crypto/x509"
	"github.com/leptonai/gpud/components"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestServerErrorForEmptyConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s, err := New(ctx, &config.Config{}, "", "", nil)
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
		{
			name: "invalid auto update exit code",
			config: &config.Config{
				Address:            "localhost:8080",
				RetentionPeriod:    metav1.Duration{Duration: time.Hour},
				EnableAutoUpdate:   false,
				AutoUpdateExitCode: 1,
			},
			expectedErr: "auto_update_exit_code is only valid when auto_update is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			s, err := New(ctx, tt.config, "", "", nil)
			require.Nil(t, s)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestServerErrInvalidStateFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s, err := New(ctx, &config.Config{State: "invalid"}, "", "", nil)
	require.Nil(t, s)
	require.Error(t, err)
}

func TestGenerateSelfSignedCert(t *testing.T) {
	s := &Server{}
	cert, err := s.generateSelfSignedCert()
	require.NoError(t, err)
	require.NotNil(t, cert)

	// Verify the certificate
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)
	require.Equal(t, "Lepton AI", leaf.Subject.Organization[0])
	require.True(t, leaf.NotAfter.After(time.Now()))
	require.True(t, leaf.NotBefore.Before(time.Now()))
}

func TestWriteToken(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := ioutil.TempDir("", "gpud-test")
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

	// Start a goroutine to read from the FIFO
	tokenCh := make(chan string, 1)
	go func() {
		f, err := os.OpenFile(fifoPath, os.O_RDONLY, 0)
		if err != nil {
			t.Errorf("Failed to open FIFO: %v", err)
			return
		}
		defer f.Close()

		buf := make([]byte, 1024)
		n, err := f.Read(buf)
		if err != nil {
			t.Errorf("Failed to read from FIFO: %v", err)
			return
		}
		tokenCh <- string(buf[:n])
	}()

	// Wait a bit for the reader to start
	time.Sleep(100 * time.Millisecond)

	// Write the token
	token := "test-token"
	err = WriteToken(token, fifoPath)
	require.NoError(t, err)

	// Verify the token was written correctly
	select {
	case readToken := <-tokenCh:
		require.Equal(t, token, readToken)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for token")
	}
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
	tempDir, err := ioutil.TempDir("", "gpud-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	err = WriteToken("test-token", tempDir)
	require.Error(t, err)
}

func TestServerWithFifoFile(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := ioutil.TempDir("", "gpud-test")
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
