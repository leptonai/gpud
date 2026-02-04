package status

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"io"
	"os"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	clientv1 "github.com/leptonai/gpud/client/v1"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
)

// newCLIContext creates a CLI context for testing with the given arguments.
func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-status-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("data-dir", "", "")
	_ = flags.Bool("watch", false, "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// TestCommand_StateFileFromContextError tests when getting state file fails.
func TestCommand_StateFileFromContextError(t *testing.T) {
	mockey.PatchConvey("state file from context error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "", errors.New("failed to get state file")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get state file")
	})
}

// TestCommand_SqliteOpenError tests when opening database fails.
func TestCommand_SqliteOpenError(t *testing.T) {
	mockey.PatchConvey("sqlite open error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("failed to open database")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

// TestCommand_ReadMachineIDError tests when reading machine ID fails.
func TestCommand_ReadMachineIDError(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	// Create a real database
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("read machine id error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			if key == pkgmetadata.MetadataKeyMachineID {
				return "", errors.New("failed to read machine id")
			}
			return "", nil
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read machine id")
	})
}

// TestCommand_ReadLoginSuccessError tests when reading login success fails.
func TestCommand_ReadLoginSuccessError(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("read login success error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()

		callCount := 0
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			callCount++
			if callCount == 1 { // First call is for machine ID
				return "test-machine-id", nil
			}
			return "", errors.New("failed to read login success")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read login success")
	})
}

// TestCommand_SystemdIsActiveError tests when checking systemd status fails.
func TestCommand_SystemdIsActiveError(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	// Create session states table
	_, err = realDB.Exec("CREATE TABLE session_states (id INTEGER PRIMARY KEY, timestamp INTEGER, success INTEGER, message TEXT)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("systemd is active error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil
		}).Build()
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, errors.New("failed to check systemd status")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check systemd status")
	})
}

// TestCommand_FindProcessByNameError tests when finding process fails.
func TestCommand_FindProcessByNameError(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	_, err = realDB.Exec("CREATE TABLE session_states (id INTEGER PRIMARY KEY, timestamp INTEGER, success INTEGER, message TEXT)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("find process by name error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil
		}).Build()
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return false }).Build()
		mockey.Mock(process.FindProcessByName).To(func(ctx context.Context, processName string) (process.ProcessStatus, error) {
			return nil, errors.New("failed to find process")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to find process")
	})
}

// TestCommand_ProcessNotRunning tests when gpud process is not found.
func TestCommand_ProcessNotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	_, err = realDB.Exec("CREATE TABLE session_states (id INTEGER PRIMARY KEY, timestamp INTEGER, success INTEGER, message TEXT)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("process not running", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil
		}).Build()
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return false }).Build()
		mockey.Mock(process.FindProcessByName).To(func(ctx context.Context, processName string) (process.ProcessStatus, error) {
			return nil, nil // No process found
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err) // Should return nil when process not found
	})
}

// TestCommand_BlockUntilServerReadyError tests when waiting for server fails.
func TestCommand_BlockUntilServerReadyError(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	_, err = realDB.Exec("CREATE TABLE session_states (id INTEGER PRIMARY KEY, timestamp INTEGER, success INTEGER, message TEXT)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("block until server ready error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil
		}).Build()
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(clientv1.BlockUntilServerReady).To(func(ctx context.Context, addr string, opts ...clientv1.OpOption) error {
			return errors.New("server not ready")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "server not ready")
	})
}

// TestCommand_GetPackageStatusError tests when getting package status fails.
func TestCommand_GetPackageStatusError(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	_, err = realDB.Exec("CREATE TABLE session_states (id INTEGER PRIMARY KEY, timestamp INTEGER, success INTEGER, message TEXT)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("get package status error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil
		}).Build()
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(clientv1.BlockUntilServerReady).To(func(ctx context.Context, addr string, opts ...clientv1.OpOption) error { return nil }).Build()
		mockey.Mock(clientv1.GetPackageStatus).To(func(ctx context.Context, url string, opts ...clientv1.OpOption) ([]packages.PackageStatus, error) {
			return nil, errors.New("failed to get package status")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get package status")
	})
}

// TestCommand_NoPackages tests when no packages are found.
func TestCommand_NoPackages(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	_, err = realDB.Exec("CREATE TABLE session_states (id INTEGER PRIMARY KEY, timestamp INTEGER, success INTEGER, message TEXT)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("no packages", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil
		}).Build()
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(clientv1.BlockUntilServerReady).To(func(ctx context.Context, addr string, opts ...clientv1.OpOption) error { return nil }).Build()
		mockey.Mock(clientv1.GetPackageStatus).To(func(ctx context.Context, url string, opts ...clientv1.OpOption) ([]packages.PackageStatus, error) {
			return []packages.PackageStatus{}, nil // Empty packages
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_SuccessWithPackages tests successful execution with packages.
func TestCommand_SuccessWithPackages(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	_, err = realDB.Exec("CREATE TABLE session_states (id INTEGER PRIMARY KEY, timestamp INTEGER, success INTEGER, message TEXT)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("success with packages", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil
		}).Build()
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(clientv1.BlockUntilServerReady).To(func(ctx context.Context, addr string, opts ...clientv1.OpOption) error { return nil }).Build()
		mockey.Mock(clientv1.GetPackageStatus).To(func(ctx context.Context, url string, opts ...clientv1.OpOption) ([]packages.PackageStatus, error) {
			return []packages.PackageStatus{
				{
					Name:       "test-package",
					ScriptPath: tmpDir + "/test.sh",
					TotalTime:  time.Minute,
					Progress:   50,
				},
			}, nil
		}).Build()

		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		t.Cleanup(func() { os.Stdout = oldStdout })

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		_ = w.Close()
		os.Stdout = oldStdout
		_, _ = io.ReadAll(r)

		require.NoError(t, err)
	})
}
