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

func TestCreateMetadataTable(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table"

	// Test successful table creation
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Verify the table was created with correct schema
	rows, err := dbRW.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	require.NoError(t, err)
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var dfltValue sql.NullString // Handle NULL values properly
		var pk string
		err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk)
		require.NoError(t, err)
		columns[name] = dataType
	}

	// Verify correct columns exist
	assert.Equal(t, "TEXT", columns[metadataColumnKey])
	assert.Equal(t, "TEXT", columns[metadataColumnValue])
	assert.Len(t, columns, 2)

	// Verify primary key constraint by checking sqlite_master
	var count int
	err = dbRW.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM pragma_index_list('%s') WHERE \"unique\" = 1", tableName)).Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 1, "Should have at least one unique index (primary key)")
}

func TestCreateMetadataTableIdempotent(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table_idempotent"

	// Create table first time
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Create table second time - should not fail due to IF NOT EXISTS
	err = createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Verify table still exists and has correct structure
	var count int
	err = dbRW.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='%s'", tableName)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestCreateMetadataTableWithClosedDB(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Close database to force error
	dbRW.Close()

	tableName := "test_metadata_table_error"

	// Should fail with closed database
	err := createMetadataTable(ctx, dbRW, tableName)
	require.Error(t, err)
}

func TestCreateMetadataTableWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table_canceled"

	// Cancel context immediately
	cancel()

	// Should fail with canceled context
	err := createMetadataTable(ctx, dbRW, tableName)
	require.Error(t, err)
}

func TestCreateMetadataTableWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table_timeout"

	// Should fail with timeout
	err := createMetadataTable(ctx, dbRW, tableName)
	require.Error(t, err)
}

func TestCreateMetadataTableTransactionRollback(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create a table with same name but different schema to simulate exec error
	tableName := "test_metadata_table_rollback"
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s (different_column TEXT)", tableName))
	require.NoError(t, err)

	// Now try to create metadata table with same name - should fail because table already exists with different schema
	// SQLite will not error on CREATE TABLE IF NOT EXISTS even if schema differs
	// So we need to create without IF NOT EXISTS to force error

	// Let's manually create the transaction and table without IF NOT EXISTS
	tx, err := dbRW.BeginTx(ctx, nil)
	require.NoError(t, err)

	// This should fail because table already exists
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE %s (
	%s TEXT PRIMARY KEY NOT NULL,
	%s TEXT NOT NULL
);`, tableName,
		metadataColumnKey,
		metadataColumnValue,
	))

	// Should get an error
	require.Error(t, err)

	// Rollback the transaction
	_ = tx.Rollback()

	// Verify original table still exists (transaction was rolled back properly)
	rows, err := dbRW.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	require.NoError(t, err)
	defer rows.Close()

	columns := make([]string, 0)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var dfltValue sql.NullString
		var pk string
		err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk)
		require.NoError(t, err)
		columns = append(columns, name)
	}

	// Should still have the original column
	assert.Contains(t, columns, "different_column")
	assert.NotContains(t, columns, metadataColumnKey)
	assert.NotContains(t, columns, metadataColumnValue)
}

func TestCreateMetadataTableWithData(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table_with_data"

	// Create table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert some test data
	query := fmt.Sprintf(`INSERT INTO %s (%s, %s) VALUES (?, ?)`, tableName, metadataColumnKey, metadataColumnValue)
	_, err = dbRW.ExecContext(ctx, query, "test_key", "test_value")
	require.NoError(t, err)

	// Verify data was inserted correctly
	var value string
	selectQuery := fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ?`, metadataColumnValue, tableName, metadataColumnKey)
	err = dbRW.QueryRowContext(ctx, selectQuery, "test_key").Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "test_value", value)
}

func TestCreateMetadataTableWithInvalidTableName(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Try to create table with invalid characters in name that will cause syntax error
	invalidTableName := "test metadata table with spaces"

	// Should fail with syntax error because of spaces in table name
	err := createMetadataTable(ctx, dbRW, invalidTableName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "syntax error")
}

func TestCreateMetadataTablePrimaryKeyConstraint(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_metadata_table_pk_constraint"

	// Create table
	err := createMetadataTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert first row
	query := fmt.Sprintf(`INSERT INTO %s (%s, %s) VALUES (?, ?)`, tableName, metadataColumnKey, metadataColumnValue)
	_, err = dbRW.ExecContext(ctx, query, "duplicate_key", "value1")
	require.NoError(t, err)

	// Try to insert duplicate key - should fail due to primary key constraint
	_, err = dbRW.ExecContext(ctx, query, "duplicate_key", "value2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UNIQUE constraint failed")
}

func TestCreateMetadataTableWithEmptyTableName(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Try to create table with empty name - should fail
	err := createMetadataTable(ctx, dbRW, "")
	require.Error(t, err)
}
