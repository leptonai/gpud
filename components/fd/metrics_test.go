package fd

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
	_ = c.setAllocatedFileHandles(ctx, 1000, now)
	_ = c.setRunningPIDs(ctx, 500, now)
	_ = c.setLimit(ctx, 2000, now)
	_ = c.setAllocatedFileHandlesPercent(ctx, 50.0, now)
	_ = c.setUsedPercent(ctx, 25.0, now)
	c.setThresholdRunningPIDs(1500)
	_ = c.setThresholdRunningPIDsPercent(ctx, 75.0, now)
	_ = c.setThresholdAllocatedFileHandles(ctx, 1800)
	_ = c.setThresholdAllocatedFileHandlesPercent(ctx, 90.0, now)

	// Verify collectors are registered
	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify specific metrics are registered
	metricNames := make(map[string]bool)
	for _, m := range metrics {
		metricNames[m.GetName()] = true
	}

	assert.True(t, metricNames["fd_last_update_unix_seconds"], "last_update_unix_seconds metric should be registered")
	assert.True(t, metricNames["fd_allocated_file_handles"], "allocated_file_handles metric should be registered")
	assert.True(t, metricNames["fd_running_pids"], "running_pids metric should be registered")
	assert.True(t, metricNames["fd_limit"], "limit metric should be registered")
	assert.True(t, metricNames["fd_allocated_file_handles_percent"], "allocated_file_handles_percent metric should be registered")
	assert.True(t, metricNames["fd_used_percent"], "used_percent metric should be registered")
	assert.True(t, metricNames["fd_threshold_running_pids"], "threshold_running_pids metric should be registered")
	assert.True(t, metricNames["fd_threshold_running_pids_percent"], "threshold_running_pids_percent metric should be registered")
	assert.True(t, metricNames["fd_threshold_allocated_file_handles"], "threshold_allocated_file_handles metric should be registered")
	assert.True(t, metricNames["fd_threshold_allocated_file_handles_percent"], "threshold_allocated_file_handles_percent metric should be registered")
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

	// Set fd metrics
	err = c.setAllocatedFileHandles(ctx, 1000, now)
	require.NoError(t, err)

	err = c.setRunningPIDs(ctx, 500, now)
	require.NoError(t, err)

	err = c.setLimit(ctx, 2000, now)
	require.NoError(t, err)

	err = c.setAllocatedFileHandlesPercent(ctx, 50.0, now)
	require.NoError(t, err)

	err = c.setUsedPercent(ctx, 25.0, now)
	require.NoError(t, err)

	c.setThresholdRunningPIDs(1500)

	err = c.setThresholdRunningPIDsPercent(ctx, 75.0, now)
	require.NoError(t, err)

	err = c.setThresholdAllocatedFileHandles(ctx, 1800)
	require.NoError(t, err)

	err = c.setThresholdAllocatedFileHandlesPercent(ctx, 90.0, now)
	require.NoError(t, err)

	// Test reading metrics
	since := now.Add(-1 * time.Hour)
	metrics, err := c.Metrics(ctx, since)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify metrics values
	var foundAllocatedHandles, foundRunningPIDs, foundLimit, foundAllocatedPercent, foundUsedPercent bool
	for _, m := range metrics {
		switch m.Metric.MetricName {
		case "fd_allocated_file_handles":
			assert.Equal(t, 1000.0, m.Metric.Value)
			foundAllocatedHandles = true
		case "fd_running_pids":
			assert.Equal(t, 500.0, m.Metric.Value)
			foundRunningPIDs = true
		case "fd_limit":
			assert.Equal(t, 2000.0, m.Metric.Value)
			foundLimit = true
		case "fd_allocated_file_handles_percent":
			assert.Equal(t, 50.0, m.Metric.Value)
			foundAllocatedPercent = true
		case "fd_used_percent":
			assert.Equal(t, 25.0, m.Metric.Value)
			foundUsedPercent = true
		}
	}

	assert.True(t, foundAllocatedHandles, "allocated file handles metric should be present")
	assert.True(t, foundRunningPIDs, "running pids metric should be present")
	assert.True(t, foundLimit, "limit metric should be present")
	assert.True(t, foundAllocatedPercent, "allocated percent metric should be present")
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
	values := []float64{1000, 2000, 3000}

	for i, v := range values {
		obsTime := now.Add(time.Duration(i) * time.Minute)
		err = c.setAllocatedFileHandles(ctx, v, obsTime)
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
		if m.Metric.MetricName == "fd_allocated_file_handles" {
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

	err = c.setAllocatedFileHandles(canceledCtx, 1000, now)
	assert.Error(t, err)

	err = c.setRunningPIDs(canceledCtx, 500, now)
	assert.Error(t, err)

	err = c.setLimit(canceledCtx, 2000, now)
	assert.Error(t, err)

	err = c.setAllocatedFileHandlesPercent(canceledCtx, 50.0, now)
	assert.Error(t, err)

	err = c.setUsedPercent(canceledCtx, 25.0, now)
	assert.Error(t, err)

	err = c.setThresholdRunningPIDsPercent(canceledCtx, 75.0, now)
	assert.Error(t, err)

	err = c.setThresholdAllocatedFileHandlesPercent(canceledCtx, 90.0, now)
	assert.Error(t, err)

	// Test reading metrics with canceled context
	_, err = c.Metrics(canceledCtx, time.Now().Add(-1*time.Hour))
	assert.Error(t, err)
}
