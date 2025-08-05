package gpucounts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
)

// TestComponent_SetHealthy tests the SetHealthy method
func TestComponent_SetHealthy(t *testing.T) {
	ctx := context.Background()

	t.Run("with event bucket - inserts SetHealthy event", func(t *testing.T) {
		fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 5,
			foundEvent:       nil, // No existing SetHealthy event
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)
		assert.True(t, mockBucket.purgeCalled)
		assert.Equal(t, fixedTime.Unix(), mockBucket.purgeBeforeTimestamp)
		assert.True(t, mockBucket.findCalled)
		assert.True(t, mockBucket.insertCalled)

		// Verify the inserted event
		assert.Len(t, mockBucket.events, 1)
		assert.Equal(t, "SetHealthy", mockBucket.events[0].Name)
		assert.Equal(t, fixedTime, mockBucket.events[0].Time)
		assert.Equal(t, Name, mockBucket.events[0].Component)
		assert.Equal(t, string(apiv1.EventTypeInfo), mockBucket.events[0].Type)
	})

	t.Run("without event bucket", func(t *testing.T) {
		c := &component{
			ctx:         ctx,
			eventBucket: nil,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)
	})

	t.Run("with purge error", func(t *testing.T) {
		testErr := errors.New("purge failed")
		mockBucket := &mockEventBucket{
			name:     Name,
			purgeErr: testErr,
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		err := c.SetHealthy()
		assert.Error(t, err)
		assert.Equal(t, testErr, err)
		assert.True(t, mockBucket.purgeCalled)
	})

	t.Run("with context timeout", func(t *testing.T) {
		// Create a context that's already canceled
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()

		mockBucket := &mockEventBucket{
			name:     Name,
			purgeErr: context.Canceled,
		}

		c := &component{
			ctx:         canceledCtx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		err := c.SetHealthy()
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("uses correct timeout", func(t *testing.T) {
		fixedTime := time.Date(2024, 2, 20, 14, 30, 0, 0, time.UTC)

		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 10,
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)
		assert.True(t, mockBucket.purgeCalled)
		assert.Equal(t, fixedTime.Unix(), mockBucket.purgeBeforeTimestamp)
	})

	t.Run("purge returns zero count", func(t *testing.T) {
		fixedTime := time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC)

		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 0, // No events purged
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)
		assert.True(t, mockBucket.purgeCalled)
		assert.Equal(t, fixedTime.Unix(), mockBucket.purgeBeforeTimestamp)
	})

	t.Run("SetHealthy event already exists - skips insertion", func(t *testing.T) {
		fixedTime := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
		existingEvent := eventstore.Event{
			Time: fixedTime.Add(-1 * time.Hour),
			Name: "SetHealthy",
		}

		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 3,
			foundEvent:       &existingEvent, // Existing SetHealthy event
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)
		assert.True(t, mockBucket.purgeCalled)
		assert.True(t, mockBucket.findCalled)
		assert.False(t, mockBucket.insertCalled) // Should not insert
		assert.Len(t, mockBucket.events, 0)      // No new events
	})

	t.Run("Find returns error", func(t *testing.T) {
		fixedTime := time.Date(2024, 5, 1, 11, 0, 0, 0, time.UTC)
		findErr := errors.New("find failed")

		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 2,
			findErr:          findErr,
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.Error(t, err)
		assert.Equal(t, findErr, err)
		assert.True(t, mockBucket.purgeCalled)
		assert.True(t, mockBucket.findCalled)
		assert.False(t, mockBucket.insertCalled)
	})

	t.Run("Insert returns error", func(t *testing.T) {
		fixedTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		insertErr := errors.New("insert failed")

		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 1,
			insertErr:        insertErr,
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.Error(t, err)
		assert.Equal(t, insertErr, err)
		assert.True(t, mockBucket.purgeCalled)
		assert.True(t, mockBucket.findCalled)
		assert.True(t, mockBucket.insertCalled)
	})

	t.Run("context canceled during Find", func(t *testing.T) {
		fixedTime := time.Date(2024, 7, 1, 13, 0, 0, 0, time.UTC)

		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 4,
			findErr:          context.Canceled,
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
		assert.True(t, mockBucket.purgeCalled)
		assert.True(t, mockBucket.findCalled)
		assert.False(t, mockBucket.insertCalled)
	})
}

// TestComponent_SetHealthy_Integration tests SetHealthy integration with the actual component
func TestComponent_SetHealthy_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("integration with real event handling", func(t *testing.T) {
		now := time.Now()

		// Create a mock event bucket with purge simulation
		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 2, // Simulating 2 events would be purged
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return now
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)

		// Verify purge was called with correct timestamp
		assert.True(t, mockBucket.purgeCalled)
		assert.Equal(t, now.Unix(), mockBucket.purgeBeforeTimestamp)
	})
}
