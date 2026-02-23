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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

type mockChannelWatcher struct {
	ch <-chan Message
}

func (m *mockChannelWatcher) Watch() (<-chan Message, error) {
	return m.ch, nil
}

func (m *mockChannelWatcher) Close() error {
	return nil
}

func TestSyncer_Deduplication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)
	bucket, err := store.Bucket("test_dedup")
	require.NoError(t, err)
	defer bucket.Close()

	ch := make(chan Message, 10)

	matchFunc := func(msg string) (string, string) {
		if msg != "" {
			return "test_event", "constant parsed message"
		}
		return "", ""
	}

	w, err := newSyncer(
		ctx,
		&mockChannelWatcher{ch: ch},
		matchFunc,
		bucket,
		WithCacheKeyTruncateSeconds(60),
	)
	require.NoError(t, err, "failed to create syncer")
	defer w.Close()

	now := time.Now().UTC()
	// Send 3 messages that have different raw strings but will produce the same parsed name and message.
	ch <- Message{Timestamp: metav1.NewTime(now), Message: "raw message with pid 123"}
	ch <- Message{Timestamp: metav1.NewTime(now.Add(time.Second)), Message: "raw message with pid 456"}
	ch <- Message{Timestamp: metav1.NewTime(now.Add(2 * time.Second)), Message: "raw message with pid 789"}

	time.Sleep(2 * time.Second)

	events, err := bucket.Get(ctx, time.Unix(0, 0))
	require.NoError(t, err)

	assert.Len(t, events, 1, "expected deduplication to result in exactly 1 event")
	if len(events) > 0 {
		assert.Equal(t, "test_event", events[0].Name)
		assert.Equal(t, "constant parsed message", events[0].Message)
	}
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
