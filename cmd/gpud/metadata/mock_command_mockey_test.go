package metadata

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/osutil"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// newCLIContext creates a CLI context for testing with the given arguments.
func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-metadata-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "", "")
	_ = flags.String("data-dir", "", "")
	_ = flags.Bool("reboot-history", false, "")
	_ = flags.String("set-key", "", "")
	_ = flags.String("set-value", "", "")

	require.NoError(t, flags.Parse(args))

	return cli.NewContext(app, flags, nil)
}

// createMetadataDB creates a test database with metadata.
func createMetadataDB(t *testing.T, stateFile string, metadata map[string]string) {
	t.Helper()

	dbRW, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	defer func() { _ = dbRW.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))

	for k, v := range metadata {
		require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, k, v))
	}
}

// insertRebootEvent inserts a test reboot event.
func insertRebootEvent(t *testing.T, stateFile string, eventTime time.Time, message string) {
	t.Helper()

	dbRW, err := sqlite.Open(stateFile)
	require.NoError(t, err)
	defer func() { _ = dbRW.Close() }()

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	require.NoError(t, err)
	defer func() { _ = dbRO.Close() }()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket("os", eventstore.WithDisablePurge())
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, bucket.Insert(ctx, eventstore.Event{
		Component: "os",
		Time:      eventTime,
		Name:      "reboot",
		Type:      "Warning",
		Message:   message,
	}))
}

// captureStdout captures stdout during function execution.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	done := make(chan string, 1)
	go func(reader *os.File) {
		b, _ := io.ReadAll(reader)
		done <- string(b)
	}(r)

	fn()

	require.NoError(t, w.Close())
	os.Stdout = oldStdout

	out := <-done
	require.NoError(t, r.Close())
	return out
}

// TestCommand_InvalidLogLevel tests the command with an invalid log level.
func TestCommand_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-metadata-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.String("data-dir", "", "")
		_ = flags.Bool("reboot-history", false, "")
		_ = flags.String("set-key", "", "")
		_ = flags.String("set-value", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := Command(cliContext)
		require.Error(t, err)
	})
}

// TestCommand_RequireRootError tests the command when not run as root.
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

// TestCommand_StateFileError tests the command when getting state file fails.
func TestCommand_StateFileError(t *testing.T) {
	mockey.PatchConvey("state file error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "", errors.New("failed to resolve state file")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get state file")
	})
}

// TestCommand_OpenStateFileError tests the command when opening state file fails.
func TestCommand_OpenStateFileError(t *testing.T) {
	mockey.PatchConvey("open state file error", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/path/gpud.state", nil
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		// sqlite.Open creates the file if it doesn't exist, so the error happens when reading
		assert.True(t, err != nil, "expected an error")
	})
}

// TestCommand_PrintsMetadataAndRebootHistory tests the command with valid metadata and reboot history.
func TestCommand_PrintsMetadataAndRebootHistory(t *testing.T) {
	mockey.PatchConvey("prints metadata and reboot history", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		dataDir := t.TempDir()
		stateFile := filepath.Join(dataDir, "gpud.state")

		rawToken := "nvapi-stg-1234567890abcdef"
		evTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
		evMessage := "test reboot event"

		createMetadataDB(t, stateFile, map[string]string{
			pkgmetadata.MetadataKeyMachineID: "machine-1",
			pkgmetadata.MetadataKeyToken:     rawToken,
		})
		insertRebootEvent(t, stateFile, evTime, evMessage)

		cliContext := newCLIContext(t, []string{
			"--data-dir", dataDir,
			"--reboot-history",
		})

		output := captureStdout(t, func() {
			require.NoError(t, Command(cliContext))
		})

		maskedToken := pkgmetadata.MaskToken(rawToken)
		assert.NotContains(t, output, rawToken)
		assert.Contains(t, output, "token: "+maskedToken)
		assert.Contains(t, output, "Reboot History:")
		assert.Contains(t, output, evTime.Format(time.RFC3339))
		assert.Contains(t, output, evMessage)
	})
}

// TestCommand_SetKeyValueUpdatesMetadata tests updating metadata with set-key and set-value.
func TestCommand_SetKeyValueUpdatesMetadata(t *testing.T) {
	mockey.PatchConvey("set key value updates metadata", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		dataDir := t.TempDir()
		stateFile := filepath.Join(dataDir, "gpud.state")

		rawToken := "nvapi-1234567890abcdef"
		setKey := "test_key"
		setValue := "test_value"

		createMetadataDB(t, stateFile, map[string]string{
			pkgmetadata.MetadataKeyToken: rawToken,
		})

		cliContext := newCLIContext(t, []string{
			"--data-dir", dataDir,
			"--set-key", setKey,
			"--set-value", setValue,
		})

		output := captureStdout(t, func() {
			require.NoError(t, Command(cliContext))
		})

		assert.Contains(t, output, "successfully updated metadata")

		dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
		require.NoError(t, err)
		defer func() { _ = dbRO.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		v, err := pkgmetadata.ReadMetadata(ctx, dbRO, setKey)
		require.NoError(t, err)
		require.Equal(t, setValue, v)
	})
}

// TestCommand_NoRebootHistory tests the command without --reboot-history flag.
func TestCommand_NoRebootHistory(t *testing.T) {
	mockey.PatchConvey("no reboot history", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		dataDir := t.TempDir()
		stateFile := filepath.Join(dataDir, "gpud.state")

		createMetadataDB(t, stateFile, map[string]string{
			pkgmetadata.MetadataKeyMachineID: "machine-1",
		})

		cliContext := newCLIContext(t, []string{
			"--data-dir", dataDir,
		})

		output := captureStdout(t, func() {
			require.NoError(t, Command(cliContext))
		})

		assert.Contains(t, output, "machine_id: machine-1")
		assert.NotContains(t, output, "Reboot History:")
	})
}

// TestCommand_EmptyRebootHistory tests the command when no reboot events exist.
func TestCommand_EmptyRebootHistory(t *testing.T) {
	mockey.PatchConvey("empty reboot history", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		dataDir := t.TempDir()
		stateFile := filepath.Join(dataDir, "gpud.state")

		createMetadataDB(t, stateFile, map[string]string{
			pkgmetadata.MetadataKeyMachineID: "machine-1",
		})

		cliContext := newCLIContext(t, []string{
			"--data-dir", dataDir,
			"--reboot-history",
		})

		output := captureStdout(t, func() {
			require.NoError(t, Command(cliContext))
		})

		assert.Contains(t, output, "Reboot History:")
		assert.Contains(t, output, "(no reboot events recorded)")
	})
}

// TestCommand_SetKeyOnlyNoValue tests that set-key without set-value is a no-op.
func TestCommand_SetKeyOnlyNoValue(t *testing.T) {
	mockey.PatchConvey("set key only no value", t, func() {
		mockey.Mock(osutil.RequireRoot).To(func() error {
			return nil
		}).Build()

		dataDir := t.TempDir()
		stateFile := filepath.Join(dataDir, "gpud.state")

		createMetadataDB(t, stateFile, map[string]string{
			pkgmetadata.MetadataKeyMachineID: "machine-1",
		})

		cliContext := newCLIContext(t, []string{
			"--data-dir", dataDir,
			"--set-key", "some_key",
			// no --set-value
		})

		output := captureStdout(t, func() {
			require.NoError(t, Command(cliContext))
		})

		// Should not contain "successfully updated" since set-value is empty
		assert.NotContains(t, output, "successfully updated metadata")
	})
}

// TestCommand_ValidLogLevels tests that valid log levels are accepted.
func TestCommand_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error", ""}

	for _, level := range validLevels {
		t.Run("level_"+level, func(t *testing.T) {
			mockey.PatchConvey("valid log level "+level, t, func() {
				mockey.Mock(osutil.RequireRoot).To(func() error {
					return nil
				}).Build()

				dataDir := t.TempDir()
				stateFile := filepath.Join(dataDir, "gpud.state")

				createMetadataDB(t, stateFile, map[string]string{
					pkgmetadata.MetadataKeyMachineID: "machine-1",
				})

				cliContext := newCLIContext(t, []string{
					"--data-dir", dataDir,
					"--log-level", level,
				})

				err := Command(cliContext)
				require.NoError(t, err)
			})
		})
	}
}
