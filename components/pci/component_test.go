package pci

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/pci/id"
	"github.com/leptonai/gpud/pkg/eventstore"
	query_config "github.com/leptonai/gpud/pkg/query/config"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponent(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO)
	assert.NoError(t, err)

	ctx := context.Background()
	cfg := Config{
		Query: query_config.Config{
			State: &query_config.State{
				DBRW: dbRW,
				DBRO: dbRO,
			},
		},
	}

	// Test component creation
	comp, err := New(ctx, cfg, store)
	require.NoError(t, err)
	require.NotNil(t, comp)

	// Test Name method
	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, id.Name, comp.Name())
	})

	// Test Start method
	t.Run("Start", func(t *testing.T) {
		err := comp.Start()
		assert.NoError(t, err)
	})

	// Test States method
	t.Run("States", func(t *testing.T) {
		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Nil(t, states)
	})

	// Test Events method
	t.Run("Events", func(t *testing.T) {
		since := time.Now().Add(-1 * time.Hour)
		events, err := comp.Events(ctx, since)
		assert.NoError(t, err)
		assert.Nil(t, events)
	})

	// Test Metrics method
	t.Run("Metrics", func(t *testing.T) {
		since := time.Now().Add(-1 * time.Hour)
		metrics, err := comp.Metrics(ctx, since)
		assert.NoError(t, err)
		assert.Nil(t, metrics)
	})

	// Test Close method
	t.Run("Close", func(t *testing.T) {
		err := comp.Close()
		assert.NoError(t, err)
	})
}

// Ensure component implements Component interface
func TestComponentInterface(t *testing.T) {
	var _ components.Component = (*component)(nil)
}
