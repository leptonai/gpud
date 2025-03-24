package memory

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
	now := time.Now()
	c.setLastUpdateUnixSeconds(float64(now.Unix()))
	c.setTotalBytes(ctx, 16000000000, now) // 16GB
	c.setAvailableBytes(4000000000)        // 4GB
	c.setUsedBytes(ctx, 12000000000, now)  // 12GB
	c.setUsedPercent(ctx, 75.0, now)
	c.setFreeBytes(4000000000) // 4GB

	// Verify collectors are registered
	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify specific metrics are registered
	metricNames := make(map[string]bool)
	for _, m := range metrics {
		metricNames[m.GetName()] = true
	}

	assert.True(t, metricNames["memory_last_update_unix_seconds"], "last_update_unix_seconds metric should be registered")
	assert.True(t, metricNames["memory_total_bytes"], "total_bytes metric should be registered")
	assert.True(t, metricNames["memory_available_bytes"], "available_bytes metric should be registered")
	assert.True(t, metricNames["memory_used_bytes"], "used_bytes metric should be registered")
	assert.True(t, metricNames["memory_used_percent"], "used_percent metric should be registered")
	assert.True(t, metricNames["memory_free_bytes"], "free_bytes metric should be registered")
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
	now := time.Now()

	// Set last update time
	c.setLastUpdateUnixSeconds(float64(now.Unix()))

	// Set memory metrics
	err = c.setTotalBytes(ctx, 16000000000, now) // 16GB
	require.NoError(t, err)

	c.setAvailableBytes(4000000000) // 4GB

	err = c.setUsedBytes(ctx, 12000000000, now) // 12GB
	require.NoError(t, err)

	err = c.setUsedPercent(ctx, 75.0, now)
	require.NoError(t, err)

	c.setFreeBytes(4000000000) // 4GB

	// Test reading metrics
	since := now.Add(-1 * time.Hour)
	metrics, err := c.Metrics(ctx, since)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify metrics values
	var foundTotalBytes, foundUsedBytes, foundUsedPercent bool
	for _, m := range metrics {
		switch m.Metric.MetricName {
		case "memory_total_bytes":
			assert.Equal(t, 16000000000.0, m.Metric.Value)
			foundTotalBytes = true
		case "memory_used_bytes":
			assert.Equal(t, 12000000000.0, m.Metric.Value)
			foundUsedBytes = true
		case "memory_used_percent":
			assert.Equal(t, 75.0, m.Metric.Value)
			foundUsedPercent = true
		}
	}

	assert.True(t, foundTotalBytes, "total bytes metric should be present")
	assert.True(t, foundUsedBytes, "used bytes metric should be present")
	assert.True(t, foundUsedPercent, "used percent metric should be present")
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
	now := time.Now()
	values := []float64{16000000000, 32000000000, 48000000000} // 16GB, 32GB, 48GB

	for i, v := range values {
		obsTime := now.Add(time.Duration(i) * time.Minute)
		err = c.setTotalBytes(ctx, v, obsTime)
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
		if m.Metric.MetricName == "memory_total_bytes" {
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
	now := time.Now()

	err = c.setTotalBytes(canceledCtx, 16000000000, now)
	assert.Error(t, err)

	err = c.setUsedBytes(canceledCtx, 12000000000, now)
	assert.Error(t, err)

	err = c.setUsedPercent(canceledCtx, 75.0, now)
	assert.Error(t, err)

	// Test reading metrics with canceled context
	_, err = c.Metrics(canceledCtx, time.Now().Add(-1*time.Hour))
	assert.Error(t, err)
}
