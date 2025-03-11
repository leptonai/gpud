package query

import (
	"testing"

	"github.com/stretchr/testify/assert"

	events_db "github.com/leptonai/gpud/pkg/eventstore"
)

func TestOpOptions(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{})
		assert.NoError(t, err)

		// Check default values
		assert.Equal(t, "ibstat", op.ibstatCommand)
		assert.False(t, op.debug)
	})

	t.Run("custom values", func(t *testing.T) {
		mockBucket := &mockEventsStore{}

		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithXidEventBucket(mockBucket),
			WithHWSlowdownEventBucket(mockBucket),
			WithIbstatCommand("/custom/ibstat"),
			WithDebug(true),
		})

		assert.NoError(t, err)

		// Check custom values
		assert.Equal(t, mockBucket, op.xidEventsBucket)
		assert.Equal(t, mockBucket, op.hwSlowdownEventsBucket)
		assert.Equal(t, "/custom/ibstat", op.ibstatCommand)
		assert.True(t, op.debug)
	})

	t.Run("partial options", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithDebug(true),
		})

		assert.NoError(t, err)

		// Check mixed default and custom values
		assert.Equal(t, "ibstat", op.ibstatCommand)
		assert.True(t, op.debug)
	})
}

// mockEventsStore implements events_db.Store interface for testing
type mockEventsStore struct {
	events_db.Bucket
}
