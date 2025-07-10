package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestReadDevPortSnapshots(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data for specific device and port
	device := "mlx5_0"
	port := uint(1)
	currentTime := time.Now()

	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-4*time.Hour), device, port, "down", 5)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-3*time.Hour), device, port, "active", 5)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Hour), device, port, "active", 7)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Hour), device, port, "down", 10)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-30*time.Minute), device, port, "active", 12)

	// Insert data for different device/port to ensure filtering works
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Hour), "mlx5_1", 2, "active", 3)

	// Test reading all snapshots for device and port
	snapshots, err := store.readDevPortSnapshots(device, port, time.Time{})
	require.NoError(t, err)
	assert.Len(t, snapshots, 5)

	// Verify snapshots are ordered by timestamp ascending
	for i := 1; i < len(snapshots); i++ {
		assert.True(t, snapshots[i-1].ts.Before(snapshots[i].ts), "Snapshots should be ordered by timestamp")
	}

	// Verify specific snapshot data
	assert.Equal(t, "down", snapshots[0].state)
	assert.Equal(t, uint64(5), snapshots[0].totalLinkDowned)
	assert.Equal(t, "active", snapshots[4].state)
	assert.Equal(t, uint64(12), snapshots[4].totalLinkDowned)
}

func TestReadDevPortSnapshotsWithSinceFilter(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_since_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data
	device := "mlx5_0"
	port := uint(1)
	currentTime := time.Now()

	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-4*time.Hour), device, port, "down", 5)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-3*time.Hour), device, port, "active", 5)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Hour), device, port, "active", 7)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Hour), device, port, "down", 10)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-30*time.Minute), device, port, "active", 12)

	// Test reading snapshots since 2.5 hours ago
	since := currentTime.Add(-2*time.Hour - 30*time.Minute)
	snapshots, err := store.readDevPortSnapshots(device, port, since)
	require.NoError(t, err)
	assert.Len(t, snapshots, 3) // Should exclude the first two entries

	// Verify correct snapshots are returned
	assert.Equal(t, "active", snapshots[0].state)
	assert.Equal(t, uint64(7), snapshots[0].totalLinkDowned)
	assert.Equal(t, "active", snapshots[2].state)
	assert.Equal(t, uint64(12), snapshots[2].totalLinkDowned)
}

func TestReadDevPortSnapshotsNoResults(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_no_results_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert data for different device/port
	currentTime := time.Now()
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Hour), "mlx5_1", 2, "active", 3)

	// Test reading snapshots for non-existent device/port
	snapshots, err := store.readDevPortSnapshots("mlx5_0", 1, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, snapshots) // Should return empty slice, not nil
}

func TestReadDevPortSnapshotsWithFutureTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_future_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data
	device := "mlx5_0"
	port := uint(1)
	currentTime := time.Now()

	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Hour), device, port, "active", 5)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Hour), device, port, "down", 7)

	// Test reading snapshots since future timestamp
	futureTime := currentTime.Add(1 * time.Hour)
	snapshots, err := store.readDevPortSnapshots(device, port, futureTime)
	require.NoError(t, err)
	assert.Empty(t, snapshots) // Should return empty slice, not nil
}

func TestReadDevPortSnapshotsWithNonExistentTable(t *testing.T) {
	ctx := context.Background()
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with non-existent table
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "non_existent_table",
		dbRO:         dbRO,
	}

	// Test reading snapshots from non-existent table
	snapshots, err := store.readDevPortSnapshots("mlx5_0", 1, time.Time{})
	require.Error(t, err)
	assert.Nil(t, snapshots)
}

func TestReadDevPortSnapshotsWithClosedDB(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_closed_db_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Close the read-only database
	dbRO.Close()

	// Test reading snapshots with closed database
	snapshots, err := store.readDevPortSnapshots("mlx5_0", 1, time.Time{})
	require.Error(t, err)
	assert.Nil(t, snapshots)
}

func TestReadDevPortSnapshotsWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_canceled_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Test reading snapshots with canceled context
	snapshots, err := store.readDevPortSnapshots("mlx5_0", 1, time.Time{})
	require.Error(t, err)
	assert.Nil(t, snapshots)
}

func TestReadDevPortSnapshotsWithTimeout(t *testing.T) {
	// Use a very short timeout to ensure it triggers reliably
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with a canceled context to ensure timeout behavior
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Cancel immediately

	store := &ibPortsStore{
		rootCtx:      canceledCtx,
		historyTable: "test_snapshots_timeout_table",
		dbRO:         dbRO,
	}

	// Create history table with the valid context
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Test reading snapshots with canceled context - should reliably fail
	snapshots, err := store.readDevPortSnapshots("mlx5_0", 1, time.Time{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
	assert.Nil(t, snapshots)
}

func TestReadDevPortSnapshotsWithInvalidData(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_invalid_data_table",
		dbRO:         dbRO,
	}

	// Create a table with incompatible schema
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %s (
		timestamp TEXT,
		state TEXT,
		total_link_downed TEXT,
		device TEXT,
		port INTEGER
	)`, store.historyTable))
	require.NoError(t, err)

	// Insert data with invalid timestamp format
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s VALUES (?, ?, ?, ?, ?)`,
		store.historyTable), "invalid_timestamp", "active", "invalid_number", "mlx5_0", 1)
	require.NoError(t, err)

	// Test reading snapshots with invalid data
	snapshots, err := store.readDevPortSnapshots("mlx5_0", 1, time.Time{})
	require.Error(t, err)
	assert.Nil(t, snapshots)
}

func TestReadDevPortSnapshotsLargeDataSet(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_large_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert large dataset
	device := "mlx5_0"
	port := uint(1)
	currentTime := time.Now()
	numEntries := 1000

	for i := 0; i < numEntries; i++ {
		ts := currentTime.Add(-time.Duration(i) * time.Minute)
		state := "active"
		if i%10 == 0 {
			state = "down"
		}
		totalLinkDowned := uint64(i / 10)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, ts, device, port, state, totalLinkDowned)
	}

	// Test reading all snapshots
	snapshots, err := store.readDevPortSnapshots(device, port, time.Time{})
	require.NoError(t, err)
	assert.Len(t, snapshots, numEntries)

	// Verify ordering
	for i := 1; i < len(snapshots); i++ {
		assert.True(t, snapshots[i-1].ts.Before(snapshots[i].ts), "Snapshots should be ordered by timestamp")
	}
}

func TestReadDevPortSnapshotsWithSpecialCharacters(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_special_chars_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert data with special characters in device name
	device := "mlx5_0-test!@#$%"
	port := uint(1)
	currentTime := time.Now()

	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Hour), device, port, "active", 5)

	// Test reading snapshots with special characters
	snapshots, err := store.readDevPortSnapshots(device, port, time.Time{})
	require.NoError(t, err)
	assert.Len(t, snapshots, 1)
	assert.Equal(t, "active", snapshots[0].state)
	assert.Equal(t, uint64(5), snapshots[0].totalLinkDowned)
}

func TestReadDevPortSnapshotsWithZeroValues(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_zero_values_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert data with zero values
	device := "mlx5_0"
	port := uint(0) // Zero port
	currentTime := time.Now()

	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Hour), device, port, "down", 0)

	// Test reading snapshots with zero values
	snapshots, err := store.readDevPortSnapshots(device, port, time.Time{})
	require.NoError(t, err)
	assert.Len(t, snapshots, 1)
	assert.Equal(t, "down", snapshots[0].state)
	assert.Equal(t, uint64(0), snapshots[0].totalLinkDowned)
}

func TestReadDevPortSnapshotsWithExactTimestampMatch(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_snapshots_exact_match_table",
		dbRO:         dbRO,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data
	device := "mlx5_0"
	port := uint(1)
	currentTime := time.Now()

	exactTime := currentTime.Add(-1 * time.Hour)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, exactTime, device, port, "active", 5)
	insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-30*time.Minute), device, port, "down", 7)

	// Test reading snapshots with exact timestamp match
	snapshots, err := store.readDevPortSnapshots(device, port, exactTime)
	require.NoError(t, err)
	assert.Len(t, snapshots, 2) // Should include the exact match and later entries

	// Verify first snapshot matches the exact timestamp
	assert.Equal(t, exactTime.Unix(), snapshots[0].ts.Unix())
	assert.Equal(t, "active", snapshots[0].state)
	assert.Equal(t, uint64(5), snapshots[0].totalLinkDowned)
}

func TestDevPortSnapshotStructure(t *testing.T) {
	// Test the devPortSnapshot struct
	ts := time.Now()
	snapshot := devPortSnapshot{
		ts:              ts,
		state:           "active",
		totalLinkDowned: 42,
	}

	assert.Equal(t, ts, snapshot.ts)
	assert.Equal(t, "active", snapshot.state)
	assert.Equal(t, uint64(42), snapshot.totalLinkDowned)
}

func TestDevPortSnapshotsSlice(t *testing.T) {
	// Test the devPortSnapshots slice type
	snapshots := devPortSnapshots{
		{ts: time.Now(), state: "active", totalLinkDowned: 1},
		{ts: time.Now().Add(1 * time.Hour), state: "down", totalLinkDowned: 2},
	}

	assert.Len(t, snapshots, 2)
	assert.Equal(t, "active", snapshots[0].state)
	assert.Equal(t, "down", snapshots[1].state)
}

// Helper function to insert snapshot test data
func insertSnapshotData(t *testing.T, ctx context.Context, dbRW *sql.DB, tableName string, ts time.Time, device string, port uint, state string, totalLinkDowned uint64) {
	t.Helper()

	query := fmt.Sprintf(`INSERT INTO %s (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tableName,
		historyTableColumnTimestamp,
		historyTableColumnDevice,
		historyTableColumnPort,
		historyTableColumnLinkLayer,
		historyTableColumnState,
		historyTableColumnPhysicalState,
		historyTableColumnRateGBSec,
		historyTableColumnTotalLinkDowned,
		historyTableColumnEventType,
		historyTableColumnEventReason,
		historyTableColumnExtraInfo,
	)

	_, err := dbRW.ExecContext(ctx, query, ts.Unix(), device, port, "infiniband", state, "linkup", 100, totalLinkDowned, "", "", "")
	require.NoError(t, err)
}
