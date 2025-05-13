package gpudstate

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	err := CreateTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Verify table exists
	var name string
	err = dbRO.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", TableNameMachineMetadata).Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, TableNameMachineMetadata, name)

	// Verify columns exist
	columns := []string{ColumnMachineID, ColumnUnixSeconds, ColumnToken, ColumnComponents}
	for _, col := range columns {
		var columnName string
		err = dbRO.QueryRow("SELECT name FROM pragma_table_info(?) WHERE name=?", TableNameMachineMetadata, col).Scan(&columnName)
		require.NoError(t, err)
		assert.Equal(t, col, columnName)
	}
}

func TestMachineIDOperations(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table
	err := CreateTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Test getting non-existent machine ID
	id, err := ReadMachineID(ctx, dbRO)
	assert.NoError(t, err)
	assert.Empty(t, id)

	// Test recording machine ID
	err = RecordMachineID(ctx, dbRW, dbRO, testMachineID)
	require.NoError(t, err)

	// Verify machine ID was recorded
	id, err = ReadMachineID(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, testMachineID, id)

	// Test attempting to record a different machine ID (should fail)
	differentID := "different-id"
	err = RecordMachineID(ctx, dbRW, dbRO, differentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already assigned")

	// Verify original machine ID is still in place
	id, err = ReadMachineID(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, testMachineID, id)

	// Test recording the same machine ID again (should succeed)
	err = RecordMachineID(ctx, dbRW, dbRO, testMachineID)
	assert.NoError(t, err)
}

func TestLoginInfoOperations(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table
	err := CreateTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Insert a machine ID for testing
	insertTestMachineID(t, ctx, dbRW, testMachineID)

	// Test getting login info for a machine ID with no token yet
	token, err := GetLoginInfo(ctx, dbRO, testMachineID)
	assert.NoError(t, err)
	assert.Empty(t, token)

	// Test getting login info for non-existent machine ID
	nonExistentID := "non-existent-id"
	token, err = GetLoginInfo(ctx, dbRO, nonExistentID)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, sql.ErrNoRows))
	assert.Empty(t, token)

	// Test updating login info
	testToken := "test-token-123"
	err = UpdateLoginInfo(ctx, dbRW, testMachineID, testToken)
	require.NoError(t, err)

	// Verify login info was updated
	token, err = GetLoginInfo(ctx, dbRO, testMachineID)
	assert.NoError(t, err)
	assert.Equal(t, testToken, token)

	// Test updating login info again with a different token
	updatedToken := "updated-token-456"
	err = UpdateLoginInfo(ctx, dbRW, testMachineID, updatedToken)
	require.NoError(t, err)

	// Verify login info was updated
	token, err = GetLoginInfo(ctx, dbRO, testMachineID)
	assert.NoError(t, err)
	assert.Equal(t, updatedToken, token)

	// Test updating non-existent machine ID (should not error but no row affected)
	err = UpdateLoginInfo(ctx, dbRW, nonExistentID, testToken)
	assert.NoError(t, err)
}

func TestRecordMetrics(t *testing.T) {
	t.Parallel()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table first to ensure we have something to measure
	err := CreateTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Insert some data to have a non-empty database
	insertTestMachineID(t, ctx, dbRW, testMachineID)

	// Verify recording metrics doesn't error
	err = RecordDBSize(ctx, dbRW)
	require.NoError(t, err)
}
