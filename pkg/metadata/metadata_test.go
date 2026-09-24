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

func TestSetAndReadMetadata_LastSentNodeLabels(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	require.NoError(t, CreateTableMetadata(ctx, dbRW))

	initialLabels := `{"rack":"r42"}`
	require.NoError(t, SetMetadata(ctx, dbRW, MetadataKeyLastSentNodeLabels, initialLabels))

	value, err := ReadMetadata(ctx, dbRO, MetadataKeyLastSentNodeLabels)
	require.NoError(t, err)
	assert.Equal(t, initialLabels, value)

	updatedLabels := `{}`
	require.NoError(t, SetMetadata(ctx, dbRW, MetadataKeyLastSentNodeLabels, updatedLabels))

	value, err = ReadMetadata(ctx, dbRO, MetadataKeyLastSentNodeLabels)
	require.NoError(t, err)
	assert.Equal(t, updatedLabels, value)

	var rowCount int
	err = dbRO.QueryRow(
		"SELECT COUNT(*) FROM "+tableNameGPUdMetadata+" WHERE "+columnKey+" = ?",
		MetadataKeyLastSentNodeLabels,
	).Scan(&rowCount)
	require.NoError(t, err)
	assert.Equal(t, 1, rowCount)
}

func TestSetMetadata_EmptyValue(t *testing.T) {
	t.Parallel()
	for _, initial := range []string{"10.16.5.100", ""} {
		t.Run(initial, func(t *testing.T) {
			db, _, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()
			ctx := context.Background()
			require.NoError(t, CreateTableMetadata(ctx, db))

			for _, value := range []string{initial, "", "10.16.5.101"} {
				require.NoError(t, SetMetadata(ctx, db, MetadataKeyPrivateIP, value))
				got, err := ReadMetadata(ctx, db, MetadataKeyPrivateIP)
				require.NoError(t, err)
				require.Equal(t, value, got)
				var count int
				require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM gpud_metadata WHERE key = ?", MetadataKeyPrivateIP).Scan(&count))
				require.Equal(t, 1, count)
			}

			var before, after int
			require.NoError(t, db.QueryRow("SELECT total_changes()").Scan(&before))
			require.NoError(t, SetMetadata(ctx, db, MetadataKeyPrivateIP, "10.16.5.101"))
			require.NoError(t, db.QueryRow("SELECT total_changes()").Scan(&after))
			require.Equal(t, before, after, "unchanged values must not update the row")
		})
	}
}

func TestSetMetadata_ConcurrentCreation(t *testing.T) {
	t.Parallel()
	db, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, CreateTableMetadata(ctx, db))

	const writers = 16
	start := make(chan struct{})
	errs := make(chan error, writers)
	for range writers {
		go func() {
			<-start
			errs <- SetMetadata(ctx, db, MetadataKeyPrivateIP, "10.16.5.101")
		}()
	}
	close(start)
	for range writers {
		assert.NoError(t, <-errs)
	}
	values, err := ReadAllMetadata(ctx, db)
	require.NoError(t, err)
	require.Equal(t, map[string]string{MetadataKeyPrivateIP: "10.16.5.101"}, values)
}

func TestReadAllMetadata_IncludesLastSentNodeLabels(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	require.NoError(t, CreateTableMetadata(ctx, dbRW))
	require.NoError(t, SetMetadata(ctx, dbRW, MetadataKeyMachineID, "machine-123"))
	require.NoError(t, SetMetadata(ctx, dbRW, MetadataKeyToken, "token-abc"))
	require.NoError(t, SetMetadata(ctx, dbRW, MetadataKeyLastSentNodeLabels, `{"team":"ml"}`))

	metadata, err := ReadAllMetadata(ctx, dbRO)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		MetadataKeyMachineID:          "machine-123",
		MetadataKeyToken:              "token-abc",
		MetadataKeyLastSentNodeLabels: `{"team":"ml"}`,
	}, metadata)
}
