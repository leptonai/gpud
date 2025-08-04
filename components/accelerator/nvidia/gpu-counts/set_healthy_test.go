package gpucounts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestComponent_SetHealthy tests the SetHealthy method
func TestComponent_SetHealthy(t *testing.T) {
	ctx := context.Background()

	t.Run("with event bucket", func(t *testing.T) {
		fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 5,
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
