package query

import (
	"database/sql"
	"testing"

	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/stretchr/testify/assert"
)

func TestOpOptions(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{})
		assert.NoError(t, err)

		// Check default values
		assert.Equal(t, "nvidia-smi", op.nvidiaSMICommand)
		assert.Equal(t, "nvidia-smi --query", op.nvidiaSMIQueryCommand)
		assert.Equal(t, "ibstat", op.ibstatCommand)
		assert.False(t, op.debug)
	})

	t.Run("custom values", func(t *testing.T) {
		mockDB := &sql.DB{}
		mockStore := &mockEventsStore{}

		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithDBRW(mockDB),
			WithDBRO(mockDB),
			WithXidEventsStore(mockStore),
			WithHWSlowdownEventsStore(mockStore),
			WithNvidiaSMICommand("/custom/nvidia-smi"),
			WithNvidiaSMIQueryCommand("/custom/nvidia-smi-query"),
			WithIbstatCommand("/custom/ibstat"),
			WithDebug(true),
		})

		assert.NoError(t, err)

		// Check custom values
		assert.Equal(t, mockDB, op.dbRW)
		assert.Equal(t, mockDB, op.dbRO)
		assert.Equal(t, mockStore, op.xidEventsStore)
		assert.Equal(t, mockStore, op.hwslowdownEventsStore)
		assert.Equal(t, "/custom/nvidia-smi", op.nvidiaSMICommand)
		assert.Equal(t, "/custom/nvidia-smi-query", op.nvidiaSMIQueryCommand)
		assert.Equal(t, "/custom/ibstat", op.ibstatCommand)
		assert.True(t, op.debug)
	})

	t.Run("partial options", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithNvidiaSMICommand("/custom/nvidia-smi"),
			WithDebug(true),
		})

		assert.NoError(t, err)

		// Check mixed default and custom values
		assert.Equal(t, "/custom/nvidia-smi", op.nvidiaSMICommand)
		assert.Equal(t, "nvidia-smi --query", op.nvidiaSMIQueryCommand)
		assert.Equal(t, "ibstat", op.ibstatCommand)
		assert.True(t, op.debug)
	})
}

// mockEventsStore implements events_db.Store interface for testing
type mockEventsStore struct {
	events_db.Store
}
