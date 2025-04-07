package fuse

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// openTestEventStore creates a test event store and returns cleanup function
func openTestEventStore(t *testing.T) (eventstore.Store, func()) {
	dbRW, dbRO, sqliteCleanup := sqlite.OpenTestDB(t)
	store, err := eventstore.New(dbRW, dbRO, 0)
	require.NoError(t, err)

	return store, func() {
		sqliteCleanup()
	}
}

func TestNew(t *testing.T) {
	// Create a test event store
	store, cleanup := openTestEventStore(t)
	defer cleanup()

	// Create the component
	comp, err := New(context.Background(), 0, 0, store)

	// Validate the component was created successfully
	require.NoError(t, err)
	assert.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

func TestComponentLifecycle(t *testing.T) {
	// Create a test event store
	store, cleanup := openTestEventStore(t)
	defer cleanup()

	// Create the component
	comp, err := New(context.Background(), 0, 0, store)
	require.NoError(t, err)

	// Test Start
	err = comp.Start()
	assert.NoError(t, err)

	// Test Close
	err = comp.Close()
	assert.NoError(t, err)
}

func TestEvents(t *testing.T) {
	// Create a test event store
	store, cleanup := openTestEventStore(t)
	defer cleanup()

	// Create the component
	comp, err := New(context.Background(), 0, 0, store)
	require.NoError(t, err)

	// Test Events - initially there should be no events
	events, err := comp.Events(context.Background(), time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestDataFunctions(t *testing.T) {
	// Test the Data struct functions directly
	t.Run("getReason with nil", func(t *testing.T) {
		var d *Data
		reason := d.getReason()
		assert.Equal(t, "no fuse data", reason)
	})

	t.Run("getStates", func(t *testing.T) {
		d := &Data{}
		states, err := d.getStates()
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, "fuse", states[0].Name)
		assert.Equal(t, components.StateHealthy, states[0].Health)
		assert.True(t, states[0].Healthy)
	})

	t.Run("getHealth", func(t *testing.T) {
		d := &Data{}
		health, healthy := d.getHealth()
		assert.Equal(t, components.StateHealthy, health)
		assert.True(t, healthy)
	})
}
