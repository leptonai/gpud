package host

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordEvent(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create event store and bucket
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	// Test with a recent reboot time
	t.Run("recent reboot should record event", func(t *testing.T) {
		recentTime := time.Now().Add(-1 * time.Hour)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		}

		err = recordEvent(ctx, store, mockLastReboot)
		assert.NoError(t, err)

		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, recentTime.Add(-1*time.Minute))
		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "reboot", events[0].Name)
		assert.Equal(t, components.EventTypeWarning, events[0].Type)
		assert.Equal(t, recentTime.Unix(), events[0].Time.Unix())
	})

	// Test with an old reboot time (beyond retention)
	t.Run("old reboot should not record event", func(t *testing.T) {
		oldTime := time.Now().Add(-2 * eventstore.DefaultRetention)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return oldTime, nil
		}

		err = recordEvent(ctx, store, mockLastReboot)
		assert.NoError(t, err)

		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		// There should still only be 1 event (from the previous test)
		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	// Test with error from lastReboot
	t.Run("error getting reboot time", func(t *testing.T) {
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return time.Time{}, errors.New("uptime command failed")
		}

		err = recordEvent(ctx, store, mockLastReboot)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "uptime command failed")
	})

	// Test with duplicate event (same timestamp)
	t.Run("duplicate event should not be recorded", func(t *testing.T) {
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		// Get the existing event
		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		require.Len(t, events, 1)

		existingTime := events[0].Time.Time

		// Try to record with same timestamp
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return existingTime, nil
		}

		err = recordEvent(ctx, store, mockLastReboot)
		assert.NoError(t, err)

		// Should still be only 1 event
		events, err = bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})
}

func TestGetEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create event store
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	// Create a bucket and insert some test events
	bucket, err := store.Bucket(defaultBucketName)
	require.NoError(t, err)

	now := time.Now()
	events := []struct {
		time    time.Time
		name    string
		message string
	}{
		{now.Add(-5 * time.Hour), "reboot", "system reboot detected 5h ago"},
		{now.Add(-3 * time.Hour), "reboot", "system reboot detected 3h ago"},
		{now.Add(-1 * time.Hour), "reboot", "system reboot detected 1h ago"},
	}

	for _, e := range events {
		err = bucket.Insert(ctx, components.Event{
			Time:    metav1.Time{Time: e.time},
			Name:    e.name,
			Type:    components.EventTypeWarning,
			Message: e.message,
		})
		require.NoError(t, err)
	}
	bucket.Close()

	// Test getting all events
	t.Run("get all events", func(t *testing.T) {
		retrievedEvents, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, len(events))
	})

	// Test getting events since a specific time
	t.Run("get events since specific time", func(t *testing.T) {
		retrievedEvents, err := getEvents(ctx, store, now.Add(-2*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, 1)
		assert.Equal(t, events[2].time.Unix(), retrievedEvents[0].Time.Time.Unix())
	})
}

func TestRecordEventEdgeCases(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create event store
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	// Test with zero value time
	t.Run("zero time value", func(t *testing.T) {
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return time.Time{}, nil
		}

		err = recordEvent(ctx, store, mockLastReboot)
		assert.NoError(t, err)

		// Since time.Time{} is way in the past, it should be filtered by retention period
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 0)
	})

	// Test with canceled context
	t.Run("canceled context", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		recentTime := time.Now().Add(-1 * time.Hour)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		}

		err = recordEvent(canceledCtx, store, mockLastReboot)
		assert.Error(t, err)
	})
}

func TestOSEventStore(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create event store
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	// Test creating new event recorder
	t.Run("create new event recorder", func(t *testing.T) {
		recorder := NewRebootEventStore(store)
		assert.NotNil(t, recorder)
	})

	// Test RecordReboot method
	t.Run("record reboot through event store", func(t *testing.T) {
		// Clean up any existing events
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		_, err = bucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		bucket.Close()

		recentTime := time.Now().Add(-1 * time.Hour)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		}

		recorder := &rebootEventStore{
			getLastRebootTime: mockLastReboot,
			eventStore:        store,
		}

		err = recorder.RecordReboot(ctx)
		assert.NoError(t, err)

		// Verify event was recorded
		bucket, err = store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, recentTime.Add(-1*time.Minute))
		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "reboot", events[0].Name)
	})

	// Test GetRebootEvents method
	t.Run("get reboot events through event store", func(t *testing.T) {
		// Clean up any existing events
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		_, err = bucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		bucket.Close()

		recorder := &rebootEventStore{
			getLastRebootTime: func(ctx context.Context) (time.Time, error) {
				return time.Now(), nil
			},
			eventStore: store,
		}

		// Insert some test events
		bucket, err = store.Bucket("os")
		require.NoError(t, err)

		now := time.Now()
		testEvents := []components.Event{
			{
				Time:    metav1.Time{Time: now.Add(-2 * time.Hour)},
				Name:    "reboot",
				Type:    components.EventTypeWarning,
				Message: "test event 1",
			},
			{
				Time:    metav1.Time{Time: now.Add(-1 * time.Hour)},
				Name:    "reboot",
				Type:    components.EventTypeWarning,
				Message: "test event 2",
			},
		}

		for _, event := range testEvents {
			err = bucket.Insert(ctx, event)
			require.NoError(t, err)
		}
		bucket.Close()

		// Test getting all events
		events, err := recorder.GetRebootEvents(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, len(testEvents))

		// Test getting events since specific time
		events, err = recorder.GetRebootEvents(ctx, now.Add(-10*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, events, 2)
	})
}

func TestEventStoreInterface(t *testing.T) {
	t.Parallel()

	// Verify that osEventStore implements EventStore interface
	var _ RebootEventStore = &rebootEventStore{}
}

func TestRecordEventWithContextTimeout(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	// Test with context timeout
	t.Run("context timeout", func(t *testing.T) {
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
		defer cancel()

		// Wait for context to timeout
		time.Sleep(2 * time.Nanosecond)

		recentTime := time.Now().Add(-1 * time.Hour)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		}

		err = recordEvent(timeoutCtx, store, mockLastReboot)
		assert.Error(t, err)
	})
}

func TestGetEventsWithEmptyBucket(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	// Test getting events from empty bucket
	t.Run("empty bucket", func(t *testing.T) {
		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Empty(t, events)
	})
}
