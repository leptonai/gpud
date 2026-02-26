package up

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"io"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	"github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	"github.com/leptonai/gpud/pkg/osutil"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

// newMockeyCLIContext creates a CLI context for mockey testing with the given arguments.
func newMockeyCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-up-mockey-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("log-file", "", "")
	_ = flags.String("data-dir", "", "")
	_ = flags.String("token", "", "")
	_ = flags.String("endpoint", "", "")
	_ = flags.String("machine-id", "", "")
	_ = flags.String("node-group", "", "")
	_ = flags.String("gpu-count", "", "")
	_ = flags.String("public-ip", "", "")
	_ = flags.String("private-ip", "", "")
	_ = flags.Bool("db-in-memory", false, "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// TestCommand_InvalidLogLevel tests the command with invalid log level.
func TestCommand_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("invalid log level", t, func() {
		cliContext := newMockeyCLIContext(t, []string{"--log-level", "invalid-level"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unrecognized level")
	})
}

// TestCommand_ValidLogLevels tests the command with various valid log levels.
func TestCommand_ValidLogLevels(t *testing.T) {
	testCases := []struct {
		name     string
		logLevel string
	}{
		{"debug level", "debug"},
		{"info level", "info"},
		{"warn level", "warn"},
		{"error level", "error"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
					return "", errors.New("stopped at resolve data dir")
				}).Build()

				cliContext := newMockeyCLIContext(t, []string{"--log-level", tc.logLevel})
				err := Command(cliContext)
				// Should fail at ResolveDataDir, not at log level parsing
				require.Error(t, err)
				assert.Contains(t, err.Error(), "stopped at resolve data dir")
			})
		})
	}
}

// TestCommand_LoginSuccessWithRecordStateError tests successful login but recordLoginSuccessState fails (warning path).
func TestCommand_LoginSuccessWithRecordStateError(t *testing.T) {
	mockey.PatchConvey("login success with record state error", t, func() {
		tmpDir := t.TempDir()

		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			return nil
		}).Build()
		// Mock recordLoginSuccessState to fail via config.ResolveDataDir
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "", errors.New("failed to resolve data dir in recordLoginSuccessState")
		}).Build()
		mockey.Mock(os.Executable).To(func() (string, error) {
			return "/usr/bin/gpud", nil
		}).Build()
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.DefaultBinExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			return nil
		}).Build()
		mockey.Mock(pkgupdate.EnableGPUdSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.RestartGPUdSystemdUnit).To(func() error { return nil }).Build()

		cliContext := newMockeyCLIContext(t, []string{"--token", "test-token"})
		// Should succeed despite recordLoginSuccessState warning
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_WithAllFlags tests the command with all optional flags set.
func TestCommand_WithAllFlags(t *testing.T) {
	mockey.PatchConvey("with all flags", t, func() {
		tmpDir := t.TempDir()
		stateFile := tmpDir + "/gpud.state"

		loginCfgCapture := login.LoginConfig{}

		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			loginCfgCapture = cfg
			return nil
		}).Build()
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(config.StateFilePath).To(func(dataDir string) string {
			return stateFile
		}).Build()
		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(sessionstates.Insert).To(func(ctx context.Context, db *sql.DB, ts int64, success bool, msg string) error {
			return nil
		}).Build()
		mockey.Mock(os.Executable).To(func() (string, error) {
			return "/usr/bin/gpud", nil
		}).Build()
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.DefaultBinExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			return nil
		}).Build()
		mockey.Mock(pkgupdate.EnableGPUdSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.RestartGPUdSystemdUnit).To(func() error { return nil }).Build()

		cliContext := newMockeyCLIContext(t, []string{
			"--token", "test-token",
			"--endpoint", "https://api.example.com",
			"--machine-id", "machine-123",
			"--node-group", "gpu-nodes",
			"--gpu-count", "8",
			"--public-ip", "1.2.3.4",
			"--private-ip", "10.0.0.1",
			"--db-in-memory",
		})
		err := Command(cliContext)
		require.NoError(t, err)

		// Verify login config captured correct values
		assert.Equal(t, "test-token", loginCfgCapture.Token)
		assert.Equal(t, "https://api.example.com", loginCfgCapture.Endpoint)
		assert.Equal(t, "machine-123", loginCfgCapture.MachineID)
		assert.Equal(t, "gpu-nodes", loginCfgCapture.NodeGroup)
		assert.Equal(t, "8", loginCfgCapture.GPUCount)
		assert.Equal(t, "1.2.3.4", loginCfgCapture.PublicIP)
		assert.Equal(t, "10.0.0.1", loginCfgCapture.PrivateIP)
	})
}

// TestCommand_WithLogFile tests the command with log file flag.
func TestCommand_WithLogFile(t *testing.T) {
	mockey.PatchConvey("with log file", t, func() {
		tmpDir := t.TempDir()
		logFile := tmpDir + "/gpud.log"

		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "", errors.New("stopped at resolve data dir")
		}).Build()

		cliContext := newMockeyCLIContext(t, []string{"--log-file", logFile})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stopped at resolve data dir")
	})
}

// TestCommand_WithEmptyToken tests that empty token value is ignored.
func TestCommand_WithEmptyToken(t *testing.T) {
	mockey.PatchConvey("with empty token", t, func() {
		tmpDir := t.TempDir()

		loginCalled := false
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			loginCalled = true
			return nil
		}).Build()
		mockey.Mock(os.Executable).To(func() (string, error) {
			return "/usr/bin/gpud", nil
		}).Build()
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.DefaultBinExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			return nil
		}).Build()
		mockey.Mock(pkgupdate.EnableGPUdSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.RestartGPUdSystemdUnit).To(func() error { return nil }).Build()

		// Token flag set but value is empty string - should still skip login
		cliContext := newMockeyCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.False(t, loginCalled, "login should not be called when token is empty")
	})
}

// TestSystemdInit_WriteFileErrorWithDifferentPaths tests systemdInit with different path scenarios.
func TestSystemdInit_WriteFileErrorWithDifferentPaths(t *testing.T) {
	mockey.PatchConvey("systemdInit write file error with path check", t, func() {
		writtenPath := ""
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd Service"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			return errors.New("permission denied")
		}).Build()

		err := systemdInit("http://localhost:8080", "/var/lib/gpud", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
		assert.Equal(t, systemd.DefaultUnitFile, writtenPath)
	})
}

// TestSystemdInit_WithDbInMemory tests systemdInit passes dbInMemory flag correctly.
func TestSystemdInit_WithDbInMemory(t *testing.T) {
	mockey.PatchConvey("systemdInit with db in memory", t, func() {
		capturedDbInMemory := false
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			capturedDbInMemory = dbInMemory
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			return nil
		}).Build()

		err := systemdInit("http://localhost", "/tmp", true)
		require.NoError(t, err)
		assert.True(t, capturedDbInMemory, "dbInMemory should be passed as true")
	})
}

// TestRecordLoginSuccessState_SQLiteOpenWithOptions tests recordLoginSuccessState with sqlite options.
func TestRecordLoginSuccessState_SQLiteOpenWithOptions(t *testing.T) {
	mockey.PatchConvey("recordLoginSuccessState sqlite open", t, func() {
		tmpDir := t.TempDir()
		stateFile := tmpDir + "/gpud.state"
		var openedPath string

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(config.StateFilePath).To(func(dataDir string) string {
			return stateFile
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			openedPath = dbPath
			return nil, errors.New("database locked")
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
		assert.Equal(t, stateFile, openedPath)
	})
}

// TestRecordLoginSuccessState_InsertWithCorrectParams tests that Insert is called with correct parameters.
func TestRecordLoginSuccessState_InsertWithCorrectParams(t *testing.T) {
	mockey.PatchConvey("recordLoginSuccessState insert params", t, func() {
		tmpDir := t.TempDir()
		stateFile := tmpDir + "/gpud.state"
		var capturedSuccess bool
		var capturedMsg string

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(config.StateFilePath).To(func(dataDir string) string {
			return stateFile
		}).Build()
		mockey.Mock(sessionstates.CreateTable).To(func(ctx context.Context, db *sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(sessionstates.Insert).To(func(ctx context.Context, db *sql.DB, ts int64, success bool, msg string) error {
			capturedSuccess = success
			capturedMsg = msg
			return nil
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		require.NoError(t, err)
		assert.True(t, capturedSuccess, "success should be true")
		assert.Equal(t, "Session connected successfully", capturedMsg)
	})
}

// TestCommand_ContextCancellation tests behavior when context is canceled.
func TestCommand_ContextCancellation(t *testing.T) {
	mockey.PatchConvey("context cancellation during login", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test", nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			return context.Canceled
		}).Build()

		cliContext := newMockeyCLIContext(t, []string{"--token", "test-token"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

// TestCommand_DeadlineExceeded tests behavior when deadline is exceeded.
func TestCommand_DeadlineExceeded(t *testing.T) {
	mockey.PatchConvey("deadline exceeded during login", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test", nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			return context.DeadlineExceeded
		}).Build()

		cliContext := newMockeyCLIContext(t, []string{"--token", "test-token"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Equal(t, context.DeadlineExceeded, err)
	})
}

// TestParseLogLevel_ValidLevels tests ParseLogLevel with valid levels.
func TestParseLogLevel_ValidLevels(t *testing.T) {
	testCases := []struct {
		name     string
		level    string
		expected string
	}{
		{"empty defaults to info", "", "info"},
		{"info level", "info", "info"},
		{"debug level", "debug", "debug"},
		{"warn level", "warn", "warn"},
		{"error level", "error", "error"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				zapLvl, err := log.ParseLogLevel(tc.level)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, zapLvl.String())
			})
		})
	}
}

// TestParseLogLevel_InvalidLevel tests ParseLogLevel with invalid level.
func TestParseLogLevel_InvalidLevel(t *testing.T) {
	mockey.PatchConvey("invalid log level", t, func() {
		_, err := log.ParseLogLevel("invalid-level")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unrecognized level")
	})
}

// TestCommand_SystemdEnvFileWithEndpoint tests that endpoint is passed to CreateDefaultEnvFile.
func TestCommand_SystemdEnvFileWithEndpoint(t *testing.T) {
	mockey.PatchConvey("systemd env file with endpoint", t, func() {
		tmpDir := t.TempDir()
		var capturedEndpoint string
		var capturedDataDir string

		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(os.Executable).To(func() (string, error) {
			return "/usr/bin/gpud", nil
		}).Build()
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.DefaultBinExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			capturedEndpoint = endpoint
			capturedDataDir = dataDir
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			return nil
		}).Build()
		mockey.Mock(pkgupdate.EnableGPUdSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.RestartGPUdSystemdUnit).To(func() error { return nil }).Build()

		cliContext := newMockeyCLIContext(t, []string{"--endpoint", "https://custom.endpoint.com"})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.Equal(t, "https://custom.endpoint.com", capturedEndpoint)
		assert.Equal(t, tmpDir, capturedDataDir)
	})
}

// TestCommand_MultipleLoginErrors tests different login error types.
func TestCommand_MultipleLoginErrors(t *testing.T) {
	errorCases := []struct {
		name        string
		loginErr    error
		errContains string
	}{
		{
			name:        "authentication failed",
			loginErr:    errors.New("authentication failed: invalid token"),
			errContains: "authentication failed",
		},
		{
			name:        "network error",
			loginErr:    errors.New("network error: connection refused"),
			errContains: "network error",
		},
		{
			name:        "server error",
			loginErr:    errors.New("server error: 500 internal server error"),
			errContains: "server error",
		},
	}

	for _, ec := range errorCases {
		t.Run(ec.name, func(t *testing.T) {
			mockey.PatchConvey(ec.name, t, func() {
				mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
					return "/tmp/test", nil
				}).Build()
				mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
				mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
					return ec.loginErr
				}).Build()

				cliContext := newMockeyCLIContext(t, []string{"--token", "test-token"})
				err := Command(cliContext)
				require.Error(t, err)
				assert.Contains(t, err.Error(), ec.errContains)
			})
		})
	}
}
