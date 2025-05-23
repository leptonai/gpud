package recorder

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestNewPrometheusRecorder(t *testing.T) {
	t.Parallel()

	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	t.Run("valid_parameters", func(t *testing.T) {
		recorderInterval := 5 * time.Minute
		recorder := NewPrometheusRecorder(ctx, recorderInterval, dbRO)

		require.NotNil(t, recorder)

		// Type assertion to access internal fields
		promRecorder, ok := recorder.(*promRecorder)
		require.True(t, ok)

		assert.Equal(t, ctx, promRecorder.ctx)
		assert.Equal(t, recorderInterval, promRecorder.recorderInterval)
		assert.Equal(t, dbRO, promRecorder.dbRO)
		assert.NotNil(t, promRecorder.getCurrentProcessUsageFunc)
		assert.NotNil(t, promRecorder.gaugeFileDescriptorUsage)
		assert.NotNil(t, promRecorder.gaugeSQLiteDBSizeInBytes)
	})

	t.Run("with_nil_database", func(t *testing.T) {
		recorder := NewPrometheusRecorder(ctx, time.Minute, nil)
		require.NotNil(t, recorder)

		promRecorder, ok := recorder.(*promRecorder)
		require.True(t, ok)
		assert.Nil(t, promRecorder.dbRO)
	})

	t.Run("with_zero_interval", func(t *testing.T) {
		recorder := NewPrometheusRecorder(ctx, 0, dbRO)
		require.NotNil(t, recorder)

		promRecorder, ok := recorder.(*promRecorder)
		require.True(t, ok)
		assert.Equal(t, time.Duration(0), promRecorder.recorderInterval)
	})
}

func TestPromRecorder_Interfaces(t *testing.T) {
	t.Parallel()

	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx := context.Background()
	recorder := NewPrometheusRecorder(ctx, time.Minute, dbRO)

	// Verify it implements the Recorder interface
	var _ pkgmetrics.Recorder = recorder
}

func TestPromRecorder_Record(t *testing.T) {
	t.Parallel()

	t.Run("successful_recording", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// Set up test database with some data to make it non-empty
		_, err := dbRW.ExecContext(ctx, "CREATE TABLE test (id INTEGER, value TEXT)")
		require.NoError(t, err)
		_, err = dbRW.ExecContext(ctx, "INSERT INTO test (id, value) VALUES (1, 'test')")
		require.NoError(t, err)

		// Create a mock function that returns a predictable value
		mockGetCurrentProcessUsage := func() (uint64, error) {
			return 42, nil
		}

		// Create custom gauges for testing
		testFileDescriptorGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_file_descriptor_usage",
			Help: "test file descriptor usage",
		})
		testSQLiteDBSizeGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_sqlite_db_size",
			Help: "test sqlite db size",
		})

		recorder := &promRecorder{
			ctx:                        ctx,
			recorderInterval:           time.Minute,
			dbRO:                       dbRO,
			getCurrentProcessUsageFunc: mockGetCurrentProcessUsage,
			gaugeFileDescriptorUsage:   testFileDescriptorGauge,
			gaugeSQLiteDBSizeInBytes:   testSQLiteDBSizeGauge,
		}

		err = recorder.record(ctx)
		assert.NoError(t, err)

		// Verify the gauges were set
		var metric dto.Metric
		err = testFileDescriptorGauge.Write(&metric)
		require.NoError(t, err)
		assert.Equal(t, float64(42), metric.GetGauge().GetValue())

		err = testSQLiteDBSizeGauge.Write(&metric)
		require.NoError(t, err)
		assert.Greater(t, metric.GetGauge().GetValue(), float64(0))
	})

	t.Run("nil_recorder", func(t *testing.T) {
		var recorder *promRecorder
		ctx := context.Background()
		err := recorder.record(ctx)
		assert.NoError(t, err) // Should not error for nil receiver
	})

	t.Run("nil_database", func(t *testing.T) {
		ctx := context.Background()
		recorder := &promRecorder{
			ctx:  ctx,
			dbRO: nil,
		}
		err := recorder.record(ctx)
		assert.NoError(t, err) // Should not error for nil database
	})

	t.Run("file_descriptor_usage_error", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// Mock function that returns an error
		mockGetCurrentProcessUsageError := func() (uint64, error) {
			return 0, errors.New("mock file descriptor error")
		}

		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_gauge",
			Help: "test gauge",
		})

		recorder := &promRecorder{
			ctx:                        ctx,
			dbRO:                       dbRO,
			getCurrentProcessUsageFunc: mockGetCurrentProcessUsageError,
			gaugeFileDescriptorUsage:   testGauge,
			gaugeSQLiteDBSizeInBytes:   testGauge,
		}

		err := recorder.record(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mock file descriptor error")
	})

	t.Run("sqlite_db_size_error", func(t *testing.T) {
		// Use a closed database to trigger an error
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		cleanup() // Close the database immediately

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		mockGetCurrentProcessUsage := func() (uint64, error) {
			return 42, nil
		}

		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_gauge",
			Help: "test gauge",
		})

		recorder := &promRecorder{
			ctx:                        ctx,
			dbRO:                       dbRO,
			getCurrentProcessUsageFunc: mockGetCurrentProcessUsage,
			gaugeFileDescriptorUsage:   testGauge,
			gaugeSQLiteDBSizeInBytes:   testGauge,
		}

		err := recorder.record(ctx)
		assert.Error(t, err)
	})

	t.Run("canceled_context", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		mockGetCurrentProcessUsage := func() (uint64, error) {
			return 42, nil
		}

		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_gauge",
			Help: "test gauge",
		})

		recorder := &promRecorder{
			ctx:                        ctx,
			dbRO:                       dbRO,
			getCurrentProcessUsageFunc: mockGetCurrentProcessUsage,
			gaugeFileDescriptorUsage:   testGauge,
			gaugeSQLiteDBSizeInBytes:   testGauge,
		}

		err := recorder.record(ctx)
		assert.Error(t, err)
	})
}

func TestPromRecorder_Start(t *testing.T) {
	t.Parallel()

	t.Run("starts_goroutine", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Use a very short interval for testing
		recorderInterval := 100 * time.Millisecond

		// Create recorder with mock function that tracks calls
		var callCount int64
		mockGetCurrentProcessUsage := func() (uint64, error) {
			atomic.AddInt64(&callCount, 1)
			return uint64(atomic.LoadInt64(&callCount)), nil
		}

		testFileDescriptorGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_start_file_descriptor_usage",
			Help: "test file descriptor usage for start test",
		})
		testSQLiteDBSizeGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_start_sqlite_db_size",
			Help: "test sqlite db size for start test",
		})

		recorder := &promRecorder{
			ctx:                        ctx,
			recorderInterval:           recorderInterval,
			dbRO:                       dbRO,
			getCurrentProcessUsageFunc: mockGetCurrentProcessUsage,
			gaugeFileDescriptorUsage:   testFileDescriptorGauge,
			gaugeSQLiteDBSizeInBytes:   testSQLiteDBSizeGauge,
		}

		// Start the recorder
		recorder.Start()

		// Wait for a few recording cycles
		time.Sleep(500 * time.Millisecond)

		// Verify that recording happened multiple times
		finalCallCount := atomic.LoadInt64(&callCount)
		assert.Greater(t, finalCallCount, int64(1), "Expected multiple recording cycles")
	})

	t.Run("stops_on_context_cancellation", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())

		recorderInterval := 50 * time.Millisecond

		var callCount int64
		mockGetCurrentProcessUsage := func() (uint64, error) {
			atomic.AddInt64(&callCount, 1)
			return uint64(atomic.LoadInt64(&callCount)), nil
		}

		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_cancel_gauge",
			Help: "test gauge for cancellation test",
		})

		recorder := &promRecorder{
			ctx:                        ctx,
			recorderInterval:           recorderInterval,
			dbRO:                       dbRO,
			getCurrentProcessUsageFunc: mockGetCurrentProcessUsage,
			gaugeFileDescriptorUsage:   testGauge,
			gaugeSQLiteDBSizeInBytes:   testGauge,
		}

		// Start the recorder
		recorder.Start()

		// Let it run for a bit
		time.Sleep(200 * time.Millisecond)
		initialCallCount := atomic.LoadInt64(&callCount)

		// Cancel the context
		cancel()

		// Wait a bit more
		time.Sleep(200 * time.Millisecond)
		finalCallCount := atomic.LoadInt64(&callCount)

		// Should have stopped increasing after cancellation
		assert.Greater(t, initialCallCount, int64(0), "Should have made some calls before cancellation")
		// Allow for one additional call due to timing, but should stop soon after
		assert.LessOrEqual(t, finalCallCount-initialCallCount, int64(2), "Should have stopped making calls after cancellation")
	})

	t.Run("handles_recording_errors", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		recorderInterval := 100 * time.Millisecond

		// Mock function that always returns an error
		mockGetCurrentProcessUsageError := func() (uint64, error) {
			return 0, errors.New("mock error")
		}

		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_error_gauge",
			Help: "test gauge for error handling test",
		})

		recorder := &promRecorder{
			ctx:                        ctx,
			recorderInterval:           recorderInterval,
			dbRO:                       dbRO,
			getCurrentProcessUsageFunc: mockGetCurrentProcessUsageError,
			gaugeFileDescriptorUsage:   testGauge,
			gaugeSQLiteDBSizeInBytes:   testGauge,
		}

		// Start the recorder - should not panic even with errors
		recorder.Start()

		// Wait for the context to timeout
		<-ctx.Done()

		// Test should complete without panicking
	})
}

func TestRecordFileDescriptorUsage(t *testing.T) {
	t.Parallel()

	t.Run("successful_recording", func(t *testing.T) {
		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_fd_usage",
			Help: "test file descriptor usage",
		})

		mockFunc := func() (uint64, error) {
			return 123, nil
		}

		err := recordFileDescriptorUsage(mockFunc, testGauge)
		assert.NoError(t, err)

		// Verify the gauge was set correctly
		var metric dto.Metric
		err = testGauge.Write(&metric)
		require.NoError(t, err)
		assert.Equal(t, float64(123), metric.GetGauge().GetValue())
	})

	t.Run("function_returns_error", func(t *testing.T) {
		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_fd_usage_error",
			Help: "test file descriptor usage with error",
		})

		mockFuncError := func() (uint64, error) {
			return 0, errors.New("mock fd error")
		}

		err := recordFileDescriptorUsage(mockFuncError, testGauge)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mock fd error")
	})
}

func TestRecordSQLiteDBSize(t *testing.T) {
	t.Parallel()

	t.Run("successful_recording", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// Create some data in the database
		_, err := dbRW.ExecContext(ctx, "CREATE TABLE test (id INTEGER, data TEXT)")
		require.NoError(t, err)
		_, err = dbRW.ExecContext(ctx, "INSERT INTO test (id, data) VALUES (1, 'test data')")
		require.NoError(t, err)

		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_db_size",
			Help: "test database size",
		})

		err = recordSQLiteDBSize(ctx, dbRO, testGauge)
		assert.NoError(t, err)

		// Verify the gauge was set to a positive value
		var metric dto.Metric
		err = testGauge.Write(&metric)
		require.NoError(t, err)
		assert.Greater(t, metric.GetGauge().GetValue(), float64(0))
	})

	t.Run("database_error", func(t *testing.T) {
		// Use a closed database to trigger an error
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		cleanup() // Close immediately

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_db_size_error",
			Help: "test database size with error",
		})

		err := recordSQLiteDBSize(ctx, dbRO, testGauge)
		assert.Error(t, err)
	})

	t.Run("canceled_context", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_db_size_canceled",
			Help: "test database size with canceled context",
		})

		err := recordSQLiteDBSize(ctx, dbRO, testGauge)
		assert.Error(t, err)
	})
}

func TestPromRecorder_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("zero_interval_behavior", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		var callCount int64
		mockGetCurrentProcessUsage := func() (uint64, error) {
			atomic.AddInt64(&callCount, 1)
			return uint64(atomic.LoadInt64(&callCount)), nil
		}

		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_zero_interval_gauge",
			Help: "test gauge for zero interval test",
		})

		recorder := &promRecorder{
			ctx:                        ctx,
			recorderInterval:           0, // Zero interval
			dbRO:                       dbRO,
			getCurrentProcessUsageFunc: mockGetCurrentProcessUsage,
			gaugeFileDescriptorUsage:   testGauge,
			gaugeSQLiteDBSizeInBytes:   testGauge,
		}

		recorder.Start()

		// Wait for timeout
		<-ctx.Done()

		// With zero interval, it should still work but record very frequently
		finalCallCount := atomic.LoadInt64(&callCount)
		assert.Greater(t, finalCallCount, int64(0), "Should have made at least one call")
	})

	t.Run("very_large_file_descriptor_count", func(t *testing.T) {
		testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_large_fd_count",
			Help: "test large file descriptor count",
		})

		// Test with a very large number
		largeCount := uint64(1<<63 - 1) // Max int64 value
		mockFunc := func() (uint64, error) {
			return largeCount, nil
		}

		err := recordFileDescriptorUsage(mockFunc, testGauge)
		assert.NoError(t, err)

		var metric dto.Metric
		err = testGauge.Write(&metric)
		require.NoError(t, err)
		assert.Equal(t, float64(largeCount), metric.GetGauge().GetValue())
	})
}
