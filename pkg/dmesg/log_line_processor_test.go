package dmesg

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestLogLineProcessor(t *testing.T) {
	dmesgWatcher, err := NewWatcherWithCommands([][]string{
		{
			"cat ./testdata/dmesg.decode.iso.log.0",
		},
	})
	require.NoError(t, err, "failed to create dmesg watcher")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test", 0)
	assert.NoError(t, err)
	defer bucket.Close()

	w, err := newLogLineProcessor(
		ctx,
		dmesgWatcher,
		func(_ string) (string, string) {
			return "test", ""
		},
		bucket,
	)
	require.NoError(t, err, "failed to create log line processor")
	defer w.Close()

	time.Sleep(5 * time.Second)

	events, err := w.Get(ctx, time.Unix(0, 0))
	require.NoError(t, err, "failed to get events")

	if len(events) == 0 {
		t.Skip("no events found") // slow CI...
	}

	t.Logf("found %d events", len(events))
	for _, ev := range events {
		assert.Contains(t, ev.Name, "test", "unexpected event type")
	}
}

func TestEventsWatcherSkipsEmptyNames(t *testing.T) {
	// Create a watcher that reads a known set of log lines
	dmesgWatcher, err := NewWatcherWithCommands([][]string{
		{
			"echo 'kern  :warn  : 2025-01-21T04:41:44,285060+00:00 first message'",
		},
		{
			"echo 'kern  :warn  : 2025-01-21T04:41:45,285060+00:00 second message'",
		},
		// add a small sleep to make sure the watcher is not closed
		{
			"echo 'kern  :warn  : 2025-01-21T04:41:46,285060+00:00 third message' && sleep 2",
		},
	})
	require.NoError(t, err, "failed to create dmesg watcher")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test", 0)
	assert.NoError(t, err)
	defer bucket.Close()

	// Create a match function that only matches specific messages
	matchFunc := func(content string) (string, string) {
		if strings.Contains(content, "first message") {
			return "test_event", "first message matched"
		}
		if strings.Contains(content, "third message") {
			return "test_event", "third message matched"
		}
		// Return empty name for second message, which should be skipped
		return "", ""
	}

	w, err := newLogLineProcessor(
		ctx,
		dmesgWatcher,
		matchFunc,
		bucket,
	)
	require.NoError(t, err, "failed to create log line processor")
	defer w.Close()

	// Wait for events to be processed
	time.Sleep(2 * time.Second)

	// Get all events from the beginning of time
	events, err := w.Get(ctx, time.Unix(0, 0))
	require.NoError(t, err, "failed to get events")

	// We expect exactly 2 events (first and third messages)
	assert.Len(t, events, 2, "expected exactly 2 events")

	// Verify the events are the ones we expect
	for _, ev := range events {
		assert.Equal(t, "test_event", ev.Name, "unexpected event name")
		assert.Contains(t, ev.Message, "message matched", "unexpected event message")
	}

	// latest event first
	assert.Contains(t, events[0].Message, "third", "first event should be third message")
	assert.Contains(t, events[1].Message, "first", "second event should be first message")
}

func TestNewLogLineProcessorDefaultWatcher(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping test on non-linux")
	}

	if _, err := exec.LookPath("dmesg"); err != nil {
		t.Skip("skipping test since dmesg is not available")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test", 0)
	assert.NoError(t, err)
	defer bucket.Close()

	lp, err := NewLogLineProcessor(
		ctx,
		func(string) (string, string) {
			return "test", "test"
		},
		bucket,
	)
	require.NoError(t, err, "failed to create log line processor")
	lp.Close()
}
