package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	"github.com/leptonai/gpud/pkg/sqlite"
)

const testTableName = "test_table"

func setupTestRegistry(t *testing.T) (*prometheus.Registry, func()) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)

	// Create the necessary table structure
	ctx := context.Background()
	err := components_metrics_state.CreateTableMetrics(ctx, dbRW, testTableName)
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	err = Register(reg, dbRW, dbRO, testTableName)
	require.NoError(t, err)
	return reg, cleanup
}

func TestRegister(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create the necessary table structure
	ctx := context.Background()
	err := components_metrics_state.CreateTableMetrics(ctx, dbRW, testTableName)
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	err = Register(reg, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	// Test double registration (should fail)
	err = Register(reg, dbRW, dbRO, testTableName)
	assert.Error(t, err)
}

func TestSetRemappedDueToUncorrectableErrors(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	ctx := context.Background()

	err := SetRemappedDueToUncorrectableErrors(ctx, "gpu1", 42, time.Now())
	require.NoError(t, err)

	metrics, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, m := range metrics {
		if m.GetName() == "accelerator_nvidia_remapped_rows_due_to_uncorrectable_errors" {
			found = true
			for _, metric := range m.GetMetric() {
				if getLabel(metric.GetLabel(), "gpu_id") == "gpu1" {
					assert.Equal(t, float64(42), metric.GetGauge().GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Uncorrectable errors metric not found")
}

func TestSetRemappingPending(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	ctx := context.Background()

	err := SetRemappingPending(ctx, "gpu1", true, time.Now())
	require.NoError(t, err)

	metrics, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, m := range metrics {
		if m.GetName() == "accelerator_nvidia_remapped_rows_remapping_pending" {
			found = true
			for _, metric := range m.GetMetric() {
				if getLabel(metric.GetLabel(), "gpu_id") == "gpu1" {
					assert.Equal(t, float64(1), metric.GetGauge().GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Remapping pending metric not found")

	// Test setting to false
	err = SetRemappingPending(ctx, "gpu1", false, time.Now())
	require.NoError(t, err)

	metrics, err = reg.Gather()
	require.NoError(t, err)
	for _, m := range metrics {
		if m.GetName() == "accelerator_nvidia_remapped_rows_remapping_pending" {
			for _, metric := range m.GetMetric() {
				if getLabel(metric.GetLabel(), "gpu_id") == "gpu1" {
					assert.Equal(t, float64(0), metric.GetGauge().GetValue())
				}
			}
		}
	}
}

func TestSetRemappingFailed(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	ctx := context.Background()

	err := SetRemappingFailed(ctx, "gpu1", true, time.Now())
	require.NoError(t, err)

	metrics, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, m := range metrics {
		if m.GetName() == "accelerator_nvidia_remapped_rows_remapping_failed" {
			found = true
			for _, metric := range m.GetMetric() {
				if getLabel(metric.GetLabel(), "gpu_id") == "gpu1" {
					assert.Equal(t, float64(1), metric.GetGauge().GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Remapping failed metric not found")

	// Test setting to false
	err = SetRemappingFailed(ctx, "gpu1", false, time.Now())
	require.NoError(t, err)

	metrics, err = reg.Gather()
	require.NoError(t, err)
	for _, m := range metrics {
		if m.GetName() == "accelerator_nvidia_remapped_rows_remapping_failed" {
			for _, metric := range m.GetMetric() {
				if getLabel(metric.GetLabel(), "gpu_id") == "gpu1" {
					assert.Equal(t, float64(0), metric.GetGauge().GetValue())
				}
			}
		}
	}
}

func TestReadMetrics(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create the necessary table structure
	ctx := context.Background()
	err := components_metrics_state.CreateTableMetrics(ctx, dbRW, testTableName)
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	err = Register(reg, dbRW, dbRO, testTableName)
	require.NoError(t, err)

	// Create a timestamp for the metrics
	now := time.Now()
	since := now.Add(-1 * time.Hour)

	// Insert some test data directly using the state package
	metric := components_metrics_state.Metric{
		UnixSeconds:         now.Unix(),
		MetricName:          SubSystem + "_due_to_uncorrectable_errors",
		MetricSecondaryName: "gpu1",
		Value:               42.0,
	}
	err = components_metrics_state.InsertMetric(ctx, dbRW, testTableName, metric)
	require.NoError(t, err)

	// Test reading uncorrectable errors
	metrics, err := ReadRemappedDueToUncorrectableErrors(ctx, since)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Greater(t, len(metrics), 0)

	// Test reading remapping pending
	metrics, err = ReadRemappingPending(ctx, since)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)

	// Test reading remapping failed
	metrics, err = ReadRemappingFailed(ctx, since)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)
}

// Helper function to get label value from prometheus labels
func getLabel(labels []*dto.LabelPair, name string) string {
	for _, label := range labels {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}
