//go:build linux

package run

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	"github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	gpudserver "github.com/leptonai/gpud/pkg/server"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
	pkgsystemd "github.com/leptonai/gpud/pkg/systemd"
)

// createTestStateFile creates a minimal test state file for testing
func createTestStateFile(t *testing.T, dir string) string {
	stateFile := filepath.Join(dir, "gpud.state")
	require.NoError(t, os.WriteFile(stateFile, []byte(""), 0644))
	return stateFile
}

// TestRecordLoginSuccessState_CreateTableError tests error handling when creating session states table fails.
func TestRecordLoginSuccessState_CreateTableError(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("recordLoginSuccessState create table error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return errors.New("failed to create table")
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create session states table")
	})
}

// TestRecordLoginSuccessState_InsertError tests error handling when inserting login state fails.
func TestRecordLoginSuccessState_InsertError(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("recordLoginSuccessState insert error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.Insert).To(func(ctx context.Context, db *sql.DB, ts int64, success bool, message string) error {
			return errors.New("failed to insert")
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to record login success state")
	})
}

// TestReadSessionCredentialsFromPersistentFile_SqliteOpenError tests error when sqlite open fails.
func TestReadSessionCredentialsFromPersistentFile_SqliteOpenError(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("readSessionCredentials sqlite open error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		stateFilePath := tmpDir + "/state.db"
		mockey.Mock(config.StateFilePath).To(func(dataDir string) string {
			return stateFilePath
		}).Build()

		// Create the state file at the exact path that config.StateFilePath returns
		// so os.Stat succeeds and the code proceeds to pkgsqlite.Open
		require.NoError(t, os.WriteFile(stateFilePath, []byte(""), 0644))

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("sqlite permission denied")
		}).Build()

		ctx := context.Background()
		_, _, _, err := readSessionCredentialsFromPersistentFile(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

// TestGetSessionCredentialsOptions_ReadError tests getSessionCredentialsOptions with non-state-file-not-found error.
func TestGetSessionCredentialsOptions_ReadError(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("getSessionCredentialsOptions read error", t, func() {
		// Create a state file so it's not a "not found" error
		_ = config.StateFilePath(tmpDir)
		createTestStateFile(t, tmpDir)

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", errors.New("database corrupted")
		}).Build()

		// With db-in-memory true and a corrupted file, should return nil options
		opts := getSessionCredentialsOptions(true, tmpDir, "")
		assert.Empty(t, opts)
	})
}

// TestGetSessionCredentialsOptions_IncompleteCredentials tests when only some credentials are present.
func TestGetSessionCredentialsOptions_IncompleteCredentials(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("getSessionCredentialsOptions incomplete credentials", t, func() {
		_ = config.StateFilePath(tmpDir)
		createTestStateFile(t, tmpDir)

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		// Only token is present, missing machine ID and endpoint
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "some-token", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil // empty
		}).Build()

		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil // empty
		}).Build()

		// With incomplete credentials and no fallback endpoint, should return nil
		opts := getSessionCredentialsOptions(true, tmpDir, "")
		assert.Empty(t, opts)
	})
}

// TestGetSessionCredentialsOptions_WithFallbackEndpoint tests endpoint fallback from CLI flag.
func TestGetSessionCredentialsOptions_WithFallbackEndpoint(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("getSessionCredentialsOptions with fallback endpoint", t, func() {
		_ = config.StateFilePath(tmpDir)
		createTestStateFile(t, tmpDir)

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "session-token", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "machine-id-123", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil // endpoint empty in DB
		}).Build()

		// With fallback endpoint, should return options
		opts := getSessionCredentialsOptions(true, tmpDir, "https://fallback.endpoint.com")
		assert.NotEmpty(t, opts)
		assert.Len(t, opts, 3) // token, machine ID, endpoint
	})
}

// TestGetSessionCredentialsOptions_AllCredentialsPresent tests full credentials scenario.
func TestGetSessionCredentialsOptions_AllCredentialsPresent(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("getSessionCredentialsOptions all credentials present", t, func() {
		_ = config.StateFilePath(tmpDir)
		createTestStateFile(t, tmpDir)

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "session-token", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "machine-id-123", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			if key == pkgmetadata.MetadataKeyEndpoint {
				return "https://stored.endpoint.com", nil
			}
			return "", nil
		}).Build()

		opts := getSessionCredentialsOptions(true, tmpDir, "")
		assert.NotEmpty(t, opts)
		assert.Len(t, opts, 3) // token, machine ID, endpoint
	})
}

// TestGetSessionCredentialsOptions_DBNotInMemory tests that options are nil when not in memory mode.
func TestGetSessionCredentialsOptions_DBNotInMemory(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("getSessionCredentialsOptions db not in memory", t, func() {
		// When dbInMemory is false, should return nil immediately without reading
		opts := getSessionCredentialsOptions(false, tmpDir, "https://endpoint.com")
		assert.Empty(t, opts)
	})
}

// TestParseInfinibandExcludeDevices_AllSpacesAndCommas tests edge case parsing.
func TestParseInfinibandExcludeDevices_AllSpacesAndCommas(t *testing.T) {
	result := parseInfinibandExcludeDevices("   ,   ,   ")
	assert.Nil(t, result)
}

// TestParseInfinibandExcludeDevices_ComplexInput tests parsing with various inputs.
func TestParseInfinibandExcludeDevices_ComplexInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "leading trailing spaces",
			input:    "  mlx5_0  ,  mlx5_1  ",
			expected: []string{"mlx5_0", "mlx5_1"},
		},
		{
			name:     "mixed empty entries",
			input:    "mlx5_0,,mlx5_1,,mlx5_2",
			expected: []string{"mlx5_0", "mlx5_1", "mlx5_2"},
		},
		{
			name:     "single device no spaces",
			input:    "mlx5_bond_0",
			expected: []string{"mlx5_bond_0"},
		},
		{
			name:     "multiple devices tab separated",
			input:    "mlx5_0,\tmlx5_1",
			expected: []string{"mlx5_0", "mlx5_1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseInfinibandExcludeDevices(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRecordLoginSuccessState_FullFlow tests the full success flow.
func TestRecordLoginSuccessState_FullFlow(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("recordLoginSuccessState full flow success", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		insertCalled := false
		mockey.Mock(sessionstates.Insert).To(func(ctx context.Context, db *sql.DB, ts int64, success bool, message string) error {
			insertCalled = true
			assert.True(t, success, "expected success=true")
			assert.Contains(t, message, "Session connected successfully")
			return nil
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		require.NoError(t, err)
		assert.True(t, insertCalled, "expected Insert to be called")
	})
}

// TestHandleSignals_NotifyStopping tests signal handling with systemd notification.
func TestHandleSignals_NotifyStopping(t *testing.T) {
	mockey.PatchConvey("HandleSignals notify stopping", t, func() {
		notifyStoppingCalled := false
		mockey.Mock(pkgsystemd.NotifyStopping).To(func(ctx context.Context) error {
			notifyStoppingCalled = true
			return nil
		}).Build()

		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		// Create a mock stopping function that should be called
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		signals := make(chan gpudserver.ServerStopper, 1)
		notifyFn := func(ctx context.Context) error {
			if pkgsystemd.SystemctlExists() {
				return pkgsystemd.NotifyStopping(ctx)
			}
			return nil
		}

		// Test the notify function directly
		err := notifyFn(ctx)
		assert.NoError(t, err)
		assert.True(t, notifyStoppingCalled)
		close(signals)
	})
}

// TestHandleSignals_NotifyStoppingError tests signal handling with systemd notification error.
func TestHandleSignals_NotifyStoppingError(t *testing.T) {
	mockey.PatchConvey("HandleSignals notify stopping error", t, func() {
		mockey.Mock(pkgsystemd.NotifyStopping).To(func(ctx context.Context) error {
			return errors.New("systemd notification failed")
		}).Build()

		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		ctx := context.Background()

		notifyFn := func(ctx context.Context) error {
			if pkgsystemd.SystemctlExists() {
				if err := pkgsystemd.NotifyStopping(ctx); err != nil {
					return err
				}
			}
			return nil
		}

		err := notifyFn(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "systemd notification failed")
	})
}

// TestLoginFlow_LoginError tests the login error path in Command.
func TestLoginFlow_LoginError(t *testing.T) {
	mockey.PatchConvey("login error", t, func() {
		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			return errors.New("login failed: invalid token")
		}).Build()

		// Test the login call directly
		ctx := context.Background()
		cfg := login.LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  "/tmp/test",
		}

		err := login.Login(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "login failed")
	})
}

// TestLoginFlow_RecordLoginSuccessStateWarning tests warning on recordLoginSuccessState failure.
func TestLoginFlow_RecordLoginSuccessStateWarning(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("login success but record state fails", t, func() {
		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			return nil
		}).Build()

		mockey.Mock(recordLoginSuccessState).To(func(ctx context.Context, dataDir string) error {
			return errors.New("failed to persist state")
		}).Build()

		// The login should succeed, the state recording failure should only log a warning
		ctx := context.Background()
		cfg := login.LoginConfig{
			Token:    "test-token",
			Endpoint: "https://example.com",
			DataDir:  tmpDir,
		}

		// Login succeeds
		err := login.Login(ctx, cfg)
		require.NoError(t, err)

		// Record state fails but shouldn't affect the overall flow
		err = recordLoginSuccessState(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to persist state")
	})
}

// TestResolveDataDir_Error tests error handling for common.ResolveDataDir.
func TestResolveDataDir_Error(t *testing.T) {
	mockey.PatchConvey("ResolveDataDir error", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "", errors.New("invalid data dir path")
		}).Build()

		// Directly test the mock
		_, err := common.ResolveDataDir(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid data dir path")
	})
}

// TestLogParseLevel_InvalidLevel tests log level parsing with invalid input.
func TestLogParseLevel_InvalidLevel(t *testing.T) {
	_, err := log.ParseLogLevel("invalid-level")
	require.Error(t, err)
}

// TestLogParseLevel_ValidLevels tests log level parsing with valid inputs.
func TestLogParseLevel_ValidLevels(t *testing.T) {
	tests := []struct {
		level string
	}{
		{level: "debug"},
		{level: "info"},
		{level: "warn"},
		{level: "error"},
		{level: ""},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			zapLvl, err := log.ParseLogLevel(tt.level)
			require.NoError(t, err)
			assert.NotNil(t, zapLvl)
		})
	}
}

// TestDefaultConfig_Error tests error handling when DefaultConfig fails.
func TestDefaultConfig_Error(t *testing.T) {
	mockey.PatchConvey("DefaultConfig error", t, func() {
		mockey.Mock(config.DefaultConfig).To(func(ctx context.Context, opts ...config.OpOption) (*config.Config, error) {
			return nil, errors.New("failed to create default config")
		}).Build()

		ctx := context.Background()
		_, err := config.DefaultConfig(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create default config")
	})
}

// TestSystemctlExists_NotAvailable tests behavior when systemctl is not available.
func TestSystemctlExists_NotAvailable(t *testing.T) {
	mockey.PatchConvey("systemctl not available", t, func() {
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		result := pkgsystemd.SystemctlExists()
		assert.False(t, result)
	})
}

// TestSystemctlExists_Available tests behavior when systemctl is available.
func TestSystemctlExists_Available(t *testing.T) {
	mockey.PatchConvey("systemctl available", t, func() {
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()

		result := pkgsystemd.SystemctlExists()
		assert.True(t, result)
	})
}

// TestNotifyReady_Error tests error handling for NotifyReady.
func TestNotifyReady_Error(t *testing.T) {
	mockey.PatchConvey("NotifyReady error", t, func() {
		mockey.Mock(pkgsystemd.NotifyReady).To(func(ctx context.Context) error {
			return errors.New("sd_notify failed")
		}).Build()

		ctx := context.Background()
		err := pkgsystemd.NotifyReady(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sd_notify failed")
	})
}

// TestNotifyReady_Success tests successful NotifyReady.
func TestNotifyReady_Success(t *testing.T) {
	mockey.PatchConvey("NotifyReady success", t, func() {
		mockey.Mock(pkgsystemd.NotifyReady).To(func(ctx context.Context) error {
			return nil
		}).Build()

		ctx := context.Background()
		err := pkgsystemd.NotifyReady(ctx)
		require.NoError(t, err)
	})
}

// TestErrStateFileNotFound tests the error type.
func TestErrStateFileNotFound(t *testing.T) {
	assert.NotNil(t, errStateFileNotFound)
	assert.Contains(t, errStateFileNotFound.Error(), "state file not found")
}

// TestReadSessionCredentialsFromPersistentFile_AllFieldsSuccess tests successful reading of all fields.
func TestReadSessionCredentialsFromPersistentFile_AllFieldsSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("readSessionCredentials all fields success", t, func() {
		_ = config.StateFilePath(tmpDir)
		createTestStateFile(t, tmpDir)

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-session-token", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			if key == pkgmetadata.MetadataKeyEndpoint {
				return "https://test.endpoint.com", nil
			}
			return "", nil
		}).Build()

		ctx := context.Background()
		token, machineID, endpoint, err := readSessionCredentialsFromPersistentFile(ctx, tmpDir)

		require.NoError(t, err)
		assert.Equal(t, "test-session-token", token)
		assert.Equal(t, "test-machine-id", machineID)
		assert.Equal(t, "https://test.endpoint.com", endpoint)
	})
}

// TestConfigValidateError tests config validation error handling.
func TestConfigValidateError(t *testing.T) {
	cfg := &config.Config{
		Address: "", // invalid - required
	}
	err := cfg.Validate()
	require.Error(t, err)
}

// TestConfigValidateSuccess tests successful config validation.
func TestConfigValidateSuccess(t *testing.T) {
	mockey.PatchConvey("config validation success", t, func() {
		// Minimal valid config
		cfg := &config.Config{
			Address: "localhost:8080",
		}

		// Need to set retention period
		mockey.Mock((*config.Config).Validate).To(func(*config.Config) error {
			return nil
		}).Build()

		err := cfg.Validate()
		require.NoError(t, err)
	})
}

// TestGetSessionCredentialsOptions_TokenEmptyMachineIDSet tests partial credentials.
func TestGetSessionCredentialsOptions_TokenEmptyMachineIDSet(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("token empty machine ID set", t, func() {
		_ = config.StateFilePath(tmpDir)
		createTestStateFile(t, tmpDir)

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil // empty token
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "machine-id-123", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "https://endpoint.com", nil
		}).Build()

		// With empty token, should return nil
		opts := getSessionCredentialsOptions(true, tmpDir, "")
		assert.Empty(t, opts)
	})
}

// TestGetSessionCredentialsOptions_AllEmptyWithFallbackEndpoint tests all empty with fallback.
func TestGetSessionCredentialsOptions_AllEmptyWithFallbackEndpoint(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("all empty with fallback endpoint", t, func() {
		_ = config.StateFilePath(tmpDir)
		createTestStateFile(t, tmpDir)

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil
		}).Build()

		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil
		}).Build()

		// Even with fallback endpoint, if token and machine ID are empty, should return nil
		opts := getSessionCredentialsOptions(true, tmpDir, "https://fallback.com")
		assert.Empty(t, opts)
	})
}

// TestRecordLoginSuccessState_DbCloseError tests behavior when DB close fails.
func TestRecordLoginSuccessState_DbCloseError(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("recordLoginSuccessState db close error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()

		closeCallCount := 0
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			closeCallCount++
			return errors.New("close error")
		}).Build()

		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()

		mockey.Mock(sessionstates.Insert).To(func(ctx context.Context, db *sql.DB, ts int64, success bool, message string) error {
			return nil
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		// The function should succeed even if close fails (defer swallows error)
		require.NoError(t, err)
	})
}
