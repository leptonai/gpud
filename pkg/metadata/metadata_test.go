package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestCreateTableMetadata(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	err := CreateTableMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Verify table exists
	var name string
	err = dbRO.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableNameGPUdMetadata).Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, tableNameGPUdMetadata, name)

	// Verify columns exist
	columns := []string{columnKey, columnValue}
	for _, col := range columns {
		var columnName string
		err = dbRO.QueryRow("SELECT name FROM pragma_table_info(?) WHERE name=?", tableNameGPUdMetadata, col).Scan(&columnName)
		require.NoError(t, err)
		assert.Equal(t, col, columnName)
	}

	// Test idempotency - calling create again should not fail
	err = CreateTableMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Test error case with canceled context
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Immediately cancel
	err = CreateTableMetadata(canceledCtx, dbRW)
	assert.Error(t, err)
}

func TestSetAndReadMetadata(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create metadata table
	err := CreateTableMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Test setting and reading metadata
	testKey := "test_key"
	testValue := "test_value"

	// Initial read should return empty string
	value, err := ReadMetadata(ctx, dbRO, testKey)
	require.NoError(t, err)
	assert.Empty(t, value)

	// Set metadata
	err = SetMetadata(ctx, dbRW, testKey, testValue)
	require.NoError(t, err)

	// Read metadata
	value, err = ReadMetadata(ctx, dbRO, testKey)
	require.NoError(t, err)
	assert.Equal(t, testValue, value)

	// Update metadata
	updatedValue := "updated_value"
	err = SetMetadata(ctx, dbRW, testKey, updatedValue)
	require.NoError(t, err)

	// Read updated metadata
	value, err = ReadMetadata(ctx, dbRO, testKey)
	require.NoError(t, err)
	assert.Equal(t, updatedValue, value)

	// Setting the same value again should not cause an error
	err = SetMetadata(ctx, dbRW, testKey, updatedValue)
	require.NoError(t, err)

	// Test with predefined metadata keys
	err = SetMetadata(ctx, dbRW, MetadataKeyMachineID, "machine-123")
	require.NoError(t, err)
	machineID, err := ReadMetadata(ctx, dbRO, MetadataKeyMachineID)
	require.NoError(t, err)
	assert.Equal(t, "machine-123", machineID)

	err = SetMetadata(ctx, dbRW, MetadataKeyToken, "token-abc")
	require.NoError(t, err)
	token, err := ReadMetadata(ctx, dbRO, MetadataKeyToken)
	require.NoError(t, err)
	assert.Equal(t, "token-abc", token)

	// Test error case in ReadMetadata during SetMetadata
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Immediately cancel
	err = SetMetadata(canceledCtx, dbRW, "error-key", "error-value")
	assert.Error(t, err)

	// Test error case in insert operation
	err = SetMetadata(canceledCtx, dbRW, "new-key", "new-value")
	assert.Error(t, err)

	// Test error case in update operation
	err = SetMetadata(canceledCtx, dbRW, testKey, "another-value")
	assert.Error(t, err)

	// Test error case in ReadMetadata
	_, err = ReadMetadata(canceledCtx, dbRO, testKey)
	assert.Error(t, err)
}
