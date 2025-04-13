package info

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponentName(t *testing.T) {
	t.Parallel()

	component := New(map[string]string{"a": "b"}, nil)
	assert.Equal(t, Name, component.Name())
}

func TestComponentStartAndClose(t *testing.T) {
	t.Parallel()

	c := New(map[string]string{"a": "b"}, nil).(*component)
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

	c := New(map[string]string{"test": "value"}, dbRO).(*component)
	c.ctx, c.cancel = context.WithCancel(context.Background())
	defer c.Close()

	// Run CheckOnce to gather data
	c.CheckOnce()

	// Check the states
	ctx := context.Background()
	states, err := c.HealthStates(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	component := New(map[string]string{}, nil)
	ctx := context.Background()

	events, err := component.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestComponentWithDB(t *testing.T) {
	t.Parallel()

	// Create component with database
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	c := New(map[string]string{}, dbRO).(*component)
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

func TestDataGetStatesForHealth(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *Data
	states, err := nilData.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)

	// Test with error
	data := &Data{
		err:     assert.AnError,
		healthy: false,
	}
	states, err = data.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.False(t, states[0].DeprecatedHealthy)
	assert.Equal(t, assert.AnError.Error(), states[0].Error)

	// Test without error
	data = &Data{
		healthy: true,
	}
	states, err = data.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)
}

func TestDataGetStatesForReason(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *Data
	states, err := nilData.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with error
	data := &Data{
		err:    assert.AnError,
		reason: "failed to get info data",
	}
	states, err = data.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "failed to get info data", states[0].Reason)

	// Test without error
	data = &Data{
		MacAddress:  "00:11:22:33:44:55",
		Annotations: map[string]string{"test": "value"},
		reason:      "daemon version: test, mac address: 00:11:22:33:44:55",
	}
	states, err = data.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Contains(t, states[0].Reason, "daemon version")
	assert.Contains(t, states[0].Reason, "mac address: 00:11:22:33:44:55")
}

func TestDataGetStatesNil(t *testing.T) {
	// Test with nil data
	var d *Data
	states, err := d.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)
	assert.Equal(t, "no data yet", states[0].Reason)
	assert.Empty(t, states[0].Error, "Error should be empty for nil data")
}

func TestDataGetError(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *Data
	errStr := nilData.getError()
	assert.Equal(t, "", errStr)

	// Test with error
	data := &Data{err: assert.AnError}
	errStr = data.getError()
	assert.Equal(t, assert.AnError.Error(), errStr)

	// Test without error
	data = &Data{}
	errStr = data.getError()
	assert.Equal(t, "", errStr)
}

func TestDataGetStatesWithExtraInfo(t *testing.T) {
	t.Parallel()

	// Test with basic data
	data := &Data{
		DaemonVersion: "test-version",
		MacAddress:    "00:11:22:33:44:55",
		Annotations:   map[string]string{"key": "value"},
		healthy:       true,
		reason:        "test reason",
	}

	states, err := data.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	// Check extraInfo contains JSON data
	assert.Contains(t, states[0].DeprecatedExtraInfo, "data")
	assert.Contains(t, states[0].DeprecatedExtraInfo, "encoding")
	assert.Equal(t, "json", states[0].DeprecatedExtraInfo["encoding"])

	// Verify the JSON data contains our fields
	jsonData := states[0].DeprecatedExtraInfo["data"]
	assert.Contains(t, jsonData, "test-version")
	assert.Contains(t, jsonData, "00:11:22:33:44:55")
	assert.Contains(t, jsonData, "key")
	assert.Contains(t, jsonData, "value")
}
