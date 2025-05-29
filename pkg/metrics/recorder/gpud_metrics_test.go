package recorder

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
)

func TestRecordFileDescriptorUsage_Success(t *testing.T) {
	t.Parallel()

	// Create a local gauge for testing
	testGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "test",
			Subsystem: "file_descriptor",
			Name:      "usage_total",
			Help:      "test gauge for file descriptor usage",
		},
	)

	// Mock function that returns a successful result
	mockGetUsage := func() (uint64, error) {
		return 42, nil
	}

	err := recordFileDescriptorUsage(mockGetUsage, testGauge)
	require.NoError(t, err)

	// Verify the gauge was set correctly
	var metric dto.Metric
	require.NoError(t, testGauge.Write(&metric))
	require.Equal(t, float64(42), metric.GetGauge().GetValue())
}

func TestRecordFileDescriptorUsage_Error(t *testing.T) {
	t.Parallel()

	// Create a local gauge for testing
	testGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "test",
			Subsystem: "file_descriptor",
			Name:      "usage_total",
			Help:      "test gauge for file descriptor usage",
		},
	)

	// Mock function that returns an error
	expectedErr := errors.New("failed to get file descriptor usage")
	mockGetUsage := func() (uint64, error) {
		return 0, expectedErr
	}

	err := recordFileDescriptorUsage(mockGetUsage, testGauge)
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestRecordSQLiteDBSize_Success(t *testing.T) {
	t.Parallel()

	// Create test database
	dbRW, dbRO, cleanup := pkgsqlite.OpenTestDB(t)
	defer cleanup()

	// Create a test table to ensure the database has some content
	_, err := dbRW.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, data TEXT)")
	require.NoError(t, err)
	_, err = dbRW.Exec("INSERT INTO test_table (data) VALUES ('test data')")
	require.NoError(t, err)

	// Create a local gauge for testing
	testGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "test",
			Subsystem: "sqlite_db",
			Name:      "size_bytes",
			Help:      "test gauge for database size",
		},
	)

	ctx := context.Background()
	err = recordSQLiteDBSize(ctx, dbRO, testGauge)
	require.NoError(t, err)

	// Verify the gauge was set to a positive value
	var metric dto.Metric
	require.NoError(t, testGauge.Write(&metric))
	require.Greater(t, metric.GetGauge().GetValue(), float64(0))
}

func TestRecordSQLiteDBSize_Error(t *testing.T) {
	t.Parallel()

	// Create test database and then close it to trigger an error
	_, dbRO, cleanup := pkgsqlite.OpenTestDB(t)
	cleanup() // Close the database connections

	// Create a local gauge for testing
	testGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "test",
			Subsystem: "sqlite_db",
			Name:      "size_bytes",
			Help:      "test gauge for database size",
		},
	)

	// Use the closed database to trigger an error
	ctx := context.Background()
	err := recordSQLiteDBSize(ctx, dbRO, testGauge)
	require.Error(t, err)
}

func TestRecordSQLiteTotalAndSeconds(t *testing.T) {
	t.Parallel()

	// Create local counters for testing
	testTotalCounter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "test",
			Subsystem: "sqlite_operation",
			Name:      "total",
			Help:      "test counter for total operations",
		},
	)

	testSecondsCounter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "test",
			Subsystem: "sqlite_operation",
			Name:      "seconds_total",
			Help:      "test counter for total seconds",
		},
	)

	// Record some test values
	recordSQLiteTotalAndSeconds(1.5, testTotalCounter, testSecondsCounter)

	// Verify the counters were incremented correctly
	var totalMetric dto.Metric
	require.NoError(t, testTotalCounter.Write(&totalMetric))
	require.Equal(t, float64(1), totalMetric.GetCounter().GetValue())

	var secondsMetric dto.Metric
	require.NoError(t, testSecondsCounter.Write(&secondsMetric))
	require.Equal(t, 1.5, secondsMetric.GetCounter().GetValue())

	// Record additional values to test accumulation
	recordSQLiteTotalAndSeconds(2.5, testTotalCounter, testSecondsCounter)

	require.NoError(t, testTotalCounter.Write(&totalMetric))
	require.Equal(t, float64(2), totalMetric.GetCounter().GetValue())

	require.NoError(t, testSecondsCounter.Write(&secondsMetric))
	require.Equal(t, 4.0, secondsMetric.GetCounter().GetValue())
}

func TestRecordSQLiteSelect(t *testing.T) {
	// Test that RecordSQLiteSelect doesn't panic and works correctly
	// We can't easily test the global metrics without overwriting them,
	// so we just ensure the function executes without error
	require.NotPanics(t, func() {
		RecordSQLiteSelect(2.5)
	})
}

func TestRecordSQLiteInsertUpdate(t *testing.T) {
	// Test that RecordSQLiteInsertUpdate doesn't panic and works correctly
	require.NotPanics(t, func() {
		RecordSQLiteInsertUpdate(3.7)
	})
}

func TestRecordSQLiteDelete(t *testing.T) {
	// Test that RecordSQLiteDelete doesn't panic and works correctly
	require.NotPanics(t, func() {
		RecordSQLiteDelete(1.2)
	})
}

func TestRecordSQLiteVacuum(t *testing.T) {
	// Test that RecordSQLiteVacuum doesn't panic and works correctly
	require.NotPanics(t, func() {
		RecordSQLiteVacuum(5.8)
	})
}

func TestRecordSQLiteTotalAndSeconds_ZeroValues(t *testing.T) {
	t.Parallel()

	// Create local counters for testing
	testTotalCounter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "test",
			Subsystem: "sqlite_operation",
			Name:      "total_zero",
			Help:      "test counter for total operations with zero values",
		},
	)

	testSecondsCounter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "test",
			Subsystem: "sqlite_operation",
			Name:      "seconds_total_zero",
			Help:      "test counter for total seconds with zero values",
		},
	)

	// Record zero values
	recordSQLiteTotalAndSeconds(0.0, testTotalCounter, testSecondsCounter)

	// Verify the counters were incremented correctly
	var totalMetric dto.Metric
	require.NoError(t, testTotalCounter.Write(&totalMetric))
	require.Equal(t, float64(1), totalMetric.GetCounter().GetValue())

	var secondsMetric dto.Metric
	require.NoError(t, testSecondsCounter.Write(&secondsMetric))
	require.Equal(t, 0.0, secondsMetric.GetCounter().GetValue())
}

func TestRecordSQLiteSelect_MultipleRecords(t *testing.T) {
	// Test that multiple calls to RecordSQLiteSelect work correctly
	require.NotPanics(t, func() {
		RecordSQLiteSelect(1.0)
		RecordSQLiteSelect(2.0)
		RecordSQLiteSelect(0.5)
	})
}

func TestRecordSQLiteInsertUpdate_MultipleRecords(t *testing.T) {
	// Test that multiple calls to RecordSQLiteInsertUpdate work correctly
	require.NotPanics(t, func() {
		RecordSQLiteInsertUpdate(0.1)
		RecordSQLiteInsertUpdate(10.5)
		RecordSQLiteInsertUpdate(0.0)
	})
}

func TestRecordSQLiteDelete_MultipleRecords(t *testing.T) {
	// Test that multiple calls to RecordSQLiteDelete work correctly
	require.NotPanics(t, func() {
		RecordSQLiteDelete(2.3)
		RecordSQLiteDelete(0.8)
	})
}

func TestRecordSQLiteVacuum_MultipleRecords(t *testing.T) {
	// Test that multiple calls to RecordSQLiteVacuum work correctly
	require.NotPanics(t, func() {
		RecordSQLiteVacuum(15.7)
		RecordSQLiteVacuum(0.1)
	})
}

func TestRecordFileDescriptorUsage_LargeValue(t *testing.T) {
	t.Parallel()

	// Create a local gauge for testing
	testGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "test",
			Subsystem: "file_descriptor",
			Name:      "usage_large",
			Help:      "test gauge for large file descriptor usage",
		},
	)

	// Mock function that returns a large value
	mockGetUsage := func() (uint64, error) {
		return 999999, nil
	}

	err := recordFileDescriptorUsage(mockGetUsage, testGauge)
	require.NoError(t, err)

	// Verify the gauge was set correctly
	var metric dto.Metric
	require.NoError(t, testGauge.Write(&metric))
	require.Equal(t, float64(999999), metric.GetGauge().GetValue())
}

func TestRecordFileDescriptorUsage_ZeroValue(t *testing.T) {
	t.Parallel()

	// Create a local gauge for testing
	testGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "test",
			Subsystem: "file_descriptor",
			Name:      "usage_zero",
			Help:      "test gauge for zero file descriptor usage",
		},
	)

	// Mock function that returns zero
	mockGetUsage := func() (uint64, error) {
		return 0, nil
	}

	err := recordFileDescriptorUsage(mockGetUsage, testGauge)
	require.NoError(t, err)

	// Verify the gauge was set correctly
	var metric dto.Metric
	require.NoError(t, testGauge.Write(&metric))
	require.Equal(t, float64(0), metric.GetGauge().GetValue())
}
