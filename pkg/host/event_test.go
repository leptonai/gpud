package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
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

func TestRebootReasonSliceModificationBug(t *testing.T) {
	t.Parallel()

	// This test specifically demonstrates the bug where modifying lastEv
	// doesn't actually modify the event in the all slice
	t.Run("demonstrate slice element modification issue", func(t *testing.T) {
		// Simulate the bug with a simple example
		type Event struct {
			Message string
		}

		events := []Event{{Message: "original"}}

		// This is what the current code does (incorrect)
		lastEv := events[len(events)-1]
		lastEv.Message = "modified"

		// The event in the slice remains unchanged
		assert.Equal(t, "original", events[0].Message, "Modifying a copy doesn't change the slice element")

		// This is what should be done (correct)
		events[len(events)-1].Message = "modified"
		assert.Equal(t, "modified", events[0].Message, "Direct modification changes the slice element")
	})
}

func TestRecordReason(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	t.Run("record single reboot reason", func(t *testing.T) {
		reason := "CPU temperature too high"
		err := recordReason(ctx, store, time.Now(), reason)
		assert.NoError(t, err)

		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, EventNameRebootReason, events[0].Name)
		assert.Equal(t, reason, events[0].Message)
		assert.Equal(t, string(apiv1.EventTypeInfo), events[0].Type)
	})

	t.Run("prevent duplicate reboot reason", func(t *testing.T) {
		// Use a fresh store to isolate this test
		dbRW2, dbRO2, cleanup2 := sqlite.OpenTestDB(t)
		defer cleanup2()
		isolatedStore, err := eventstore.New(dbRW2, dbRO2, eventstore.DefaultRetention)
		require.NoError(t, err)

		now := time.Now()
		reason := "System overheating"

		// Record the same reason twice
		err = recordReason(ctx, isolatedStore, now, reason)
		assert.NoError(t, err)

		err = recordReason(ctx, isolatedStore, now, reason)
		assert.NoError(t, err)

		bucket, err := isolatedStore.Bucket(EventBucketName)
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1, "Should have only one event even after duplicate recording")
	})

	t.Run("record multiple different reboot reasons", func(t *testing.T) {
		// Use a fresh store to isolate this test
		dbRW3, dbRO3, cleanup3 := sqlite.OpenTestDB(t)
		defer cleanup3()
		isolatedStore, err := eventstore.New(dbRW3, dbRO3, eventstore.DefaultRetention)
		require.NoError(t, err)

		now := time.Now()
		reasons := []string{
			"CPU temperature too high",
			"Manual reboot requested",
			"Kernel panic detected",
		}

		for i, reason := range reasons {
			err = recordReason(ctx, isolatedStore, now.Add(time.Duration(i)*time.Second), reason)
			assert.NoError(t, err)
		}

		bucket, err := isolatedStore.Bucket(EventBucketName)
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, len(reasons))

		// Verify all reasons are recorded
		recordedReasons := make(map[string]bool)
		for _, event := range events {
			assert.Equal(t, EventNameRebootReason, event.Name)
			recordedReasons[event.Message] = true
		}

		for _, reason := range reasons {
			assert.True(t, recordedReasons[reason], "Reason '%s' should be recorded", reason)
		}
	})
}

func TestRebootReasonAppending(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	t.Run("append reboot reason to reboot event", func(t *testing.T) {
		// Clean up any existing events
		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = bucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		bucket.Close()

		now := time.Now()

		// Insert a reboot event and a reboot reason event
		bucket, err = store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert reboot event first
		err = bucket.Insert(ctx, eventstore.Event{
			Time:    now.Add(-1 * time.Hour),
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		})
		require.NoError(t, err)

		// Insert reboot reason event (with slightly earlier timestamp)
		err = bucket.Insert(ctx, eventstore.Event{
			Time:    now.Add(-1 * time.Hour).Add(-5 * time.Minute),
			Name:    EventNameRebootReason,
			Type:    string(apiv1.EventTypeInfo),
			Message: "CPU temperature too high",
		})
		require.NoError(t, err)
		bucket.Close()

		// Get events using getEvents function
		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1, "Should have only the reboot event")
		assert.Contains(t, events[0].Message, "system reboot detected")
		assert.Contains(t, events[0].Message, "(reboot reason: CPU temperature too high)")
	})

	t.Run("append multiple reboot reasons to same reboot event", func(t *testing.T) {
		// Use a fresh store to isolate this test
		dbRW2, dbRO2, cleanup2 := sqlite.OpenTestDB(t)
		defer cleanup2()
		isolatedStore, err := eventstore.New(dbRW2, dbRO2, eventstore.DefaultRetention)
		require.NoError(t, err)

		now := time.Now()

		// Insert events in the bucket
		bucket, err := isolatedStore.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert reboot event
		err = bucket.Insert(ctx, eventstore.Event{
			Time:    now,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		})
		require.NoError(t, err)

		// Insert multiple reboot reason events (all before the reboot event)
		reasons := []string{
			"CPU temperature too high",
			"Manual reboot requested",
			"Kernel panic detected",
		}

		for i, reason := range reasons {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    now.Add(-time.Duration(i+1) * time.Minute),
				Name:    EventNameRebootReason,
				Type:    string(apiv1.EventTypeInfo),
				Message: reason,
			})
			require.NoError(t, err)
		}
		bucket.Close()

		// Get events using getEvents function
		events, err := getEvents(ctx, isolatedStore, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1, "Should have only the reboot event")

		// Check that all reasons are appended
		for _, reason := range reasons {
			assert.Contains(t, events[0].Message, fmt.Sprintf("(reboot reason: %s)", reason))
		}
	})

	t.Run("no reboot reason appending when no reboot events", func(t *testing.T) {
		// Use a fresh store to isolate this test
		dbRW3, dbRO3, cleanup3 := sqlite.OpenTestDB(t)
		defer cleanup3()
		isolatedStore, err := eventstore.New(dbRW3, dbRO3, eventstore.DefaultRetention)
		require.NoError(t, err)

		// Insert only reboot reason events (no reboot events)
		bucket, err := isolatedStore.Bucket(EventBucketName)
		require.NoError(t, err)

		err = bucket.Insert(ctx, eventstore.Event{
			Time:    time.Now(),
			Name:    EventNameRebootReason,
			Type:    string(apiv1.EventTypeInfo),
			Message: "CPU temperature too high",
		})
		require.NoError(t, err)
		bucket.Close()

		// Get events using getEvents function
		events, err := getEvents(ctx, isolatedStore, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 0, "Should have no events since reboot reasons are filtered out without reboot events")
	})

	t.Run("prevent duplicate reboot reason appending", func(t *testing.T) {
		// Use a fresh store to isolate this test
		dbRW4, dbRO4, cleanup4 := sqlite.OpenTestDB(t)
		defer cleanup4()
		isolatedStore, err := eventstore.New(dbRW4, dbRO4, eventstore.DefaultRetention)
		require.NoError(t, err)

		now := time.Now()

		// Insert events in the bucket
		bucket, err := isolatedStore.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert reboot event
		err = bucket.Insert(ctx, eventstore.Event{
			Time:    now,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		})
		require.NoError(t, err)

		// Insert the same reboot reason twice
		reason := "CPU temperature too high"
		for i := 0; i < 2; i++ {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    now.Add(-time.Duration(i+1) * time.Minute),
				Name:    EventNameRebootReason,
				Type:    string(apiv1.EventTypeInfo),
				Message: reason,
			})
			require.NoError(t, err)
		}
		bucket.Close()

		// Get events using getEvents function
		events, err := getEvents(ctx, isolatedStore, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1, "Should have only the reboot event")

		// Count occurrences of the reason in the message
		reasonPattern := fmt.Sprintf("(reboot reason: %s)", reason)
		occurrences := strings.Count(events[0].Message, reasonPattern)
		assert.Equal(t, 1, occurrences, "Reboot reason should appear only once")
	})

	t.Run("append reasons to correct reboot events in descending order", func(t *testing.T) {
		// Use a fresh store to isolate this test
		dbRW5, dbRO5, cleanup5 := sqlite.OpenTestDB(t)
		defer cleanup5()
		isolatedStore, err := eventstore.New(dbRW5, dbRO5, eventstore.DefaultRetention)
		require.NoError(t, err)

		baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

		// Insert events in the bucket
		bucket, err := isolatedStore.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert multiple reboot events
		rebootTimes := []time.Time{
			baseTime,                     // Latest reboot
			baseTime.Add(-2 * time.Hour), // Middle reboot
			baseTime.Add(-4 * time.Hour), // Oldest reboot
		}

		for i, rebootTime := range rebootTimes {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    rebootTime,
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: fmt.Sprintf("system reboot detected %d", i),
			})
			require.NoError(t, err)
		}

		// Insert reboot reasons for each reboot (slightly before each reboot)
		reasons := []string{
			"Reason for latest reboot",
			"Reason for middle reboot",
			"Reason for oldest reboot",
		}

		for i, rebootTime := range rebootTimes {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    rebootTime.Add(-5 * time.Minute),
				Name:    EventNameRebootReason,
				Type:    string(apiv1.EventTypeInfo),
				Message: reasons[i],
			})
			require.NoError(t, err)
		}
		bucket.Close()

		// Get events using getEvents function
		events, err := getEvents(ctx, isolatedStore, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 3, "Should have all 3 reboot events")

		// Verify events are in descending order
		assert.True(t, events[0].Time.After(events[1].Time))
		assert.True(t, events[1].Time.After(events[2].Time))

		// Verify each reboot has the correct reason appended
		assert.Contains(t, events[0].Message, "system reboot detected 0")
		assert.Contains(t, events[0].Message, "(reboot reason: Reason for latest reboot)")

		assert.Contains(t, events[1].Message, "system reboot detected 1")
		assert.Contains(t, events[1].Message, "(reboot reason: Reason for middle reboot)")

		assert.Contains(t, events[2].Message, "system reboot detected 2")
		assert.Contains(t, events[2].Message, "(reboot reason: Reason for oldest reboot)")
	})
}

func TestEventsSortingOrder(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	t.Run("verify events are returned in descending order", func(t *testing.T) {
		// Clean up any existing events
		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = bucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		bucket.Close()

		// Insert events in random order
		baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		timestamps := []time.Time{
			baseTime.Add(-3 * time.Hour),
			baseTime,
			baseTime.Add(-1 * time.Hour),
			baseTime.Add(-5 * time.Hour),
			baseTime.Add(-2 * time.Hour),
		}

		bucket, err = store.Bucket(EventBucketName)
		require.NoError(t, err)

		for i, ts := range timestamps {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    ts,
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: fmt.Sprintf("reboot at index %d", i),
			})
			require.NoError(t, err)
		}
		bucket.Close()

		// Get events
		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, len(timestamps))

		// Verify descending order
		for i := 1; i < len(events); i++ {
			assert.True(t, events[i-1].Time.After(events[i].Time) || events[i-1].Time.Equal(events[i].Time),
				"Event at index %d (time: %v) should be after or equal to event at index %d (time: %v)",
				i-1, events[i-1].Time, i, events[i].Time)
		}

		// Verify the first event is the latest and last is the oldest
		assert.Equal(t, baseTime.Unix(), events[0].Time.Unix(), "First event should be the latest")
		assert.Equal(t, baseTime.Add(-5*time.Hour).Unix(), events[len(events)-1].Time.Unix(), "Last event should be the oldest")
	})

	t.Run("verify sorting with mixed event types", func(t *testing.T) {
		// Use a fresh store to isolate this test
		dbRW2, dbRO2, cleanup2 := sqlite.OpenTestDB(t)
		defer cleanup2()
		isolatedStore, err := eventstore.New(dbRW2, dbRO2, eventstore.DefaultRetention)
		require.NoError(t, err)

		baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

		// Insert mixed events
		bucket, err := isolatedStore.Bucket(EventBucketName)
		require.NoError(t, err)

		events := []struct {
			time    time.Time
			name    string
			message string
		}{
			{baseTime, EventNameReboot, "reboot 1"},
			{baseTime.Add(-10 * time.Minute), EventNameRebootReason, "reason 1"},
			{baseTime.Add(-2 * time.Hour), EventNameReboot, "reboot 2"},
			{baseTime.Add(-2 * time.Hour).Add(-10 * time.Minute), EventNameRebootReason, "reason 2"},
			{baseTime.Add(-1 * time.Hour), "other_event", "should be filtered"},
		}

		for _, ev := range events {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    ev.time,
				Name:    ev.name,
				Type:    string(apiv1.EventTypeWarning),
				Message: ev.message,
			})
			require.NoError(t, err)
		}
		bucket.Close()

		// Get events
		retrievedEvents, err := getEvents(ctx, isolatedStore, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, 2, "Should have only reboot events")

		// Verify descending order
		assert.True(t, retrievedEvents[0].Time.Equal(baseTime))
		assert.True(t, retrievedEvents[1].Time.Equal(baseTime.Add(-2*time.Hour)))

		// Verify reasons are appended correctly
		assert.Contains(t, retrievedEvents[0].Message, "(reboot reason: reason 1)")
		assert.Contains(t, retrievedEvents[1].Message, "(reboot reason: reason 2)")
	})
}

func TestRebootReasonAppendingBugFix(t *testing.T) {
	t.Parallel()

	// This test specifically tests that the reboot reason appending logic
	// correctly modifies the event in the slice, not just a copy
	t.Run("verify reboot reason is actually appended to event in slice", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		now := time.Now()

		// Insert events in specific order to test the appending logic
		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert a reboot event
		rebootEvent := eventstore.Event{
			Time:    now,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		}
		err = bucket.Insert(ctx, rebootEvent)
		require.NoError(t, err)

		// Insert a reboot reason event (slightly earlier)
		reasonEvent := eventstore.Event{
			Time:    now.Add(-5 * time.Minute),
			Name:    EventNameRebootReason,
			Type:    string(apiv1.EventTypeInfo),
			Message: "CPU overheating",
		}
		err = bucket.Insert(ctx, reasonEvent)
		require.NoError(t, err)
		bucket.Close()

		// Get events using getEvents function
		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1, "Should have only the reboot event")

		// IMPORTANT: This test will fail with the current implementation
		// because lastEv is a copy, not a reference to the actual event in the slice
		assert.Contains(t, events[0].Message, "(reboot reason: CPU overheating)",
			"The reboot reason should be appended to the actual event in the returned slice")
	})

	t.Run("edge case - empty all slice when processing reboot reason", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		// Insert only a reboot reason event (no reboot events)
		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		err = bucket.Insert(ctx, eventstore.Event{
			Time:    time.Now(),
			Name:    EventNameRebootReason,
			Type:    string(apiv1.EventTypeInfo),
			Message: "Power failure",
		})
		require.NoError(t, err)
		bucket.Close()

		// Get events - should handle empty all slice gracefully
		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 0, "Should return empty slice when no reboot events exist")
	})

	t.Run("verify events maintain descending order after processing", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

		// Insert events in bucket
		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert multiple reboot events in random order
		rebootTimes := []time.Time{
			baseTime.Add(-2 * time.Hour),
			baseTime,
			baseTime.Add(-4 * time.Hour),
			baseTime.Add(-1 * time.Hour),
		}

		for i, tm := range rebootTimes {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    tm,
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: fmt.Sprintf("reboot %d", i),
			})
			require.NoError(t, err)
		}

		// Add some reboot reasons
		err = bucket.Insert(ctx, eventstore.Event{
			Time:    baseTime.Add(-5 * time.Minute),
			Name:    EventNameRebootReason,
			Type:    string(apiv1.EventTypeInfo),
			Message: "Reason for latest",
		})
		require.NoError(t, err)

		err = bucket.Insert(ctx, eventstore.Event{
			Time:    baseTime.Add(-1 * time.Hour).Add(-5 * time.Minute),
			Name:    EventNameRebootReason,
			Type:    string(apiv1.EventTypeInfo),
			Message: "Reason for hour ago",
		})
		require.NoError(t, err)

		bucket.Close()

		// Get events
		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 4, "Should have all 4 reboot events")

		// Verify descending order is maintained
		for i := 1; i < len(events); i++ {
			assert.True(t, events[i-1].Time.After(events[i].Time) || events[i-1].Time.Equal(events[i].Time),
				"Events should maintain descending timestamp order")
		}

		// Verify correct events have reasons appended
		assert.Equal(t, baseTime.Unix(), events[0].Time.Unix())
		assert.Contains(t, events[0].Message, "(reboot reason: Reason for latest)")

		assert.Equal(t, baseTime.Add(-1*time.Hour).Unix(), events[1].Time.Unix())
		assert.Contains(t, events[1].Message, "(reboot reason: Reason for hour ago)")
	})

	t.Run("test multiple reasons with same message are deduplicated", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		now := time.Now()

		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert a reboot event
		err = bucket.Insert(ctx, eventstore.Event{
			Time:    now,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		})
		require.NoError(t, err)

		// Insert same reason multiple times
		for i := 0; i < 3; i++ {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    now.Add(-time.Duration(i+1) * time.Minute),
				Name:    EventNameRebootReason,
				Type:    string(apiv1.EventTypeInfo),
				Message: "Power loss",
			})
			require.NoError(t, err)
		}
		bucket.Close()

		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)

		// Should only append the reason once
		assert.Equal(t, 1, strings.Count(events[0].Message, "(reboot reason: Power loss)"))
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

	recorder := NewRebootEventStore(store)

	now := time.Now()
	baseTime := now.Add(-4 * time.Hour)

	// Test that GetRebootEvents only returns reboot events from the os bucket
	t.Run("get only reboot events from os bucket", func(t *testing.T) {
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
		retrievedEvents, err := recorder.GetRebootEvents(ctx, baseTime.Add(-5*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, 2, "Should have 2 reboot events only")

		// Verify events are sorted by timestamp (descending)
		for i := 1; i < len(retrievedEvents); i++ {
			assert.True(t, retrievedEvents[i-1].Time.After(retrievedEvents[i].Time) || retrievedEvents[i-1].Time.Equal(retrievedEvents[i].Time),
				"Events should be sorted by timestamp descending")
		}

		// Verify all returned events are reboot events
		for _, event := range retrievedEvents {
			assert.Equal(t, EventNameReboot, event.Name, "All returned events should be reboot events")
		}
	})

	// Test filtering by time range
	t.Run("get events filtered by time range", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert multiple reboot events with different timestamps
		osBucket, err = store.Bucket(EventBucketName)
		require.NoError(t, err)

		rebootEvents := []eventstore.Event{
			{
				Time:    baseTime.Add(-5 * time.Hour), // too old, should be filtered out
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: "old reboot",
			},
			{
				Time:    baseTime.Add(-2 * time.Hour), // within range
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: "recent reboot 1",
			},
			{
				Time:    baseTime.Add(-1 * time.Hour), // within range
				Name:    EventNameReboot,
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
		events, err := recorder.GetRebootEvents(ctx, baseTime.Add(-3*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, events, 2, "Should have 2 recent reboot events")

		// Verify all events are within the time range
		sinceTime := baseTime.Add(-3 * time.Hour)
		for _, event := range events {
			assert.True(t, event.Time.After(sinceTime) || event.Time.Equal(sinceTime),
				"All events should be after the since time")
			assert.Equal(t, EventNameReboot, event.Name)
		}
	})

	// Test with events outside the time range in os bucket
	t.Run("get events with time filtering in os bucket", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Insert mixed reboot and non-reboot events in os bucket
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
				Time:    baseTime.Add(-30 * time.Minute),
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: "another reboot",
			},
			{
				Time:    baseTime.Add(-5 * time.Hour),
				Name:    EventNameReboot,
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
		retrievedEvents, err := recorder.GetRebootEvents(ctx, sinceTime)
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, 2, "Should have only reboot events within the time range")

		// Verify only reboot events within range are returned
		for _, event := range retrievedEvents {
			assert.True(t, event.Time.After(sinceTime) || event.Time.Equal(sinceTime),
				"All events should be after the since time")
			assert.Equal(t, EventNameReboot, event.Name, "All events should be reboot events")
		}
	})

	// Test with empty os bucket
	t.Run("get events with empty os bucket", func(t *testing.T) {
		// Clean up any existing events
		osBucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)
		_, err = osBucket.Purge(ctx, time.Now().Unix())
		require.NoError(t, err)
		osBucket.Close()

		// Get events from empty bucket
		events, err := recorder.GetRebootEvents(ctx, baseTime.Add(-1*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, events, 0, "Should have no events from empty bucket")
	})

	// Test filtering of non-reboot events from os bucket
	t.Run("filter non-reboot events from os bucket", func(t *testing.T) {
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
		retrievedEvents, err := recorder.GetRebootEvents(ctx, baseTime.Add(-2*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, 2, "Should have only 2 reboot events")

		// Verify only reboot events are returned
		for _, event := range retrievedEvents {
			assert.Equal(t, EventNameReboot, event.Name, "All events should be reboot events")
		}
	})

	// Test proper sorting of reboot events
	t.Run("proper sorting of reboot events", func(t *testing.T) {
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
			baseTime.Add(-1 * time.Hour), // newest
			baseTime.Add(-2 * time.Hour), // middle
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

		// Get all events
		events, err := recorder.GetRebootEvents(ctx, baseTime.Add(-4*time.Hour))
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
			assert.Equal(t, EventNameReboot, events[i].Name, "All events should be reboot events")
		}
	})
}

func TestEventSortingDescendingOrder(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Run("verify events are sorted in descending order", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		now := time.Now()

		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert events in random order
		times := []time.Time{
			now.Add(-2 * time.Hour),
			now.Add(-5 * time.Hour),
			now.Add(-1 * time.Hour),
			now.Add(-3 * time.Hour),
			now.Add(-30 * time.Minute),
		}

		for i, tm := range times {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    tm,
				Name:    EventNameReboot,
				Type:    string(apiv1.EventTypeWarning),
				Message: fmt.Sprintf("reboot %d", i),
			})
			require.NoError(t, err)
		}
		bucket.Close()

		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 5)

		// Verify descending order - latest events first
		for i := 1; i < len(events); i++ {
			assert.True(t, events[i-1].Time.After(events[i].Time) || events[i-1].Time.Equal(events[i].Time),
				"Event at index %d (time: %v) should be after or equal to event at index %d (time: %v)",
				i-1, events[i-1].Time, i, events[i].Time)
		}

		// Verify the latest event is first
		assert.Equal(t, now.Add(-30*time.Minute).Unix(), events[0].Time.Unix())
		// Verify the oldest event is last
		assert.Equal(t, now.Add(-5*time.Hour).Unix(), events[len(events)-1].Time.Unix())
	})
}

func TestSafelyAppendingMultipleRebootReasons(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Run("safely append multiple distinct reasons", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		now := time.Now()

		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert a reboot event
		err = bucket.Insert(ctx, eventstore.Event{
			Time:    now,
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		})
		require.NoError(t, err)

		// Insert multiple distinct reasons
		reasons := []string{"Power loss", "Kernel panic", "Hardware failure"}
		for i, reason := range reasons {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    now.Add(-time.Duration(i+1) * time.Minute),
				Name:    EventNameRebootReason,
				Type:    string(apiv1.EventTypeInfo),
				Message: reason,
			})
			require.NoError(t, err)
		}
		bucket.Close()

		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)

		// Due to the bug, this will fail - but the test documents expected behavior
		for _, reason := range reasons {
			assert.Contains(t, events[0].Message, reason,
				"All distinct reasons should be appended")
		}
	})

	t.Run("handle mixed reboot and reason events", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		now := time.Now()

		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert mixed events
		events := []struct {
			time    time.Time
			name    string
			message string
		}{
			{now, EventNameReboot, "reboot 1"},
			{now.Add(-1 * time.Minute), EventNameRebootReason, "reason 1"},
			{now.Add(-2 * time.Hour), EventNameReboot, "reboot 2"},
			{now.Add(-2*time.Hour - 1*time.Minute), EventNameRebootReason, "reason 2"},
			{now.Add(-3 * time.Hour), EventNameReboot, "reboot 3"},
			// No reason for reboot 3
		}

		for _, ev := range events {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    ev.time,
				Name:    ev.name,
				Type:    string(apiv1.EventTypeWarning),
				Message: ev.message,
			})
			require.NoError(t, err)
		}
		bucket.Close()

		result, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, result, 3, "Should have 3 reboot events")

		// Verify correct pairing (though this will fail due to the bug)
		assert.Contains(t, result[0].Message, "reboot 1")
		assert.Contains(t, result[0].Message, "reason 1")

		assert.Contains(t, result[1].Message, "reboot 2")
		assert.Contains(t, result[1].Message, "reason 2")

		assert.Contains(t, result[2].Message, "reboot 3")
		assert.NotContains(t, result[2].Message, "reason")
	})
}

func TestEdgeCasesForRebootEvents(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Run("handle empty event list", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Empty(t, events, "Should return empty list for empty bucket")
	})

	t.Run("handle only reboot reason events without reboot", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert only reason events
		err = bucket.Insert(ctx, eventstore.Event{
			Time:    time.Now(),
			Name:    EventNameRebootReason,
			Type:    string(apiv1.EventTypeInfo),
			Message: "orphan reason",
		})
		require.NoError(t, err)
		bucket.Close()

		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Empty(t, events, "Should not return reason events without corresponding reboot")
	})

	t.Run("handle only non-reboot events", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Insert only non-reboot events
		nonRebootEvents := []string{"kmsg_error", "os_warning", "disk_error", "network_failure"}
		for i, name := range nonRebootEvents {
			err = bucket.Insert(ctx, eventstore.Event{
				Time:    time.Now().Add(-time.Duration(i) * time.Minute),
				Name:    name,
				Type:    string(apiv1.EventTypeWarning),
				Message: fmt.Sprintf("%s occurred", name),
			})
			require.NoError(t, err)
		}
		bucket.Close()

		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Empty(t, events, "Should filter out all non-reboot events")
	})

	t.Run("handle very long reason messages", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		require.NoError(t, err)

		bucket, err := store.Bucket(EventBucketName)
		require.NoError(t, err)

		// Create a very long reason
		longReason := strings.Repeat("Very long reason text ", 100)

		err = bucket.Insert(ctx, eventstore.Event{
			Time:    time.Now(),
			Name:    EventNameReboot,
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot",
		})
		require.NoError(t, err)

		err = bucket.Insert(ctx, eventstore.Event{
			Time:    time.Now().Add(-1 * time.Minute),
			Name:    EventNameRebootReason,
			Type:    string(apiv1.EventTypeInfo),
			Message: longReason,
		})
		require.NoError(t, err)
		bucket.Close()

		events, err := getEvents(ctx, store, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)

		// Verify the long reason can be appended (though it won't due to the bug)
		assert.Contains(t, events[0].Message, longReason)
	})
}
