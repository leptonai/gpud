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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestCommand_ReturnsErrorForInvalidLogLevel(t *testing.T) {
	cliContext := newCLIContext(t, []string{
		"--log-level", "not-a-log-level",
	})

	err := Command(cliContext)
	require.Error(t, err)
}

func TestCommand_ReturnsErrorWhenSystemdActive(t *testing.T) {
	prevSystemctlExists := systemctlExists
	prevSystemdIsActive := systemdIsActive
	systemctlExists = func() bool { return true }
	systemdIsActive = func(string) (bool, error) { return true, nil }
	t.Cleanup(func() {
		systemctlExists = prevSystemctlExists
		systemdIsActive = prevSystemdIsActive
	})

	cliContext := newCLIContext(t, []string{
		"--log-level", "info",
	})

	err := Command(cliContext)
	require.Error(t, err)
	assert.Equal(t, "gpud is running (must be stopped before running compact)", err.Error())
}

func TestCommand_ReturnsErrorWhenSystemdActiveCheckFails(t *testing.T) {
	prevSystemctlExists := systemctlExists
	prevSystemdIsActive := systemdIsActive
	systemctlExists = func() bool { return true }
	systemdIsActive = func(string) (bool, error) { return false, errors.New("boom") }
	t.Cleanup(func() {
		systemctlExists = prevSystemctlExists
		systemdIsActive = prevSystemdIsActive
	})

	cliContext := newCLIContext(t, []string{
		"--log-level", "info",
	})

	err := Command(cliContext)
	require.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestCommand_ReturnsErrorWhenPortOpen(t *testing.T) {
	prevSystemctlExists := systemctlExists
	prevIsPortOpen := isPortOpen
	systemctlExists = func() bool { return false }
	isPortOpen = func(int) bool { return true }
	t.Cleanup(func() {
		systemctlExists = prevSystemctlExists
		isPortOpen = prevIsPortOpen
	})

	cliContext := newCLIContext(t, []string{
		"--log-level", "info",
	})

	err := Command(cliContext)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("gpud is running on port %d (must be stopped before running compact)", config.GPUdPortNumber()), err.Error())
}

func TestCommand_Success(t *testing.T) {
	prevSystemctlExists := systemctlExists
	prevIsPortOpen := isPortOpen
	prevStateFileFromContext := stateFileFromContext
	prevSQLiteOpen := sqliteOpen
	prevSQLiteReadDBSize := sqliteReadDBSize
	prevSQLiteCompact := sqliteCompactDatabase
	systemctlExists = func() bool { return false }
	isPortOpen = func(int) bool { return false }
	stateFileFromContext = func(*cli.Context) (string, error) { return "test.state", nil }

	dbRW, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = dbRW.Close() })

	dbRO, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = dbRO.Close() })

	openCount := 0
	sqliteOpen = func(string, ...sqlite.OpOption) (*sql.DB, error) {
		openCount++
		if openCount == 1 {
			return dbRW, nil
		}
		return dbRO, nil
	}

	readSizes := []uint64{100, 50}
	readCount := 0
	sqliteReadDBSize = func(_ context.Context, _ *sql.DB) (uint64, error) {
		if readCount >= len(readSizes) {
			return 0, errors.New("unexpected ReadDBSize call")
		}
		size := readSizes[readCount]
		readCount++
		return size, nil
	}

	compactCalls := 0
	sqliteCompactDatabase = func(_ context.Context, gotDB *sql.DB) error {
		compactCalls++
		assert.Same(t, dbRW, gotDB)
		return nil
	}

	t.Cleanup(func() {
		systemctlExists = prevSystemctlExists
		isPortOpen = prevIsPortOpen
		stateFileFromContext = prevStateFileFromContext
		sqliteOpen = prevSQLiteOpen
		sqliteReadDBSize = prevSQLiteReadDBSize
		sqliteCompactDatabase = prevSQLiteCompact
	})

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
