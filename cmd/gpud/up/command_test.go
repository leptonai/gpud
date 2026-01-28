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
	"github.com/leptonai/gpud/pkg/login"
	"github.com/leptonai/gpud/pkg/osutil"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

// newCLIContext creates a CLI context for testing with the given arguments.
func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-up-test", flag.ContinueOnError)
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

// TestCommand_ResolveDataDirError tests the command when ResolveDataDir fails.
func TestCommand_ResolveDataDirError(t *testing.T) {
	mockey.PatchConvey("resolve data dir error", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "", errors.New("failed to resolve data dir")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve data dir")
	})
}

// TestCommand_RequireRootError tests the command when RequireRoot fails.
func TestCommand_RequireRootError(t *testing.T) {
	mockey.PatchConvey("require root error", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test", nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return errors.New("must be run as root")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be run as root")
	})
}

// TestCommand_LoginError tests the command when login fails.
func TestCommand_LoginError(t *testing.T) {
	mockey.PatchConvey("login error", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test", nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			return errors.New("login failed")
		}).Build()

		cliContext := newCLIContext(t, []string{"--token", "test-token"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "login failed")
	})
}

// TestCommand_OsExecutableError tests the command when os.Executable fails.
func TestCommand_OsExecutableError(t *testing.T) {
	mockey.PatchConvey("os executable error", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test", nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(os.Executable).To(func() (string, error) {
			return "", errors.New("failed to get executable path")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get executable path")
	})
}

// TestCommand_SystemctlNotExists tests the command when systemctl doesn't exist.
func TestCommand_SystemctlNotExists(t *testing.T) {
	mockey.PatchConvey("systemctl not exists", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test", nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(os.Executable).To(func() (string, error) {
			return "/usr/bin/gpud", nil
		}).Build()
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool { return false }).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires systemd")
	})
}

// TestCommand_DefaultBinNotExists tests the command when gpud binary doesn't exist.
func TestCommand_DefaultBinNotExists(t *testing.T) {
	mockey.PatchConvey("default bin not exists", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test", nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(os.Executable).To(func() (string, error) {
			return "/usr/bin/gpud", nil
		}).Build()
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.DefaultBinExists).To(func() bool { return false }).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gpud binary not found")
	})
}

// TestCommand_SystemdInitError tests the command when systemd init fails.
func TestCommand_SystemdInitError(t *testing.T) {
	mockey.PatchConvey("systemd init error", t, func() {
		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test", nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(os.Executable).To(func() (string, error) {
			return "/usr/bin/gpud", nil
		}).Build()
		mockey.Mock(pkdsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.DefaultBinExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			return errors.New("failed to create env file")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create env file")
	})
}

// TestCommand_EnableSystemdUnitError tests the command when enabling systemd unit fails.
func TestCommand_EnableSystemdUnitError(t *testing.T) {
	mockey.PatchConvey("enable systemd unit error", t, func() {
		tmpDir := t.TempDir()

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
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			return nil
		}).Build()
		mockey.Mock(pkgupdate.EnableGPUdSystemdUnit).To(func() error {
			return errors.New("failed to enable systemd unit")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to enable systemd unit")
	})
}

// TestCommand_RestartSystemdUnitError tests the command when restarting systemd unit fails.
func TestCommand_RestartSystemdUnitError(t *testing.T) {
	mockey.PatchConvey("restart systemd unit error", t, func() {
		tmpDir := t.TempDir()

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
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			return nil
		}).Build()
		mockey.Mock(pkgupdate.EnableGPUdSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.RestartGPUdSystemdUnit).To(func() error {
			return errors.New("failed to restart systemd unit")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to restart systemd unit")
	})
}

// TestCommand_SuccessWithoutToken tests successful execution without token.
func TestCommand_SuccessWithoutToken(t *testing.T) {
	mockey.PatchConvey("success without token", t, func() {
		tmpDir := t.TempDir()

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

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_SuccessWithToken tests successful execution with token.
func TestCommand_SuccessWithToken(t *testing.T) {
	mockey.PatchConvey("success with token", t, func() {
		tmpDir := t.TempDir()
		stateFile := tmpDir + "/gpud.state"

		// Create a real database for the state file
		realDB, err := sqlite.Open(stateFile)
		require.NoError(t, err)
		t.Cleanup(func() { _ = realDB.Close() })

		loginCalled := false

		mockey.Mock(common.ResolveDataDir).To(func(cliContext *cli.Context) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			loginCalled = true
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

		cliContext := newCLIContext(t, []string{"--token", "test-token"})
		err = Command(cliContext)
		require.NoError(t, err)
		assert.True(t, loginCalled, "expected login to be called")
	})
}

// TestRecordLoginSuccessState_ResolveDataDirError tests recordLoginSuccessState when resolving data dir fails.
func TestRecordLoginSuccessState_ResolveDataDirError(t *testing.T) {
	mockey.PatchConvey("recordLoginSuccessState resolve error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "", errors.New("failed to resolve")
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, "/tmp/test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve data dir")
	})
}

// TestRecordLoginSuccessState_OpenStateFileError tests recordLoginSuccessState when opening state file fails.
func TestRecordLoginSuccessState_OpenStateFileError(t *testing.T) {
	mockey.PatchConvey("recordLoginSuccessState open error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "/tmp/test", nil
		}).Build()
		mockey.Mock(config.StateFilePath).To(func(dataDir string) string {
			return "/tmp/test/gpud.state"
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("failed to open database")
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, "/tmp/test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

// TestRecordLoginSuccessState_CreateTableError tests recordLoginSuccessState when creating table fails.
func TestRecordLoginSuccessState_CreateTableError(t *testing.T) {
	mockey.PatchConvey("recordLoginSuccessState create table error", t, func() {
		tmpDir := t.TempDir()
		stateFile := tmpDir + "/gpud.state"

		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(config.StateFilePath).To(func(dataDir string) string {
			return stateFile
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

// TestRecordLoginSuccessState_InsertError tests recordLoginSuccessState when inserting fails.
func TestRecordLoginSuccessState_InsertError(t *testing.T) {
	mockey.PatchConvey("recordLoginSuccessState insert error", t, func() {
		tmpDir := t.TempDir()
		stateFile := tmpDir + "/gpud.state"

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
			return errors.New("failed to insert")
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to record login success state")
	})
}

// TestRecordLoginSuccessState_Success tests successful recordLoginSuccessState.
func TestRecordLoginSuccessState_Success(t *testing.T) {
	mockey.PatchConvey("recordLoginSuccessState success", t, func() {
		tmpDir := t.TempDir()
		stateFile := tmpDir + "/gpud.state"

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

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		require.NoError(t, err)
	})
}

// TestSystemdInit_CreateEnvFileError tests systemdInit when creating env file fails.
func TestSystemdInit_CreateEnvFileError(t *testing.T) {
	mockey.PatchConvey("systemdInit create env file error", t, func() {
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			return errors.New("failed to create env file")
		}).Build()

		err := systemdInit("http://localhost", "/tmp", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create env file")
	})
}

// TestSystemdInit_WriteFileError tests systemdInit when writing unit file fails.
func TestSystemdInit_WriteFileError(t *testing.T) {
	mockey.PatchConvey("systemdInit write file error", t, func() {
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			return errors.New("failed to write file")
		}).Build()

		err := systemdInit("http://localhost", "/tmp", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write file")
	})
}

// TestSystemdInit_Success tests successful systemdInit.
func TestSystemdInit_Success(t *testing.T) {
	mockey.PatchConvey("systemdInit success", t, func() {
		mockey.Mock(systemd.CreateDefaultEnvFile).To(func(endpoint, dataDir string, dbInMemory bool) error {
			return nil
		}).Build()
		mockey.Mock(systemd.GPUdServiceUnitFileContents).To(func() string {
			return "[Unit]\nDescription=GPUd"
		}).Build()
		mockey.Mock(os.WriteFile).To(func(name string, data []byte, perm os.FileMode) error {
			return nil
		}).Build()

		err := systemdInit("http://localhost", "/tmp", false)
		require.NoError(t, err)
	})
}
