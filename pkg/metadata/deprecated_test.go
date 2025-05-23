package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func createDeprecatedTableMachineMetadata(ctx context.Context, dbRW *sql.DB) error {
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s TEXT PRIMARY KEY,
	%s INTEGER,
	%s TEXT,
	%s TEXT
);`, deprecatedTableNameMachineMetadata, deprecatedColumnMachineID, deprecatedColumnUnixSeconds, deprecatedColumnToken, deprecatedColumnComponents))
	return err
}

const testMachineID = "test-machine-id"

// insertTestMachineID is a helper function to insert a machine ID for testing
func insertTestMachineID(t *testing.T, ctx context.Context, dbRW *sql.DB, machineID string) {
	t.Helper()
	_, err := dbRW.ExecContext(ctx,
		"INSERT OR REPLACE INTO machine_metadata (machine_id, unix_seconds) VALUES (?, ?)",
		machineID, time.Now().Unix())
	require.NoError(t, err)
}

func TestCreateTableMachineMetadata(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	err := createDeprecatedTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Verify table exists
	var name string
	err = dbRO.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", deprecatedTableNameMachineMetadata).Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, deprecatedTableNameMachineMetadata, name)

	// Verify columns exist
	columns := []string{deprecatedColumnMachineID, deprecatedColumnUnixSeconds, deprecatedColumnToken, deprecatedColumnComponents}
	for _, col := range columns {
		var columnName string
		err = dbRO.QueryRow("SELECT name FROM pragma_table_info(?) WHERE name=?", deprecatedTableNameMachineMetadata, col).Scan(&columnName)
		require.NoError(t, err)
		assert.Equal(t, col, columnName)
	}
}

func TestLoginInfoOperations(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table
	err := createDeprecatedTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Insert a machine ID for testing
	insertTestMachineID(t, ctx, dbRW, testMachineID)

	// Test getting login info for a machine ID with no token yet
	token, err := readTokenFromDeprecatedTable(ctx, dbRO, testMachineID)
	assert.NoError(t, err)
	assert.Empty(t, token)

	// Test getting login info for non-existent machine ID
	nonExistentID := "non-existent-id"
	token, err = readTokenFromDeprecatedTable(ctx, dbRO, nonExistentID)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, sql.ErrNoRows))
	assert.Empty(t, token)
}

func TestRecordMetrics(t *testing.T) {
	t.Parallel()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table first to ensure we have something to measure
	err := createDeprecatedTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Insert some data to have a non-empty database
	insertTestMachineID(t, ctx, dbRW, testMachineID)
}

func TestReadMachineIDFromDeprecatedTable(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Test 1: When table doesn't exist
	machineID, err := readMachineIDFromDeprecatedTable(ctx, dbRO)
	assert.Error(t, err) // Should error because table doesn't exist
	assert.Empty(t, machineID)

	// Create table for further tests
	err = createDeprecatedTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Test 2: When table exists but no entries
	machineID, err = readMachineIDFromDeprecatedTable(ctx, dbRO)
	require.NoError(t, err)
	assert.Empty(t, machineID)

	// Test 3: With machine ID in table
	testMachineID := "test-machine-id"
	_, err = dbRW.ExecContext(ctx,
		"INSERT INTO machine_metadata (machine_id, unix_seconds) VALUES (?, ?)",
		testMachineID, time.Now().Unix())
	require.NoError(t, err)

	machineID, err = readMachineIDFromDeprecatedTable(ctx, dbRO)
	require.NoError(t, err)
	assert.Equal(t, testMachineID, machineID)
}

func TestReadTokenFromDeprecatedTable(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create deprecated table
	err := createDeprecatedTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	machineID := "test-machine-id"

	// Test 1: When machine ID doesn't exist
	token, err := readTokenFromDeprecatedTable(ctx, dbRO, machineID)
	assert.Error(t, err) // Should be sql.ErrNoRows
	assert.Empty(t, token)

	// Test 2: When machine ID exists but no token
	_, err = dbRW.ExecContext(ctx,
		"INSERT INTO machine_metadata (machine_id, unix_seconds) VALUES (?, ?)",
		machineID, time.Now().Unix())
	require.NoError(t, err)

	token, err = readTokenFromDeprecatedTable(ctx, dbRO, machineID)
	require.NoError(t, err)
	assert.Empty(t, token) // COALESCE returns empty string

	// Test 3: When machine ID and token exist
	testToken := "test-token"
	_, err = dbRW.ExecContext(ctx,
		"UPDATE machine_metadata SET token = ? WHERE machine_id = ?",
		testToken, machineID)
	require.NoError(t, err)

	token, err = readTokenFromDeprecatedTable(ctx, dbRO, machineID)
	require.NoError(t, err)
	assert.Equal(t, testToken, token)
}
