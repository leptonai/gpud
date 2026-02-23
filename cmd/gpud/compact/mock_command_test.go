package compact

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/netutil"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
)

func TestCommand_ReturnsErrorForInvalidLogLevel(t *testing.T) {
	cliContext := newCLIContext(t, []string{
		"--log-level", "not-a-log-level",
	})

	err := Command(cliContext)
	require.Error(t, err)
}

func TestCommand_ReturnsErrorWhenSystemdActive(t *testing.T) {
	mockey.PatchConvey("systemd active", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.IsActive).To(func(string) (bool, error) { return true, nil }).Build()

		cliContext := newCLIContext(t, []string{
			"--log-level", "info",
		})

		err := Command(cliContext)
		require.Error(t, err)
		assert.Equal(t, "gpud is running (must be stopped before running compact)", err.Error())
	})
}

func TestCommand_ReturnsErrorWhenSystemdActiveCheckFails(t *testing.T) {
	mockey.PatchConvey("systemd active check fails", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return true }).Build()
		mockey.Mock(systemd.IsActive).To(func(string) (bool, error) { return false, errors.New("boom") }).Build()

		cliContext := newCLIContext(t, []string{
			"--log-level", "info",
		})

		err := Command(cliContext)
		require.Error(t, err)
		assert.Equal(t, "boom", err.Error())
	})
}

func TestCommand_ReturnsErrorWhenPortOpen(t *testing.T) {
	mockey.PatchConvey("port open", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return false }).Build()
		mockey.Mock(netutil.IsPortOpen).To(func(int) bool { return true }).Build()

		cliContext := newCLIContext(t, []string{
			"--log-level", "info",
		})

		err := Command(cliContext)
		require.Error(t, err)
		assert.Equal(t, fmt.Sprintf("gpud is running on port %d (must be stopped before running compact)", config.DefaultGPUdPort), err.Error())
	})
}

func TestCommand_ReturnsErrorWhenStateFileFromContextFails(t *testing.T) {
	mockey.PatchConvey("state file from context fails", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return false }).Build()
		mockey.Mock(netutil.IsPortOpen).To(func(int) bool { return false }).Build()
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(*cli.Context) (string, error) {
			return "", errors.New("failed to get state file")
		}).Build()

		cliContext := newCLIContext(t, []string{
			"--log-level", "info",
		})

		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get state file")
	})
}

func TestCommand_ReturnsErrorWhenSqliteOpenFails(t *testing.T) {
	mockey.PatchConvey("sqlite open fails", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return false }).Build()
		mockey.Mock(netutil.IsPortOpen).To(func(int) bool { return false }).Build()
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(*cli.Context) (string, error) {
			return "test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(string, ...sqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("failed to open db")
		}).Build()

		cliContext := newCLIContext(t, []string{
			"--log-level", "info",
		})

		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

func TestCommand_ReturnsErrorWhenReadDBSizeFails(t *testing.T) {
	mockey.PatchConvey("read db size fails", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return false }).Build()
		mockey.Mock(netutil.IsPortOpen).To(func(int) bool { return false }).Build()
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(*cli.Context) (string, error) {
			return "test.state", nil
		}).Build()

		dbRW, err := sql.Open("sqlite3", ":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = dbRW.Close() })

		dbRO, err := sql.Open("sqlite3", ":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = dbRO.Close() })

		openCount := 0
		mockey.Mock(sqlite.Open).To(func(string, ...sqlite.OpOption) (*sql.DB, error) {
			openCount++
			if openCount == 1 {
				return dbRW, nil
			}
			return dbRO, nil
		}).Build()

		mockey.Mock(sqlite.ReadDBSize).To(func(_ context.Context, _ *sql.DB) (uint64, error) {
			return 0, errors.New("failed to read db size")
		}).Build()

		cliContext := newCLIContext(t, []string{
			"--log-level", "info",
		})

		err = Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read state file size")
	})
}

func TestCommand_ReturnsErrorWhenCompactFails(t *testing.T) {
	mockey.PatchConvey("compact fails", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return false }).Build()
		mockey.Mock(netutil.IsPortOpen).To(func(int) bool { return false }).Build()
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(*cli.Context) (string, error) {
			return "test.state", nil
		}).Build()

		dbRW, err := sql.Open("sqlite3", ":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = dbRW.Close() })

		dbRO, err := sql.Open("sqlite3", ":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = dbRO.Close() })

		openCount := 0
		mockey.Mock(sqlite.Open).To(func(string, ...sqlite.OpOption) (*sql.DB, error) {
			openCount++
			if openCount == 1 {
				return dbRW, nil
			}
			return dbRO, nil
		}).Build()

		mockey.Mock(sqlite.ReadDBSize).To(func(_ context.Context, _ *sql.DB) (uint64, error) {
			return 100, nil
		}).Build()

		mockey.Mock(sqlite.Compact).To(func(_ context.Context, _ *sql.DB) error {
			return errors.New("failed to compact")
		}).Build()

		cliContext := newCLIContext(t, []string{
			"--log-level", "info",
		})

		err = Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to compact state file")
	})
}

func TestCommand_Success(t *testing.T) {
	mockey.PatchConvey("success", t, func() {
		mockey.Mock(systemd.SystemctlExists).To(func() bool { return false }).Build()
		mockey.Mock(netutil.IsPortOpen).To(func(int) bool { return false }).Build()
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(*cli.Context) (string, error) { return "test.state", nil }).Build()

		dbRW, err := sql.Open("sqlite3", ":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = dbRW.Close() })

		dbRO, err := sql.Open("sqlite3", ":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = dbRO.Close() })

		openCount := 0
		mockey.Mock(sqlite.Open).To(func(string, ...sqlite.OpOption) (*sql.DB, error) {
			openCount++
			if openCount == 1 {
				return dbRW, nil
			}
			return dbRO, nil
		}).Build()

		readSizes := []uint64{100, 50}
		readCount := 0
		mockey.Mock(sqlite.ReadDBSize).To(func(_ context.Context, _ *sql.DB) (uint64, error) {
			if readCount >= len(readSizes) {
				return 0, errors.New("unexpected ReadDBSize call")
			}
			size := readSizes[readCount]
			readCount++
			return size, nil
		}).Build()

		compactCalls := 0
		mockey.Mock(sqlite.Compact).To(func(_ context.Context, gotDB *sql.DB) error {
			compactCalls++
			assert.Same(t, dbRW, gotDB)
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{
			"--log-level", "info",
		})

		output := captureStdout(t, func() {
			require.NoError(t, Command(cliContext))
		})

		assert.Equal(t, 2, openCount)
		assert.Equal(t, 2, readCount)
		assert.Equal(t, 1, compactCalls)
		assert.Contains(t, output, "successfully compacted state file")
	})
}

func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-compact-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "", "")
	_ = flags.String("data-dir", "", "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

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
