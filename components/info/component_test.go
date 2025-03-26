package info

import (
	"context"
	"errors"
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

	// Test with context.DeadlineExceeded error
	data = &Data{err: context.DeadlineExceeded}
	health, healthy = data.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	// Test with context.Canceled error
	data = &Data{err: context.Canceled}
	health, healthy = data.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

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

	// Test with generic error
	data := &Data{err: errors.New("test error")}
	reason = data.getReason()
	assert.Equal(t, "failed to get info data -- test error", reason)

	// Test with context.DeadlineExceeded error
	data = &Data{err: context.DeadlineExceeded}
	reason = data.getReason()
	assert.Equal(t, "check failed with context deadline exceeded -- transient error, please retry", reason)

	// Test with context.Canceled error
	data = &Data{err: context.Canceled}
	reason = data.getReason()
	assert.Equal(t, "check failed with context canceled -- transient error, please retry", reason)

	// Test without error and with all fields populated
	data = &Data{
		DaemonVersion: "test-version",
		MacAddress:    "00:11:22:33:44:55",
		Annotations:   map[string]string{"test": "value", "env": "dev"},
	}
	reason = data.getReason()
	assert.Contains(t, reason, "daemon version: test-version")
	assert.Contains(t, reason, "mac address: 00:11:22:33:44:55")
	assert.Contains(t, reason, `annotations: map["env":"dev" "test":"value"]`)

	// Test without error and without annotations
	data = &Data{
		DaemonVersion: "test-version",
		MacAddress:    "00:11:22:33:44:55",
		Annotations:   map[string]string{},
	}
	reason = data.getReason()
	assert.Contains(t, "daemon version: test-version, mac address: 00:11:22:33:44:55", reason)

	// Test with version.Version from the actual package
	data = &Data{
		MacAddress:  "00:11:22:33:44:55",
		Annotations: map[string]string{},
	}
	reason = data.getReason()
	assert.Contains(t, reason, "daemon version: ")
	assert.Contains(t, reason, "mac address: 00:11:22:33:44:55")
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
