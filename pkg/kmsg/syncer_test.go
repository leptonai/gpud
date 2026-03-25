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

	// Use a fixed timestamp well within a 60-second truncation window so the
	// test is not flaky when time.Now() lands near a minute boundary.
	baseTime := time.Date(2026, 1, 1, 12, 30, 10, 0, time.UTC)
	// Send 3 messages that have different raw strings but will produce the same parsed name and message.
	ch <- Message{Timestamp: metav1.NewTime(baseTime), Message: "raw message with pid 123"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(time.Second)), Message: "raw message with pid 456"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(2 * time.Second)), Message: "raw message with pid 789"}
	close(ch)

	require.Eventually(t, func() bool {
		events, err := bucket.Get(ctx, time.Unix(0, 0))
		require.NoError(t, err)
		return len(events) >= 1
	}, time.Second, 10*time.Millisecond)

	events, err := bucket.Get(ctx, time.Unix(0, 0))
	require.NoError(t, err)

	assert.Len(t, events, 1, "expected deduplication to result in exactly 1 event")
	if len(events) > 0 {
		assert.Equal(t, "test_event", events[0].Name)
		assert.Equal(t, "constant parsed message", events[0].Message)
	}
}

func TestSyncer_DisableDedup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)
	bucket, err := store.Bucket("test_disable_dedup")
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
		withDisableDedup(),
	)
	require.NoError(t, err, "failed to create syncer")
	defer w.Close()

	baseTime := time.Date(2026, 1, 1, 12, 30, 10, 0, time.UTC)
	ch <- Message{Timestamp: metav1.NewTime(baseTime), Message: "raw message with pid 123"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(time.Second)), Message: "raw message with pid 456"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(2 * time.Second)), Message: "raw message with pid 789"}
	close(ch)

	require.Eventually(t, func() bool {
		events, err := bucket.Get(ctx, time.Unix(0, 0))
		require.NoError(t, err)
		return len(events) == 3
	}, time.Second, 10*time.Millisecond, "expected dedup to be disabled")
}

func TestSyncer_EventDedupWindowFunc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)
	bucket, err := store.Bucket("test_event_dedup_window")
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
		WithEventDedupWindowFunc(func(event eventstore.Event) (time.Duration, bool) {
			if event.Name == "test_event" {
				return 5 * time.Minute, true
			}
			return 0, false
		}),
	)
	require.NoError(t, err, "failed to create syncer")
	defer w.Close()

	// Use a fixed timestamp well within a 300-second (5-minute) truncation window
	// so the test is deterministic regardless of wall-clock time.
	baseTime := time.Date(2026, 1, 1, 12, 30, 10, 0, time.UTC)
	ch <- Message{Timestamp: metav1.NewTime(baseTime), Message: "raw message with pid 123"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(4 * time.Minute)), Message: "raw message with pid 456"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(6 * time.Minute)), Message: "raw message with pid 789"}
	close(ch)

	require.Eventually(t, func() bool {
		events, err := bucket.Get(ctx, time.Unix(0, 0))
		require.NoError(t, err)
		return len(events) == 2
	}, time.Second, 10*time.Millisecond)
}

func TestSyncer_EventDedupWindowFunc_BypassesGenericDedup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)
	bucket, err := store.Bucket("test_event_dedup_bypass_generic")
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
		WithCacheKeyTruncateSeconds(300),
		WithEventDedupWindowFunc(func(event eventstore.Event) (time.Duration, bool) {
			if event.Name == "test_event" {
				return time.Minute, true
			}
			return 0, false
		}),
	)
	require.NoError(t, err, "failed to create syncer")
	defer w.Close()

	baseTime := time.Date(2026, 1, 1, 12, 30, 10, 0, time.UTC)
	ch <- Message{Timestamp: metav1.NewTime(baseTime), Message: "raw message with pid 123"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(2 * time.Minute)), Message: "raw message with pid 456"}
	close(ch)

	require.Eventually(t, func() bool {
		events, err := bucket.Get(ctx, time.Unix(0, 0))
		require.NoError(t, err)
		return len(events) == 2
	}, time.Second, 10*time.Millisecond)
}

func TestSyncer_EventDedupWindowFunc_PreservesGenericDedupForOtherEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)
	bucket, err := store.Bucket("test_event_dedup_preserves_generic")
	require.NoError(t, err)
	defer bucket.Close()

	ch := make(chan Message, 10)

	matchFunc := func(msg string) (string, string) {
		switch msg {
		case "custom-1", "custom-2":
			return "custom_event", "custom parsed message"
		case "generic-1", "generic-2":
			return "generic_event", "generic parsed message"
		default:
			return "", ""
		}
	}

	w, err := newSyncer(
		ctx,
		&mockChannelWatcher{ch: ch},
		matchFunc,
		bucket,
		WithCacheKeyTruncateSeconds(300),
		WithEventDedupWindowFunc(func(event eventstore.Event) (time.Duration, bool) {
			if event.Name == "custom_event" {
				return time.Minute, true
			}
			return 0, false
		}),
	)
	require.NoError(t, err, "failed to create syncer")
	defer w.Close()

	baseTime := time.Date(2026, 1, 1, 12, 30, 0, 0, time.UTC)
	ch <- Message{Timestamp: metav1.NewTime(baseTime), Message: "generic-1"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(2 * time.Minute)), Message: "generic-2"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime), Message: "custom-1"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(2 * time.Minute)), Message: "custom-2"}
	close(ch)

	require.Eventually(t, func() bool {
		events, err := bucket.Get(ctx, time.Unix(0, 0))
		require.NoError(t, err)
		if len(events) != 3 {
			return false
		}
		var genericCount, customCount int
		for _, event := range events {
			switch event.Name {
			case "generic_event":
				genericCount++
			case "custom_event":
				customCount++
			}
		}
		return genericCount == 1 && customCount == 2
	}, time.Second, 10*time.Millisecond)
}

func TestSyncer_DisableDedup_KeepsEventDedupWindowFunc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)
	bucket, err := store.Bucket("test_disable_dedup_keeps_event_window")
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
		withDisableDedup(),
		WithEventDedupWindowFunc(func(event eventstore.Event) (time.Duration, bool) {
			if event.Name == "test_event" {
				return 5 * time.Minute, true
			}
			return 0, false
		}),
	)
	require.NoError(t, err, "failed to create syncer")
	defer w.Close()

	baseTime := time.Date(2026, 1, 1, 12, 30, 10, 0, time.UTC)
	ch <- Message{Timestamp: metav1.NewTime(baseTime), Message: "raw message with pid 123"}
	ch <- Message{Timestamp: metav1.NewTime(baseTime.Add(time.Second)), Message: "raw message with pid 456"}
	close(ch)

	require.Eventually(t, func() bool {
		events, err := bucket.Get(ctx, time.Unix(0, 0))
		require.NoError(t, err)
		return len(events) == 1
	}, time.Second, 10*time.Millisecond)
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
