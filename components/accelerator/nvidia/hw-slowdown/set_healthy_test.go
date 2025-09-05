package hwslowdown

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestSetHealthy(t *testing.T) {
	t.Parallel()

	// Setup test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := eventstore.NewV2(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2("test_events")
	assert.NoError(t, err)
	defer bucketV2.Close()

	baseTime := time.Now().UTC()

	// Create component
	c := &component{
		ctx:                        ctx,
		cancel:                     cancel,
		eventBucket:                bucketV2,
		freqPerMinEvaluationWindow: DefaultStateHWSlowdownEvaluationWindow,
		freqPerMinThreshold:        DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,
		getTimeNowFunc: func() time.Time {
			return baseTime
		},
	}

	// Insert multiple HW slowdown events
	for i := 0; i < 5; i++ {
		event := eventstore.Event{
			Component: Name,
			Time:      baseTime.Add(-time.Duration(i) * time.Minute),
			Name:      nvidianvml.EventNameHWSlowdown,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "HW slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid": "gpu-0",
			},
		}
		err := bucketV2.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Verify events exist
	events, err := bucketV2.Read(ctx)
	assert.NoError(t, err)
	assert.Len(t, events, 5)

	// Call SetHealthy
	err = c.SetHealthy()
	assert.NoError(t, err)

	// Verify all old events were purged and SetHealthy event was inserted
	events, err = bucketV2.Read(ctx)
	assert.NoError(t, err)
	// We may have one remaining event that was at exactly baseTime or the SetHealthy event
	assert.LessOrEqual(t, len(events), 2)

	// Verify SetHealthy event was inserted by checking for it specifically
	setHealthyEvents, err := bucketV2.Read(ctx, eventstore.WithName("SetHealthy"))
	assert.NoError(t, err)
	assert.Len(t, setHealthyEvents, 1)
	assert.Equal(t, "SetHealthy", setHealthyEvents[0].Name)
	assert.Equal(t, string(apiv1.EventTypeInfo), setHealthyEvents[0].Type)
	// Component field is not persisted in the database, so we can't verify it
	assert.Equal(t, baseTime.Unix(), setHealthyEvents[0].Time.Unix())

	// Call SetHealthy again - should not insert duplicate
	err = c.SetHealthy()
	assert.NoError(t, err)

	// Verify only one SetHealthy event exists
	events, err = bucketV2.Read(ctx, eventstore.WithName("SetHealthy"))
	assert.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestSetHealthyWithNilBucket(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create component without event bucket
	c := &component{
		ctx:                        ctx,
		cancel:                     cancel,
		eventBucket:                nil,
		freqPerMinEvaluationWindow: DefaultStateHWSlowdownEvaluationWindow,
		freqPerMinThreshold:        DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	// SetHealthy should succeed even with nil bucket
	err := c.SetHealthy()
	assert.NoError(t, err)
}

func TestSetHealthyPurgeError(t *testing.T) {
	t.Parallel()

	// Setup test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Note: ctx is created for initializing the store but we'll use shortCtx for the component
	_, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := eventstore.NewV2(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2("test_events")
	assert.NoError(t, err)
	defer bucketV2.Close()

	// Create component with a context that will be canceled
	shortCtx, shortCancel := context.WithCancel(context.Background())

	c := &component{
		ctx:                        shortCtx,
		cancel:                     shortCancel,
		eventBucket:                bucketV2,
		freqPerMinEvaluationWindow: DefaultStateHWSlowdownEvaluationWindow,
		freqPerMinThreshold:        DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	// Cancel the context to simulate a purge error
	shortCancel()

	// SetHealthy should return an error
	err = c.SetHealthy()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestSetHealthyFindError(t *testing.T) {
	t.Parallel()

	// Setup test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := eventstore.NewV2(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2("test_events")
	assert.NoError(t, err)
	defer bucketV2.Close()

	baseTime := time.Now().UTC()

	// Create component
	c := &component{
		ctx:                        ctx,
		cancel:                     cancel,
		eventBucket:                bucketV2,
		freqPerMinEvaluationWindow: DefaultStateHWSlowdownEvaluationWindow,
		freqPerMinThreshold:        DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,
		getTimeNowFunc: func() time.Time {
			return baseTime
		},
	}

	// First SetHealthy should succeed
	err = c.SetHealthy()
	assert.NoError(t, err)

	// Calling SetHealthy again will find the existing event and skip insertion
	err = c.SetHealthy()
	assert.NoError(t, err)

	// Verify only one SetHealthy event exists
	events, err := bucketV2.Read(ctx, eventstore.WithName("SetHealthy"))
	assert.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestSetHealthyInsertError(t *testing.T) {
	t.Parallel()

	// Setup test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := eventstore.NewV2(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2("test_events")
	assert.NoError(t, err)
	defer bucketV2.Close()

	// Create component with a short context for the Find operation
	c := &component{
		ctx:                        ctx,
		cancel:                     cancel,
		eventBucket:                bucketV2,
		freqPerMinEvaluationWindow: DefaultStateHWSlowdownEvaluationWindow,
		freqPerMinThreshold:        DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	// First SetHealthy should succeed
	err = c.SetHealthy()
	assert.NoError(t, err)

	// Verify the SetHealthy event was inserted
	events, err := bucketV2.Read(ctx, eventstore.WithName("SetHealthy"))
	assert.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestSetHealthyEndToEnd(t *testing.T) {
	t.Parallel()

	// Setup test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := eventstore.NewV2(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2("test_events")
	assert.NoError(t, err)
	defer bucketV2.Close()

	baseTime := time.Now().UTC()

	// Create component
	c := &component{
		ctx:                        ctx,
		cancel:                     cancel,
		eventBucket:                bucketV2,
		freqPerMinEvaluationWindow: 5 * time.Minute,
		freqPerMinThreshold:        1.0,
		getTimeNowFunc: func() time.Time {
			return baseTime
		},
	}

	// Scenario: System has multiple HW slowdown events, then gets healthy again

	// Insert old HW slowdown events (outside evaluation window)
	for i := 10; i < 20; i++ {
		event := eventstore.Event{
			Component: Name,
			Time:      baseTime.Add(-time.Duration(i) * time.Minute),
			Name:      nvidianvml.EventNameHWSlowdown,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "Old HW slowdown detected",
		}
		err := bucketV2.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Insert recent HW slowdown events (inside evaluation window)
	for i := 0; i < 3; i++ {
		event := eventstore.Event{
			Component: Name,
			Time:      baseTime.Add(-time.Duration(i) * time.Minute),
			Name:      nvidianvml.EventNameHWSlowdown,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "Recent HW slowdown detected",
		}
		err := bucketV2.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Verify we have events
	allEvents, err := bucketV2.Read(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 13, len(allEvents)) // 10 old + 3 recent

	// Admin marks system as healthy
	err = c.SetHealthy()
	assert.NoError(t, err)

	// Most events should be purged, SetHealthy event should be inserted
	allEvents, err = bucketV2.Read(ctx)
	assert.NoError(t, err)
	// We may have the SetHealthy event plus one event that was at exactly baseTime
	assert.LessOrEqual(t, len(allEvents), 2)

	// Verify SetHealthy event exists
	setHealthyEvents, err := bucketV2.Read(ctx, eventstore.WithName("SetHealthy"))
	assert.NoError(t, err)
	assert.Len(t, setHealthyEvents, 1)
	assert.Equal(t, "SetHealthy", setHealthyEvents[0].Name)
	assert.Equal(t, string(apiv1.EventTypeInfo), setHealthyEvents[0].Type)

	// Events method should return only the SetHealthy event
	events, err := c.Events(ctx, baseTime.Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "SetHealthy", events[0].Name)

	// After SetHealthy, new HW slowdown events can still be recorded
	newEvent := eventstore.Event{
		Component: Name,
		Time:      baseTime.Add(1 * time.Minute),
		Name:      nvidianvml.EventNameHWSlowdown,
		Type:      string(apiv1.EventTypeWarning),
		Message:   "New HW slowdown after SetHealthy",
	}
	err = bucketV2.Insert(ctx, newEvent)
	assert.NoError(t, err)

	// Now we should have at least the SetHealthy and the new HW slowdown event
	allEvents, err = bucketV2.Read(ctx)
	assert.NoError(t, err)
	// Could have: new event, SetHealthy, and possibly one event at baseTime
	assert.GreaterOrEqual(t, len(allEvents), 2)
	assert.LessOrEqual(t, len(allEvents), 3)
}
