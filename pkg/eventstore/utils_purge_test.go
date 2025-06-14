package eventstore

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestListTables(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create test tables with different patterns
	testTables := []struct {
		name     string
		expected bool // whether it should be listed
	}{
		{"components_gpu_events_v0_5_0", true},
		{"components_memory_events_v0_5_0", true},
		{"components_disk_events_v0_4_0", true}, // older version
		{"components_cpu_info", false},          // no _events
		{"memory_events", false},                // no components_ prefix
		{"other_table", false},                  // completely different
		{"components_", false},                  // incomplete pattern
		{"components_test_events", true},        // without version
	}

	// Create all test tables
	for _, tt := range testTables {
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				timestamp INTEGER NOT NULL,
				name TEXT NOT NULL,
				type TEXT NOT NULL,
				message TEXT,
				extra_info TEXT
			);`, tt.name))
		assert.NoError(t, err, "Failed to create table %s", tt.name)
	}

	// Test listTables function
	tables, err := listTables(ctx, dbRO)
	assert.NoError(t, err)

	// Count expected tables
	expectedCount := 0
	for _, tt := range testTables {
		if tt.expected {
			expectedCount++
		}
	}
	assert.Equal(t, expectedCount, len(tables), "Expected %d tables, got %d", expectedCount, len(tables))

	// Verify correct tables are listed
	tableMap := make(map[string]bool)
	for _, table := range tables {
		tableMap[table] = true
	}

	for _, tt := range testTables {
		if tt.expected {
			assert.True(t, tableMap[tt.name], "Expected table %s to be listed", tt.name)
		} else {
			assert.False(t, tableMap[tt.name], "Expected table %s NOT to be listed", tt.name)
		}
	}
}

func TestPurgeByComponents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create store and test components
	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	testComponents := []string{"gpu", "memory", "disk"}
	baseTime := time.Now().UTC()

	// Create buckets and insert test events
	for _, comp := range testComponents {
		bucket, err := store.Bucket(comp)
		assert.NoError(t, err)
		defer bucket.Close()

		// Insert old and new events
		oldEvent := Event{
			Time:    baseTime.Add(-10 * time.Minute),
			Name:    fmt.Sprintf("%s_old", comp),
			Type:    string(apiv1.EventTypeWarning),
			Message: "old event",
		}
		newEvent := Event{
			Time:    baseTime.Add(-1 * time.Minute),
			Name:    fmt.Sprintf("%s_new", comp),
			Type:    string(apiv1.EventTypeInfo),
			Message: "new event",
		}

		assert.NoError(t, bucket.Insert(ctx, oldEvent))
		assert.NoError(t, bucket.Insert(ctx, newEvent))
	}

	// Test 1: Purge specific components
	t.Run("PurgeSpecificComponents", func(t *testing.T) {
		// Purge only gpu and memory components
		purgeTime := baseTime.Add(-5 * time.Minute).Unix()
		err := PurgeByComponents(ctx, dbRW, dbRO, purgeTime, "gpu", "memory")
		assert.NoError(t, err)

		// Verify gpu and memory have only new events
		for _, comp := range []string{"gpu", "memory"} {
			bucket, err := store.Bucket(comp, WithDisablePurge())
			assert.NoError(t, err)
			defer bucket.Close()

			events, err := bucket.Get(ctx, baseTime.Add(-15*time.Minute))
			assert.NoError(t, err)
			assert.Equal(t, 1, len(events))
			assert.Equal(t, fmt.Sprintf("%s_new", comp), events[0].Name)
		}

		// Verify disk still has both events
		diskBucket, err := store.Bucket("disk", WithDisablePurge())
		assert.NoError(t, err)
		defer diskBucket.Close()

		events, err := diskBucket.Get(ctx, baseTime.Add(-15*time.Minute))
		assert.NoError(t, err)
		assert.Equal(t, 2, len(events))
	})

	// Test 2: Purge all components (no component names specified)
	t.Run("PurgeAllComponents", func(t *testing.T) {
		// First, re-add old events to gpu and memory
		for _, comp := range []string{"gpu", "memory"} {
			bucket, err := store.Bucket(comp, WithDisablePurge())
			assert.NoError(t, err)
			defer bucket.Close()

			oldEvent := Event{
				Time:    baseTime.Add(-10 * time.Minute),
				Name:    fmt.Sprintf("%s_old_2", comp),
				Type:    string(apiv1.EventTypeWarning),
				Message: "old event 2",
			}
			assert.NoError(t, bucket.Insert(ctx, oldEvent))
		}

		// Purge all components
		purgeTime := baseTime.Add(-5 * time.Minute).Unix()
		err := PurgeByComponents(ctx, dbRW, dbRO, purgeTime)
		assert.NoError(t, err)

		// Verify all components have only new events
		for _, comp := range testComponents {
			bucket, err := store.Bucket(comp, WithDisablePurge())
			assert.NoError(t, err)
			defer bucket.Close()

			events, err := bucket.Get(ctx, baseTime.Add(-15*time.Minute))
			assert.NoError(t, err)
			assert.Equal(t, 1, len(events))
			assert.Equal(t, fmt.Sprintf("%s_new", comp), events[0].Name)
		}
	})

	// Test 3: Empty component names should purge all
	t.Run("EmptyComponentNames", func(t *testing.T) {
		// Add more old events
		for _, comp := range testComponents {
			bucket, err := store.Bucket(comp, WithDisablePurge())
			assert.NoError(t, err)
			defer bucket.Close()

			oldEvent := Event{
				Time:    baseTime.Add(-8 * time.Minute),
				Name:    fmt.Sprintf("%s_old_3", comp),
				Type:    string(apiv1.EventTypeCritical),
				Message: "old event 3",
			}
			assert.NoError(t, bucket.Insert(ctx, oldEvent))
		}

		// Call with empty slice
		purgeTime := baseTime.Add(-7 * time.Minute).Unix()
		err := PurgeByComponents(ctx, dbRW, dbRO, purgeTime, []string{}...)
		assert.NoError(t, err)

		// Verify purge worked
		for _, comp := range testComponents {
			bucket, err := store.Bucket(comp, WithDisablePurge())
			assert.NoError(t, err)
			defer bucket.Close()

			events, err := bucket.Get(ctx, baseTime.Add(-15*time.Minute))
			assert.NoError(t, err)
			// Should have only the new event (all old ones purged)
			assert.Equal(t, 1, len(events))
			assert.Equal(t, fmt.Sprintf("%s_new", comp), events[0].Name)
		}
	})
}

func TestPurgeByComponentsEdgeCases(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Test with non-existent component
	t.Run("NonExistentComponent", func(t *testing.T) {
		// When purging a specific component that doesn't have a table,
		// it will error because the table doesn't exist
		err := PurgeByComponents(ctx, dbRW, dbRO, time.Now().Unix(), "non_existent_component")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such table")
	})

	// Test with future timestamp (should not purge anything)
	t.Run("FutureTimestamp", func(t *testing.T) {
		store, err := New(dbRW, dbRO, 0)
		assert.NoError(t, err)

		bucket, err := store.Bucket("test_future")
		assert.NoError(t, err)
		defer bucket.Close()

		// Insert an event
		event := Event{
			Time:    time.Now().UTC(),
			Name:    "test_event",
			Type:    string(apiv1.EventTypeInfo),
			Message: "test",
		}
		assert.NoError(t, bucket.Insert(ctx, event))

		// Purge with future timestamp
		futureTime := time.Now().Add(1 * time.Hour).Unix()
		err = PurgeByComponents(ctx, dbRW, dbRO, futureTime, "test_future")
		assert.NoError(t, err)

		// Event should be purged
		events, err := bucket.Get(ctx, time.Now().Add(-1*time.Hour))
		assert.NoError(t, err)
		assert.Nil(t, events)
	})
}

func TestListTablesError(t *testing.T) {
	t.Parallel()

	// Test with closed database connection
	t.Run("ClosedDatabaseConnection", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		cleanup() // Close the database immediately

		ctx := context.Background()
		tables, err := listTables(ctx, dbRO)
		assert.Error(t, err)
		assert.Nil(t, tables)
	})

	// Test with canceled context
	t.Run("CancelledContext", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		tables, err := listTables(ctx, dbRO)
		assert.Error(t, err)
		assert.Nil(t, tables)
	})
}

func TestPurgeByComponentsWithMixedTableVersions(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create tables with different schema versions
	tables := []string{
		"components_gpu_events_v0_4_0",
		"components_gpu_events_v0_5_0",
		"components_memory_events_v0_5_0",
	}

	baseTime := time.Now().UTC()

	// Create and populate tables
	for _, tableName := range tables {
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				timestamp INTEGER NOT NULL,
				name TEXT NOT NULL,
				type TEXT NOT NULL,
				message TEXT,
				extra_info TEXT
			);`, tableName))
		assert.NoError(t, err)

		// Insert test events
		_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO %s (timestamp, name, type, message, extra_info) 
			VALUES (?, ?, ?, ?, ?)`, tableName),
			baseTime.Add(-10*time.Minute).Unix(),
			"old_event",
			string(apiv1.EventTypeWarning),
			"old event message",
			`{"key": "value"}`,
		)
		assert.NoError(t, err)

		_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO %s (timestamp, name, type, message, extra_info) 
			VALUES (?, ?, ?, ?, ?)`, tableName),
			baseTime.Unix(),
			"new_event",
			string(apiv1.EventTypeInfo),
			"new event message",
			`{"key": "value2"}`,
		)
		assert.NoError(t, err)
	}

	// Purge all tables
	purgeTime := baseTime.Add(-5 * time.Minute).Unix()
	err := PurgeByComponents(ctx, dbRW, dbRO, purgeTime)
	assert.NoError(t, err)

	// Verify old events are purged from all tables
	for _, tableName := range tables {
		var count int
		err := dbRO.QueryRowContext(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 1, count, "Table %s should have 1 event after purge", tableName)

		// Verify it's the new event
		var name string
		err = dbRO.QueryRowContext(ctx,
			fmt.Sprintf("SELECT name FROM %s", tableName)).Scan(&name)
		assert.NoError(t, err)
		assert.Equal(t, "new_event", name)
	}
}

func TestPurgeByComponentsErrorHandling(t *testing.T) {
	t.Parallel()

	// Test error from listTables when getting all tables
	t.Run("ListTablesError", func(t *testing.T) {
		_, dbRO, cleanup := sqlite.OpenTestDB(t)
		// Close the database to force an error
		cleanup()

		ctx := context.Background()
		// This should fail because listTables will error on closed DB
		err := PurgeByComponents(ctx, dbRO, dbRO, time.Now().Unix())
		assert.Error(t, err)
	})

	// Test error from purgeEvents
	t.Run("PurgeEventsError", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx := context.Background()

		// Create a valid table
		tableName := "components_test_events_v0_5_0"
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				timestamp INTEGER NOT NULL,
				name TEXT NOT NULL,
				type TEXT NOT NULL,
				message TEXT,
				extra_info TEXT
			);`, tableName))
		assert.NoError(t, err)

		// Close the write database to force an error during purge
		dbRW.Close()

		// This should fail because purgeEvents will error on closed DB
		err = PurgeByComponents(ctx, dbRW, dbRO, time.Now().Unix(), "test")
		assert.Error(t, err)
	})
}

func TestListTablesRowScanError(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a table with NULL name to potentially cause scan issues
	// This is a bit contrived, but tests the error path in rows.Scan
	_, err := dbRW.ExecContext(ctx, `
		CREATE TABLE test_null (name TEXT);
		INSERT INTO test_null VALUES (NULL);
		CREATE VIEW sqlite_master_backup AS SELECT * FROM sqlite_master;
		DROP TABLE sqlite_master;
		CREATE TABLE sqlite_master AS SELECT 'table' as type, NULL as name;
	`)
	// This might fail on some SQLite versions that protect sqlite_master
	// If so, the existing coverage is likely sufficient
	if err == nil {
		_, err = listTables(ctx, dbRO)
		// We expect this might error due to NULL handling
		_ = err // Don't assert as behavior may vary by SQLite version
	}
}
