package store

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestTombstoneSuccess(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_tombstone_success_metadata"

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		metadataTable: metadataTable,
		dbRW:          dbRW,
		dbRO:          dbRO,
	}

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Set a tombstone timestamp
	testTime := time.Now().UTC()
	err = store.Tombstone(testTime)
	require.NoError(t, err)

	// Verify the tombstone timestamp was set correctly
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.NoError(t, err)
	assert.Equal(t, testTime.Unix(), retrievedTime.Unix())
	assert.True(t, retrievedTime.Equal(testTime.Truncate(time.Second)))
}

func TestTombstoneWithoutMetadataTable(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with non-existent metadata table
	store := &ibPortsStore{
		rootCtx:       ctx,
		metadataTable: "non_existent_table",
		dbRW:          dbRW,
		dbRO:          dbRO,
	}

	// Trying to set tombstone on non-existent table should fail
	testTime := time.Now().UTC()
	err := store.Tombstone(testTime)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestTombstoneWithClosedDatabase(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_tombstone_closed_db_metadata"

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		metadataTable: metadataTable,
		dbRW:          dbRW,
		dbRO:          dbRO,
	}

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Close the database
	dbRW.Close()

	// Trying to set tombstone on closed database should fail
	testTime := time.Now().UTC()
	err = store.Tombstone(testTime)
	require.Error(t, err)
}

func TestTombstoneWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_tombstone_canceled_ctx_metadata"

	// Create store with canceled context
	store := &ibPortsStore{
		rootCtx:       ctx,
		metadataTable: metadataTable,
		dbRW:          dbRW,
		dbRO:          dbRO,
	}

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Cancel the context
	cancel()

	// Trying to set tombstone with canceled context should fail
	testTime := time.Now().UTC()
	err = store.Tombstone(testTime)
	require.Error(t, err)
}

func TestTombstoneUpdate(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_tombstone_update_metadata"

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		metadataTable: metadataTable,
		dbRW:          dbRW,
		dbRO:          dbRO,
	}

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Set initial tombstone timestamp
	initialTime := time.Now().UTC().Add(-1 * time.Hour)
	err = store.Tombstone(initialTime)
	require.NoError(t, err)

	// Update tombstone timestamp
	updatedTime := time.Now().UTC()
	err = store.Tombstone(updatedTime)
	require.NoError(t, err)

	// Verify the tombstone timestamp was updated
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.NoError(t, err)
	assert.Equal(t, updatedTime.Unix(), retrievedTime.Unix())
	assert.True(t, retrievedTime.Equal(updatedTime.Truncate(time.Second)))
}

func TestSetTombstoneTimestampSuccess(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_set_tombstone_timestamp_success"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Set tombstone timestamp
	testTime := time.Now().UTC()
	err = setTombstoneTimestamp(ctx, dbRW, metadataTable, testTime)
	require.NoError(t, err)

	// Verify the timestamp was set by querying directly
	query := `SELECT v FROM ` + metadataTable + ` WHERE k = ?`
	row := dbRW.QueryRowContext(ctx, query, metadataKeyTombstoneTimestamp)

	var timestampStr string
	err = row.Scan(&timestampStr)
	require.NoError(t, err)
	assert.Equal(t, testTime.Unix(), parseTimestamp(t, timestampStr))
}

func TestSetTombstoneTimestampWithNonExistentTable(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Try to set tombstone timestamp on non-existent table
	testTime := time.Now().UTC()
	err := setTombstoneTimestamp(ctx, dbRW, "non_existent_table", testTime)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestSetTombstoneTimestampWithClosedDatabase(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_set_tombstone_timestamp_closed_db"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Close the database
	dbRW.Close()

	// Try to set tombstone timestamp on closed database
	testTime := time.Now().UTC()
	err = setTombstoneTimestamp(ctx, dbRW, metadataTable, testTime)
	require.Error(t, err)
}

func TestSetTombstoneTimestampWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_set_tombstone_timestamp_canceled_ctx"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Cancel the context
	cancel()

	// Try to set tombstone timestamp with canceled context
	testTime := time.Now().UTC()
	err = setTombstoneTimestamp(ctx, dbRW, metadataTable, testTime)
	require.Error(t, err)
}

func TestGetTombstoneTimestampSuccess(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_get_tombstone_timestamp_success"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Set a tombstone timestamp
	testTime := time.Now().UTC()
	err = setTombstoneTimestamp(ctx, dbRW, metadataTable, testTime)
	require.NoError(t, err)

	// Get the tombstone timestamp
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.NoError(t, err)
	assert.Equal(t, testTime.Unix(), retrievedTime.Unix())
	assert.True(t, retrievedTime.Equal(testTime.Truncate(time.Second)))
}

func TestGetTombstoneTimestampNotFound(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_get_tombstone_timestamp_not_found"

	// Create metadata table but don't set any tombstone timestamp
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Get the tombstone timestamp (should return zero time)
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.NoError(t, err)
	assert.True(t, retrievedTime.IsZero())
}

func TestGetTombstoneTimestampWithNonExistentTable(t *testing.T) {
	ctx := context.Background()
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Try to get tombstone timestamp from non-existent table
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, "non_existent_table")
	require.Error(t, err)
	// Error could be "no such table" or "attempt to write a readonly database" depending on SQLite version
	assert.True(t,
		containsAny(err.Error(), []string{"no such table", "readonly database"}),
		"Expected error to contain 'no such table' or 'readonly database', got: %s", err.Error())
	assert.True(t, retrievedTime.IsZero())
}

func TestGetTombstoneTimestampWithClosedDatabase(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_get_tombstone_timestamp_closed_db"

	// Create metadata table and set tombstone timestamp
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	testTime := time.Now().UTC()
	err = setTombstoneTimestamp(ctx, dbRW, metadataTable, testTime)
	require.NoError(t, err)

	// Close the database
	dbRO.Close()

	// Try to get tombstone timestamp from closed database
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.Error(t, err)
	assert.True(t, retrievedTime.IsZero())
}

func TestGetTombstoneTimestampWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_get_tombstone_timestamp_canceled_ctx"

	// Create metadata table and set tombstone timestamp
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	testTime := time.Now().UTC()
	err = setTombstoneTimestamp(ctx, dbRW, metadataTable, testTime)
	require.NoError(t, err)

	// Cancel the context
	cancel()

	// Try to get tombstone timestamp with canceled context
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.Error(t, err)
	assert.True(t, retrievedTime.IsZero())
}

func TestGetTombstoneTimestampWithInvalidData(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_get_tombstone_timestamp_invalid_data"

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Insert invalid timestamp data (non-numeric string)
	query := `INSERT INTO ` + metadataTable + ` (k, v) VALUES (?, ?)`
	_, err = dbRW.ExecContext(ctx, query, metadataKeyTombstoneTimestamp, "invalid_timestamp")
	require.NoError(t, err)

	// Try to get tombstone timestamp with invalid data
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse timestamp")
	assert.True(t, retrievedTime.IsZero())
}

func TestGetTombstoneTimestampWithTimeout(t *testing.T) {
	baseCtx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_get_tombstone_timestamp_timeout"

	// Create metadata table with base context first
	err := createMetadataTable(baseCtx, dbRW, metadataTable)
	require.NoError(t, err)

	// Now create a context with very short timeout
	ctx, cancel := context.WithTimeout(baseCtx, 1*time.Millisecond)
	defer cancel()

	// Wait for context to timeout
	time.Sleep(10 * time.Millisecond)

	// Try to get tombstone timestamp with timed-out context
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
	assert.True(t, retrievedTime.IsZero())
}

func TestTombstoneTimestampPersistence(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_tombstone_timestamp_persistence"

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		metadataTable: metadataTable,
		dbRW:          dbRW,
		dbRO:          dbRO,
	}

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Set multiple tombstone timestamps and verify persistence
	timestamps := []time.Time{
		time.Now().UTC().Add(-3 * time.Hour),
		time.Now().UTC().Add(-2 * time.Hour),
		time.Now().UTC().Add(-1 * time.Hour),
		time.Now().UTC(),
	}

	for i, ts := range timestamps {
		// Set tombstone timestamp
		err = store.Tombstone(ts)
		require.NoError(t, err, "Failed to set tombstone timestamp %d", i)

		// Verify it was set correctly
		retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
		require.NoError(t, err, "Failed to get tombstone timestamp %d", i)
		assert.Equal(t, ts.Unix(), retrievedTime.Unix(), "Timestamp %d mismatch", i)
	}

	// Verify only the last timestamp is stored (INSERT OR REPLACE behavior)
	finalTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.NoError(t, err)
	assert.Equal(t, timestamps[len(timestamps)-1].Unix(), finalTime.Unix())
}

func TestTombstoneZeroTime(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_tombstone_zero_time"

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		metadataTable: metadataTable,
		dbRW:          dbRW,
		dbRO:          dbRO,
	}

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Set tombstone to zero time
	zeroTime := time.Time{}
	err = store.Tombstone(zeroTime)
	require.NoError(t, err)

	// Verify zero time was set correctly
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.NoError(t, err)
	assert.Equal(t, zeroTime.Unix(), retrievedTime.Unix())
}

func TestTombstoneFutureTime(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	metadataTable := "test_tombstone_future_time"

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		metadataTable: metadataTable,
		dbRW:          dbRW,
		dbRO:          dbRO,
	}

	// Create metadata table
	err := createMetadataTable(ctx, dbRW, metadataTable)
	require.NoError(t, err)

	// Set tombstone to future time
	futureTime := time.Now().UTC().Add(24 * time.Hour)
	err = store.Tombstone(futureTime)
	require.NoError(t, err)

	// Verify future time was set correctly
	retrievedTime, err := getTombstoneTimestamp(ctx, dbRO, metadataTable)
	require.NoError(t, err)
	assert.Equal(t, futureTime.Unix(), retrievedTime.Unix())
}

// Helper function to parse timestamp string to int64
func parseTimestamp(t *testing.T, timestampStr string) int64 {
	t.Helper()
	ts, err := time.Parse("2006-01-02 15:04:05", timestampStr)
	if err != nil {
		// Try parsing as Unix timestamp string
		unixTS, err := strconv.ParseInt(timestampStr, 10, 64)
		require.NoError(t, err)
		return unixTS
	}
	return ts.Unix()
}

// Helper function to check if string contains any of the given substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
