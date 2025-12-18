package metadata

import (
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/eventstore"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestCommand_PrintsMetadataAndRebootHistory(t *testing.T) {
	prevRequireRoot := requireRoot
	requireRoot = func() error { return nil }
	t.Cleanup(func() { requireRoot = prevRequireRoot })

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
}

func TestCommand_SetKeyValueUpdatesMetadata(t *testing.T) {
	prevRequireRoot := requireRoot
	requireRoot = func() error { return nil }
	t.Cleanup(func() { requireRoot = prevRequireRoot })

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
}

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
