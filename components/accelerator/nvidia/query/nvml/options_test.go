package nvml

import (
	"database/sql"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	events_db "github.com/leptonai/gpud/components/db"
)

func TestOpOptions(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{})
		require.NoError(t, err)

		// Check that default in-memory databases are created
		assert.NotNil(t, op.dbRW)
		assert.NotNil(t, op.dbRO)
		assert.Nil(t, op.xidEventsStore)
		assert.Nil(t, op.hwslowdownEventsStore)
		assert.Nil(t, op.gpmMetricsIDs)
	})

	t.Run("custom values", func(t *testing.T) {
		mockDB := &sql.DB{}
		mockStore := &mockEventsStore{}
		testMetrics := []nvml.GpmMetricId{
			nvml.GPM_METRIC_SM_OCCUPANCY,
			nvml.GPM_METRIC_FP32_UTIL,
		}

		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithDBRW(mockDB),
			WithDBRO(mockDB),
			WithXidEventsStore(mockStore),
			WithHWSlowdownEventsStore(mockStore),
			WithGPMMetricsID(testMetrics...),
		})
		require.NoError(t, err)

		// Check custom values
		assert.Equal(t, mockDB, op.dbRW)
		assert.Equal(t, mockDB, op.dbRO)
		assert.Equal(t, mockStore, op.xidEventsStore)
		assert.Equal(t, mockStore, op.hwslowdownEventsStore)

		// Check GPM metrics
		assert.NotNil(t, op.gpmMetricsIDs)
		assert.Len(t, op.gpmMetricsIDs, len(testMetrics))
		for _, metric := range testMetrics {
			_, exists := op.gpmMetricsIDs[metric]
			assert.True(t, exists, "Metric %v should exist in gpmMetricsIDs", metric)
		}
	})

	t.Run("partial options", func(t *testing.T) {
		mockDB := &sql.DB{}
		testMetrics := []nvml.GpmMetricId{nvml.GPM_METRIC_SM_OCCUPANCY}

		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithDBRW(mockDB),
			WithGPMMetricsID(testMetrics...),
		})
		require.NoError(t, err)

		// Check mixed custom and default values
		assert.Equal(t, mockDB, op.dbRW)
		assert.NotNil(t, op.dbRO) // Should create default read-only DB
		assert.Nil(t, op.xidEventsStore)
		assert.Nil(t, op.hwslowdownEventsStore)

		// Check GPM metrics
		assert.NotNil(t, op.gpmMetricsIDs)
		assert.Len(t, op.gpmMetricsIDs, 1)
		_, exists := op.gpmMetricsIDs[nvml.GPM_METRIC_SM_OCCUPANCY]
		assert.True(t, exists)
	})

	t.Run("multiple GPM metrics", func(t *testing.T) {
		op := &Op{}
		// Add metrics in multiple calls to test accumulation
		err := op.applyOpts([]OpOption{
			WithGPMMetricsID(nvml.GPM_METRIC_SM_OCCUPANCY),
			WithGPMMetricsID(nvml.GPM_METRIC_FP32_UTIL),
		})
		require.NoError(t, err)

		assert.Len(t, op.gpmMetricsIDs, 2)
		_, exists := op.gpmMetricsIDs[nvml.GPM_METRIC_SM_OCCUPANCY]
		assert.True(t, exists)
		_, exists = op.gpmMetricsIDs[nvml.GPM_METRIC_FP32_UTIL]
		assert.True(t, exists)
	})
}

// mockEventsStore implements events_db.Store interface for testing
type mockEventsStore struct {
	events_db.Store
}
