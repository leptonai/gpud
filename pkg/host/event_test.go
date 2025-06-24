package host

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
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

		err = recordEvent(ctx, store, recentTime, mockLastReboot)
		assert.NoError(t, err)

		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, recentTime.Add(-1*time.Minute))
		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, EventNameReboot, events[0].Name)
		assert.Equal(t, string(apiv1.EventTypeWarning), events[0].Type)
		assert.Equal(t, recentTime.Unix(), events[0].Time.Unix())
	})

	// Test with an old reboot time (beyond retention)
	t.Run("old reboot should not record event", func(t *testing.T) {
		oldTime := time.Now().Add(-2 * eventstore.DefaultRetention)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return oldTime, nil
		}

		now := time.Now()
		err = recordEvent(ctx, store, now, mockLastReboot)
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

		now := time.Now()
		err = recordEvent(ctx, store, now, mockLastReboot)
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

		existingTime := events[0].Time

		// Use a fresh store to isolate this test
		dbRW2, dbRO2, cleanup2 := sqlite.OpenTestDB(t)
		defer cleanup2()
		isolatedStore, err := eventstore.New(dbRW2, dbRO2, eventstore.DefaultRetention)
		require.NoError(t, err)

		isolatedBucket, err := isolatedStore.Bucket("os")
		require.NoError(t, err)
		err = isolatedBucket.Insert(ctx, eventstore.Event{
			Time:    existingTime,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: fmt.Sprintf("system reboot detected %v", existingTime),
		})
		require.NoError(t, err)
		isolatedBucket.Close()

		// Try to record with same timestamp
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return existingTime, nil
		}

		now := time.Now()
		err = recordEvent(ctx, isolatedStore, now, mockLastReboot)
		assert.NoError(t, err)

		// Should still be only 1 event
		isolatedBucket, err = isolatedStore.Bucket("os")
		require.NoError(t, err)
		events, err = isolatedBucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	// Test with duplicate event with timestamp that's a few seconds different
	t.Run("event with timestamp less than a minute different should not be recorded", func(t *testing.T) {
		// Use a fresh store to isolate this test
		dbRW2, dbRO2, cleanup2 := sqlite.OpenTestDB(t)
		defer cleanup2()
		isolatedStore, err := eventstore.New(dbRW2, dbRO2, eventstore.DefaultRetention)
		require.NoError(t, err)

		baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

		// Insert first event
		isolatedBucket, err := isolatedStore.Bucket("os")
		require.NoError(t, err)
		err = isolatedBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: fmt.Sprintf("system reboot detected %v", baseTime),
		})
		require.NoError(t, err)
		isolatedBucket.Close()

		// Now try to record one 30 seconds later
		slightlyDifferentTime := baseTime.Add(30 * time.Second)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return slightlyDifferentTime, nil
		}

		now := time.Now()
		err = recordEvent(ctx, isolatedStore, now, mockLastReboot)
		assert.NoError(t, err)

		// Should still be only 1 event
		isolatedBucket, err = isolatedStore.Bucket("os")
		require.NoError(t, err)
		events, err := isolatedBucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	// Test with non-duplicate event with timestamp more than a minute different
	t.Run("event with timestamp more than a minute different should be recorded", func(t *testing.T) {
		// Create a separate database for this test
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// Create event store and bucket for this test
		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		// Fixed test time to keep comparisons deterministic
		now := time.Date(2025, 5, 21, 15, 0, 0, 0, time.UTC)

		// First event - should be recorded (at 1:00 PM)
		baseTime := time.Date(2025, 5, 21, 13, 0, 0, 0, time.UTC)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return baseTime, nil
		}

		err = recordEvent(ctx, store, now, mockLastReboot)
		assert.NoError(t, err)

		// Check first event was recorded
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		events, err := bucket.Get(ctx, time.Time{})
		require.NoError(t, err)
		require.Len(t, events, 1, "Should have exactly 1 event to start")
		bucket.Close()

		// Second event - should also be recorded (at 1:02 PM, more than a minute after first)
		laterTime := time.Date(2025, 5, 21, 13, 2, 0, 0, time.UTC)
		mockLastReboot2 := func(ctx context.Context) (time.Time, error) {
			return laterTime, nil
		}

		err = recordEvent(ctx, store, now, mockLastReboot2)
		assert.NoError(t, err)

		// Verify both events were recorded
		bucket, err = store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err = bucket.Get(ctx, time.Time{})
		require.NoError(t, err)
		assert.Len(t, events, 2, "Should have recorded both events")
	})
}

// Test to specifically test the scenario from the image where we have duplicate reboot events
func TestDuplicateRebootEvents(t *testing.T) {
	t.Parallel()

	t.Run("prevent duplicate timestamp reboot events", func(t *testing.T) {
		// Create a separate database for this test
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// Create event store and bucket for this test
		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		baseTime := time.Date(2025, 5, 21, 14, 56, 59, 0, time.UTC)
		now := time.Date(2025, 5, 21, 15, 0, 0, 0, time.UTC)

		// First event - should be recorded
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return baseTime, nil
		}
		err = recordEvent(ctx, store, now, mockLastReboot)
		assert.NoError(t, err)

		// Try to record same event again - should be skipped
		err = recordEvent(ctx, store, now, mockLastReboot)
		assert.NoError(t, err)

		// Check we only have one event
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	t.Run("handle multiple reboot events with image timestamps", func(t *testing.T) {
		// Create a separate database for this test
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// Create event store and bucket for this test
		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		timestamps := []time.Time{
			time.Date(2025, 5, 21, 5, 26, 28, 0, time.UTC),
			time.Date(2025, 5, 21, 14, 18, 59, 0, time.UTC),
			time.Date(2025, 5, 21, 14, 56, 59, 0, time.UTC),

			// Try recording the same timestamps again (simulating duplicate detection)
			time.Date(2025, 5, 21, 5, 26, 28, 0, time.UTC),
			time.Date(2025, 5, 21, 14, 18, 59, 0, time.UTC),
			time.Date(2025, 5, 21, 14, 56, 59, 0, time.UTC),
		}

		now := time.Date(2025, 5, 21, 15, 0, 0, 0, time.UTC)

		// Record each timestamp sequentially
		for _, ts := range timestamps {
			finalTs := ts // Capture for closure
			mockLastReboot := func(ctx context.Context) (time.Time, error) {
				return finalTs, nil
			}

			err = recordEvent(ctx, store, now, mockLastReboot)
			assert.NoError(t, err)
		}

		// Verify we have only 3 events (not 6)
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 3, "Should have 3 events, one for each unique timestamp")

		// Verify the events have the correct timestamps
		timeSet := make(map[int64]bool)
		for _, event := range events {
			timeSet[event.Time.Unix()] = true
		}

		// Check all three unique timestamps are present
		expectedTimestamps := []time.Time{
			time.Date(2025, 5, 21, 5, 26, 28, 0, time.UTC),
			time.Date(2025, 5, 21, 14, 18, 59, 0, time.UTC),
			time.Date(2025, 5, 21, 14, 56, 59, 0, time.UTC),
		}
		for _, ts := range expectedTimestamps {
			assert.True(t, timeSet[ts.Unix()], "Missing timestamp %v", ts)
		}
	})

	t.Run("timestamps just under one minute apart should be considered duplicates", func(t *testing.T) {
		// Create a separate database for this test
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// Create event store and bucket for this test
		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		baseTime := time.Date(2025, 5, 21, 15, 0, 0, 0, time.UTC)
		now := time.Date(2025, 5, 21, 15, 10, 0, 0, time.UTC)

		// Record first event
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return baseTime, nil
		}
		err = recordEvent(ctx, store, now, mockLastReboot)
		assert.NoError(t, err)

		// Try to record event 30 seconds later
		almostOneMinuteLater := baseTime.Add(30 * time.Second)
		mockLastReboot = func(ctx context.Context) (time.Time, error) {
			return almostOneMinuteLater, nil
		}
		err = recordEvent(ctx, store, now, mockLastReboot)
		assert.NoError(t, err)

		// Check we only have one event
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1, "Events under 1 minute apart should be considered duplicates")
	})

	t.Run("timestamps over one minute apart should be recorded as separate events", func(t *testing.T) {
		// Create a separate database for this test
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// Create event store and bucket for this test
		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		baseTime := time.Date(2025, 5, 21, 16, 0, 0, 0, time.UTC)
		now := time.Date(2025, 5, 21, 16, 10, 0, 0, time.UTC)

		// Record first event
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return baseTime, nil
		}
		err = recordEvent(ctx, store, now, mockLastReboot)
		assert.NoError(t, err)

		// Try to record event 61 seconds later
		overOneMinuteLater := baseTime.Add(61 * time.Second)
		mockLastReboot = func(ctx context.Context) (time.Time, error) {
			return overOneMinuteLater, nil
		}
		err = recordEvent(ctx, store, now, mockLastReboot)
		assert.NoError(t, err)

		// Check we have two events
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 2, "Events over 1 minute apart should be recorded separately")
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
	bucket, err := store.Bucket(EventBucketName)
	require.NoError(t, err)

	now := time.Now()
	events := []struct {
		time    time.Time
		name    string
		message string
	}{
		{now.Add(-5 * time.Hour), EventNameReboot, "system reboot detected 5h ago"},
		{now.Add(-3 * time.Hour), EventNameReboot, "system reboot detected 3h ago"},
		{now.Add(-1 * time.Hour), EventNameReboot, "system reboot detected 1h ago"},
	}

	for _, e := range events {
		err = bucket.Insert(ctx, eventstore.Event{
			Time:    e.time,
			Name:    e.name,
			Type:    string(apiv1.EventTypeWarning),
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
		assert.Equal(t, events[2].time.Unix(), retrievedEvents[0].Time.Unix())
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

		now := time.Now()
		err = recordEvent(ctx, store, now, mockLastReboot)
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

		err = recordEvent(canceledCtx, store, recentTime, mockLastReboot)
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
		assert.Equal(t, EventNameReboot, events[0].Name)
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
		testEvents := eventstore.Events{
			{
				Time:    now.Add(-2 * time.Hour),
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: "test event 1",
			},
			{
				Time:    now.Add(-1 * time.Hour),
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
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

		now := time.Now()
		err = recordEvent(timeoutCtx, store, now, mockLastReboot)
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

func TestGetRebootEventsWithOtherBuckets(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	recorder := NewRebootEventStore(store)

	now := time.Now()
	baseTime := now.Add(-4 * time.Hour)

	// Test with one other bucket containing events
	t.Run("get events with one other bucket", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert reboot events in the main "os" bucket
		osBucket, err = store.Bucket(EventBucketName)
		require.NoError(t, err)

		rebootEvents := []eventstore.Event{
			{
				Time:    baseTime.Add(-2 * time.Hour),
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    baseTime.Add(-1 * time.Hour),
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		for _, event := range rebootEvents {
			err = osBucket.Insert(ctx, event)
			require.NoError(t, err)
		}
		osBucket.Close()

		// Create another bucket and insert different events
		otherBucket, err := store.Bucket("disk")
		require.NoError(t, err)

		diskEvents := []eventstore.Event{
			{
				Time:    baseTime.Add(-3 * time.Hour),
				Name:    "disk_error",
				Type:    string(apiv1.EventTypeFatal),
				Message: "disk error detected",
			},
			{
				Time:    baseTime.Add(-30 * time.Minute),
				Name:    "disk_warning",
				Type:    string(apiv1.EventTypeWarning),
				Message: "disk space low",
			},
		}

		for _, event := range diskEvents {
			err = otherBucket.Insert(ctx, event)
			require.NoError(t, err)
		}

		// Get events with the other bucket
		events, err := recorder.GetRebootEvents(ctx, baseTime.Add(-5*time.Hour), otherBucket)
		assert.NoError(t, err)
		assert.Len(t, events, 4, "Should have 2 reboot events + 2 disk events")

		// Verify events are sorted by timestamp (descending)
		for i := 1; i < len(events); i++ {
			assert.True(t, events[i-1].Time.After(events[i].Time) || events[i-1].Time.Equal(events[i].Time),
				"Events should be sorted by timestamp descending")
		}

		// Verify we have both reboot and disk events
		rebootCount := 0
		diskCount := 0
		for _, event := range events {
			if event.Name == EventNameReboot {
				rebootCount++
			} else if event.Name == "disk_error" || event.Name == "disk_warning" {
				diskCount++
			}
		}
		assert.Equal(t, 2, rebootCount, "Should have 2 reboot events")
		assert.Equal(t, 2, diskCount, "Should have 2 disk events")

		otherBucket.Close()
	})

	// Test with multiple other buckets
	t.Run("get events with multiple other buckets", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert reboot event
		osBucket, err = store.Bucket(EventBucketName)
		require.NoError(t, err)
		err = osBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		})
		require.NoError(t, err)
		osBucket.Close()

		// Create first other bucket (memory)
		memoryBucket, err := store.Bucket("memory")
		require.NoError(t, err)
		err = memoryBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime.Add(-1 * time.Hour),
			Name:    "memory_pressure",
			Type:    string(apiv1.EventTypeWarning),
			Message: "high memory usage",
		})
		require.NoError(t, err)

		// Create second other bucket (network)
		networkBucket, err := store.Bucket("network")
		require.NoError(t, err)
		err = networkBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime.Add(-2 * time.Hour),
			Name:    "network_timeout",
			Type:    string(apiv1.EventTypeFatal),
			Message: "network timeout",
		})
		require.NoError(t, err)

		// Get events with multiple other buckets
		events, err := recorder.GetRebootEvents(ctx, baseTime.Add(-3*time.Hour), memoryBucket, networkBucket)
		assert.NoError(t, err)
		assert.Len(t, events, 3, "Should have 1 reboot + 1 memory + 1 network event")

		// Verify all event types are present
		eventNames := make(map[string]int)
		for _, event := range events {
			eventNames[event.Name]++
		}
		assert.Equal(t, 1, eventNames[EventNameReboot])
		assert.Equal(t, 1, eventNames["memory_pressure"])
		assert.Equal(t, 1, eventNames["network_timeout"])

		memoryBucket.Close()
		networkBucket.Close()
	})

	// Test with other buckets containing events outside the "since" time range
	t.Run("get events with other buckets outside since time", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert reboot event within range
		osBucket, err = store.Bucket(EventBucketName)
		require.NoError(t, err)
		err = osBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		})
		require.NoError(t, err)
		osBucket.Close()

		// Create other bucket with events both inside and outside range
		cpuBucket, err := store.Bucket("cpu")
		require.NoError(t, err)

		// Event within range
		err = cpuBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime.Add(-30 * time.Minute),
			Name:    "cpu_high",
			Type:    string(apiv1.EventTypeWarning),
			Message: "high CPU usage",
		})
		require.NoError(t, err)

		// Event outside range (too old)
		err = cpuBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime.Add(-5 * time.Hour),
			Name:    "cpu_old",
			Type:    string(apiv1.EventTypeInfo),
			Message: "old CPU event",
		})
		require.NoError(t, err)

		// Get events since 2 hours ago
		sinceTime := baseTime.Add(-2 * time.Hour)
		events, err := recorder.GetRebootEvents(ctx, sinceTime, cpuBucket)
		assert.NoError(t, err)
		assert.Len(t, events, 2, "Should have only events within the time range")

		// Verify only events within range are returned
		for _, event := range events {
			assert.True(t, event.Time.After(sinceTime) || event.Time.Equal(sinceTime),
				"All events should be after the since time")
			assert.NotEqual(t, "cpu_old", event.Name, "Old events should be filtered out")
		}

		cpuBucket.Close()
	})

	// Test with empty other buckets
	t.Run("get events with empty other buckets", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert reboot event
		osBucket, err = store.Bucket(EventBucketName)
		require.NoError(t, err)
		err = osBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		})
		require.NoError(t, err)
		osBucket.Close()

		// Create empty other bucket
		emptyBucket, err := store.Bucket("empty")
		require.NoError(t, err)

		// Get events with empty other bucket
		events, err := recorder.GetRebootEvents(ctx, baseTime.Add(-1*time.Hour), emptyBucket)
		assert.NoError(t, err)
		assert.Len(t, events, 1, "Should have only the reboot event")
		assert.Equal(t, EventNameReboot, events[0].Name)

		emptyBucket.Close()
	})

	// Test filtering of non-reboot events from main bucket
	t.Run("filter non-reboot events from main bucket", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert mixed events in the main "os" bucket
		osBucket, err = store.Bucket(EventBucketName)
		require.NoError(t, err)

		events := []eventstore.Event{
			{
				Time:    baseTime,
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    baseTime.Add(-1 * time.Hour),
				Name:    "kmsg_error", // This should be filtered out
				Type:    string(apiv1.EventTypeFatal),
				Message: "kernel message error",
			},
			{
				Time:    baseTime.Add(-30 * time.Minute),
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: "another reboot",
			},
		}

		for _, event := range events {
			err = osBucket.Insert(ctx, event)
			require.NoError(t, err)
		}
		osBucket.Close()

		// Create other bucket with events
		otherBucket, err := store.Bucket("other")
		require.NoError(t, err)
		err = otherBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime.Add(-45 * time.Minute),
			Name:    "other_event",
			Type:    string(apiv1.EventTypeInfo),
			Message: "other event",
		})
		require.NoError(t, err)

		// Get events
		retrievedEvents, err := recorder.GetRebootEvents(ctx, baseTime.Add(-2*time.Hour), otherBucket)
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, 3, "Should have 2 reboot events + 1 other event")

		// Verify only reboot events from main bucket and all events from other bucket
		rebootCount := 0
		otherCount := 0
		kmsgCount := 0
		for _, event := range retrievedEvents {
			if event.Name == EventNameReboot {
				rebootCount++
			} else if event.Name == "other_event" {
				otherCount++
			} else if event.Name == "kmsg_error" {
				kmsgCount++
			}
		}
		assert.Equal(t, 2, rebootCount, "Should have 2 reboot events")
		assert.Equal(t, 1, otherCount, "Should have 1 other event")
		assert.Equal(t, 0, kmsgCount, "Should not have kmsg events from main bucket")

		otherBucket.Close()
	})

	// Test proper sorting with events from multiple buckets
	t.Run("proper sorting with multiple buckets", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert reboot events with specific timestamps
		osBucket, err = store.Bucket(EventBucketName)
		require.NoError(t, err)

		rebootTimes := []time.Time{
			baseTime.Add(-3 * time.Hour), // oldest
			baseTime.Add(-1 * time.Hour), // newest reboot
		}

		for i, timestamp := range rebootTimes {
			err = osBucket.Insert(ctx, eventstore.Event{
				Time:    timestamp,
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: fmt.Sprintf("reboot %d", i),
			})
			require.NoError(t, err)
		}
		osBucket.Close()

		// Create other bucket with events at specific times
		otherBucket, err := store.Bucket("test")
		require.NoError(t, err)

		otherTimes := []time.Time{
			baseTime.Add(-2 * time.Hour),    // middle
			baseTime.Add(-30 * time.Minute), // newest overall
		}

		for i, timestamp := range otherTimes {
			err = otherBucket.Insert(ctx, eventstore.Event{
				Time:    timestamp,
				Name:    fmt.Sprintf("test_event_%d", i),
				Type:    string(apiv1.EventTypeInfo),
				Message: fmt.Sprintf("test event %d", i),
			})
			require.NoError(t, err)
		}

		// Get all events
		events, err := recorder.GetRebootEvents(ctx, baseTime.Add(-4*time.Hour), otherBucket)
		assert.NoError(t, err)
		assert.Len(t, events, 4, "Should have all 4 events")

		// Verify correct sorting (descending order by timestamp)
		expectedOrder := []time.Time{
			baseTime.Add(-30 * time.Minute), // newest
			baseTime.Add(-1 * time.Hour),
			baseTime.Add(-2 * time.Hour),
			baseTime.Add(-3 * time.Hour), // oldest
		}

		for i, expectedTime := range expectedOrder {
			assert.Equal(t, expectedTime.Unix(), events[i].Time.Unix(),
				"Event %d should have timestamp %v, got %v", i, expectedTime, events[i].Time)
		}

		otherBucket.Close()
	})
}
