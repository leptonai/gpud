package gpudstate

import (
	"context"
	"sync"
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

func TestUpdateAPIVersionIfNotExists(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table
	err := CreateTableAPIVersion(ctx, dbRW)
	require.NoError(t, err)

	// Test empty version string
	version, err := UpdateAPIVersionIfNotExists(ctx, dbRW, "")
	assert.Equal(t, ErrEmptyAPIVersion, err)
	assert.Empty(t, version)

	// Test when version doesn't exist
	testVersion := "v1.0.0"
	version, err = UpdateAPIVersionIfNotExists(ctx, dbRW, testVersion)
	assert.NoError(t, err)
	assert.Equal(t, testVersion, version)

	// Test when version exists
	newVersion := "v2.0.0"
	version, err = UpdateAPIVersionIfNotExists(ctx, dbRW, newVersion)
	assert.NoError(t, err)
	assert.Equal(t, testVersion, version) // Should return existing version

	// Verify version wasn't updated
	version, err = ReadLatestAPIVersion(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, testVersion, version)
}

func TestUpdateAPIVersionIfNotExistsComprehensive(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table
	err := CreateTableAPIVersion(ctx, dbRW)
	require.NoError(t, err)

	// Test 1: Empty version string
	version, err := UpdateAPIVersionIfNotExists(ctx, dbRW, "")
	assert.Equal(t, ErrEmptyAPIVersion, err)
	assert.Empty(t, version)

	// Test 2: Empty database - should insert version
	version, err = UpdateAPIVersionIfNotExists(ctx, dbRW, "v1.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "v1.0.0", version)

	// Verify version was inserted
	dbVersion, err := ReadLatestAPIVersion(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, "v1.0.0", dbVersion)

	// Test 3: Existing version - should not update
	version, err = UpdateAPIVersionIfNotExists(ctx, dbRW, "v2.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "v1.0.0", version) // Should return existing version

	// Verify version wasn't changed
	dbVersion, err = ReadLatestAPIVersion(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, "v1.0.0", dbVersion)

	// Test 4: Special characters in version
	// First, clear the table
	_, err = dbRW.ExecContext(ctx, "DELETE FROM api_version")
	require.NoError(t, err)

	specialVersion := "v1.0.0-alpha.1+build.123"
	version, err = UpdateAPIVersionIfNotExists(ctx, dbRW, specialVersion)
	assert.NoError(t, err)
	assert.Equal(t, specialVersion, version)

	// Test 5: Multiple concurrent updates - should maintain consistency
	// Clear the table
	_, err = dbRW.ExecContext(ctx, "DELETE FROM api_version")
	require.NoError(t, err)

	var wg sync.WaitGroup
	var mu sync.Mutex
	firstVersionSet := false
	var firstVersion string

	// Run multiple updates concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := UpdateAPIVersionIfNotExists(ctx, dbRW, "v2.0.0")
			assert.NoError(t, err)
			mu.Lock()
			if !firstVersionSet {
				firstVersionSet = true
				firstVersion = v
			} else {
				assert.Equal(t, firstVersion, v)
			}
			mu.Unlock()
		}(i)
	}

	// Run in main goroutine too
	for i := 0; i < 10; i++ {
		v, err := UpdateAPIVersionIfNotExists(ctx, dbRW, "v1.0.0")
		assert.NoError(t, err)
		mu.Lock()
		if !firstVersionSet {
			firstVersionSet = true
			firstVersion = v
		} else {
			assert.Equal(t, firstVersion, v)
		}
		mu.Unlock()
	}

	wg.Wait() // Wait for all goroutines to finish

	// Verify final version
	dbVersion, err = ReadLatestAPIVersion(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, firstVersion, dbVersion)

	// Test 6: Empty version string during concurrent updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		v, err := UpdateAPIVersionIfNotExists(ctx, dbRW, "")
		assert.Equal(t, ErrEmptyAPIVersion, err)
		assert.Empty(t, v)
	}()
	wg.Wait()

	// Test 7: Context cancellation
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately
	_, err = UpdateAPIVersionIfNotExists(cancelCtx, dbRW, "v3.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")

	// Verify version wasn't changed after context cancellation
	dbVersion, err = ReadLatestAPIVersion(ctx, dbRO)
	assert.NoError(t, err)
	assert.Equal(t, firstVersion, dbVersion)
}

func TestUpdateAPIVersionTransactionBehavior(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table
	err := CreateTableAPIVersion(ctx, dbRW)
	require.NoError(t, err)

	// Test successful transaction
	version, err := UpdateAPIVersionIfNotExists(ctx, dbRW, "v1.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "v1.0.0", version)

	// Verify the version was actually committed
	var count int
	err = dbRO.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_version WHERE version = ?", "v1.0.0").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Version should be committed in the database")

	// Test that subsequent updates don't create new rows
	version, err = UpdateAPIVersionIfNotExists(ctx, dbRW, "v2.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "v1.0.0", version) // Should return existing version

	err = dbRO.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_version").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Should still have only one row in the database")

	// Test empty version (should rollback)
	version, err = UpdateAPIVersionIfNotExists(ctx, dbRW, "")
	assert.Equal(t, ErrEmptyAPIVersion, err)
	assert.Empty(t, version)

	// Verify no additional rows were added after failed attempt
	err = dbRO.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_version").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Failed transaction should not add new rows")
}
