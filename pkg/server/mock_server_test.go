package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	lepconfig "github.com/leptonai/gpud/pkg/config"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	gpudpackages "github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgmetricssyncer "github.com/leptonai/gpud/pkg/metrics/syncer"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
	pkgsession "github.com/leptonai/gpud/pkg/session"
	"github.com/leptonai/gpud/pkg/sqlite"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

type mockNVMLInstance struct {
	shutdownCalled bool
}

func (m *mockNVMLInstance) NVMLExists() bool { return true }
func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}
func (m *mockNVMLInstance) Devices() map[string]device.Device { return nil }
func (m *mockNVMLInstance) ProductName() string               { return "mock-gpu" }
func (m *mockNVMLInstance) Architecture() string              { return "mock-arch" }
func (m *mockNVMLInstance) Brand() string                     { return "mock-brand" }
func (m *mockNVMLInstance) DriverVersion() string             { return "mock-version" }
func (m *mockNVMLInstance) DriverMajor() int                  { return 1 }
func (m *mockNVMLInstance) CUDAVersion() string               { return "mock-cuda" }
func (m *mockNVMLInstance) FabricManagerSupported() bool      { return false }
func (m *mockNVMLInstance) FabricStateSupported() bool        { return false }
func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNVMLInstance) Shutdown() error {
	m.shutdownCalled = true
	return nil
}
func (m *mockNVMLInstance) InitError() error { return nil }

type mockFileInfo struct{}

func (m mockFileInfo) Name() string       { return "mock" }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() os.FileMode  { return 0 }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return false }
func (m mockFileInfo) Sys() any           { return nil }

// TestWaitUntilMachineID_AlreadySet tests WaitUntilMachineID when machine ID is already set.
func TestWaitUntilMachineID_AlreadySet(t *testing.T) {
	mockey.PatchConvey("WaitUntilMachineID with already set ID", t, func() {
		s := &Server{
			machineID: "test-machine-id",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Should return immediately since machineID is already set
		done := make(chan struct{})
		go func() {
			s.WaitUntilMachineID(ctx)
			close(done)
		}()

		select {
		case <-done:
			// Expected - returned immediately
		case <-time.After(500 * time.Millisecond):
			t.Fatal("WaitUntilMachineID should have returned immediately")
		}
	})
}

// TestWaitUntilMachineID_ContextCanceled tests WaitUntilMachineID when context is canceled.
func TestWaitUntilMachineID_ContextCanceled(t *testing.T) {
	mockey.PatchConvey("WaitUntilMachineID with context canceled", t, func() {
		dbRW, err := sqlite.Open(":memory:")
		require.NoError(t, err)
		defer func() { _ = dbRW.Close() }()

		dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
		require.NoError(t, err)
		defer func() { _ = dbRO.Close() }()

		s := &Server{
			machineID: "", // Not set
			dbRO:      dbRO,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		done := make(chan struct{})
		go func() {
			s.WaitUntilMachineID(ctx)
			close(done)
		}()

		select {
		case <-done:
			// Expected - returned due to context cancellation
		case <-time.After(500 * time.Millisecond):
			t.Fatal("WaitUntilMachineID should have returned after context canceled")
		}
	})
}

// TestWaitUntilMachineID_ReadFromDB tests WaitUntilMachineID reading from database.
func TestWaitUntilMachineID_ReadFromDB(t *testing.T) {
	mockey.PatchConvey("WaitUntilMachineID reads from DB", t, func() {
		// Create a safety timer before mocking time functions
		safetyTimer := time.NewTimer(5 * time.Second)
		defer safetyTimer.Stop()

		readCount := 0
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			readCount++
			if readCount >= 2 {
				return "db-machine-id", nil
			}
			return "", nil
		}).Build()

		// Create a fast ticker before mocking, then return it from mock
		fastTicker := time.NewTicker(time.Millisecond)
		mockey.Mock(time.NewTicker).To(func(d time.Duration) *time.Ticker {
			return fastTicker
		}).Build()

		dbRW, err := sqlite.Open(":memory:")
		require.NoError(t, err)
		defer func() { _ = dbRW.Close() }()

		dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
		require.NoError(t, err)
		defer func() { _ = dbRO.Close() }()

		s := &Server{
			machineID: "", // Not set initially
			dbRO:      dbRO,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		done := make(chan struct{})
		go func() {
			s.WaitUntilMachineID(ctx)
			close(done)
		}()

		select {
		case <-done:
			// Expected - returned after reading from DB
			s.machineIDMu.RLock()
			machineID := s.machineID
			s.machineIDMu.RUnlock()
			assert.Equal(t, "db-machine-id", machineID)
		case <-safetyTimer.C:
			t.Fatal("WaitUntilMachineID should have returned after reading from DB")
		}
	})
}

// TestWaitUntilMachineID_ReadError tests WaitUntilMachineID when read fails.
func TestWaitUntilMachineID_ReadError(t *testing.T) {
	mockey.PatchConvey("WaitUntilMachineID handles read error", t, func() {
		// Create a safety timer before mocking time functions
		safetyTimer := time.NewTimer(5 * time.Second)
		defer safetyTimer.Stop()

		callCount := 0
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			callCount++
			if callCount >= 3 {
				return "recovered-machine-id", nil
			}
			return "", errors.New("database error")
		}).Build()

		// Create a fast ticker before mocking, then return it from mock
		fastTicker := time.NewTicker(time.Millisecond)
		mockey.Mock(time.NewTicker).To(func(d time.Duration) *time.Ticker {
			return fastTicker
		}).Build()

		dbRW, err := sqlite.Open(":memory:")
		require.NoError(t, err)
		defer func() { _ = dbRW.Close() }()

		dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
		require.NoError(t, err)
		defer func() { _ = dbRO.Close() }()

		s := &Server{
			machineID: "", // Not set initially
			dbRO:      dbRO,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		done := make(chan struct{})
		go func() {
			s.WaitUntilMachineID(ctx)
			close(done)
		}()

		select {
		case <-done:
			// Expected - recovered after errors
			s.machineIDMu.RLock()
			machineID := s.machineID
			s.machineIDMu.RUnlock()
			assert.Equal(t, "recovered-machine-id", machineID)
		case <-safetyTimer.C:
			t.Fatal("WaitUntilMachineID should have recovered")
		}
	})
}

// TestUpdateFromVersionFile_Disabled tests updateFromVersionFile when auto-update is disabled.
func TestUpdateFromVersionFile_Disabled(t *testing.T) {
	mockey.PatchConvey("updateFromVersionFile with disabled auto-update", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		done := make(chan struct{})
		go func() {
			updateFromVersionFile(ctx, -1, "")
			close(done)
		}()

		select {
		case <-done:
			// Expected - returned immediately when disabled
		case <-time.After(500 * time.Millisecond):
			t.Fatal("updateFromVersionFile should return immediately when disabled")
		}
	})
}

// TestUpdateFromVersionFile_EmptyVersionFile tests updateFromVersionFile with empty version file.
func TestUpdateFromVersionFile_EmptyVersionFile(t *testing.T) {
	mockey.PatchConvey("updateFromVersionFile with empty version file", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		done := make(chan struct{})
		go func() {
			updateFromVersionFile(ctx, 0, "")
			close(done)
		}()

		select {
		case <-done:
			// Expected - returned immediately when version file is empty
		case <-time.After(500 * time.Millisecond):
			t.Fatal("updateFromVersionFile should return immediately when version file is empty")
		}
	})
}

// TestUpdateFromVersionFile_UpdateError tests updateFromVersionFile when update fails.
func TestUpdateFromVersionFile_UpdateError(t *testing.T) {
	mockey.PatchConvey("updateFromVersionFile handles update error", t, func() {
		// Create a safety timer before mocking time.After
		safetyTimer := time.NewTimer(5 * time.Second)
		defer safetyTimer.Stop()

		updateCalled := false
		mockey.Mock(pkgupdate.UpdateTargetVersion).To(func(versionFile string, exitCode int) error {
			updateCalled = true
			return errors.New("update failed")
		}).Build()

		// Mock time.After to return immediately so the 30s wait is skipped
		mockey.Mock(time.After).To(func(d time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		done := make(chan struct{})
		go func() {
			updateFromVersionFile(ctx, 0, "/some/version/file")
			close(done)
		}()

		select {
		case <-done:
			// Expected - returned after context timeout
			assert.True(t, updateCalled, "Update should have been called")
		case <-safetyTimer.C:
			t.Fatal("updateFromVersionFile should return after context is done")
		}
	})
}

// TestNew_ConfigValidationError tests New when config validation fails.
func TestNew_ConfigValidationError(t *testing.T) {
	mockey.PatchConvey("New with config validation error", t, func() {
		ctx := context.Background()

		s, err := New(ctx, log.NewNopAuditLogger(), &lepconfig.Config{}, nil)
		require.Error(t, err)
		require.Nil(t, s)
		assert.Contains(t, err.Error(), "failed to validate config")
	})
}

// TestNew_DataDirResolutionError tests New when data dir resolution fails.
func TestNew_DataDirResolutionError(t *testing.T) {
	mockey.PatchConvey("New with data dir resolution error", t, func() {
		mockey.Mock(lepconfig.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "", errors.New("failed to resolve data dir")
		}).Build()

		ctx := context.Background()
		cfg := &lepconfig.Config{
			Address:                "localhost:8080",
			MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
		}

		s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
		require.Error(t, err)
		require.Nil(t, s)
		assert.Contains(t, err.Error(), "failed to resolve data dir")
	})
}

// TestNew_SQLiteOpenError tests New when sqlite open fails.
func TestNew_SQLiteOpenError(t *testing.T) {
	mockey.PatchConvey("New with sqlite open error", t, func() {
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		mockey.Mock(sqlite.Open).To(func(path string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("failed to open database")
		}).Build()

		ctx := context.Background()
		cfg := &lepconfig.Config{
			Address:                "localhost:8080",
			DataDir:                tmpDir,
			MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
		}

		s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
		require.Error(t, err)
		require.Nil(t, s)
		assert.Contains(t, err.Error(), "failed to open")
	})
}

// TestNew_WithMockNVML tests New with mocked NVML.
func TestNew_WithMockNVML(t *testing.T) {
	mockey.PatchConvey("New with mocked NVML", t, func() {
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		// Use the NVML mock environment variable
		require.NoError(t, os.Setenv(nvmllib.EnvMockAllSuccess, "true"))
		defer func() { _ = os.Unsetenv(nvmllib.EnvMockAllSuccess) }()

		// Mock httputil.CreateURL to fail so New() returns an error
		// after NVML initialization succeeds but before starting goroutines.
		// Go's url.Parse is lenient and accepts "invalid-address" as a valid hostname,
		// so we need an explicit mock to trigger the error path.
		mockey.Mock(httputil.CreateURL).To(func(scheme string, endpoint string, path string) (string, error) {
			return "", errors.New("invalid address for testing")
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg := &lepconfig.Config{
			Address:                "invalid-address",
			DataDir:                tmpDir,
			MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
			Components:             []string{"-disable-all"},
		}

		s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
		// Will fail due to mocked CreateURL, but should pass NVML initialization
		require.Error(t, err)
		require.Nil(t, s)
		assert.Contains(t, err.Error(), "failed to create local GPUd server endpoint")
	})
}

// TestNew_PluginSpecsFileNotFound tests New with non-existent plugin specs file.
func TestNew_PluginSpecsFileNotFound(t *testing.T) {
	mockey.PatchConvey("New with non-existent plugin specs file", t, func() {
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		require.NoError(t, os.Setenv(nvmllib.EnvMockAllSuccess, "true"))
		defer func() { _ = os.Unsetenv(nvmllib.EnvMockAllSuccess) }()

		// Mock httputil.CreateURL to fail so New() returns an error
		// after plugin specs processing but before starting goroutines.
		mockey.Mock(httputil.CreateURL).To(func(scheme string, endpoint string, path string) (string, error) {
			return "", errors.New("invalid address for testing")
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg := &lepconfig.Config{
			Address:                "invalid-address",
			DataDir:                tmpDir,
			MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
			Components:             []string{"-disable-all"},
			PluginSpecsFile:        filepath.Join(tmpDir, "nonexistent-plugins.yaml"),
		}

		s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
		// Should fail at mocked CreateURL, plugin file warning is logged but not an error
		require.Error(t, err)
		require.Nil(t, s)
	})
}

// TestUserToken_ConcurrentAccess tests UserToken thread safety.
func TestUserToken_ConcurrentAccess(t *testing.T) {
	mockey.PatchConvey("UserToken concurrent access", t, func() {
		token := &UserToken{}

		// Concurrent writes and reads
		done := make(chan bool, 20)
		for i := 0; i < 10; i++ {
			go func(i int) {
				token.mu.Lock()
				token.userToken = "token-" + string(rune('A'+i))
				token.mu.Unlock()
				done <- true
			}(i)
			go func() {
				token.mu.RLock()
				_ = token.userToken
				token.mu.RUnlock()
				done <- true
			}()
		}

		for i := 0; i < 20; i++ {
			<-done
		}

		// Should not panic due to race conditions
		token.mu.RLock()
		finalToken := token.userToken
		token.mu.RUnlock()
		assert.NotEmpty(t, finalToken)
	})
}

// TestCreateURL_EdgeCases tests createURL with edge cases.
func TestCreateURL_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{
			name:     "empty string",
			endpoint: "",
			expected: "https://",
		},
		{
			name:     "just protocol",
			endpoint: "http://",
			expected: "https://",
		},
		{
			name:     "with path and query",
			endpoint: "https://example.com/path?query=1",
			expected: "https://example.com",
		},
		{
			name:     "IPv6 address",
			endpoint: "[::1]:8080",
			expected: "https://[::1]:8080",
		},
		{
			name:     "localhost",
			endpoint: "localhost",
			expected: "https://localhost",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := createURL(tc.endpoint)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDoCompact_CompactError tests doCompact when compact fails.
func TestDoCompact_CompactError(t *testing.T) {
	mockey.PatchConvey("doCompact handles compact error", t, func() {
		compactCalled := false
		mockey.Mock(sqlite.Compact).To(func(ctx context.Context, db *sql.DB) error {
			compactCalled = true
			return errors.New("compact failed")
		}).Build()

		dbRW, err := sqlite.Open(":memory:")
		require.NoError(t, err)
		defer func() { _ = dbRW.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		done := make(chan struct{})
		go func() {
			doCompact(ctx, dbRW, 50*time.Millisecond)
			close(done)
		}()

		select {
		case <-done:
			assert.True(t, compactCalled, "Compact should have been called")
		case <-time.After(500 * time.Millisecond):
			t.Fatal("doCompact should return after context is done")
		}
	})
}

// TestServerMachineIDMutex tests Server machineID mutex operations.
func TestServerMachineIDMutex(t *testing.T) {
	mockey.PatchConvey("Server machineID mutex", t, func() {
		s := &Server{}

		// Concurrent read and write
		done := make(chan bool, 20)
		for i := 0; i < 10; i++ {
			go func(i int) {
				s.machineIDMu.Lock()
				s.machineID = "machine-" + string(rune('A'+i))
				s.machineIDMu.Unlock()
				done <- true
			}(i)
			go func() {
				s.machineIDMu.RLock()
				_ = s.machineID
				s.machineIDMu.RUnlock()
				done <- true
			}()
		}

		for i := 0; i < 20; i++ {
			<-done
		}

		// Should not panic due to race conditions
		s.machineIDMu.RLock()
		finalID := s.machineID
		s.machineIDMu.RUnlock()
		assert.NotEmpty(t, finalID)
	})
}

// TestHandleSignals_SIGINT tests HandleSignals with SIGINT signal triggers graceful shutdown.
func TestHandleSignals_SIGINT(t *testing.T) {
	mockey.PatchConvey("HandleSignals with SIGINT triggers shutdown", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		signals := make(chan os.Signal, 1)
		serverC := make(chan ServerStopper, 1)

		notifyStoppingCalled := false
		notifyStopping := func(ctx context.Context) error {
			notifyStoppingCalled = true
			return nil
		}

		done := HandleSignals(ctx, cancel, signals, serverC, notifyStopping)

		mockSrv := &mockServer{}
		serverC <- mockSrv
		time.Sleep(50 * time.Millisecond)

		// Send SIGINT
		signals <- unix.SIGINT

		select {
		case <-done:
			// Expected - shutdown was triggered
		case <-time.After(2 * time.Second):
			t.Fatal("HandleSignals should have closed done channel after SIGINT")
		}

		// Verify context was canceled
		select {
		case <-ctx.Done():
			// Expected
		default:
			t.Fatal("Context should have been canceled after SIGINT")
		}

		assert.True(t, mockSrv.stopCalled, "Server.Stop() should have been called for SIGINT")
		assert.True(t, notifyStoppingCalled, "notifyStopping should have been called for SIGINT")
	})
}

// TestHandleSignals_NotifyStoppingError tests HandleSignals when notifyStopping returns an error.
func TestHandleSignals_NotifyStoppingError(t *testing.T) {
	mockey.PatchConvey("HandleSignals handles notifyStopping error", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		signals := make(chan os.Signal, 1)
		serverC := make(chan ServerStopper, 1)

		notifyStopping := func(ctx context.Context) error {
			return errors.New("notify stopping failed")
		}

		done := HandleSignals(ctx, cancel, signals, serverC, notifyStopping)

		mockSrv := &mockServer{}
		serverC <- mockSrv
		time.Sleep(50 * time.Millisecond)

		// Send SIGTERM
		signals <- syscall.SIGTERM

		select {
		case <-done:
			// Expected - should still complete despite error in notifyStopping
		case <-time.After(2 * time.Second):
			t.Fatal("HandleSignals should have closed done channel even with notifyStopping error")
		}

		assert.True(t, mockSrv.stopCalled, "Server.Stop() should still have been called")
	})
}

// TestHandleSignals_NoServerSet tests HandleSignals when no server has been sent yet.
func TestHandleSignals_NoServerSet(t *testing.T) {
	mockey.PatchConvey("HandleSignals with no server set", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		signals := make(chan os.Signal, 1)
		serverC := make(chan ServerStopper, 1)

		notifyStoppingCalled := false
		notifyStopping := func(ctx context.Context) error {
			notifyStoppingCalled = true
			return nil
		}

		done := HandleSignals(ctx, cancel, signals, serverC, notifyStopping)

		// Do NOT send a server on serverC - send signal directly
		signals <- syscall.SIGTERM

		select {
		case <-done:
			// Expected - shutdown should still proceed with nil server
		case <-time.After(2 * time.Second):
			t.Fatal("HandleSignals should complete even without a server")
		}

		assert.True(t, notifyStoppingCalled, "notifyStopping should have been called")
	})
}

// TestHandleSignals_MultipleSIGPIPE tests that multiple SIGPIPE signals are all ignored.
func TestHandleSignals_MultipleSIGPIPE(t *testing.T) {
	mockey.PatchConvey("HandleSignals ignores multiple SIGPIPE signals", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		signals := make(chan os.Signal, 10)
		serverC := make(chan ServerStopper, 1)

		notifyStoppingCalled := false
		notifyStopping := func(ctx context.Context) error {
			notifyStoppingCalled = true
			return nil
		}

		done := HandleSignals(ctx, cancel, signals, serverC, notifyStopping)

		mockSrv := &mockServer{}
		serverC <- mockSrv

		// Send multiple SIGPIPE signals
		for i := 0; i < 5; i++ {
			signals <- unix.SIGPIPE
		}

		time.Sleep(100 * time.Millisecond)

		// Verify server is still running
		select {
		case <-ctx.Done():
			t.Fatal("Context should not have been canceled from SIGPIPE")
		default:
			// Expected
		}

		assert.False(t, mockSrv.stopCalled, "Server should not have been stopped from SIGPIPE")
		assert.False(t, notifyStoppingCalled, "notifyStopping should not have been called for SIGPIPE")

		// Clean up
		signals <- syscall.SIGTERM
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("cleanup timed out")
		}
	})
}

// TestHealthz_DefaultJSON tests the healthz handler returns JSON by default.
func TestHealthz_DefaultJSON(t *testing.T) {
	mockey.PatchConvey("healthz returns default JSON", t, func() {
		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/healthz", nil)

		handler := healthz()
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Healthz
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, "v1", resp.Version)
	})
}

// TestHealthz_IndentedJSON tests the healthz handler with indented JSON.
func TestHealthz_IndentedJSON(t *testing.T) {
	mockey.PatchConvey("healthz returns indented JSON", t, func() {
		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/healthz", nil)
		c.Request.Header.Set("json-indent", "true")

		handler := healthz()
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)

		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "\n")

		var resp Healthz
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, "v1", resp.Version)
	})
}

// TestHealthz_YAMLResponse tests the healthz handler with YAML content type.
func TestHealthz_YAMLResponse(t *testing.T) {
	mockey.PatchConvey("healthz returns YAML", t, func() {
		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/healthz", nil)
		c.Request.Header.Set("Content-Type", "application/yaml")

		handler := healthz()
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Healthz
		err := yaml.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, "v1", resp.Version)
	})
}

// TestHealthz_YAMLMarshalError tests the healthz handler when YAML marshal fails.
func TestHealthz_YAMLMarshalError(t *testing.T) {
	mockey.PatchConvey("healthz handles YAML marshal error", t, func() {
		mockey.Mock(yaml.Marshal).To(func(v interface{}) ([]byte, error) {
			return nil, errors.New("yaml marshal failed")
		}).Build()

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/healthz", nil)
		c.Request.Header.Set("Content-Type", "application/yaml")

		handler := healthz()
		handler(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["message"], "failed to marshal components")
	})
}

// TestHandleAdminConfig_JSONDefault tests the admin config handler returns JSON by default.
func TestHandleAdminConfig_JSONDefault(t *testing.T) {
	mockey.PatchConvey("handleAdminConfig returns JSON by default", t, func() {
		cfg := &lepconfig.Config{
			Address: "0.0.0.0:9090",
		}

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/admin/config", nil)

		handler := handleAdminConfig(cfg)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp lepconfig.Config
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "0.0.0.0:9090", resp.Address)
	})
}

// TestHandleAdminConfig_IndentedJSON tests admin config handler with indented JSON.
func TestHandleAdminConfig_IndentedJSON(t *testing.T) {
	mockey.PatchConvey("handleAdminConfig returns indented JSON", t, func() {
		cfg := &lepconfig.Config{
			Address: "0.0.0.0:9090",
		}

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/admin/config", nil)
		c.Request.Header.Set("json-indent", "true")

		handler := handleAdminConfig(cfg)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)

		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "\n")

		var resp lepconfig.Config
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "0.0.0.0:9090", resp.Address)
	})
}

// TestHandleAdminConfig_YAMLMarshalError tests admin config handler when YAML marshal fails.
func TestHandleAdminConfig_YAMLMarshalError(t *testing.T) {
	mockey.PatchConvey("handleAdminConfig handles YAML marshal error", t, func() {
		mockey.Mock(yaml.Marshal).To(func(v interface{}) ([]byte, error) {
			return nil, errors.New("yaml marshal failed")
		}).Build()

		cfg := &lepconfig.Config{
			Address: "0.0.0.0:9090",
		}

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/admin/config", nil)
		c.Request.Header.Set("Content-Type", "application/yaml")

		handler := handleAdminConfig(cfg)
		handler(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["message"], "failed to marshal components")
	})
}

func TestHandleAdminPackagesStatus_Success(t *testing.T) {
	mockey.PatchConvey("handleAdminPackagesStatus returns package status", t, func() {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/admin/packages", nil)

		manager := &gpudmanager.Manager{}
		expected := []gpudpackages.PackageStatus{
			{
				Name:        "pkg-1",
				IsInstalled: true,
			},
		}

		mockey.Mock((*gpudmanager.Manager).Status).To(func(_ *gpudmanager.Manager, _ context.Context) ([]gpudpackages.PackageStatus, error) {
			return expected, nil
		}).Build()

		handler := handleAdminPackagesStatus(manager)
		handler(ctx)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got []gpudpackages.PackageStatus
		err := json.Unmarshal(rec.Body.Bytes(), &got)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, expected[0].Name, got[0].Name)
		assert.True(t, got[0].IsInstalled)
	})
}

func TestHandleAdminPackagesStatus_Error(t *testing.T) {
	mockey.PatchConvey("handleAdminPackagesStatus returns error on failure", t, func() {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/admin/packages", nil)

		manager := &gpudmanager.Manager{}
		mockey.Mock((*gpudmanager.Manager).Status).To(func(_ *gpudmanager.Manager, _ context.Context) ([]gpudpackages.PackageStatus, error) {
			return nil, errors.New("status failed")
		}).Build()

		handler := handleAdminPackagesStatus(manager)
		handler(ctx)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to get package status")
	})
}

// TestMachineInfo_WithMockedGetMachineInfo tests machineInfo with a successful mocked GetMachineInfo.
func TestMachineInfo_WithMockedGetMachineInfo(t *testing.T) {
	mockey.PatchConvey("machineInfo returns mocked machine info", t, func() {
		expectedInfo := &apiv1.MachineInfo{
			GPUdVersion:     "test-version",
			KernelVersion:   "5.4.0-test",
			OSImage:         "TestOS",
			Hostname:        "test-host",
			OperatingSystem: "linux",
		}

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return expectedInfo, nil
		}).Build()

		handler := &globalHandler{
			gpudInstance: &components.GPUdInstance{},
		}

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/machine-info", nil)
		handler.machineInfo(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp apiv1.MachineInfo
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "test-version", resp.GPUdVersion)
		assert.Equal(t, "5.4.0-test", resp.KernelVersion)
		assert.Equal(t, "TestOS", resp.OSImage)
		assert.Equal(t, "test-host", resp.Hostname)
		assert.Equal(t, "linux", resp.OperatingSystem)
	})
}

// TestMachineInfo_GetMachineInfoError tests machineInfo when GetMachineInfo returns an error.
func TestMachineInfo_GetMachineInfoError(t *testing.T) {
	mockey.PatchConvey("machineInfo handles GetMachineInfo error", t, func() {
		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return nil, errors.New("failed to query GPU info")
		}).Build()

		handler := &globalHandler{
			gpudInstance: &components.GPUdInstance{},
		}

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/machine-info", nil)
		handler.machineInfo(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["message"], "failed to get machine info")
	})
}

// TestMachineInfo_NilGPUdInstance tests machineInfo when gpudInstance is nil.
func TestMachineInfo_NilGPUdInstance(t *testing.T) {
	mockey.PatchConvey("machineInfo returns not found when gpudInstance is nil", t, func() {
		handler := &globalHandler{
			gpudInstance: nil,
		}

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/machine-info", nil)
		handler.machineInfo(c)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "gpud instance not found", resp["message"])
	})
}

// TestGetPluginSpecs_YAMLMarshalError tests getPluginSpecs when YAML marshal fails.
func TestGetPluginSpecs_YAMLMarshalError(t *testing.T) {
	mockey.PatchConvey("getPluginSpecs handles YAML marshal error", t, func() {
		mockey.Mock(yaml.Marshal).To(func(v interface{}) ([]byte, error) {
			return nil, errors.New("yaml marshal failed")
		}).Build()

		handler, _, _ := setupTestHandler([]components.Component{})

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/plugins", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)

		handler.getPluginSpecs(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["message"], "failed to marshal custom plugins")
	})
}

// TestGenerateSelfSignedCert_Success tests that generateSelfSignedCert produces a valid TLS certificate.
func TestGenerateSelfSignedCert_Success(t *testing.T) {
	mockey.PatchConvey("generateSelfSignedCert produces valid certificate", t, func() {
		s := &Server{}
		cert, err := s.generateSelfSignedCert()
		require.NoError(t, err)
		assert.NotEmpty(t, cert.Certificate, "Certificate should have DER data")
		assert.NotNil(t, cert.PrivateKey, "Private key should be set")

		// Verify the certificate has exactly one certificate in the chain
		assert.Len(t, cert.Certificate, 1, "Should have exactly one certificate")
	})
}

// TestServerStop_WithComponentCloseError tests Server Stop when a component's Close returns an error.
func TestServerStop_WithComponentCloseError(t *testing.T) {
	mockey.PatchConvey("Server Stop handles component close error", t, func() {
		// Create a component that returns error on Close
		comp := &mockComponent{
			name:            "close-error-comp",
			isSupported:     true,
			deregisterError: errors.New("close failed"),
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		dbRW, err := sqlite.Open(":memory:")
		require.NoError(t, err)

		dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
		require.NoError(t, err)

		s := &Server{
			dbRW:               dbRW,
			dbRO:               dbRO,
			componentsRegistry: registry,
		}

		// Should not panic even if component close fails
		s.Stop()

		// Databases should still be closed
		_, err = dbRW.Exec("SELECT 1")
		assert.Error(t, err, "DB should be closed after Stop")
	})
}

// TestServerStop_WithNilFields tests Server Stop with all nil fields.
func TestServerStop_WithNilFields(t *testing.T) {
	mockey.PatchConvey("Server Stop with nil fields does not panic", t, func() {
		s := &Server{
			dbRW:               nil,
			dbRO:               nil,
			fifoPath:           "",
			fifo:               nil,
			session:            nil,
			componentsRegistry: nil,
			gpudInstance:       nil,
		}

		// Should not panic
		s.Stop()
	})
}

// TestServerStop_WithFifoCleanup tests Server Stop cleans up FIFO file.
func TestServerStop_WithFifoCleanup(t *testing.T) {
	mockey.PatchConvey("Server Stop cleans up FIFO", t, func() {
		tmpDir, err := os.MkdirTemp("", "gpud-fifo-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		fifoPath := filepath.Join(tmpDir, "test.fifo")
		err = syscall.Mkfifo(fifoPath, 0666)
		require.NoError(t, err)

		// Open the FIFO in RDWR mode so it does not block
		fifo, err := os.OpenFile(fifoPath, os.O_RDWR, 0)
		require.NoError(t, err)

		s := &Server{
			fifoPath: fifoPath,
			fifo:     fifo,
		}

		s.Stop()

		// Verify FIFO file was removed
		_, err = os.Stat(fifoPath)
		assert.True(t, os.IsNotExist(err), "FIFO file should have been removed")
	})
}

// TestDoCompact_NegativePeriod tests doCompact with a negative compact period.
func TestDoCompact_NegativePeriod(t *testing.T) {
	mockey.PatchConvey("doCompact with negative period returns immediately", t, func() {
		dbRW, err := sqlite.Open(":memory:")
		require.NoError(t, err)
		defer func() { _ = dbRW.Close() }()

		ctx := context.Background()

		done := make(chan struct{})
		go func() {
			doCompact(ctx, dbRW, -1*time.Second)
			close(done)
		}()

		select {
		case <-done:
			// Expected - returns immediately with negative period
		case <-time.After(500 * time.Millisecond):
			t.Fatal("doCompact should return immediately with negative period")
		}
	})
}

// TestDoCompact_CompactSuccess tests doCompact with a successful compact operation.
func TestDoCompact_CompactSuccess(t *testing.T) {
	mockey.PatchConvey("doCompact succeeds", t, func() {
		compactCount := 0
		mockey.Mock(sqlite.Compact).To(func(ctx context.Context, db *sql.DB) error {
			compactCount++
			return nil
		}).Build()

		dbRW, err := sqlite.Open(":memory:")
		require.NoError(t, err)
		defer func() { _ = dbRW.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		done := make(chan struct{})
		go func() {
			doCompact(ctx, dbRW, 50*time.Millisecond)
			close(done)
		}()

		select {
		case <-done:
			assert.Greater(t, compactCount, 0, "Compact should have been called at least once")
		case <-time.After(500 * time.Millisecond):
			t.Fatal("doCompact should return after context is done")
		}
	})
}

// TestUpdateFromVersionFile_UpdateSuccess tests updateFromVersionFile when update succeeds.
func TestUpdateFromVersionFile_UpdateSuccess(t *testing.T) {
	mockey.PatchConvey("updateFromVersionFile handles successful update", t, func() {
		safetyTimer := time.NewTimer(5 * time.Second)
		defer safetyTimer.Stop()

		updateCallCount := 0
		mockey.Mock(pkgupdate.UpdateTargetVersion).To(func(versionFile string, exitCode int) error {
			updateCallCount++
			assert.Equal(t, "/test/version/file", versionFile)
			assert.Equal(t, 42, exitCode)
			return nil
		}).Build()

		mockey.Mock(time.After).To(func(d time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		done := make(chan struct{})
		go func() {
			updateFromVersionFile(ctx, 42, "/test/version/file")
			close(done)
		}()

		select {
		case <-done:
			assert.Greater(t, updateCallCount, 0, "Update should have been called at least once")
		case <-safetyTimer.C:
			t.Fatal("updateFromVersionFile should return after context is done")
		}
	})
}

// TestCreateURL_ProtocolVariants tests createURL with various protocol prefixes.
func TestCreateURL_ProtocolVariants(t *testing.T) {
	mockey.PatchConvey("createURL handles various protocol variants", t, func() {
		tests := []struct {
			name     string
			endpoint string
			expected string
		}{
			{
				name:     "http with port",
				endpoint: "http://myhost:8080",
				expected: "https://myhost:8080",
			},
			{
				name:     "https with port",
				endpoint: "https://myhost:443",
				expected: "https://myhost:443",
			},
			{
				name:     "http with path and fragment",
				endpoint: "http://myhost:8080/api/v1#section",
				expected: "https://myhost:8080",
			},
			{
				name:     "endpoint with user info",
				endpoint: "https://user:pass@myhost:8080/path",
				expected: "https://myhost:8080",
			},
			{
				name:     "plain IP with port",
				endpoint: "192.168.1.100:9090",
				expected: "https://192.168.1.100:9090",
			},
			{
				name:     "hostname only no port",
				endpoint: "my-service.local",
				expected: "https://my-service.local",
			},
		}

		for _, tc := range tests {
			result := createURL(tc.endpoint)
			assert.Equal(t, tc.expected, result, "createURL(%q)", tc.endpoint)
		}
	})
}

func TestStartListener_ListenErrorExits(t *testing.T) {
	mockey.PatchConvey("startListener exits when ListenAndServeTLS fails", t, func() {
		s := &Server{}
		cfg := &lepconfig.Config{
			Address: "127.0.0.1:0",
		}

		gin.SetMode(gin.TestMode)
		router := gin.New()

		cert, err := s.generateSelfSignedCert()
		require.NoError(t, err)

		nvml := &mockNVMLInstance{}
		syncer := &pkgmetricssyncer.Syncer{}

		exitCode := 0
		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCode = code
			exitCalled = true
		}).Build()

		stopCalled := false
		mockey.Mock((*Server).Stop).To(func(_ *Server) {
			stopCalled = true
		}).Build()

		syncerStopped := false
		mockey.Mock((*pkgmetricssyncer.Syncer).Stop).To(func(_ *pkgmetricssyncer.Syncer) {
			syncerStopped = true
		}).Build()

		mockey.Mock((*http.Server).ListenAndServeTLS).To(func(_ *http.Server, _, _ string) error {
			return errors.New("listen failed")
		}).Build()

		s.startListener(nvml, syncer, cfg, router, cert)

		assert.True(t, exitCalled, "expected os.Exit to be invoked")
		assert.Equal(t, 1, exitCode)
		assert.True(t, stopCalled, "expected Server.Stop to be invoked on exit")
		assert.True(t, nvml.shutdownCalled, "expected nvml shutdown to be invoked")
		assert.True(t, syncerStopped, "expected metrics syncer to be stopped")
	})
}

func TestUpdateToken_ReadTokenAndMkfifoOpenError(t *testing.T) {
	mockey.PatchConvey("updateToken reads token then exits on fifo open error", t, func() {
		s := &Server{
			machineID:         "machine-1",
			epLocalGPUdServer: "https://local",
			epControlPlane:    "https://control",
			fifoPath:          "/tmp/gpud-fifo",
			gpudInstance:      &components.GPUdInstance{},
		}
		userToken := &UserToken{}

		readCalled := false
		mockey.Mock(pkgmetadata.ReadToken).To(func(_ context.Context, _ *sql.DB) (string, error) {
			readCalled = true
			return "token-123", nil
		}).Build()

		newSessionCalled := false
		mockey.Mock(pkgsession.NewSession).To(func(_ context.Context, _, _, _ string, _ ...pkgsession.OpOption) (*pkgsession.Session, error) {
			newSessionCalled = true
			return &pkgsession.Session{}, nil
		}).Build()

		mockey.Mock(os.Stat).To(func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}).Build()
		mockey.Mock(syscall.Mkfifo).To(func(_ string, _ uint32) error {
			return nil
		}).Build()
		mockey.Mock(os.OpenFile).To(func(_ string, _ int, _ os.FileMode) (*os.File, error) {
			return nil, errors.New("open failed")
		}).Build()

		s.updateToken(context.Background(), nil, userToken)

		assert.True(t, readCalled)
		assert.True(t, newSessionCalled)
		userToken.mu.RLock()
		defer userToken.mu.RUnlock()
		assert.Equal(t, "token-123", userToken.userToken)
	})
}

func TestUpdateToken_RemoveError(t *testing.T) {
	mockey.PatchConvey("updateToken returns when fifo removal fails", t, func() {
		s := &Server{
			fifoPath: "/tmp/gpud-fifo",
		}
		userToken := &UserToken{}

		mockey.Mock(pkgmetadata.ReadToken).To(func(_ context.Context, _ *sql.DB) (string, error) {
			return "", errors.New("read failed")
		}).Build()

		mockey.Mock(os.Stat).To(func(_ string) (os.FileInfo, error) {
			return mockFileInfo{}, nil
		}).Build()
		mockey.Mock(os.Remove).To(func(_ string) error {
			return errors.New("remove failed")
		}).Build()

		s.updateToken(context.Background(), nil, userToken)
	})
}

// TestNew_DBInMemoryMode tests New with DBInMemory enabled.
func TestNew_DBInMemoryMode(t *testing.T) {
	mockey.PatchConvey("New with DBInMemory mode", t, func() {
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		require.NoError(t, os.Setenv(nvmllib.EnvMockAllSuccess, "true"))
		defer func() { _ = os.Unsetenv(nvmllib.EnvMockAllSuccess) }()

		mockey.Mock(httputil.CreateURL).To(func(scheme string, endpoint string, path string) (string, error) {
			return "", errors.New("invalid address for testing")
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg := &lepconfig.Config{
			Address:                "localhost:0",
			DataDir:                tmpDir,
			MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
			Components:             []string{"-disable-all"},
			DBInMemory:             true,
		}

		s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
		require.Error(t, err)
		require.Nil(t, s)
		assert.Contains(t, err.Error(), "failed to create local GPUd server endpoint")
	})
}

// TestNew_DBInMemoryWithSessionCredentials tests New with DBInMemory and session credentials.
func TestNew_DBInMemoryWithSessionCredentials(t *testing.T) {
	mockey.PatchConvey("New with DBInMemory and session credentials", t, func() {
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		require.NoError(t, os.Setenv(nvmllib.EnvMockAllSuccess, "true"))
		defer func() { _ = os.Unsetenv(nvmllib.EnvMockAllSuccess) }()

		mockey.Mock(httputil.CreateURL).To(func(scheme string, endpoint string, path string) (string, error) {
			return "", errors.New("invalid address for testing")
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg := &lepconfig.Config{
			Address:                "localhost:0",
			DataDir:                tmpDir,
			MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
			Components:             []string{"-disable-all"},
			DBInMemory:             true,
			SessionToken:           "test-session-token",
			SessionMachineID:       "test-machine-id",
			SessionEndpoint:        "https://api.example.com",
		}

		s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
		require.Error(t, err)
		require.Nil(t, s)
		// Should reach the CreateURL mock error, meaning credentials were seeded successfully
		assert.Contains(t, err.Error(), "failed to create local GPUd server endpoint")
	})
}

// TestDefaultSignalsToHandle tests the DefaultSignalsToHandle constant.
func TestDefaultSignalsToHandle(t *testing.T) {
	mockey.PatchConvey("DefaultSignalsToHandle contains expected signals", t, func() {
		assert.Contains(t, DefaultSignalsToHandle, unix.SIGTERM)
		assert.Contains(t, DefaultSignalsToHandle, unix.SIGINT)
		assert.Contains(t, DefaultSignalsToHandle, unix.SIGUSR1)
		assert.Contains(t, DefaultSignalsToHandle, unix.SIGPIPE)
		assert.Len(t, DefaultSignalsToHandle, 4)
	})
}

// TestDefaultHealthz tests the DefaultHealthz constant values.
func TestDefaultHealthz(t *testing.T) {
	mockey.PatchConvey("DefaultHealthz has expected values", t, func() {
		assert.Equal(t, "ok", DefaultHealthz.Status)
		assert.Equal(t, "v1", DefaultHealthz.Version)
	})
}

// TestURLPathConstants tests that URL path constants are as expected.
func TestURLPathConstants(t *testing.T) {
	mockey.PatchConvey("URL path constants are correct", t, func() {
		assert.Equal(t, "/healthz", URLPathHealthz)
		assert.Equal(t, "/machine-info", URLPathMachineInfo)
		assert.Equal(t, "/inject-fault", URLPathInjectFault)
		assert.Equal(t, "/plugins", URLPathComponentsCustomPlugins)
		assert.Equal(t, "/swagger/*any", URLPathSwagger)
	})
}

// TestGetPluginSpecs_EmptyRegistry tests getPluginSpecs with an empty registry.
func TestGetPluginSpecs_EmptyRegistry(t *testing.T) {
	mockey.PatchConvey("getPluginSpecs with empty registry returns empty list", t, func() {
		handler, _, _ := setupTestHandler([]components.Component{})

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/plugins", nil)
		handler.getPluginSpecs(c)

		assert.Equal(t, http.StatusOK, w.Code)

		// Response should be null or empty array
		responseBody := strings.TrimSpace(w.Body.String())
		assert.True(t, responseBody == "null" || responseBody == "[]",
			"Expected null or empty array, got: %s", responseBody)
	})
}

// TestGetPluginSpecs_InvalidContentType tests getPluginSpecs with an invalid content type.
func TestGetPluginSpecs_InvalidContentType_Mockey(t *testing.T) {
	mockey.PatchConvey("getPluginSpecs invalid content type", t, func() {
		handler, _, _ := setupTestHandler([]components.Component{})

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/plugins", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, "text/xml")

		handler.getPluginSpecs(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "invalid content type", resp["message"])
	})
}

// TestInjectFault_NilInjector tests injectFault when fault injector is nil.
func TestInjectFault_NilInjector_Mockey(t *testing.T) {
	mockey.PatchConvey("injectFault returns not found when injector is nil", t, func() {
		handler := &globalHandler{
			faultInjector: nil,
		}

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("POST", "/inject-fault", strings.NewReader(`{"kernel_message":{"priority":"KERN_INFO","message":"test"}}`))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.injectFault(c)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "fault injector not set up", resp["message"])
	})
}

// TestInjectFault_InvalidBody tests injectFault with an invalid JSON body.
func TestInjectFault_InvalidBody_Mockey(t *testing.T) {
	mockey.PatchConvey("injectFault handles invalid request body", t, func() {
		mockInjector := new(mockFaultInjector)
		handler := &globalHandler{
			faultInjector: mockInjector,
		}

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("POST", "/inject-fault", strings.NewReader(`{invalid json`))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.injectFault(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["message"], "failed to decode request body")
	})
}

// TestInjectFault_EmptyBody tests injectFault with empty request body (no fault entry).
func TestInjectFault_EmptyBody_Mockey(t *testing.T) {
	mockey.PatchConvey("injectFault handles empty body validation error", t, func() {
		mockInjector := new(mockFaultInjector)
		handler := &globalHandler{
			faultInjector: mockInjector,
		}

		gin.SetMode(gin.TestMode)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("POST", "/inject-fault", strings.NewReader(`{}`))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.injectFault(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["message"], "invalid request")
	})
}

// TestHandleAdminPackagesPath tests the admin packages URL path constant.
func TestHandleAdminPackagesPath(t *testing.T) {
	mockey.PatchConvey("admin packages path is correct", t, func() {
		assert.Equal(t, "/admin/packages", URLPathAdminPackages)
	})
}

// TestServerStop_WithRebootEventStore tests Server Stop with a reboot event store that implements Closer.
func TestServerStop_WithRebootEventStore(t *testing.T) {
	mockey.PatchConvey("Server Stop closes reboot event store", t, func() {
		dbRW, err := sqlite.Open(":memory:")
		require.NoError(t, err)

		dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
		require.NoError(t, err)

		s := &Server{
			dbRW:               dbRW,
			dbRO:               dbRO,
			componentsRegistry: components.NewRegistry(nil),
			gpudInstance: &components.GPUdInstance{
				// RebootEventStore is not set (nil) - should handle gracefully
				RebootEventStore: nil,
			},
		}

		// Should not panic
		s.Stop()

		_, err = dbRW.Exec("SELECT 1")
		assert.Error(t, err, "DB should be closed")
	})
}

// TestNew_MetadataReadError tests New when ReadMetadata fails (e.g., for machine UID read).
func TestNew_MetadataReadError(t *testing.T) {
	mockey.PatchConvey("New handles metadata read error", t, func() {
		tmpDir, err := os.MkdirTemp("", "gpud-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		require.NoError(t, os.Setenv(nvmllib.EnvMockAllSuccess, "true"))
		defer func() { _ = os.Unsetenv(nvmllib.EnvMockAllSuccess) }()

		// Mock ReadMetadata to fail - this is called via ReadMachineID during New()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", errors.New("metadata read failed")
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg := &lepconfig.Config{
			Address:                "localhost:0",
			DataDir:                tmpDir,
			MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
			Components:             []string{"-disable-all"},
		}

		s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
		require.Error(t, err)
		require.Nil(t, s)
		// ReadMachineID calls ReadMetadata internally, so the error surfaces as "failed to read machine uid"
		assert.Contains(t, err.Error(), "failed to read machine uid")
	})
}

// TestUserToken_EmptyInitially tests that UserToken starts with an empty token.
func TestUserToken_EmptyInitially(t *testing.T) {
	mockey.PatchConvey("UserToken is empty initially", t, func() {
		token := &UserToken{}

		token.mu.RLock()
		value := token.userToken
		token.mu.RUnlock()

		assert.Empty(t, value, "UserToken should be empty initially")
	})
}

// TestUserToken_SetAndGet tests setting and getting the user token.
func TestUserToken_SetAndGet(t *testing.T) {
	mockey.PatchConvey("UserToken set and get", t, func() {
		token := &UserToken{}

		token.mu.Lock()
		token.userToken = "my-secret-token"
		token.mu.Unlock()

		token.mu.RLock()
		value := token.userToken
		token.mu.RUnlock()

		assert.Equal(t, "my-secret-token", value)
	})
}

// TestGetReqComponents_EmptyQuery tests getReqComponentNames returns all components when query is empty.
func TestGetReqComponents_EmptyQuery(t *testing.T) {
	mockey.PatchConvey("getReqComponentNames returns all when query empty", t, func() {
		comp1 := &mockComponent{name: "alpha", isSupported: true}
		comp2 := &mockComponent{name: "beta", isSupported: true}

		handler, _, _ := setupTestHandler([]components.Component{comp1, comp2})

		gin.SetMode(gin.TestMode)
		_, c, _ := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/states", nil)

		names, err := handler.getReqComponentNames(c)
		require.NoError(t, err)
		assert.Len(t, names, 2)
		assert.Contains(t, names, "alpha")
		assert.Contains(t, names, "beta")
	})
}

// TestGetReqComponents_SpecificComponent tests getReqComponentNames with specific components.
func TestGetReqComponents_SpecificComponent(t *testing.T) {
	mockey.PatchConvey("getReqComponentNames returns specific component", t, func() {
		comp1 := &mockComponent{name: "alpha", isSupported: true}
		comp2 := &mockComponent{name: "beta", isSupported: true}

		handler, _, _ := setupTestHandler([]components.Component{comp1, comp2})

		gin.SetMode(gin.TestMode)
		_, c, _ := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/states?components=alpha", nil)

		names, err := handler.getReqComponentNames(c)
		require.NoError(t, err)
		assert.Len(t, names, 1)
		assert.Equal(t, "alpha", names[0])
	})
}

// TestGetReqComponents_NotFound tests getReqComponentNames when component is not found.
func TestGetReqComponents_NotFound(t *testing.T) {
	mockey.PatchConvey("getReqComponentNames returns error for missing component", t, func() {
		comp1 := &mockComponent{name: "alpha", isSupported: true}

		handler, _, _ := setupTestHandler([]components.Component{comp1})

		gin.SetMode(gin.TestMode)
		_, c, _ := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/states?components=nonexistent", nil)

		names, err := handler.getReqComponentNames(c)
		assert.Error(t, err)
		assert.Nil(t, names)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestGetReqTime_EmptyParams tests getReqTime returns current time when no params.
func TestGetReqTime_EmptyParams(t *testing.T) {
	mockey.PatchConvey("getReqTime returns current time when no params", t, func() {
		handler := &globalHandler{}

		gin.SetMode(gin.TestMode)
		_, c, _ := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/events", nil)

		before := time.Now()
		startTime, endTime, err := handler.getReqTime(c)
		after := time.Now()

		require.NoError(t, err)
		assert.True(t, !startTime.Before(before) && !startTime.After(after), "startTime should be around now")
		assert.True(t, !endTime.Before(before) && !endTime.After(after), "endTime should be around now")
	})
}
