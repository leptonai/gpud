package events

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// Helper function to create a reboot store with a mock lastRebootTime function
func createTestRebootStore(bucket eventstore.Bucket, mockLastRebootFunc func(context.Context) (time.Time, error)) *rebootsStore {
	return &rebootsStore{
		getTimeNowFunc:    func() time.Time { return time.Now().UTC() },
		getLastRebootTime: mockLastRebootFunc,
		bucket:            bucket,
	}
}

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

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		// Create reboot store with mock function
		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		})

		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		events, err := bucket.Get(ctx, recentTime.Add(-1*time.Minute))
		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, RebootEventName, events[0].Name)
		assert.Equal(t, string(apiv1.EventTypeWarning), events[0].Type)
		assert.Equal(t, recentTime.Unix(), events[0].Time.Unix())
	})

	// Test with an old reboot time (beyond retention)
	t.Run("old reboot should not record event", func(t *testing.T) {
		oldTime := time.Now().Add(-2 * eventstore.DefaultRetention)

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		// Create reboot store with mock function
		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return oldTime, nil
		})

		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// There should still only be 1 event (from the previous test)
		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	// Test with error from lastReboot
	t.Run("error getting reboot time", func(t *testing.T) {
		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		// Create reboot store with mock function that returns error
		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return time.Time{}, errors.New("uptime command failed")
		})

		err = rebootStore.Record(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "uptime command failed")
	})

	// Test with duplicate event (same timestamp)
	t.Run("duplicate event should not be recorded", func(t *testing.T) {
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
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

		isolatedBucket, err := isolatedStore.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		err = isolatedBucket.Insert(ctx, eventstore.Event{
			Time:    existingTime,
			Name:    RebootEventName,
			Type:    string(apiv1.EventTypeWarning),
			Message: fmt.Sprintf("system reboot detected %v", existingTime),
		})
		require.NoError(t, err)
		isolatedBucket.Close()

		// Create new bucket for the reboot store
		isolatedBucket2, err := isolatedStore.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer isolatedBucket2.Close()

		// Try to record with same timestamp
		rebootStore := createTestRebootStore(isolatedBucket2, func(ctx context.Context) (time.Time, error) {
			return existingTime, nil
		})

		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// Should still be only 1 event
		events, err = isolatedBucket2.Get(ctx, time.Time{})
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
		isolatedBucket, err := isolatedStore.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		err = isolatedBucket.Insert(ctx, eventstore.Event{
			Time:    baseTime,
			Name:    RebootEventName,
			Type:    string(apiv1.EventTypeWarning),
			Message: fmt.Sprintf("system reboot detected %v", baseTime),
		})
		require.NoError(t, err)
		isolatedBucket.Close()

		// Now try to record one 30 seconds later
		slightlyDifferentTime := baseTime.Add(30 * time.Second)

		// Create new bucket for the reboot store
		isolatedBucket2, err := isolatedStore.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer isolatedBucket2.Close()

		rebootStore := createTestRebootStore(isolatedBucket2, func(ctx context.Context) (time.Time, error) {
			return slightlyDifferentTime, nil
		})

		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// Should still be only 1 event
		events, err := isolatedBucket2.Get(ctx, time.Time{})
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

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		// Use recent times to avoid retention filtering
		now := time.Now()
		baseTime := now.Add(-2 * time.Hour) // 2 hours ago

		// First event - should be recorded
		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return baseTime, nil
		})

		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// Check first event was recorded
		events, err := bucket.Get(ctx, time.Time{})
		require.NoError(t, err)
		require.Len(t, events, 1, "Should have exactly 1 event to start")

		// Second event - should also be recorded (more than a minute after first)
		laterTime := baseTime.Add(2 * time.Minute) // 2 minutes after first event
		rebootStore2 := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return laterTime, nil
		})

		err = rebootStore2.Record(ctx)
		assert.NoError(t, err)

		// Verify both events were recorded
		events, err = bucket.Get(ctx, time.Time{})
		require.NoError(t, err)
		assert.Len(t, events, 2, "Should have recorded both events")
	})

	// Test with outdated reboot event (previous reboot happened after current reboot)
	t.Run("outdated reboot event should not be inserted", func(t *testing.T) {
		// Create a separate database for this test
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// Create event store
		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		// Use current time and relative offsets to avoid retention issues
		now := time.Now()

		// Insert a more recent reboot event first
		laterRebootTime := now.Add(-1 * time.Hour) // 1 hour ago

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return laterRebootTime, nil
		})
		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// Verify the event was recorded
		events, err := bucket.Get(ctx, time.Time{})
		require.NoError(t, err)
		require.Len(t, events, 1, "Should have 1 event initially")

		// Now try to insert an older reboot event
		earlierRebootTime := now.Add(-2 * time.Hour) // 2 hours ago (older than the first)
		rebootStore2 := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return earlierRebootTime, nil
		})
		err = rebootStore2.Record(ctx)
		assert.NoError(t, err) // Should succeed but not insert

		// Verify that the older event was not inserted

		events, err = bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1, "Should still have only 1 event")

		// Verify it's still the later event (more recent one)
		assert.Equal(t, laterRebootTime.Unix(), events[0].Time.Unix(), "Should keep the later reboot event")
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

		baseTime := time.Now().Add(-1 * time.Hour) // Use recent time

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		// First event - should be recorded
		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return baseTime, nil
		})
		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// Try to record same event again - should be skipped
		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// Check we only have one event

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

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		// Use recent times to avoid retention filtering
		now := time.Now()
		timestamps := []time.Time{
			now.Add(-5 * time.Hour),
			now.Add(-3 * time.Hour),
			now.Add(-1 * time.Hour),

			// Try recording the same timestamps again (simulating duplicate detection)
			now.Add(-5 * time.Hour),
			now.Add(-3 * time.Hour),
			now.Add(-1 * time.Hour),
		}

		// Record each timestamp sequentially
		for _, ts := range timestamps {
			finalTs := ts // Capture for closure
			rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
				return finalTs, nil
			})

			err = rebootStore.Record(ctx)
			assert.NoError(t, err)
		}

		// Verify we have only 3 events (not 6)

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
			now.Add(-5 * time.Hour),
			now.Add(-3 * time.Hour),
			now.Add(-1 * time.Hour),
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

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		baseTime := time.Now().Add(-1 * time.Hour) // Use recent time

		// Record first event
		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return baseTime, nil
		})
		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// Try to record event 30 seconds later
		almostOneMinuteLater := baseTime.Add(30 * time.Second)
		rebootStore2 := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return almostOneMinuteLater, nil
		})
		err = rebootStore2.Record(ctx)
		assert.NoError(t, err)

		// Check we only have one event
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

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		baseTime := time.Now().Add(-1 * time.Hour) // Use recent time

		// Record first event
		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return baseTime, nil
		})
		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// Try to record event 61 seconds later
		overOneMinuteLater := baseTime.Add(61 * time.Second)
		rebootStore2 := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return overOneMinuteLater, nil
		})
		err = rebootStore2.Record(ctx)
		assert.NoError(t, err)

		// Check we have two events
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
	bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
	require.NoError(t, err)

	now := time.Now()
	events := []struct {
		time    time.Time
		name    string
		message string
	}{
		{now.Add(-5 * time.Hour), RebootEventName, "system reboot detected 5h ago"},
		{now.Add(-3 * time.Hour), RebootEventName, "system reboot detected 3h ago"},
		{now.Add(-1 * time.Hour), RebootEventName, "system reboot detected 1h ago"},
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

	// Create reboot store for testing
	rebootStore := NewRebootsStore(bucket)

	// Test getting all events
	t.Run("get all events", func(t *testing.T) {
		retrievedEvents, err := rebootStore.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, len(events))
	})

	// Test getting events since a specific time
	t.Run("get events since specific time", func(t *testing.T) {
		retrievedEvents, err := rebootStore.Get(ctx, now.Add(-2*time.Hour))
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
		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return time.Time{}, nil
		})

		err = rebootStore.Record(ctx)
		assert.NoError(t, err)

		// Since time.Time{} is way in the past, it should be filtered by retention period
		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 0)
	})

	// Test with canceled context
	t.Run("canceled context", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		recentTime := time.Now().Add(-1 * time.Hour)
		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		})

		err = rebootStore.Record(canceledCtx)
		assert.Error(t, err)
	})
}

func TestRebootsStore(t *testing.T) {
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
		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		recorder := NewRebootsStore(bucket)
		assert.NotNil(t, recorder)
	})

	// Test Record method
	t.Run("record reboot through event store", func(t *testing.T) {
		// Clean up any existing events
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		_, err = bucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		bucket.Close()

		// Create new bucket for test
		bucket, err = store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		recentTime := time.Now().Add(-1 * time.Hour)
		recorder := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		})

		err = recorder.Record(ctx)
		assert.NoError(t, err)

		// Verify event was recorded
		events, err := bucket.Get(ctx, recentTime.Add(-1*time.Minute))
		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, RebootEventName, events[0].Name)
	})

	// Test GetRebootEvents method
	t.Run("get reboot events through event store", func(t *testing.T) {
		// Clean up any existing events
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		_, err = bucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		bucket.Close()

		// Create new bucket for test
		bucket, err = store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)

		recorder := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return time.Now(), nil
		})

		// Insert some test events

		now := time.Now()
		testEvents := eventstore.Events{
			{
				Time:    now.Add(-2 * time.Hour),
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "test event 1",
			},
			{
				Time:    now.Add(-1 * time.Hour),
				Name:    RebootEventName,
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
		events, err := recorder.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, len(testEvents))

		// Test getting events since specific time
		events, err = recorder.Get(ctx, now.Add(-10*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, events, 2)
	})
}

func TestRebootsStoreInterface(t *testing.T) {
	t.Parallel()

	// Verify that rebootsStore implements RebootsStore interface
	var _ RebootsStore = &rebootsStore{}
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

		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		recentTime := time.Now().Add(-1 * time.Hour)
		rebootStore := createTestRebootStore(bucket, func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		})

		err = rebootStore.Record(timeoutCtx)
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
		// Create bucket
		bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		defer bucket.Close()

		rebootStore := NewRebootsStore(bucket)
		events, err := rebootStore.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Empty(t, events)
	})
}

func TestGetRebootEventsFiltering(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	// Create bucket
	bucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
	require.NoError(t, err)
	defer bucket.Close()

	recorder := NewRebootsStore(bucket)

	now := time.Now()
	baseTime := now.Add(-4 * time.Hour)

	// Test that GetRebootEvents only returns reboot events from the os bucket
	t.Run("get only reboot events from os bucket", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert mixed events in the main "os" bucket
		osBucket, err = store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)

		events := []eventstore.Event{
			{
				Time:    baseTime.Add(-2 * time.Hour),
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    baseTime.Add(-1 * time.Hour),
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    baseTime.Add(-3 * time.Hour),
				Name:    "kmsg_error", // This should be filtered out
				Type:    string(apiv1.EventTypeFatal),
				Message: "kernel message error",
			},
			{
				Time:    baseTime.Add(-30 * time.Minute),
				Name:    "os_warning", // This should be filtered out
				Type:    string(apiv1.EventTypeWarning),
				Message: "OS warning",
			},
		}

		for _, event := range events {
			err = osBucket.Insert(ctx, event)
			require.NoError(t, err)
		}
		osBucket.Close()

		// Get events - should only return reboot events
		retrievedEvents, err := recorder.Get(ctx, baseTime.Add(-5*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, 2, "Should have 2 reboot events only")

		// Verify events are sorted by timestamp (descending)
		for i := 1; i < len(retrievedEvents); i++ {
			assert.True(t, retrievedEvents[i-1].Time.After(retrievedEvents[i].Time) || retrievedEvents[i-1].Time.Equal(retrievedEvents[i].Time),
				"Events should be sorted by timestamp descending")
		}

		// Verify all returned events are reboot events
		for _, event := range retrievedEvents {
			assert.Equal(t, RebootEventName, event.Name, "All returned events should be reboot events")
		}
	})

	// Test filtering by time range
	t.Run("get events filtered by time range", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert multiple reboot events with different timestamps
		osBucket, err = store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)

		rebootEvents := []eventstore.Event{
			{
				Time:    baseTime.Add(-5 * time.Hour), // too old, should be filtered out
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "old reboot",
			},
			{
				Time:    baseTime.Add(-2 * time.Hour), // within range
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "recent reboot 1",
			},
			{
				Time:    baseTime.Add(-1 * time.Hour), // within range
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "recent reboot 2",
			},
		}

		for _, event := range rebootEvents {
			err = osBucket.Insert(ctx, event)
			require.NoError(t, err)
		}
		osBucket.Close()

		// Get events since 3 hours ago - should only get 2 recent events
		events, err := recorder.Get(ctx, baseTime.Add(-3*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, events, 2, "Should have 2 recent reboot events")

		// Verify all events are within the time range
		sinceTime := baseTime.Add(-3 * time.Hour)
		for _, event := range events {
			assert.True(t, event.Time.After(sinceTime) || event.Time.Equal(sinceTime),
				"All events should be after the since time")
			assert.Equal(t, RebootEventName, event.Name)
		}
	})

	// Test with events outside the time range in os bucket
	t.Run("get events with time filtering in os bucket", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert mixed reboot and non-reboot events in os bucket
		osBucket, err = store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)

		events := []eventstore.Event{
			{
				Time:    baseTime,
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    baseTime.Add(-30 * time.Minute),
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "another reboot",
			},
			{
				Time:    baseTime.Add(-5 * time.Hour),
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "old reboot",
			},
			{
				Time:    baseTime.Add(-1 * time.Hour),
				Name:    "kmsg_error", // Non-reboot event, should be filtered
				Type:    string(apiv1.EventTypeFatal),
				Message: "kernel message error",
			},
		}

		for _, event := range events {
			err = osBucket.Insert(ctx, event)
			require.NoError(t, err)
		}
		osBucket.Close()

		// Get events since 2 hours ago
		sinceTime := baseTime.Add(-2 * time.Hour)
		retrievedEvents, err := recorder.Get(ctx, sinceTime)
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, 2, "Should have only reboot events within the time range")

		// Verify only reboot events within range are returned
		for _, event := range retrievedEvents {
			assert.True(t, event.Time.After(sinceTime) || event.Time.Equal(sinceTime),
				"All events should be after the since time")
			assert.Equal(t, RebootEventName, event.Name, "All events should be reboot events")
		}
	})

	// Test with empty os bucket
	t.Run("get events with empty os bucket", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Get events from empty bucket
		events, err := recorder.Get(ctx, baseTime.Add(-1*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, events, 0, "Should have no events from empty bucket")
	})

	// Test filtering of non-reboot events from os bucket
	t.Run("filter non-reboot events from os bucket", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert mixed events in the main "os" bucket
		osBucket, err = store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)

		events := []eventstore.Event{
			{
				Time:    baseTime,
				Name:    RebootEventName,
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
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: "another reboot",
			},
			{
				Time:    baseTime.Add(-45 * time.Minute),
				Name:    "os_warning", // This should be filtered out
				Type:    string(apiv1.EventTypeWarning),
				Message: "OS warning",
			},
		}

		for _, event := range events {
			err = osBucket.Insert(ctx, event)
			require.NoError(t, err)
		}
		osBucket.Close()

		// Get events
		retrievedEvents, err := recorder.Get(ctx, baseTime.Add(-2*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, 2, "Should have only 2 reboot events")

		// Verify only reboot events are returned
		for _, event := range retrievedEvents {
			assert.Equal(t, RebootEventName, event.Name, "All events should be reboot events")
		}
	})

	// Test proper sorting of reboot events
	t.Run("proper sorting of reboot events", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert reboot events with specific timestamps
		osBucket, err = store.Bucket(RebootBucketName, eventstore.WithDisablePurge())
		require.NoError(t, err)

		rebootTimes := []time.Time{
			baseTime.Add(-3 * time.Hour), // oldest
			baseTime.Add(-1 * time.Hour), // newest
			baseTime.Add(-2 * time.Hour), // middle
		}

		for i, timestamp := range rebootTimes {
			err = osBucket.Insert(ctx, eventstore.Event{
				Time:    timestamp,
				Name:    RebootEventName,
				Type:    string(apiv1.EventTypeWarning),
				Message: fmt.Sprintf("reboot %d", i),
			})
			require.NoError(t, err)
		}
		osBucket.Close()

		// Get all events
		events, err := recorder.Get(ctx, baseTime.Add(-4*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, events, 3, "Should have all 3 reboot events")

		// Verify correct sorting (descending order by timestamp)
		expectedOrder := []time.Time{
			baseTime.Add(-1 * time.Hour), // newest
			baseTime.Add(-2 * time.Hour), // middle
			baseTime.Add(-3 * time.Hour), // oldest
		}

		for i, expectedTime := range expectedOrder {
			assert.Equal(t, expectedTime.Unix(), events[i].Time.Unix(),
				"Event %d should have timestamp %v, got %v", i, expectedTime, events[i].Time)
			assert.Equal(t, RebootEventName, events[i].Name, "All events should be reboot events")
		}
	})
}
