package nvml

import (
	"database/sql"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

type mockEventBucket struct {
	eventstore.Bucket
}

func TestOpApplyOpts(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts(nil)
		assert.NoError(t, err)

		// Check default databases are created as in-memory
		assert.NotNil(t, op.dbRW)
		assert.NotNil(t, op.dbRO)
	})

	t.Run("with custom databases", func(t *testing.T) {
		dbRW, err := sqlite.Open(":memory:")
		assert.NoError(t, err)
		defer dbRW.Close()

		dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
		assert.NoError(t, err)
		defer dbRO.Close()

		op := &Op{}
		err = op.applyOpts([]OpOption{
			WithDBRW(dbRW),
			WithDBRO(dbRO),
		})
		assert.NoError(t, err)

		assert.Equal(t, dbRW, op.dbRW)
		assert.Equal(t, dbRO, op.dbRO)
	})

	t.Run("with events bucket", func(t *testing.T) {
		bucket := &mockEventBucket{}
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithHWSlowdownEventBucket(bucket),
		})
		assert.NoError(t, err)
		assert.Equal(t, bucket, op.hwSlowdownEventBucket)
	})

	t.Run("with GPM metrics", func(t *testing.T) {
		metrics := []nvml.GpmMetricId{
			nvml.GPM_METRIC_SM_OCCUPANCY,
			nvml.GPM_METRIC_FP32_UTIL,
		}
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithGPMMetricsID(metrics...),
		})
		assert.NoError(t, err)

		// Verify all metrics are in the map
		assert.Len(t, op.gpmMetricsIDs, len(metrics))
		for _, id := range metrics {
			_, exists := op.gpmMetricsIDs[id]
			assert.True(t, exists, "metric %v should exist in map", id)
		}
	})

	t.Run("with multiple GPM metrics calls", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithGPMMetricsID(nvml.GPM_METRIC_SM_OCCUPANCY),
			WithGPMMetricsID(nvml.GPM_METRIC_FP32_UTIL),
		})
		assert.NoError(t, err)
		assert.Len(t, op.gpmMetricsIDs, 2)
	})

	t.Run("with all options combined", func(t *testing.T) {
		dbRW, err := sqlite.Open(":memory:")
		assert.NoError(t, err)
		defer dbRW.Close()

		dbRO, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
		assert.NoError(t, err)
		defer dbRO.Close()

		bucket := &mockEventBucket{}
		metrics := []nvml.GpmMetricId{nvml.GPM_METRIC_SM_OCCUPANCY}

		op := &Op{}
		err = op.applyOpts([]OpOption{
			WithDBRW(dbRW),
			WithDBRO(dbRO),
			WithHWSlowdownEventBucket(bucket),
			WithGPMMetricsID(metrics...),
		})
		assert.NoError(t, err)

		assert.Equal(t, dbRW, op.dbRW)
		assert.Equal(t, dbRO, op.dbRO)
		assert.Equal(t, bucket, op.hwSlowdownEventBucket)
		assert.Len(t, op.gpmMetricsIDs, len(metrics))
	})
}

func TestWithDBRW(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	assert.NoError(t, err)
	defer db.Close()

	op := &Op{}
	opt := WithDBRW(db)
	opt(op)
	assert.Equal(t, db, op.dbRW)
}

func TestWithDBRO(t *testing.T) {
	db, err := sqlite.Open(":memory:", sqlite.WithReadOnly(true))
	assert.NoError(t, err)
	defer db.Close()

	op := &Op{}
	opt := WithDBRO(db)
	opt(op)
	assert.Equal(t, db, op.dbRO)
}

func TestWithHWSlowdownEventBucket(t *testing.T) {
	bucket := &mockEventBucket{}
	op := &Op{}
	opt := WithHWSlowdownEventBucket(bucket)
	opt(op)
	assert.Equal(t, bucket, op.hwSlowdownEventBucket)
}

func TestWithGPMMetricsID(t *testing.T) {
	t.Run("single metric", func(t *testing.T) {
		op := &Op{}
		metric := nvml.GPM_METRIC_SM_OCCUPANCY
		opt := WithGPMMetricsID(metric)
		opt(op)
		assert.Len(t, op.gpmMetricsIDs, 1)
		_, exists := op.gpmMetricsIDs[metric]
		assert.True(t, exists)
	})

	t.Run("multiple metrics", func(t *testing.T) {
		op := &Op{}
		metrics := []nvml.GpmMetricId{
			nvml.GPM_METRIC_SM_OCCUPANCY,
			nvml.GPM_METRIC_FP32_UTIL,
		}
		opt := WithGPMMetricsID(metrics...)
		opt(op)
		assert.Len(t, op.gpmMetricsIDs, len(metrics))
		for _, id := range metrics {
			_, exists := op.gpmMetricsIDs[id]
			assert.True(t, exists)
		}
	})

	t.Run("duplicate metrics", func(t *testing.T) {
		op := &Op{}
		metric := nvml.GPM_METRIC_SM_OCCUPANCY
		opt1 := WithGPMMetricsID(metric)
		opt2 := WithGPMMetricsID(metric)
		opt1(op)
		opt2(op)
		assert.Len(t, op.gpmMetricsIDs, 1)
	})
}

func TestOpOptionsErrorHandling(t *testing.T) {
	t.Run("invalid database connection", func(t *testing.T) {
		// Create an invalid database connection
		invalidDB, err := sql.Open("sqlite3", "/nonexistent/path")
		assert.NoError(t, err) // Open doesn't actually connect
		defer invalidDB.Close()

		op := &Op{}
		err = op.applyOpts([]OpOption{
			WithDBRW(invalidDB),
		})
		// The error will come from the default database creation since we don't use the invalid one
		assert.NoError(t, err)
	})

	t.Run("nil database connections", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithDBRW(nil),
			WithDBRO(nil),
		})
		assert.NoError(t, err)
		assert.NotNil(t, op.dbRW) // Should create default in-memory DB
		assert.NotNil(t, op.dbRO) // Should create default in-memory DB
	})

	t.Run("nil events bucket", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithHWSlowdownEventBucket(nil),
		})
		assert.NoError(t, err)
		assert.Nil(t, op.hwSlowdownEventBucket)
	})
}
