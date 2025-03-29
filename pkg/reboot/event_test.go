package reboot

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
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

		err = RecordEvent(ctx, store, "os", mockLastReboot)
		assert.NoError(t, err)

		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, recentTime.Add(-1*time.Minute))
		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "reboot", events[0].Name)
		assert.Equal(t, common.EventTypeWarning, events[0].Type)
		assert.Equal(t, recentTime.Unix(), events[0].Time.Unix())
	})

	// Test with an old reboot time (beyond retention)
	t.Run("old reboot should not record event", func(t *testing.T) {
		oldTime := time.Now().Add(-2 * eventstore.DefaultRetention)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return oldTime, nil
		}

		err = RecordEvent(ctx, store, "os", mockLastReboot)
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

		err = RecordEvent(ctx, store, "os", mockLastReboot)
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

		err = RecordEvent(ctx, store, "os", mockLastReboot)
		assert.NoError(t, err)

		// Should still be only 1 event
		events, err = bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	// Test with close timestamp but not exact match
	t.Run("very close timestamps should not create duplicate", func(t *testing.T) {
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		// Get the existing event
		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		require.Len(t, events, 1)

		existingTime := events[0].Time.Time

		// Try with timestamp 30 seconds different (less than 1 minute threshold)
		closeTime := existingTime.Add(30 * time.Second)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return closeTime, nil
		}

		err = RecordEvent(ctx, store, "os", mockLastReboot)
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

	bucketName := "test-bucket"

	// Create a bucket and insert some test events
	bucket, err := store.Bucket(bucketName)
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
			Type:    common.EventTypeWarning,
			Message: e.message,
		})
		require.NoError(t, err)
	}
	bucket.Close()

	// Test getting all events
	t.Run("get all events", func(t *testing.T) {
		retrievedEvents, err := GetEvents(ctx, store, bucketName, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, retrievedEvents, len(events))
	})

	// Test getting events since a specific time
	t.Run("get events since specific time", func(t *testing.T) {
		retrievedEvents, err := GetEvents(ctx, store, bucketName, now.Add(-2*time.Hour))
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

		err = RecordEvent(ctx, store, "os", mockLastReboot)
		assert.NoError(t, err)

		// Since time.Time{} is way in the past, it should be filtered by retention period
		bucket, err := store.Bucket("os")
		require.NoError(t, err)
		defer bucket.Close()

		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 0)
	})

	// Test with cancelled context
	t.Run("cancelled context", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		recentTime := time.Now().Add(-1 * time.Hour)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		}

		err = RecordEvent(cancelledCtx, store, "os", mockLastReboot)
		assert.Error(t, err)
	})
}
