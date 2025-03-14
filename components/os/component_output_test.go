package os

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	os_id "github.com/leptonai/gpud/components/os/id"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkg_host "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGet(t *testing.T) {
	defer func() {
		getSystemdDetectVirtFunc = pkg_host.SystemdDetectVirt
	}()
	getSystemdDetectVirtFunc = func(ctx context.Context) (pkg_host.VirtualizationEnvironment, error) {
		return pkg_host.VirtualizationEnvironment{}, context.DeadlineExceeded
	}

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	getFunc := createGet(nil)
	_, err := getFunc(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	expectedError := "failed to get virtualization environment using 'systemd-detect-virt': context deadline exceeded"
	if err.Error() != expectedError {
		t.Fatalf("expected error: %s, got: %s", expectedError, err.Error())
	}
}

func TestCreateRebootEvent(t *testing.T) {
	t.Run("First Boot Event", func(t *testing.T) {
		// Create a new test database for this test case
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		assert.NoError(t, err)
		bucket, err := store.Bucket(os_id.Name)
		assert.NoError(t, err)
		defer bucket.Close()

		now := time.Now().Truncate(time.Second)

		bootTime := now.Add(-1 * time.Hour)
		err = createRebootEvent(
			context.Background(),
			bucket,
			now,
			func() (time.Time, error) {
				return bootTime, nil
			},
			func(ctx context.Context) (time.Time, error) {
				return bootTime, nil
			},
		)
		assert.NoError(t, err)

		latestEvent, err := bucket.Latest(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, latestEvent)
		assert.Equal(t, "reboot", latestEvent.Name)
		assert.Equal(t, bootTime, latestEvent.Time.Time)
		assert.Contains(t, latestEvent.Message, "system boot detected")
	})

	t.Run("Valid Old Reboot Event", func(t *testing.T) {
		// Create a new test database for this test case
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		assert.NoError(t, err)
		bucket, err := store.Bucket(os_id.Name)
		assert.NoError(t, err)
		defer bucket.Close()

		now := time.Now().Truncate(time.Second)

		// Insert initial event
		initialTime := now.Add(-2 * time.Hour)
		err = bucket.Insert(context.Background(), components.Event{
			Time:    metav1.Time{Time: initialTime},
			Name:    "reboot",
			Type:    common.EventTypeWarning,
			Message: fmt.Sprintf("system reboot detected %v", initialTime),
		})
		assert.NoError(t, err)

		bootTime := now.Add(-1 * time.Hour)
		err = createRebootEvent(
			context.Background(),
			bucket,
			now,
			func() (time.Time, error) {
				return bootTime, nil
			},
			func(ctx context.Context) (time.Time, error) {
				return bootTime, nil
			},
		)
		assert.NoError(t, err)

		latestEvent, err := bucket.Latest(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, latestEvent)
		assert.Equal(t, "reboot", latestEvent.Name)
		assert.Equal(t, bootTime, latestEvent.Time.Time)
		// With firstBoot == lastBoot, we expect a boot message
		assert.Contains(t, latestEvent.Message, "system boot detected")
	})

	t.Run("Skip Event Due to Retention", func(t *testing.T) {
		// Create a new test database for this test case
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		assert.NoError(t, err)
		bucket, err := store.Bucket(os_id.Name)
		assert.NoError(t, err)
		defer bucket.Close()

		now := time.Now().Truncate(time.Second)

		// Insert a new event
		initialTime := now.Add(-30 * time.Minute)
		err = bucket.Insert(context.Background(), components.Event{
			Time:    metav1.Time{Time: initialTime},
			Name:    "reboot",
			Type:    common.EventTypeWarning,
			Message: fmt.Sprintf("system reboot detected %v", initialTime),
		})
		assert.NoError(t, err)

		bootTime := now.Add(-2 * DefaultRetentionPeriod)
		err = createRebootEvent(
			context.Background(),
			bucket,
			now,
			func() (time.Time, error) {
				return bootTime, nil
			},
			func(ctx context.Context) (time.Time, error) {
				return bootTime, nil
			},
		)
		assert.NoError(t, err)

		// The event should be skipped due to retention period, so latest should be our inserted event
		latestEvent, err := bucket.Latest(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "reboot", latestEvent.Name)
		assert.Equal(t, initialTime, latestEvent.Time.Time)

		// Verify no event with the bootTime exists
		events, err := bucket.Get(context.Background(), bootTime)
		assert.NoError(t, err)
		found := false
		for _, e := range events {
			if e.Time.Time.Equal(bootTime) {
				found = true
				break
			}
		}
		assert.False(t, found, "Should not find an event at bootTime due to retention period")
	})

	t.Run("Valid New Reboot Event", func(t *testing.T) {
		// Create a new test database for this test case
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		assert.NoError(t, err)
		bucket, err := store.Bucket(os_id.Name)
		assert.NoError(t, err)
		defer bucket.Close()

		now := time.Now().Truncate(time.Second)

		// For a reboot message, firstBoot needs to be AFTER lastBoot per the implementation logic
		firstBootTime := now.Add(1 * time.Hour)    // Later time for first boot (changed order)
		lastBootTime := now.Add(-30 * time.Minute) // Earlier time for last boot

		// Insert a previous event
		err = bucket.Insert(context.Background(), components.Event{
			Time:    metav1.Time{Time: now.Add(-1 * time.Hour)},
			Name:    "reboot",
			Type:    common.EventTypeWarning,
			Message: "previous event",
		})
		assert.NoError(t, err)

		// Create a reboot event
		err = createRebootEvent(
			context.Background(),
			bucket,
			now,
			func() (time.Time, error) {
				return firstBootTime, nil
			},
			func(ctx context.Context) (time.Time, error) {
				return lastBootTime, nil
			},
		)
		assert.NoError(t, err)

		latestEvent, err := bucket.Latest(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, latestEvent)
		assert.Equal(t, "reboot", latestEvent.Name)
		assert.Equal(t, lastBootTime, latestEvent.Time.Time)
		assert.Contains(t, latestEvent.Message, "system reboot detected")
	})

	t.Run("Skip Event if Latest Event Matches", func(t *testing.T) {
		// Create a new test database for this test case
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
		assert.NoError(t, err)
		bucket, err := store.Bucket(os_id.Name)
		assert.NoError(t, err)
		defer bucket.Close()

		now := time.Now().Truncate(time.Second)

		// Insert an event with the same timestamp we'll try to create
		bootTime := now.Add(1 * time.Hour)
		err = bucket.Insert(context.Background(), components.Event{
			Time:    metav1.Time{Time: bootTime},
			Name:    "reboot",
			Type:    common.EventTypeWarning,
			Message: "test duplicate event",
		})
		assert.NoError(t, err)

		// Try to create another event with the same timestamp
		err = createRebootEvent(
			context.Background(),
			bucket,
			now,
			func() (time.Time, error) {
				return bootTime, nil
			},
			func(ctx context.Context) (time.Time, error) {
				return bootTime, nil
			},
		)
		assert.NoError(t, err)

		// Check that no new event was inserted (same timestamp)
		// Query from way back to ensure we see all events
		events, err := bucket.Get(context.Background(), now.Add(-24*time.Hour))
		assert.NoError(t, err)
		// Verify we have exactly one event and it's our original event
		assert.Equal(t, 1, len(events))
		assert.Equal(t, bootTime, events[0].Time.Time)
		assert.Equal(t, "test duplicate event", events[0].Message)
	})
}
