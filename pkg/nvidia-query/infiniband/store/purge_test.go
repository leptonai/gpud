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

func TestPurge(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_purge_table"

	// Create history table
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert test data with various timestamps and event types
	currentTime := time.Now()

	// Old data with empty event type (should be purged)
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-2*time.Hour), "mlx5_0", 1, "")
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-3*time.Hour), "mlx5_1", 2, "")

	// Old data with event type (should NOT be purged)
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-4*time.Hour), "mlx5_2", 3, "ib_port_drop")
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-5*time.Hour), "mlx5_3", 4, "ib_port_flap")

	// Recent data with empty event type (should NOT be purged)
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-30*time.Minute), "mlx5_4", 5, "")

	// Recent data with event type (should NOT be purged)
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-15*time.Minute), "mlx5_5", 6, "ib_port_drop")

	// Purge data older than 1 hour
	purgeBeforeTimestamp := currentTime.Add(-1 * time.Hour).Unix()
	purged, err := purge(ctx, dbRW, tableName, purgeBeforeTimestamp, false)
	require.NoError(t, err)

	// Should have purged 2 rows (the ones with empty event types older than 1 hour)
	assert.Equal(t, 2, purged)

	// Verify remaining data
	rows, err := dbRW.QueryContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	require.NoError(t, err)
	defer rows.Close()

	var count int
	require.True(t, rows.Next())
	err = rows.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 4, count) // 6 original - 2 purged = 4 remaining
}

func TestPurgeWithNoData(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_purge_empty_table"

	// Create empty table
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Purge from empty table
	purged, err := purge(ctx, dbRW, tableName, time.Now().Unix(), false)
	require.NoError(t, err)
	assert.Equal(t, 0, purged)
}

func TestPurgeWithAllEventsHavingEventTypes(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_purge_all_events"

	// Create history table
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert old data but all with event types
	currentTime := time.Now()
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-2*time.Hour), "mlx5_0", 1, "ib_port_drop")
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-3*time.Hour), "mlx5_1", 2, "ib_port_flap")
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-4*time.Hour), "mlx5_2", 3, "some_event")

	// Purge data older than 1 hour
	purgeBeforeTimestamp := currentTime.Add(-1 * time.Hour).Unix()
	purged, err := purge(ctx, dbRW, tableName, purgeBeforeTimestamp, false)
	require.NoError(t, err)

	// Should have purged 0 rows (all have event types)
	assert.Equal(t, 0, purged)

	// Verify all data remains
	rows, err := dbRW.QueryContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	require.NoError(t, err)
	defer rows.Close()

	var count int
	require.True(t, rows.Next())
	err = rows.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestPurgeWithOnlyRecentData(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_purge_recent_data"

	// Create history table
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert only recent data
	currentTime := time.Now()
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-10*time.Minute), "mlx5_0", 1, "")
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-20*time.Minute), "mlx5_1", 2, "")
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-30*time.Minute), "mlx5_2", 3, "")

	// Purge data older than 1 hour
	purgeBeforeTimestamp := currentTime.Add(-1 * time.Hour).Unix()
	purged, err := purge(ctx, dbRW, tableName, purgeBeforeTimestamp, false)
	require.NoError(t, err)

	// Should have purged 0 rows (all data is recent)
	assert.Equal(t, 0, purged)

	// Verify all data remains
	rows, err := dbRW.QueryContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	require.NoError(t, err)
	defer rows.Close()

	var count int
	require.True(t, rows.Next())
	err = rows.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestPurgeWithNonExistentTable(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Try to purge from non-existent table
	purged, err := purge(ctx, dbRW, "non_existent_table", time.Now().Unix(), false)
	require.Error(t, err)
	assert.Equal(t, 0, purged)
}

func TestPurgeEventsWithClosedDB(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Close database to force error
	dbRW.Close()

	// Purge should fail
	_, err := purge(ctx, dbRW, "test_table", time.Now().Unix(), false)
	require.Error(t, err)
}

func TestPurgeWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_purge_canceled"

	// Create history table
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Purge should fail with canceled context
	_, err = purge(ctx, dbRW, tableName, time.Now().Unix(), false)
	require.Error(t, err)
}

func TestPurgeWithLargeDataSet(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_purge_large_dataset"

	// Create history table
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert large dataset
	currentTime := time.Now()
	numOldEntries := 100
	numRecentEntries := 50

	// Insert old entries with empty event types (should be purged)
	for i := 0; i < numOldEntries; i++ {
		insertData(t, ctx, dbRW, tableName, currentTime.Add(-time.Duration(i+120)*time.Minute), fmt.Sprintf("mlx5_%d", i), uint(i%10), "")
	}

	// Insert recent entries with empty event types (should NOT be purged)
	for i := 0; i < numRecentEntries; i++ {
		insertData(t, ctx, dbRW, tableName, currentTime.Add(-time.Duration(i+1)*time.Minute), fmt.Sprintf("mlx5_recent_%d", i), uint(i%10), "")
	}

	// Purge data older than 1 hour
	purgeBeforeTimestamp := currentTime.Add(-1 * time.Hour).Unix()
	purged, err := purge(ctx, dbRW, tableName, purgeBeforeTimestamp, false)
	require.NoError(t, err)

	// Should have purged all old entries
	assert.Equal(t, numOldEntries, purged)

	// Verify remaining data count
	rows, err := dbRW.QueryContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	require.NoError(t, err)
	defer rows.Close()

	var count int
	require.True(t, rows.Next())
	err = rows.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numRecentEntries, count)
}

func TestRunPurgeWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with very short purge interval for testing
	store := &ibPortsStore{
		rootCtx:                  ctx,
		historyTable:             "test_purge_table",
		metadataTable:            "test_metadata_table",
		dbRW:                     dbRW,
		dbRO:                     dbRO,
		getTimeNow:               func() time.Time { return time.Now().UTC() },
		minInsertInterval:        0,
		purgeInterval:            1 * time.Millisecond, // Very short interval
		ibPortDropLookbackPeriod: 1 * time.Minute,
	}

	// Create tables
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Start purge goroutine
	go store.runPurge()

	// Cancel context quickly to test context cancellation
	cancel()

	// Give some time for goroutine to exit
	time.Sleep(10 * time.Millisecond)
}

func TestRunPurgeWithPurgeError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:                  ctx,
		historyTable:             "nonexistent_table", // This will cause purge errors
		metadataTable:            "test_metadata_table",
		dbRW:                     dbRW,
		dbRO:                     dbRO,
		getTimeNow:               func() time.Time { return time.Now().UTC() },
		minInsertInterval:        0,
		purgeInterval:            1 * time.Millisecond,
		ibPortDropLookbackPeriod: 1 * time.Minute,
	}

	// Create metadata table but not history table to cause errors
	err := createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Use a context with timeout to avoid infinite test
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	store.rootCtx = ctx

	// This will test the error handling path in runPurge
	store.runPurge()
}

func TestRunPurgeWithGetLastScanTimestampError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:                  ctx,
		historyTable:             "test_history_table",
		metadataTable:            "nonexistent_metadata_table", // This will cause getLastScanTimestamp errors
		dbRW:                     dbRW,
		dbRO:                     dbRO,
		getTimeNow:               func() time.Time { return time.Now().UTC() },
		minInsertInterval:        0,
		purgeInterval:            1 * time.Millisecond,
		ibPortDropLookbackPeriod: 1 * time.Minute,
	}

	// Create history table but not metadata table to cause errors
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Use a context with timeout to avoid infinite test
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	store.rootCtx = ctx

	// This will test the error handling path in runPurge when getLastScanTimestamp fails
	store.runPurge()
}

func TestRunPurgeWithOldScanTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:                  ctx,
		historyTable:             "test_history_table",
		metadataTable:            "test_metadata_table",
		dbRW:                     dbRW,
		dbRO:                     dbRO,
		getTimeNow:               func() time.Time { return time.Now().UTC() },
		minInsertInterval:        0,
		purgeInterval:            1 * time.Millisecond,
		ibPortDropLookbackPeriod: 10 * time.Minute,
	}

	// Create tables
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Set an old scan timestamp to trigger the special logic
	oldScanTime := time.Now().Add(-2 * time.Hour)
	err = setLastScanTimestamp(ctx, dbRW, store.metadataTable, oldScanTime)
	require.NoError(t, err)

	// Use a context with timeout to avoid infinite test
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	store.rootCtx = ctx

	// This will test the special case where scan timestamp is behind purge timestamp
	store.runPurge()
}

func TestPurgeAllEventsTrue(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_purge_all_events_true"

	// Create history table
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert test data with various timestamps and event types
	currentTime := time.Now()

	// Old data with empty event type (should be purged)
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-2*time.Hour), "mlx5_0", 1, "")
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-3*time.Hour), "mlx5_1", 2, "")

	// Old data with event type (should also be purged when purgeAllEvents=true)
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-4*time.Hour), "mlx5_2", 3, "ib_port_drop")
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-5*time.Hour), "mlx5_3", 4, "ib_port_flap")

	// Recent data with empty event type (should NOT be purged)
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-30*time.Minute), "mlx5_4", 5, "")

	// Recent data with event type (should NOT be purged)
	insertData(t, ctx, dbRW, tableName, currentTime.Add(-15*time.Minute), "mlx5_5", 6, "ib_port_drop")

	// Purge data older than 1 hour with purgeAllEvents=true
	purgeBeforeTimestamp := currentTime.Add(-1 * time.Hour).Unix()
	purged, err := purge(ctx, dbRW, tableName, purgeBeforeTimestamp, true)
	require.NoError(t, err)

	// Should have purged 4 rows (all entries older than 1 hour, regardless of event type)
	assert.Equal(t, 4, purged)

	// Verify remaining data
	rows, err := dbRW.QueryContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	require.NoError(t, err)
	defer rows.Close()

	var count int
	require.True(t, rows.Next())
	err = rows.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count) // 6 original - 4 purged = 2 remaining
}

// Helper function to insert test data
func insertData(t *testing.T, ctx context.Context, dbRW *sql.DB, tableName string, ts time.Time, device string, port uint, eventType string) {
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

	_, err := dbRW.ExecContext(ctx, query, ts.Unix(), device, port, "infiniband", "active", "linkup", 100, 0, eventType, "", "")
	require.NoError(t, err)
}
