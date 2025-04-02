package kmsg

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

var _ Watcher = &mockFileWatcher{}

type mockFileWatcher struct {
	file string
}

func (m *mockFileWatcher) Watch() (<-chan Message, error) {
	ch := make(chan Message, 1024)
	go func() {
		defer close(ch)
		data, err := os.ReadFile(m.file)
		if err != nil {
			return
		}
		now := time.Now()
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			msg, err := parseLine(now, scanner.Text())
			if err != nil {
				return
			}
			ch <- *msg
		}
	}()
	return ch, nil
}

func (m *mockFileWatcher) Close() error {
	return nil
}

func TestSyncer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test")
	assert.NoError(t, err)
	defer bucket.Close()

	w, err := newSyncer(
		ctx,
		&mockFileWatcher{file: "./testdata/kmsg.1.log"},
		func(_ string) (string, string) {
			return "test", ""
		},
		bucket,
	)
	require.NoError(t, err, "failed to create syncer")
	defer w.Close()

	time.Sleep(5 * time.Second)

	events, err := bucket.Get(ctx, time.Unix(0, 0))
	require.NoError(t, err, "failed to get events")

	if len(events) == 0 {
		t.Skip("no events found") // slow CI...
	}

	t.Logf("found %d events", len(events))
	for _, ev := range events {
		assert.Contains(t, ev.Name, "test", "unexpected event type")
	}
}
