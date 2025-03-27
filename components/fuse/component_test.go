package fuse

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/fuse"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestNew(t *testing.T) {
	t.Parallel()

	// Create test databases
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create test context
	ctx := context.Background()

	// Create test event store
	eventStore, err := eventstore.New(dbRW, dbRO, time.Minute)
	require.NoError(t, err)

	// Test component creation
	c, err := New(ctx, 90.0, 80.0, eventStore)
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, Name, c.Name())

	// Test with default thresholds
	c, err = New(ctx, 0, 0, eventStore)
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, DefaultCongestedPercentAgainstThreshold, c.(*component).congestedPercentAgainstThreshold)
	assert.Equal(t, DefaultMaxBackgroundPercentAgainstThreshold, c.(*component).maxBackgroundPercentAgainstThreshold)
}

func TestComponentLifecycle(t *testing.T) {
	t.Parallel()

	// Create test databases
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create test context
	ctx := context.Background()

	// Create test event store
	eventStore, err := eventstore.New(dbRW, dbRO, time.Minute)
	require.NoError(t, err)

	// Create component
	c, err := New(ctx, 90.0, 80.0, eventStore)
	require.NoError(t, err)

	// Test Start
	err = c.Start()
	require.NoError(t, err)

	// Test States with nil data
	states, err := c.States(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "fuse", states[0].Name)

	// if the check has not been run yet, the component is healthy
	if states[0].Reason == "no fuse data" {
		assert.Equal(t, components.StateHealthy, states[0].Health)
		assert.True(t, states[0].Healthy)
	}

	// Test Close
	err = c.Close()
	require.NoError(t, err)
}

func TestDataStates(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	states, err := d.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "fuse", states[0].Name)
	assert.Equal(t, "no fuse data", states[0].Reason)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)

	// Test data with error
	d = &Data{
		err: assert.AnError,
	}
	states, err = d.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "fuse", states[0].Name)
	assert.Equal(t, "failed to get fuse data -- assert.AnError general error for testing", states[0].Reason)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)

	// Test healthy data
	d = &Data{
		ConnectionInfos: make([]fuse.ConnectionInfo, 3),
	}
	states, err = d.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "fuse", states[0].Name)
	assert.Equal(t, "found 3 fuse connections", states[0].Reason)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
}

func TestDataHealth(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	health, healthy := d.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	// Test data with error
	d = &Data{
		err: assert.AnError,
	}
	health, healthy = d.getHealth()
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)

	// Test healthy data
	d = &Data{
		ConnectionInfos: make([]fuse.ConnectionInfo, 3),
	}
	health, healthy = d.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)
}

func TestDataReason(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	assert.Equal(t, "no fuse data", d.getReason())

	// Test data with error
	d = &Data{
		err: assert.AnError,
	}
	assert.Equal(t, "failed to get fuse data -- assert.AnError general error for testing", d.getReason())

	// Test healthy data
	d = &Data{
		ConnectionInfos: make([]fuse.ConnectionInfo, 3),
	}
	assert.Equal(t, "found 3 fuse connections", d.getReason())
}
