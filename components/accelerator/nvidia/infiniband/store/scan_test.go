package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestScan(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some mock devices and ports to scan
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil, "mlx5_1": nil})
	s.updateAllPortValues(map[uint]any{1: nil, 2: nil})

	// Test basic scan operation
	err = store.Scan()
	require.NoError(t, err)
}

func TestScanWithNoDevicesOrPorts(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Scan with no devices or ports should still work
	err = store.Scan()
	require.NoError(t, err)
}

func TestScanWithTombstoneTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some mock devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Set tombstone timestamp
	tombstoneTime := time.Now().Add(-1 * time.Hour)
	err = setTombstoneTimestamp(ctx, dbRW, s.metadataTable, tombstoneTime)
	require.NoError(t, err)

	// Scan should use tombstone timestamp for since calculation
	err = store.Scan()
	require.NoError(t, err)
}

func TestScanWithTombstoneTimestampError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Corrupt the metadata table to cause getTombstoneTimestamp to fail
	s.configMu.Lock()
	s.metadataTable = "non_existent_table"
	s.configMu.Unlock()

	// Scan should fail when getTombstoneTimestamp fails
	err = store.Scan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestScanWithScanIBPortDropsError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Corrupt the history table to cause scanIBPortDrops to fail
	s.configMu.Lock()
	s.historyTable = "non_existent_table"
	s.configMu.Unlock()

	// Scan should fail when scanIBPortDrops fails
	err = store.Scan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestScanWithScanIBPortFlapsError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Corrupt the history table to make scanIBPortFlaps fail
	s.configMu.Lock()
	s.historyTable = "non_existent_table"
	s.configMu.Unlock()

	// This should fail when trying to scan flaps
	err = store.Scan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestScanWithSetEventTypeError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Insert test data that would create a drop event
	currentTime := time.Now()
	insertData(t, ctx, dbRW, s.historyTable, currentTime.Add(-10*time.Minute), "mlx5_0", 1, "")
	insertData(t, ctx, dbRW, s.historyTable, currentTime.Add(-9*time.Minute), "mlx5_0", 1, "")
	insertData(t, ctx, dbRW, s.historyTable, currentTime.Add(-8*time.Minute), "mlx5_0", 1, "")

	// Corrupt the history table to make SetEventType fail
	// This will cause the UPDATE statement in SetEventType to fail
	s.configMu.Lock()
	s.historyTable = "non_existent_table"
	s.configMu.Unlock()

	// This should fail when trying to set event types
	err = store.Scan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestScanWithCustomThresholds(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Set specific thresholds for testing
	s.configMu.Lock()
	s.ibPortDropThreshold = 30 * time.Minute
	s.ibPortFlapDownIntervalThreshold = 45 * time.Minute
	s.retentionPeriod = 2 * time.Hour
	s.configMu.Unlock()

	// Add devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Scan should work with configured thresholds
	err = store.Scan()
	require.NoError(t, err)
}

func TestScanWithZeroTombstoneTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Verify tombstone timestamp is zero initially
	tombstoneTS, err := getTombstoneTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.True(t, tombstoneTS.IsZero())

	// Scan should use retention period when tombstone is zero
	err = store.Scan()
	require.NoError(t, err)
}

func TestScanWithTombstoneBeforeRetentionPeriod(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Set tombstone timestamp to be before retention period
	tombstoneTime := time.Now().Add(-3 * time.Hour)
	err = setTombstoneTimestamp(ctx, dbRW, s.metadataTable, tombstoneTime)
	require.NoError(t, err)

	// Scan should use retention period since tombstone is older
	err = store.Scan()
	require.NoError(t, err)
}

func TestScanWithContextTimeout(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Use a context with very short timeout
	shortCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
	defer cancel()
	s.configMu.Lock()
	s.rootCtx = shortCtx
	s.configMu.Unlock()

	// Add devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Scan should fail due to context timeout
	err = store.Scan()
	require.Error(t, err)
}
