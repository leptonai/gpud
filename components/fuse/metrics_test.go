package fuse

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/gpud-metrics/state"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestRegisterCollectors(t *testing.T) {
	t.Parallel()

	// Create test databases
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create component instance
	c := &component{}

	// Create prometheus registry
	reg := prometheus.NewRegistry()

	// Test table name
	tableName := "test_metrics"

	// Create metrics table
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := state.CreateTableMetrics(ctx, dbRW, tableName); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Register collectors
	err := c.RegisterCollectors(reg, dbRW, dbRO, tableName)
	require.NoError(t, err)

	// Set some test metrics
	deviceName := "test_device"
	now := time.Now()
	c.setLastUpdateUnixSeconds(float64(now.Unix()))
	c.setConnectionsCongestedPercent(ctx, deviceName, 75.0, now)
	c.setConnectionsMaxBackgroundPercent(ctx, deviceName, 80.0, now)

	// Verify collectors are registered
	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify specific metrics are registered
	metricNames := make(map[string]bool)
	for _, m := range metrics {
		metricNames[m.GetName()] = true
	}

	assert.True(t, metricNames["fuse_last_update_unix_seconds"], "last_update_unix_seconds metric should be registered")
	assert.True(t, metricNames["fuse_connections_congested_percent_against_threshold"], "connections_congested_percent metric should be registered")
	assert.True(t, metricNames["fuse_connections_max_background_percent_against_threshold"], "connections_max_background_percent metric should be registered")
}

func TestMetricsOperations(t *testing.T) {
	t.Parallel()

	// Create test databases
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create component instance
	c := &component{}

	// Create prometheus registry
	reg := prometheus.NewRegistry()

	// Test table name
	tableName := "test_metrics"

	// Create metrics table
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := state.CreateTableMetrics(ctx, dbRW, tableName); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Register collectors
	err := c.RegisterCollectors(reg, dbRW, dbRO, tableName)
	require.NoError(t, err)

	// Test setting metrics
	deviceName := "test_device"
	now := time.Now()

	// Set last update time
	c.setLastUpdateUnixSeconds(float64(now.Unix()))

	// Set FUSE metrics
	err = c.setConnectionsCongestedPercent(ctx, deviceName, 75.0, now)
	require.NoError(t, err)

	err = c.setConnectionsMaxBackgroundPercent(ctx, deviceName, 80.0, now)
	require.NoError(t, err)

	// Test reading metrics
	since := now.Add(-1 * time.Hour)
	metrics, err := c.Metrics(ctx, since)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify metrics values
	var foundCongestedPercent, foundMaxBackgroundPercent bool
	for _, m := range metrics {
		switch m.Metric.MetricName {
		case "fuse_connections_congested_percent_against_threshold":
			assert.Equal(t, 75.0, m.Metric.Value)
			assert.Equal(t, deviceName, m.Metric.MetricSecondaryName)
			foundCongestedPercent = true
		case "fuse_connections_max_background_percent_against_threshold":
			assert.Equal(t, 80.0, m.Metric.Value)
			assert.Equal(t, deviceName, m.Metric.MetricSecondaryName)
			foundMaxBackgroundPercent = true
		}
	}

	assert.True(t, foundCongestedPercent, "congested percent metric should be present")
	assert.True(t, foundMaxBackgroundPercent, "max background percent metric should be present")
}

func TestMetricsStoreOperations(t *testing.T) {
	t.Parallel()

	// Create test databases
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create component instance
	c := &component{}

	// Create prometheus registry
	reg := prometheus.NewRegistry()

	// Test table name
	tableName := "test_metrics"

	// Create metrics table
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := state.CreateTableMetrics(ctx, dbRW, tableName); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Register collectors
	err := c.RegisterCollectors(reg, dbRW, dbRO, tableName)
	require.NoError(t, err)

	// Test multiple observations
	deviceName := "test_device"
	now := time.Now()
	values := []float64{75.0, 85.0, 95.0}

	for i, v := range values {
		obsTime := now.Add(time.Duration(i) * time.Minute)
		err = c.setConnectionsCongestedPercent(ctx, deviceName, v, obsTime)
		require.NoError(t, err)
	}

	// Test reading metrics with time range
	since := now.Add(-2 * time.Hour)
	metrics, err := c.Metrics(ctx, since)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify all values are present
	metricValues := make(map[float64]bool)
	for _, m := range metrics {
		if m.Metric.MetricName == "fuse_connections_congested_percent_against_threshold" {
			metricValues[m.Metric.Value] = true
		}
	}

	for _, v := range values {
		assert.True(t, metricValues[v], "metric value %f should be present", v)
	}
}

func TestErrorCases(t *testing.T) {
	t.Parallel()

	// Create test databases
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create component instance
	c := &component{}

	// Create prometheus registry
	reg := prometheus.NewRegistry()

	// Test table name
	tableName := "test_metrics"

	// Create metrics table
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := state.CreateTableMetrics(ctx, dbRW, tableName); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Register collectors
	err := c.RegisterCollectors(reg, dbRW, dbRO, tableName)
	require.NoError(t, err)

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// Test setting metrics with canceled context
	deviceName := "test_device"
	now := time.Now()

	err = c.setConnectionsCongestedPercent(canceledCtx, deviceName, 75.0, now)
	assert.Error(t, err)

	err = c.setConnectionsMaxBackgroundPercent(canceledCtx, deviceName, 80.0, now)
	assert.Error(t, err)

	// Test reading metrics with canceled context
	_, err = c.Metrics(canceledCtx, time.Now().Add(-1*time.Hour))
	assert.Error(t, err)
}
