package latency

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
	providerRegion := "us-west-2"
	now := time.Now()
	c.setLastUpdateUnixSeconds(float64(now.Unix()))
	_ = c.setEdgeInMilliseconds(ctx, providerRegion, 50.0)

	// Verify collectors are registered
	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify specific metrics are registered
	metricNames := make(map[string]bool)
	for _, m := range metrics {
		metricNames[m.GetName()] = true
	}

	assert.True(t, metricNames["network_latency_last_update_unix_seconds"], "last_update_unix_seconds metric should be registered")
	assert.True(t, metricNames["network_latency_edge_in_milliseconds"], "edge_in_milliseconds metric should be registered")
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
	providerRegion := "us-west-2"
	now := time.Now()

	// Set last update time
	c.setLastUpdateUnixSeconds(float64(now.Unix()))

	// Set latency metrics
	err = c.setEdgeInMilliseconds(ctx, providerRegion, 50.0)
	require.NoError(t, err)

	// Test reading metrics
	since := now.Add(-1 * time.Hour)
	metrics, err := c.Metrics(ctx, since)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify metrics values
	var foundEdgeLatency bool
	for _, m := range metrics {
		switch m.Metric.MetricName {
		case "network_latency_edge_in_milliseconds":
			assert.Equal(t, 50.0, m.Metric.Value)
			assert.Equal(t, providerRegion, m.MetricSecondaryName)
			foundEdgeLatency = true
		}
	}

	assert.True(t, foundEdgeLatency, "edge latency metric should be present")
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
	providerRegion := "us-west-2"
	now := time.Now()
	values := []float64{50.0, 75.0, 100.0}

	for _, v := range values {
		time.Sleep(time.Second + 50*time.Millisecond)
		err = c.setEdgeInMilliseconds(ctx, providerRegion, v)
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
		if m.Metric.MetricName == "network_latency_edge_in_milliseconds" && m.MetricSecondaryName == providerRegion {
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
	providerRegion := "us-west-2"

	err = c.setEdgeInMilliseconds(canceledCtx, providerRegion, 50.0)
	assert.Error(t, err)

	// Test reading metrics with canceled context
	_, err = c.Metrics(canceledCtx, time.Now().Add(-1*time.Hour))
	assert.Error(t, err)
}
