package info

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponentName(t *testing.T) {
	t.Parallel()

	component := New(map[string]string{"a": "b"}, nil, prometheus.DefaultGatherer)
	assert.Equal(t, Name, component.Name())
}

func TestComponentStartAndClose(t *testing.T) {
	t.Parallel()

	c := New(map[string]string{"a": "b"}, nil, prometheus.DefaultGatherer).(*component)
	c.ctx, c.cancel = context.WithCancel(context.Background())

	err := c.Start()
	assert.NoError(t, err)

	// Wait a bit for CheckOnce to execute
	time.Sleep(100 * time.Millisecond)

	// Verify data was collected
	c.lastMu.RLock()
	assert.NotNil(t, c.lastData)
	c.lastMu.RUnlock()

	err = c.Close()
	assert.NoError(t, err)
}

func TestComponentStates(t *testing.T) {
	t.Parallel()

	// Create component with database
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	c := New(map[string]string{"test": "value"}, dbRO, prometheus.DefaultGatherer).(*component)
	c.ctx, c.cancel = context.WithCancel(context.Background())
	defer c.Close()

	// Run CheckOnce to gather data
	c.CheckOnce()

	// Check the states
	ctx := context.Background()
	states, err := c.States(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	component := New(map[string]string{}, nil, prometheus.DefaultGatherer)
	ctx := context.Background()

	events, err := component.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestComponentMetrics(t *testing.T) {
	t.Parallel()

	component := New(map[string]string{}, nil, prometheus.DefaultGatherer)
	ctx := context.Background()

	metrics, err := component.Metrics(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestComponentWithDB(t *testing.T) {
	t.Parallel()

	// Create component with database
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	c := New(map[string]string{}, dbRO, prometheus.DefaultGatherer).(*component)
	c.ctx, c.cancel = context.WithCancel(context.Background())
	defer c.Close()

	// Run CheckOnce to gather database metrics
	c.CheckOnce()

	// Verify that database metrics are collected
	c.lastMu.RLock()
	data := c.lastData
	c.lastMu.RUnlock()

	assert.NotNil(t, data)
}

func TestDataGetHealth(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *Data
	health, healthy := nilData.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	// Test with error
	data := &Data{err: assert.AnError}
	health, healthy = data.getHealth()
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)

	// Test without error
	data = &Data{}
	health, healthy = data.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)
}

func TestDataGetReason(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *Data
	reason := nilData.getReason()
	assert.Equal(t, "no info data", reason)

	// Test with error
	data := &Data{err: assert.AnError}
	reason = data.getReason()
	assert.Contains(t, reason, "failed to get info data")

	// Test without error
	data = &Data{
		MacAddress:  "00:11:22:33:44:55",
		Annotations: map[string]string{"test": "value"},
	}
	reason = data.getReason()
	assert.Contains(t, reason, "daemon version")
	assert.Contains(t, reason, "mac address: 00:11:22:33:44:55")
	assert.Contains(t, reason, "annotations")
}

func TestDataGetStatesNil(t *testing.T) {
	// Test with nil data
	var d *Data
	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "Healthy", states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no data yet", states[0].Reason)
}
