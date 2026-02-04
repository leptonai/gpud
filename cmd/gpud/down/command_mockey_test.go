package down

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

	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/osutil"
	"github.com/leptonai/gpud/pkg/sqlite"
	pkgsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

// newCLIContext creates a CLI context for testing with the given arguments.
func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-down-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("data-dir", "", "")
	_ = flags.Bool("reset-state", false, "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// TestCommand_InvalidLogLevel tests the command with an invalid log level.
func TestCommand_InvalidLogLevel(t *testing.T) {
	cliContext := newCLIContext(t, []string{"--log-level", "invalid-level"})
	err := Command(cliContext)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognized level")
}

// TestCommand_RequireRootError tests that the command returns error when not run as root.
func TestCommand_RequireRootError(t *testing.T) {
	mockey.PatchConvey("require root error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return errors.New("must be run as root")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be run as root")
	})
}

// TestCommand_SystemctlNotExists tests the command when systemctl doesn't exist.
func TestCommand_SystemctlNotExists(t *testing.T) {
	mockey.PatchConvey("systemctl not exists", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return false }).Build()

		exitCalled := false
		exitCode := 0
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
			exitCode = code
			panic("os.Exit called")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		defer func() {
			if r := recover(); r != nil {
				// Expected panic from os.Exit
				_ = r
			}
		}()

		_ = Command(cliContext)

		assert.True(t, exitCalled, "expected os.Exit to be called")
		assert.Equal(t, 1, exitCode)
	})
}

// TestCommand_GpudNotRunning tests the command when gpud is not running.
func TestCommand_GpudNotRunning(t *testing.T) {
	mockey.PatchConvey("gpud not running", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) { return false, nil }).Build()

		exitCalled := false
		exitCode := 0
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
			exitCode = code
			panic("os.Exit called")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		defer func() {
			if r := recover(); r != nil {
				// Expected panic from os.Exit
				_ = r
			}
		}()

		_ = Command(cliContext)

		assert.True(t, exitCalled, "expected os.Exit to be called")
		assert.Equal(t, 0, exitCode)
	})
}

// TestCommand_IsActiveError tests the command when checking service status fails.
func TestCommand_IsActiveError(t *testing.T) {
	mockey.PatchConvey("isActive error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) {
			return false, errors.New("failed to check status")
		}).Build()

		exitCalled := false
		exitCode := 0
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
			exitCode = code
			panic("os.Exit called")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		defer func() {
			if r := recover(); r != nil {
				// Expected panic from os.Exit
				_ = r
			}
		}()

		_ = Command(cliContext)

		assert.True(t, exitCalled, "expected os.Exit to be called")
		assert.Equal(t, 1, exitCode)
	})
}

// TestCommand_StopSystemdUnitError tests the command when stopping the unit fails.
func TestCommand_StopSystemdUnitError(t *testing.T) {
	mockey.PatchConvey("stop systemd unit error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(pkgupdate.StopSystemdUnit).To(func() error {
			return errors.New("failed to stop unit")
		}).Build()

		exitCalled := false
		exitCode := 0
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
			exitCode = code
			panic("os.Exit called")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		defer func() {
			if r := recover(); r != nil {
				// Expected panic from os.Exit
				_ = r
			}
		}()

		_ = Command(cliContext)

		assert.True(t, exitCalled, "expected os.Exit to be called")
		assert.Equal(t, 1, exitCode)
	})
}

// TestCommand_DisableSystemdUnitError tests the command when disabling the unit fails.
func TestCommand_DisableSystemdUnitError(t *testing.T) {
	mockey.PatchConvey("disable systemd unit error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(pkgupdate.StopSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.DisableGPUdSystemdUnit).To(func() error {
			return errors.New("failed to disable unit")
		}).Build()

		exitCalled := false
		exitCode := 0
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
			exitCode = code
			panic("os.Exit called")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		defer func() {
			if r := recover(); r != nil {
				// Expected panic from os.Exit
				_ = r
			}
		}()

		_ = Command(cliContext)

		assert.True(t, exitCalled, "expected os.Exit to be called")
		assert.Equal(t, 1, exitCode)
	})
}

// TestCommand_SuccessWithoutResetState tests successful stop without reset-state flag.
func TestCommand_SuccessWithoutResetState(t *testing.T) {
	mockey.PatchConvey("success without reset-state", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(pkgupdate.StopSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.DisableGPUdSystemdUnit).To(func() error { return nil }).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_SuccessWithResetState tests successful stop with reset-state flag.
func TestCommand_SuccessWithResetState(t *testing.T) {
	// Create a real temporary database for testing
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("success with reset-state", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(pkgupdate.StopSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.DisableGPUdSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()

		deleteMetadataCalled := false
		mockey.Mock(pkgmetadata.DeleteAllMetadata).To(func(ctx context.Context, db *sql.DB) error {
			deleteMetadataCalled = true
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{"--reset-state"})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.True(t, deleteMetadataCalled, "expected DeleteAllMetadata to be called")
	})
}

// TestCommand_ResetStateGetStateFileError tests reset-state when getting state file fails.
func TestCommand_ResetStateGetStateFileError(t *testing.T) {
	mockey.PatchConvey("reset-state get state file error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(pkgupdate.StopSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.DisableGPUdSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "", errors.New("failed to resolve state file")
		}).Build()

		cliContext := newCLIContext(t, []string{"--reset-state"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get state file")
	})
}

// TestCommand_ResetStateOpenDBError tests reset-state when opening database fails.
func TestCommand_ResetStateOpenDBError(t *testing.T) {
	mockey.PatchConvey("reset-state open db error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(pkgupdate.StopSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.DisableGPUdSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("failed to open database")
		}).Build()

		cliContext := newCLIContext(t, []string{"--reset-state"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

// TestCommand_IsActiveVerifiesServiceName tests that the correct service name is passed to IsActive.
func TestCommand_IsActiveVerifiesServiceName(t *testing.T) {
	mockey.PatchConvey("isActive verifies service name", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()

		var receivedServiceName string
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) {
			receivedServiceName = service
			return true, nil
		}).Build()
		mockey.Mock(pkgupdate.StopSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.DisableGPUdSystemdUnit).To(func() error { return nil }).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.Equal(t, "gpud.service", receivedServiceName)
	})
}

// TestCommand_ResetStateDeleteMetadataError tests reset-state when deleting metadata fails.
func TestCommand_ResetStateDeleteMetadataError(t *testing.T) {
	// Create a real temporary database for testing
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/test.state"
	realDB, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("reset-state delete metadata error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error { return nil }).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(pkgsystemd.IsActive).To(func(service string) (bool, error) { return true, nil }).Build()
		mockey.Mock(pkgupdate.StopSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(pkgupdate.DisableGPUdSystemdUnit).To(func() error { return nil }).Build()
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()
		mockey.Mock(pkgmetadata.DeleteAllMetadata).To(func(ctx context.Context, db *sql.DB) error {
			return errors.New("failed to delete metadata")
		}).Build()

		cliContext := newCLIContext(t, []string{"--reset-state"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete metadata")
	})
}
