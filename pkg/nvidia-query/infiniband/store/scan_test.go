package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestGetLastScanTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Test case 1: No rows - should return zero time
	ts, err := getLastScanTimestamp(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.True(t, ts.IsZero())

	// Test case 2: Insert timestamp and retrieve it
	testTime := time.Unix(1234567890, 0).UTC()
	err = setLastScanTimestamp(ctx, dbRW, tableName, testTime)
	require.NoError(t, err)

	ts, err = getLastScanTimestamp(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Equal(t, testTime, ts)

	// Test case 3: Update timestamp and retrieve it
	newTestTime := time.Unix(1234567999, 0).UTC()
	err = setLastScanTimestamp(ctx, dbRW, tableName, newTestTime)
	require.NoError(t, err)

	ts, err = getLastScanTimestamp(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Equal(t, newTestTime, ts)
}

func TestGetLastScanTimestampWithInvalidData(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert invalid timestamp data (non-numeric string)
	query := fmt.Sprintf(`INSERT INTO %s (%s, %s) VALUES (?, ?)`, tableName, metadataColumnKey, metadataColumnValue)
	_, err = dbRW.ExecContext(ctx, query, metadataKeyLastScanTimestamp, "invalid_timestamp")
	require.NoError(t, err)

	// Should return error when trying to parse invalid timestamp
	_, err = getLastScanTimestamp(ctx, dbRO, tableName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse timestamp")
	assert.Contains(t, err.Error(), "invalid_timestamp")
}

func TestGetLastScanTimestampWithEmptyString(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert empty string as timestamp
	query := fmt.Sprintf(`INSERT INTO %s (%s, %s) VALUES (?, ?)`, tableName, metadataColumnKey, metadataColumnValue)
	_, err = dbRW.ExecContext(ctx, query, metadataKeyLastScanTimestamp, "")
	require.NoError(t, err)

	// Should return error when trying to parse empty timestamp
	_, err = getLastScanTimestamp(ctx, dbRO, tableName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse timestamp")
}

func TestGetLastScanTimestampWithClosedDB(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Close database to force error
	dbRO.Close()

	// Should fail with closed database
	_, err = getLastScanTimestamp(ctx, dbRO, tableName)
	require.Error(t, err)
}

func TestGetLastScanTimestampWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Cancel context immediately
	cancel()

	// Should fail with canceled context
	_, err = getLastScanTimestamp(ctx, dbRO, tableName)
	require.Error(t, err)
}

func TestGetLastScanTimestampWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(context.Background(), dbRW, tableName)
	require.NoError(t, err)

	// Should fail with timeout
	_, err = getLastScanTimestamp(ctx, dbRO, tableName)
	require.Error(t, err)
}

func TestGetLastScanTimestampWithNonExistentTable(t *testing.T) {
	ctx := context.Background()
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "non_existent_table"

	// Should fail with non-existent table
	_, err := getLastScanTimestamp(ctx, dbRO, tableName)
	require.Error(t, err)
}

func TestSetLastScanTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Test case 1: Insert new timestamp
	testTime := time.Unix(1234567890, 0).UTC()
	err = setLastScanTimestamp(ctx, dbRW, tableName, testTime)
	require.NoError(t, err)

	// Verify timestamp was inserted
	var value string
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ?`, metadataColumnValue, tableName, metadataColumnKey)
	err = dbRO.QueryRowContext(ctx, query, metadataKeyLastScanTimestamp).Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "1234567890", value)

	// Test case 2: Update existing timestamp (INSERT OR REPLACE)
	newTestTime := time.Unix(1234567999, 0).UTC()
	err = setLastScanTimestamp(ctx, dbRW, tableName, newTestTime)
	require.NoError(t, err)

	// Verify timestamp was updated
	err = dbRO.QueryRowContext(ctx, query, metadataKeyLastScanTimestamp).Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "1234567999", value)

	// Verify only one row exists
	var count int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s = ?`, tableName, metadataColumnKey)
	err = dbRO.QueryRowContext(ctx, countQuery, metadataKeyLastScanTimestamp).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSetLastScanTimestampWithZeroTime(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert zero time
	zeroTime := time.Time{}
	err = setLastScanTimestamp(ctx, dbRW, tableName, zeroTime)
	require.NoError(t, err)

	// Verify zero time was inserted correctly
	var value string
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ?`, metadataColumnValue, tableName, metadataColumnKey)
	err = dbRO.QueryRowContext(ctx, query, metadataKeyLastScanTimestamp).Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%d", zeroTime.Unix()), value)
}

func TestSetLastScanTimestampWithFutureTime(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert future time
	futureTime := time.Unix(2234567890, 0).UTC()
	err = setLastScanTimestamp(ctx, dbRW, tableName, futureTime)
	require.NoError(t, err)

	// Verify future time was inserted correctly
	var value string
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ?`, metadataColumnValue, tableName, metadataColumnKey)
	err = dbRO.QueryRowContext(ctx, query, metadataKeyLastScanTimestamp).Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "2234567890", value)
}

func TestSetLastScanTimestampWithClosedDB(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Close database to force error
	dbRW.Close()

	// Should fail with closed database
	testTime := time.Unix(1234567890, 0).UTC()
	err = setLastScanTimestamp(ctx, dbRW, tableName, testTime)
	require.Error(t, err)
}

func TestSetLastScanTimestampWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Cancel context immediately
	cancel()

	// Should fail with canceled context
	testTime := time.Unix(1234567890, 0).UTC()
	err = setLastScanTimestamp(ctx, dbRW, tableName, testTime)
	require.Error(t, err)
}

func TestSetLastScanTimestampWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(context.Background(), dbRW, tableName)
	require.NoError(t, err)

	// Should fail with timeout
	testTime := time.Unix(1234567890, 0).UTC()
	err = setLastScanTimestamp(ctx, dbRW, tableName, testTime)
	require.Error(t, err)
}

func TestSetLastScanTimestampWithNonExistentTable(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "non_existent_table"

	// Should fail with non-existent table
	testTime := time.Unix(1234567890, 0).UTC()
	err := setLastScanTimestamp(ctx, dbRW, tableName, testTime)
	require.Error(t, err)
}

func TestGetSetLastScanTimestampRoundTrip(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Test multiple round trips with different timestamps
	testTimes := []time.Time{
		time.Unix(0, 0).UTC(),                               // Epoch
		time.Unix(1234567890, 0).UTC(),                      // Random timestamp
		time.Unix(2147483647, 0).UTC(),                      // Max 32-bit timestamp
		time.Unix(253402300799, 0).UTC(),                    // Year 9999
		time.Now().UTC().Truncate(time.Second).In(time.UTC), // Current time (truncated to seconds, explicitly UTC)
	}

	for _, testTime := range testTimes {
		// Set timestamp
		err = setLastScanTimestamp(ctx, dbRW, tableName, testTime)
		require.NoError(t, err)

		// Get timestamp
		retrievedTime, err := getLastScanTimestamp(ctx, dbRO, tableName)
		require.NoError(t, err)

		// Verify they match
		assert.Equal(t, testTime, retrievedTime)
	}
}

func TestGetLastScanTimestampWithNegativeTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert negative timestamp manually
	query := fmt.Sprintf(`INSERT INTO %s (%s, %s) VALUES (?, ?)`, tableName, metadataColumnKey, metadataColumnValue)
	_, err = dbRW.ExecContext(ctx, query, metadataKeyLastScanTimestamp, "-1234567890")
	require.NoError(t, err)

	// Should handle negative timestamps correctly
	ts, err := getLastScanTimestamp(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Equal(t, time.Unix(-1234567890, 0).UTC(), ts)
}

func TestScanWithEmptyData(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Scan should work even with no devices or ports
	err = store.Scan()
	require.NoError(t, err)
}

func TestScanWithMockData(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some mock devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Scan should complete without error even with no actual data in history table
	err = store.Scan()
	require.NoError(t, err)
}

// Tests for the Scan method's tombstone logic
func TestScanWithTombstoneOverwritesSince(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some mock devices and ports to avoid scanning nothing
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Set up timestamps
	now := time.Now().UTC()
	lastScanTime := now.Add(-2 * time.Hour)  // Last scan was 2 hours ago
	tombstoneTime := now.Add(-1 * time.Hour) // Tombstone is 1 hour ago (after last scan)

	// Set last scan timestamp to be older than tombstone
	err = setLastScanTimestamp(ctx, dbRW, s.metadataTable, lastScanTime)
	require.NoError(t, err)

	// Set tombstone timestamp to be newer than last scan
	err = setTombstoneTimestamp(ctx, dbRW, s.metadataTable, tombstoneTime)
	require.NoError(t, err)

	// Run scan
	err = store.Scan()
	require.NoError(t, err)

	// Verify that the last scan timestamp was updated (not the old since time)
	newLastScanTime, err := getLastScanTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)

	// The new last scan time should be after the tombstone time (which was the effective since)
	assert.True(t, newLastScanTime.After(tombstoneTime),
		"Expected new last scan time %v to be after tombstone time %v",
		newLastScanTime, tombstoneTime)

	// Verify tombstone timestamp is still there and unchanged
	retrievedTombstone, err := getTombstoneTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.Equal(t, tombstoneTime.Unix(), retrievedTombstone.Unix())
}

func TestScanWithTombstoneBeforeSince(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some mock devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Set up timestamps where tombstone is BEFORE the last scan
	now := time.Now().UTC()
	tombstoneTime := now.Add(-2 * time.Hour) // Tombstone is 2 hours ago
	lastScanTime := now.Add(-1 * time.Hour)  // Last scan was 1 hour ago (after tombstone)

	// Set last scan timestamp to be newer than tombstone
	err = setLastScanTimestamp(ctx, dbRW, s.metadataTable, lastScanTime)
	require.NoError(t, err)

	// Set tombstone timestamp to be older than last scan
	err = setTombstoneTimestamp(ctx, dbRW, s.metadataTable, tombstoneTime)
	require.NoError(t, err)

	// Run scan - tombstone should NOT overwrite since in this case
	err = store.Scan()
	require.NoError(t, err)

	// Verify last scan timestamp was updated (showing scan completed)
	newLastScanTime, err := getLastScanTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.True(t, newLastScanTime.After(lastScanTime))
}

func TestScanWithZeroTombstone(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some mock devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Set up only last scan timestamp (no tombstone)
	now := time.Now().UTC()
	lastScanTime := now.Add(-1 * time.Hour)

	err = setLastScanTimestamp(ctx, dbRW, s.metadataTable, lastScanTime)
	require.NoError(t, err)

	// Don't set tombstone timestamp (should be zero)
	tombstone, err := getTombstoneTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.True(t, tombstone.IsZero())

	// Run scan - zero tombstone should not affect since
	err = store.Scan()
	require.NoError(t, err)

	// Verify scan completed successfully
	newLastScanTime, err := getLastScanTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.True(t, newLastScanTime.After(lastScanTime))
}

func TestScanWithEqualTombstoneAndSince(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some mock devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Set tombstone and last scan to the same time
	now := time.Now().UTC()
	sameTime := now.Add(-1 * time.Hour)

	err = setLastScanTimestamp(ctx, dbRW, s.metadataTable, sameTime)
	require.NoError(t, err)

	err = setTombstoneTimestamp(ctx, dbRW, s.metadataTable, sameTime)
	require.NoError(t, err)

	// Run scan - equal times means tombstone.After(since) is false, so no overwrite
	err = store.Scan()
	require.NoError(t, err)

	// Verify scan completed successfully
	newLastScanTime, err := getLastScanTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.True(t, newLastScanTime.After(sameTime))
}

func TestScanWithBothTimestampsZero(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some mock devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Don't set any timestamps - both should be zero
	lastScan, err := getLastScanTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.True(t, lastScan.IsZero())

	tombstone, err := getTombstoneTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.True(t, tombstone.IsZero())

	// Run scan with both timestamps being zero
	err = store.Scan()
	require.NoError(t, err)

	// Verify scan completed and set a new last scan timestamp
	newLastScanTime, err := getLastScanTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.False(t, newLastScanTime.IsZero())
}

func TestScanFailsWhenGetLastScanTimestampFails(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Don't create metadata table to force getLastScanTimestamp to fail
	// (metadata table creation is part of New() so we need to override the table name)
	s.metadataTable = "non_existent_metadata_table"

	// Scan should fail when it can't get last scan timestamp
	err = store.Scan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestScanFailsWhenGetTombstoneTimestampFails(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Insert invalid data to make getTombstoneTimestamp fail
	query := fmt.Sprintf(`INSERT INTO %s (%s, %s) VALUES (?, ?)`, s.metadataTable, metadataColumnKey, metadataColumnValue)
	_, err = dbRW.ExecContext(ctx, query, metadataKeyTombstoneTimestamp, "invalid_timestamp")
	require.NoError(t, err)

	// Scan should fail when it can't parse tombstone timestamp
	err = store.Scan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse timestamp")
}

func TestScanFailsWhenSetLastScanTimestampFails(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add mock data so scan doesn't exit early
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Close the RW database to make setLastScanTimestamp fail
	dbRW.Close()

	// Scan should fail when it can't set the final timestamp
	err = store.Scan()
	require.Error(t, err)
}

func TestScanLookbackPeriodsWithTombstone(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add mock devices and ports
	s.updateAllDeviceValues(map[string]any{"mlx5_0": nil})
	s.updateAllPortValues(map[uint]any{1: nil})

	// Set lookback periods to known values for testing
	s.ibPortDropLookbackPeriod = 10 * time.Minute
	s.ibPortFlapLookbackPeriod = 15 * time.Minute

	// Set up timestamps where tombstone overwrites since
	now := time.Now().UTC()
	lastScanTime := now.Add(-2 * time.Hour)
	tombstoneTime := now.Add(-1 * time.Hour) // This will be the effective "since"

	err = setLastScanTimestamp(ctx, dbRW, s.metadataTable, lastScanTime)
	require.NoError(t, err)

	err = setTombstoneTimestamp(ctx, dbRW, s.metadataTable, tombstoneTime)
	require.NoError(t, err)

	// Run scan - this tests that the lookback periods are applied to the tombstone time
	// The exact behavior depends on scanIBPortDrops/scanIBPortFlaps implementation
	// but the important thing is that the scan doesn't fail
	err = store.Scan()
	require.NoError(t, err)

	// Verify scan completed
	newLastScanTime, err := getLastScanTimestamp(ctx, dbRO, s.metadataTable)
	require.NoError(t, err)
	assert.True(t, newLastScanTime.After(tombstoneTime))
}
