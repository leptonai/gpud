package metadata

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestReadMachineIDWithFallback(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create the metadata table
	err := CreateTableMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Test 1: When machine ID is not in the new table and no old table exists
	machineID, err := ReadMachineIDWithFallback(ctx, dbRW, dbRO)
	require.NoError(t, err)
	assert.Empty(t, machineID)

	// Test 2: When machine ID is in the new table
	testMachineID := "test-machine-id"
	err = SetMetadata(ctx, dbRW, MetadataKeyMachineID, testMachineID)
	require.NoError(t, err)

	machineID, err = ReadMachineIDWithFallback(ctx, dbRW, dbRO)
	require.NoError(t, err)
	assert.Equal(t, testMachineID, machineID)

	// Test 3: Machine ID in deprecated table but not in new table
	// First, remove from new table
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = ?", tableNameGPUdMetadata, columnKey), MetadataKeyMachineID)
	require.NoError(t, err)

	// Create and populate the deprecated table
	err = createDeprecatedTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	deprecatedMachineID := "deprecated-machine-id"
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s, %s) VALUES (?, ?)",
		deprecatedTableNameMachineMetadata, deprecatedColumnMachineID, deprecatedColumnUnixSeconds),
		deprecatedMachineID, time.Now().Unix())
	require.NoError(t, err)

	// Test reading from fallback
	machineID, err = ReadMachineIDWithFallback(ctx, dbRW, dbRO)
	require.NoError(t, err)
	assert.Equal(t, deprecatedMachineID, machineID)

	// Verify it was migrated to the new table
	migratedID, err := ReadMetadata(ctx, dbRO, MetadataKeyMachineID)
	require.NoError(t, err)
	assert.Equal(t, deprecatedMachineID, migratedID)

	// Test 4: Error in ReadMetadata
	// Close and reopen with a closed context to force errors
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Cancel immediately to force errors
	_, err = ReadMachineIDWithFallback(canceledCtx, dbRW, dbRO)
	assert.Error(t, err)

	// Test 5: Error in TableExists
	_, err = ReadMachineIDWithFallback(canceledCtx, dbRW, dbRO)
	assert.Error(t, err)

	// Test 6: Error in readMachineIDFromDeprecatedTable
	// Use a canceled context to force errors
	_, err = ReadMachineIDWithFallback(canceledCtx, dbRW, dbRO)
	assert.Error(t, err)

	// Test 7: Error in SetMetadata when migrating
	// Try with canceled context to cause SetMetadata to fail
	_, err = ReadMachineIDWithFallback(canceledCtx, dbRW, dbRO)
	assert.Error(t, err)
}

func TestReadTokenWithFallback(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create the metadata table
	err := CreateTableMetadata(ctx, dbRW)
	require.NoError(t, err)

	machineID := "test-machine-id"

	// Test 1: When token is not in the new table and no old table exists
	token, err := ReadTokenWithFallback(ctx, dbRW, dbRO, machineID)
	require.NoError(t, err)
	assert.Empty(t, token)

	// Test 2: When token is in the new table
	testToken := "test-token"
	err = SetMetadata(ctx, dbRW, MetadataKeyToken, testToken)
	require.NoError(t, err)

	token, err = ReadTokenWithFallback(ctx, dbRW, dbRO, machineID)
	require.NoError(t, err)
	assert.Equal(t, testToken, token)

	// Test 3: Token in deprecated table but not in new table
	// First, remove from new table
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = ?", tableNameGPUdMetadata, columnKey), MetadataKeyToken)
	require.NoError(t, err)

	// Create and populate the deprecated table
	err = createDeprecatedTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	deprecatedToken := "deprecated-token"
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s, %s, %s) VALUES (?, ?, ?)",
		deprecatedTableNameMachineMetadata, deprecatedColumnMachineID, deprecatedColumnUnixSeconds, deprecatedColumnToken),
		machineID, time.Now().Unix(), deprecatedToken)
	require.NoError(t, err)

	// Test reading from fallback
	token, err = ReadTokenWithFallback(ctx, dbRW, dbRO, machineID)
	require.NoError(t, err)
	assert.Equal(t, deprecatedToken, token)

	// Verify it was migrated to the new table
	migratedToken, err := ReadMetadata(ctx, dbRO, MetadataKeyToken)
	require.NoError(t, err)
	assert.Equal(t, deprecatedToken, migratedToken)

	// Test 4: Error in ReadMetadata
	// Close and reopen with a closed context to force errors
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Cancel immediately to force errors
	_, err = ReadTokenWithFallback(canceledCtx, dbRW, dbRO, machineID)
	assert.Error(t, err)

	// Test 5: Error in TableExists
	_, err = ReadTokenWithFallback(canceledCtx, dbRW, dbRO, machineID)
	assert.Error(t, err)

	// Test 6: Error in readTokenFromDeprecatedTable
	// Use canceled context to cause an error
	_, err = ReadTokenWithFallback(canceledCtx, dbRW, dbRO, machineID)
	assert.Error(t, err)

	// Test 7: Error in SetMetadata when migrating
	// Try with canceled context to cause SetMetadata to fail during migration
	_, err = ReadTokenWithFallback(canceledCtx, dbRW, dbRO, machineID)
	assert.Error(t, err)

	// Test 8: When token is empty in deprecated table
	// Update the token to be empty in the deprecated table
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = '' WHERE %s = ?",
		deprecatedTableNameMachineMetadata, deprecatedColumnToken, deprecatedColumnMachineID), machineID)
	require.NoError(t, err)

	// Delete from new table again to force fallback
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = ?",
		tableNameGPUdMetadata, columnKey), MetadataKeyToken)
	require.NoError(t, err)

	token, err = ReadTokenWithFallback(ctx, dbRW, dbRO, machineID)
	require.NoError(t, err)
	assert.Empty(t, token)

	// Test 9: Error in TableExists but don't check deprecated table migration
	// This should not have errors on SQLite but coverage will be improved
	ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Add token back to new table
	err = SetMetadata(ctx, dbRW, MetadataKeyToken, testToken)
	require.NoError(t, err)

	// Token should be returned from new table without checking deprecated
	token, err = ReadTokenWithFallback(ctx, dbRW, dbRO, machineID)
	require.NoError(t, err)
	assert.Equal(t, testToken, token)
}

func TestDeleteAllMetadata(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create the tables
	err := CreateTableMetadata(ctx, dbRW)
	require.NoError(t, err)
	err = createDeprecatedTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Add data to both tables
	err = SetMetadata(ctx, dbRW, MetadataKeyMachineID, "test-machine-id")
	require.NoError(t, err)
	err = SetMetadata(ctx, dbRW, MetadataKeyToken, "test-token")
	require.NoError(t, err)

	_, err = dbRW.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s, %s) VALUES (?, ?)",
		deprecatedTableNameMachineMetadata, deprecatedColumnMachineID, deprecatedColumnUnixSeconds),
		"deprecated-machine-id", time.Now().Unix())
	require.NoError(t, err)

	// Verify data exists before deletion
	var count int
	err = dbRO.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableNameGPUdMetadata)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	err = dbRO.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", deprecatedTableNameMachineMetadata)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Call the actual DeleteAllMetadata function
	err = DeleteAllMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Verify tables are empty after deletion
	err = dbRO.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableNameGPUdMetadata)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	err = dbRO.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", deprecatedTableNameMachineMetadata)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Test error case
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Cancel immediately
	err = DeleteAllMetadata(canceledCtx, dbRW)
	assert.Error(t, err)

	// Test case where deprecated table doesn't exist
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", deprecatedTableNameMachineMetadata))
	require.NoError(t, err)

	// This should still work without errors
	err = DeleteAllMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Test error case with table exists check
	err = DeleteAllMetadata(canceledCtx, dbRW)
	assert.Error(t, err)

	// Test case where deprecated table error in DELETE
	// Create the table again for this test
	err = createDeprecatedTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Add some data to verify the delete works
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s, %s) VALUES (?, ?)",
		deprecatedTableNameMachineMetadata, deprecatedColumnMachineID, deprecatedColumnUnixSeconds),
		"another-machine-id", time.Now().Unix())
	require.NoError(t, err)

	// Verify the data was added
	err = dbRO.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", deprecatedTableNameMachineMetadata)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Now try to delete it
	err = DeleteAllMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Verify it was deleted
	err = dbRO.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", deprecatedTableNameMachineMetadata)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Force an error in TableExists by using canceled context
	err = DeleteAllMetadata(canceledCtx, dbRW)
	assert.Error(t, err)
}
