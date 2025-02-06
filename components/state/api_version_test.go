package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTableAPIVersion(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	err := CreateTableAPIVersion(ctx, dbRW)
	require.NoError(t, err)

	// Verify table exists
	var name string
	err = dbRO.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", TableNameAPIVersion).Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, TableNameAPIVersion, name)

	// Verify column exists
	var columnName string
	err = dbRO.QueryRow("SELECT name FROM pragma_table_info(?) WHERE name=?", TableNameAPIVersion, ColumnAPIVersion).Scan(&columnName)
	require.NoError(t, err)
	assert.Equal(t, ColumnAPIVersion, columnName)
}

func TestAPIVersionOperations(t *testing.T) {
	t.Skip()

	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table
	err := CreateTableAPIVersion(ctx, dbRW)
	require.NoError(t, err)

	// Test reading non-existent version
	version, err := ReadAPIVersion(ctx, dbRO)
	assert.Equal(t, sql.ErrNoRows, err)
	assert.Empty(t, version)

	// Test updating version
	testVersion := "v1.0.0"
	err = UpdateAPIVersion(ctx, dbRW, testVersion)
	assert.NoError(t, err)

	// Test reading updated version
	version, err = ReadAPIVersion(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, testVersion, version)

	// Test updating to new version
	newVersion := "v2.0.0"
	err = UpdateAPIVersion(ctx, dbRW, newVersion)
	assert.NoError(t, err)

	// Test reading new version
	version, err = ReadAPIVersion(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, newVersion, version)

	// Verify directly in the database
	var dbVersion string
	err = dbRO.QueryRowContext(ctx, "SELECT version FROM api_version").Scan(&dbVersion)
	assert.NoError(t, err)
	assert.Equal(t, newVersion, dbVersion)
}

func TestUpdateAPIVersionIfNotExists(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table
	err := CreateTableAPIVersion(ctx, dbRW)
	require.NoError(t, err)

	// Test when version doesn't exist
	testVersion := "v1.0.0"
	version, err := UpdateAPIVersionIfNotExists(ctx, dbRW, testVersion)
	assert.NoError(t, err)
	assert.Equal(t, testVersion, version)

	// Test when version exists
	newVersion := "v2.0.0"
	version, err = UpdateAPIVersionIfNotExists(ctx, dbRW, newVersion)
	assert.NoError(t, err)
	assert.Equal(t, testVersion, version) // Should return existing version

	// Verify version wasn't updated
	version, err = ReadAPIVersion(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, testVersion, version)
}
