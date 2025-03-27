package disk

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
	mountPoint := "/test/mount"
	now := time.Now()
	c.setLastUpdateUnixSeconds(float64(now.Unix()))
	_ = c.setTotalBytes(ctx, mountPoint, 1000000, now)
	c.setFreeBytes(mountPoint, 500000)
	_ = c.setUsedBytes(ctx, mountPoint, 500000, now)
	_ = c.setUsedBytesPercent(ctx, mountPoint, 50.0, now)
	c.setUsedInodesPercent(mountPoint, 45.0)

	// Verify collectors are registered
	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify specific metrics are registered
	metricNames := make(map[string]bool)
	for _, m := range metrics {
		metricNames[m.GetName()] = true
	}

	assert.True(t, metricNames["disk_last_update_unix_seconds"], "last_update_unix_seconds metric should be registered")
	assert.True(t, metricNames["disk_total_bytes"], "total_bytes metric should be registered")
	assert.True(t, metricNames["disk_free_bytes"], "free_bytes metric should be registered")
	assert.True(t, metricNames["disk_used_bytes"], "used_bytes metric should be registered")
	assert.True(t, metricNames["disk_used_bytes_percent"], "used_bytes_percent metric should be registered")
	assert.True(t, metricNames["disk_used_inodes_percent"], "used_inodes_percent metric should be registered")
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
	mountPoint := "/test/mount"
	now := time.Now()

	// Set last update time
	c.setLastUpdateUnixSeconds(float64(now.Unix()))

	// Set disk metrics
	err = c.setTotalBytes(ctx, mountPoint, 1000000, now)
	require.NoError(t, err)

	c.setFreeBytes(mountPoint, 500000)

	err = c.setUsedBytes(ctx, mountPoint, 500000, now)
	require.NoError(t, err)

	err = c.setUsedBytesPercent(ctx, mountPoint, 50.0, now)
	require.NoError(t, err)

	c.setUsedInodesPercent(mountPoint, 45.0)

	// Test reading metrics
	since := now.Add(-1 * time.Hour)
	metrics, err := c.Metrics(ctx, since)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify metrics values
	var foundTotalBytes, foundUsedBytes, foundUsedPercent bool
	for _, m := range metrics {
		switch m.Metric.MetricName {
		case "disk_total_bytes":
			assert.Equal(t, 1000000.0, m.Metric.Value)
			assert.Equal(t, mountPoint, m.ExtraInfo["mount_point"])
			foundTotalBytes = true
		case "disk_used_bytes":
			assert.Equal(t, 500000.0, m.Metric.Value)
			assert.Equal(t, mountPoint, m.ExtraInfo["mount_point"])
			foundUsedBytes = true
		case "disk_used_bytes_percent":
			assert.Equal(t, 50.0, m.Metric.Value)
			assert.Equal(t, mountPoint, m.ExtraInfo["mount_point"])
			foundUsedPercent = true
		}
	}

	assert.True(t, foundTotalBytes, "total bytes metric should be present")
	assert.True(t, foundUsedBytes, "used bytes metric should be present")
	assert.True(t, foundUsedPercent, "used bytes percent metric should be present")
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
	mountPoint := "/test/mount"
	now := time.Now()
	values := []float64{1000000, 2000000, 3000000}

	for i, v := range values {
		obsTime := now.Add(time.Duration(i) * time.Minute)
		err = c.setTotalBytes(ctx, mountPoint, v, obsTime)
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
		if m.Metric.MetricName == "disk_total_bytes" {
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
	mountPoint := "/test/mount"
	now := time.Now()

	err = c.setTotalBytes(canceledCtx, mountPoint, 1000000, now)
	assert.Error(t, err)

	err = c.setUsedBytes(canceledCtx, mountPoint, 500000, now)
	assert.Error(t, err)

	err = c.setUsedBytesPercent(canceledCtx, mountPoint, 50.0, now)
	assert.Error(t, err)

	// Test reading metrics with canceled context
	_, err = c.Metrics(canceledCtx, time.Now().Add(-1*time.Hour))
	assert.Error(t, err)
}
