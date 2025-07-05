package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestNewStore(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)
	require.NotNil(t, store)

	// Verify that the store implements the Store interface
	var _ Store = store
}

func TestInsert(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Type assert to access internal fields for testing
	s := store.(*ibPortsStore)

	// Mock time for predictable testing
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return fixedTime }
	s.minInsertInterval = 0 // Disable interval check for testing

	// Test nil event
	err = store.Insert(nil)
	require.NoError(t, err)

	// Test valid event with multiple ports
	event := &IBPortsEvent{
		Time: fixedTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:          "mlx5_0",
				Port:            1,
				LinkLayer:       "InfiniBand",
				State:           "Active",
				PhysicalState:   "LinkUp",
				RateGBSec:       400,
				TotalLinkDowned: 0,
			},
			{
				Device:          "mlx5_1",
				Port:            1,
				LinkLayer:       "Ethernet",
				State:           "Down",
				PhysicalState:   "LinkDown",
				RateGBSec:       0,
				TotalLinkDowned: 5,
			},
		},
	}

	err = store.Insert(event)
	require.NoError(t, err)

	// Verify data was inserted using Read
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1) // All entries have same timestamp

	// Get the entries from the group
	entries := entryGroups[0].Entries
	assert.Equal(t, fixedTime.Unix(), entryGroups[0].Timestamp.Unix())
	assert.Len(t, entries, 2)

	// Verify data integrity for first entry
	entry0 := entries[0]
	assert.Equal(t, fixedTime.Unix(), entry0.Timestamp.Unix())
	assert.Equal(t, "mlx5_0", entry0.Device)
	assert.Equal(t, uint(1), entry0.Port)
	assert.Equal(t, "infiniband", entry0.LinkLayer) // Stored as lowercase
	assert.Equal(t, "active", entry0.State)         // Stored as lowercase
	assert.Equal(t, "linkup", entry0.PhysState)     // Stored as lowercase
	assert.Equal(t, 400, entry0.RateGBSec)
	assert.Equal(t, 0, entry0.TotalLinkDowned)

	// Verify second entry
	entry1 := entries[1]
	assert.Equal(t, "mlx5_1", entry1.Device)
	assert.Equal(t, "ethernet", entry1.LinkLayer) // Stored as lowercase
	assert.Equal(t, "down", entry1.State)         // Stored as lowercase
	assert.Equal(t, "linkdown", entry1.PhysState) // Stored as lowercase
	assert.Equal(t, 0, entry1.RateGBSec)
	assert.Equal(t, 5, entry1.TotalLinkDowned)
}

func TestInsertWithMinInterval(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Type assert to access internal fields
	s := store.(*ibPortsStore)

	// Set up time mocking
	currentTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return currentTime }
	s.minInsertInterval = 10 * time.Second

	// First insert should succeed
	event1 := &IBPortsEvent{
		Time: currentTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	err = store.Insert(event1)
	require.NoError(t, err)

	// Verify first insert succeeded
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	assert.Len(t, entryGroups[0].Entries, 1)

	// Second insert within min interval should be skipped
	currentTime = currentTime.Add(5 * time.Second)
	event2 := &IBPortsEvent{
		Time: currentTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_1",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     200,
			},
		},
	}

	err = store.Insert(event2)
	require.NoError(t, err)

	// Verify only first insert succeeded (second was skipped)
	entryGroups, err = store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	assert.Len(t, entryGroups[0].Entries, 1)

	// Third insert after min interval should succeed
	currentTime = currentTime.Add(10 * time.Second)
	s.getTimeNow = func() time.Time { return currentTime }

	event3 := &IBPortsEvent{
		Time: currentTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_2",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     300,
			},
		},
	}

	err = store.Insert(event3)
	require.NoError(t, err)

	// Verify second insert succeeded
	entryGroups, err = store.Read(time.Time{})
	require.NoError(t, err)
	assert.Len(t, entryGroups, 2) // Two different timestamps
}

func TestReadEntries(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0 // Disable interval check for testing

	// Insert test data at different times
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// First event (older)
	s.getTimeNow = func() time.Time { return baseTime }
	event1 := &IBPortsEvent{
		Time: baseTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:          "mlx5_0",
				Port:            1,
				LinkLayer:       "InfiniBand",
				State:           "Active",
				PhysicalState:   "LinkUp",
				RateGBSec:       400,
				TotalLinkDowned: 0,
			},
		},
	}

	err = store.Insert(event1)
	require.NoError(t, err)

	// Second event (newer)
	newerTime := baseTime.Add(1 * time.Hour)
	s.getTimeNow = func() time.Time { return newerTime }
	event2 := &IBPortsEvent{
		Time: newerTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:          "mlx5_1",
				Port:            2,
				LinkLayer:       "Ethernet",
				State:           "Down",
				PhysicalState:   "LinkDown",
				RateGBSec:       0,
				TotalLinkDowned: 5,
			},
		},
	}

	err = store.Insert(event2)
	require.NoError(t, err)

	// Read all entries
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	assert.Len(t, entryGroups, 2) // Two different timestamps

	// Verify first group (should be sorted by timestamp ASC)
	firstGroup := entryGroups[0]
	assert.Equal(t, baseTime.Unix(), firstGroup.Timestamp.Unix())
	require.Len(t, firstGroup.Entries, 1)
	firstEntry := firstGroup.Entries[0]
	assert.Equal(t, "mlx5_0", firstEntry.Device)
	assert.Equal(t, uint(1), firstEntry.Port)
	assert.Equal(t, "infiniband", firstEntry.LinkLayer)
	assert.Equal(t, "active", firstEntry.State)
	assert.Equal(t, "linkup", firstEntry.PhysState)
	assert.Equal(t, 400, firstEntry.RateGBSec)
	assert.Equal(t, 0, firstEntry.TotalLinkDowned)
	assert.Equal(t, baseTime.Unix(), firstEntry.Timestamp.Unix())

	// Verify second group
	secondGroup := entryGroups[1]
	assert.Equal(t, newerTime.Unix(), secondGroup.Timestamp.Unix())
	require.Len(t, secondGroup.Entries, 1)
	secondEntry := secondGroup.Entries[0]
	assert.Equal(t, "mlx5_1", secondEntry.Device)
	assert.Equal(t, uint(2), secondEntry.Port)
	assert.Equal(t, "ethernet", secondEntry.LinkLayer)
	assert.Equal(t, "down", secondEntry.State)
	assert.Equal(t, "linkdown", secondEntry.PhysState)
	assert.Equal(t, 0, secondEntry.RateGBSec)
	assert.Equal(t, 5, secondEntry.TotalLinkDowned)
	assert.Equal(t, newerTime.Unix(), secondEntry.Timestamp.Unix())

	// Read entries since specific time (should only get the newer one)
	entryGroups, err = store.Read(baseTime.Add(30 * time.Minute))
	require.NoError(t, err)
	assert.Len(t, entryGroups, 1)
	assert.Equal(t, "mlx5_1", entryGroups[0].Entries[0].Device)
}

func TestReadEntriesEmpty(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Read from empty store
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	assert.Len(t, entryGroups, 0)

	// Read with specific timestamp from empty store
	entryGroups, err = store.Read(time.Now())
	require.NoError(t, err)
	assert.Len(t, entryGroups, 0)
}

func TestInsertTransactionRollback(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Insert valid data first
	event := &IBPortsEvent{
		Time: time.Now(),
		IBPorts: []infiniband.IBPort{
			{
				Device:        "valid_device",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	err = store.Insert(event)
	require.NoError(t, err)

	// Verify insertion worked
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	assert.Len(t, entryGroups[0].Entries, 1)

	// Close the database to force an error on next insert
	dbRW.Close()

	// Try to insert again - should fail
	err = store.Insert(event)
	require.Error(t, err)
}

func TestConcurrentInserts(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Run concurrent inserts
	numGoroutines := 10
	done := make(chan bool, numGoroutines)

	for i := range numGoroutines {
		go func(id int) {
			event := &IBPortsEvent{
				Time: time.Now(),
				IBPorts: []infiniband.IBPort{
					{
						Device:        "device_" + string(rune('A'+id)),
						Port:          uint(id + 1),
						LinkLayer:     "InfiniBand",
						State:         "Active",
						PhysicalState: "LinkUp",
						RateGBSec:     100 * (id + 1),
					},
				},
			}

			err := store.Insert(event)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for range numGoroutines {
		<-done
	}

	// Verify all inserts succeeded using Read
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	// Count total entries across all groups
	totalEntries := 0
	for _, group := range entryGroups {
		totalEntries += len(group.Entries)
	}
	assert.Equal(t, numGoroutines, totalEntries)
}

func TestStoreInterfaceCompliance(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Verify interface compliance at compile time and runtime
	var store Store
	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)
	require.NotNil(t, store)

	// Test interface methods
	err = store.Insert(nil)
	require.NoError(t, err)

	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	assert.NotNil(t, entryGroups)
}

func TestEntryStruct(t *testing.T) {
	// Test that the Entry struct fields are properly exported and accessible
	e := IBPortEntry{
		Timestamp:       time.Now(),
		Device:          "mlx5_0",
		Port:            1,
		LinkLayer:       "infiniband",
		State:           "active",
		PhysState:       "linkup",
		RateGBSec:       400,
		TotalLinkDowned: 0,
		Event:           "test",
		ExtraInfo:       map[string]string{"key": "value"},
	}

	// This test ensures the struct is properly defined and accessible
	assert.NotNil(t, e.Timestamp)
	assert.Equal(t, "mlx5_0", e.Device)
	assert.Equal(t, uint(1), e.Port)
	assert.Equal(t, "infiniband", e.LinkLayer)
	assert.Equal(t, "active", e.State)
	assert.Equal(t, "linkup", e.PhysState)
	assert.Equal(t, 400, e.RateGBSec)
	assert.Equal(t, 0, e.TotalLinkDowned)
	assert.Equal(t, "test", e.Event)
	assert.Equal(t, "value", e.ExtraInfo["key"])
}

func TestNewStoreError(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Close the database to force an error during table creation
	dbRW.Close()

	dbRO, _, cleanupRO := sqlite.OpenTestDB(t)
	defer cleanupRO()

	// This should fail during table creation
	_, err := New(ctx, dbRW, dbRO)
	require.Error(t, err)
}

func TestInsertWithDataTransformation(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Test that string fields are properly transformed (trimmed and lowercased)
	event := &IBPortsEvent{
		Time: time.Now(),
		IBPorts: []infiniband.IBPort{
			{
				Device:        "  MLX5_0  ", // Should be trimmed and lowercased
				Port:          1,
				LinkLayer:     "  INFINIBAND  ", // Should be trimmed and lowercased
				State:         "  ACTIVE  ",     // Should be trimmed and lowercased
				PhysicalState: "  LINKUP  ",     // Should be trimmed and lowercased
				RateGBSec:     400,
			},
		},
	}

	err = store.Insert(event)
	require.NoError(t, err)

	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	require.Len(t, entryGroups[0].Entries, 1)

	entry := entryGroups[0].Entries[0]
	assert.Equal(t, "mlx5_0", entry.Device)        // Trimmed and lowercased
	assert.Equal(t, "infiniband", entry.LinkLayer) // Trimmed and lowercased
	assert.Equal(t, "active", entry.State)         // Trimmed and lowercased
	assert.Equal(t, "linkup", entry.PhysState)     // Trimmed and lowercased
}

func TestMultipleEventsOverTime(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Insert multiple events over time
	for i := range 5 {
		eventTime := baseTime.Add(time.Duration(i) * time.Hour)
		s.getTimeNow = func() time.Time { return eventTime }

		event := &IBPortsEvent{
			Time: eventTime,
			IBPorts: []infiniband.IBPort{
				{
					Device:        "mlx5_0",
					Port:          1,
					LinkLayer:     "InfiniBand",
					State:         "Active",
					PhysicalState: "LinkUp",
					RateGBSec:     100 * (i + 1),
				},
			},
		}

		err = store.Insert(event)
		require.NoError(t, err)
	}

	// Read all entries
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	assert.Len(t, entryGroups, 5) // One group per timestamp

	// Verify entries are sorted by timestamp (ascending)
	for i := range 4 {
		assert.True(t, entryGroups[i].Timestamp.Before(entryGroups[i+1].Timestamp) || entryGroups[i].Timestamp.Equal(entryGroups[i+1].Timestamp))
	}

	// Read entries since middle time
	middleTime := baseTime.Add(2 * time.Hour)
	entryGroups, err = store.Read(middleTime)
	require.NoError(t, err)
	assert.Len(t, entryGroups, 3) // Should get last 3 groups

	// Verify the first returned entry is from the expected time
	assert.True(t, entryGroups[0].Timestamp.Equal(middleTime) || entryGroups[0].Timestamp.After(middleTime))
}

func TestCreateTableErrors(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Close database to simulate error conditions
	dbRW.Close()

	// Test transaction begin failure
	err := createHistoryTable(ctx, dbRW, "test_table")
	require.Error(t, err)
}

func TestGetLastTimestampWithClosedDB(t *testing.T) {
	ctx := context.Background()
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Close the read-only database to force an error
	dbRO.Close()

	// This should return an error when trying to query a closed database
	_, err := getLastTimestamp(ctx, dbRO, "non_existent_table")
	require.Error(t, err)
}

func TestReadEntriesWithInvalidExtraInfo(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table manually with invalid JSON in extra_info
	tableName := "test_table_with_invalid_json"
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert data with invalid JSON manually using SQL
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_name, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "{invalid_json")
	require.NoError(t, err)

	// Create store with this table
	store := &ibPortsStore{
		rootCtx:           ctx,
		historyTable:      tableName,
		dbRW:              dbRW,
		dbRO:              dbRO,
		getTimeNow:        func() time.Time { return time.Now().UTC() },
		minInsertInterval: 0,
		retention:         0, // Disable purge for this test
		purgeInterval:     0,
	}

	// Reading entries should fail due to invalid JSON
	_, err = store.Read(time.Time{})
	require.Error(t, err)
}

func TestReadEntriesWithClosedDatabase(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Close the read-only database
	dbRO.Close()

	// ReadEntries should fail
	_, err = store.Read(time.Time{})
	require.Error(t, err)
}

func TestInsertWithBeginTxError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Close the write database to force BeginTx error
	dbRW.Close()

	event := &IBPortsEvent{
		Time: time.Now(),
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	// Insert should fail due to closed database
	err = store.Insert(event)
	require.Error(t, err)
}

func TestInsertWithPrepareError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create a store with an invalid table name to cause prepare error
	store := &ibPortsStore{
		rootCtx:           ctx,
		historyTable:      "nonexistent_table",
		dbRW:              dbRW,
		dbRO:              dbRO,
		getTimeNow:        func() time.Time { return time.Now().UTC() },
		minInsertInterval: 0,
		retention:         0,
		purgeInterval:     0,
	}

	event := &IBPortsEvent{
		Time: time.Now(),
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	// Insert should fail due to nonexistent table
	err := store.Insert(event)
	require.Error(t, err)
}

func TestPurgeEvents(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Insert some test data
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		eventTime := baseTime.Add(time.Duration(i) * time.Hour)
		s.getTimeNow = func() time.Time { return eventTime }

		event := &IBPortsEvent{
			Time: eventTime,
			IBPorts: []infiniband.IBPort{
				{
					Device:        "mlx5_0",
					Port:          1,
					LinkLayer:     "InfiniBand",
					State:         "Active",
					PhysicalState: "LinkUp",
					RateGBSec:     400,
				},
			},
		}

		err = store.Insert(event)
		require.NoError(t, err)
	}

	// Verify all 5 entries exist
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	assert.Len(t, entryGroups, 5)

	// Purge entries older than 2 hours from base time
	purgeBeforeTime := baseTime.Add(2 * time.Hour)
	purged, err := purge(ctx, dbRW, s.historyTable, purgeBeforeTime.Unix())
	require.NoError(t, err)
	assert.Equal(t, 2, purged) // Should purge first 2 entries (all have empty event_name)

	// Verify only 3 entries remain
	entryGroups, err = store.Read(time.Time{})
	require.NoError(t, err)
	assert.Len(t, entryGroups, 3)

	// Verify the remaining entries are the correct ones
	assert.True(t, entryGroups[0].Timestamp.Equal(baseTime.Add(2*time.Hour)) || entryGroups[0].Timestamp.After(baseTime.Add(2*time.Hour)))
}

func TestPurgeEventsSelectiveRetention(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table
	tableName := "test_selective_purge"
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Insert test data with mixed event_name values
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_name, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)

	// Insert 6 entries with timestamps from 0 to 5 hours
	testData := []struct {
		hourOffset int
		eventName  string
		device     string
	}{
		{0, "", "mlx5_0"},           // Should be purged (old + empty event_name)
		{1, "ib_flap", "mlx5_0"},    // Should be retained (old but has event_name)
		{2, "", "mlx5_1"},           // Should be purged (old + empty event_name)
		{3, "link_down", "mlx5_1"},  // Should be retained (old but has event_name)
		{4, "", "mlx5_2"},           // Should be retained (new + empty event_name)
		{5, "port_error", "mlx5_2"}, // Should be retained (new + has event_name)
	}

	for _, data := range testData {
		timestamp := baseTime.Add(time.Duration(data.hourOffset) * time.Hour)
		_, err = dbRW.ExecContext(ctx, insertSQL,
			timestamp.Unix(), data.device, 1, "infiniband", "active", "linkup", 400, 0, data.eventName, "")
		require.NoError(t, err)
	}

	// Verify all 6 entries exist initially
	entryGroups, err := (&ibPortsStore{
		rootCtx:      ctx,
		historyTable: tableName,
		dbRO:         dbRO,
	}).Read(time.Time{})
	require.NoError(t, err)
	assert.Len(t, entryGroups, 6)

	// Purge entries older than 3.5 hours (should affect first 4 entries, but only delete those with empty event_name)
	purgeBeforeTime := baseTime.Add(3*time.Hour + 30*time.Minute)
	purged, err := purge(ctx, dbRW, tableName, purgeBeforeTime.Unix())
	require.NoError(t, err)

	// Should purge only entries at hour 0 and 2 (empty event_name and old)
	assert.Equal(t, 2, purged)

	// Verify 4 entries remain
	entryGroups, err = (&ibPortsStore{
		rootCtx:      ctx,
		historyTable: tableName,
		dbRO:         dbRO,
	}).Read(time.Time{})
	require.NoError(t, err)
	assert.Len(t, entryGroups, 4)

	// Verify the correct entries remain by checking timestamps and event names
	remainingTimes := make(map[int64]string)
	for _, group := range entryGroups {
		for _, entry := range group.Entries {
			hourOffset := int((entry.Timestamp.Unix() - baseTime.Unix()) / 3600)
			remainingTimes[int64(hourOffset)] = entry.Event
		}
	}

	// Should have entries from hours 1, 3, 4, 5
	assert.Contains(t, remainingTimes, int64(1))
	assert.Equal(t, "ib_flap", remainingTimes[int64(1)])
	assert.Contains(t, remainingTimes, int64(3))
	assert.Equal(t, "link_down", remainingTimes[int64(3)])
	assert.Contains(t, remainingTimes, int64(4))
	assert.Equal(t, "", remainingTimes[int64(4)]) // This one was retained because it's not old enough
	assert.Contains(t, remainingTimes, int64(5))
	assert.Equal(t, "port_error", remainingTimes[int64(5)])

	// Should NOT have entries from hours 0 and 2
	assert.NotContains(t, remainingTimes, int64(0))
	assert.NotContains(t, remainingTimes, int64(2))
}

func TestPurgeEventsWithClosedDB(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Close database to force error
	dbRW.Close()

	// Purge should fail
	_, err := purge(ctx, dbRW, "test_table", time.Now().Unix())
	require.Error(t, err)
}

func TestRunPurgeWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with very short purge interval for testing
	store := &ibPortsStore{
		rootCtx:           ctx,
		historyTable:      "test_purge_table",
		dbRW:              dbRW,
		dbRO:              dbRO,
		getTimeNow:        func() time.Time { return time.Now().UTC() },
		minInsertInterval: 0,
		retention:         1 * time.Millisecond, // Very short retention
		purgeInterval:     1 * time.Millisecond, // Very short interval
	}

	// Create table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Start purge goroutine
	go store.runPurge()

	// Cancel context quickly to test context cancellation
	cancel()

	// Give some time for goroutine to exit
	time.Sleep(10 * time.Millisecond)
}

func TestRunPurgeWithPurgeError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:           ctx,
		historyTable:      "nonexistent_table", // This will cause purge errors
		dbRW:              dbRW,
		dbRO:              dbRO,
		getTimeNow:        func() time.Time { return time.Now().UTC() },
		minInsertInterval: 0,
		retention:         1 * time.Millisecond,
		purgeInterval:     1 * time.Millisecond,
	}

	// Use a context with timeout to avoid infinite test
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	store.rootCtx = ctx

	// This will test the error handling path in runPurge
	store.runPurge()
}

func TestSelectAllDevices(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table
	tableName := "test_select_devices"
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Test empty table - should return empty map
	devices, err := selectAllDevices(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Len(t, devices, 0)

	// Insert test data with various devices
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_name, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)

	// Insert multiple entries with same device
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 2, "infiniband", "active", "linkup", 400, 0, "", "")
	require.NoError(t, err)

	// Insert different devices
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_1", 1, "ethernet", "down", "linkdown", 0, 0, "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_2", 1, "infiniband", "active", "linkup", 200, 0, "", "")
	require.NoError(t, err)

	// Get all devices - should return unique devices
	devices, err = selectAllDevices(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Len(t, devices, 3)
	assert.Contains(t, devices, "mlx5_0")
	assert.Contains(t, devices, "mlx5_1")
	assert.Contains(t, devices, "mlx5_2")
}

func TestSelectAllDevicesWithErrors(t *testing.T) {
	ctx := context.Background()
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with non-existent table
	devices, err := selectAllDevices(ctx, dbRO, "non_existent_table")
	require.Error(t, err)
	assert.Nil(t, devices)

	// Test with closed database
	dbRO.Close()
	devices, err = selectAllDevices(ctx, dbRO, "any_table")
	require.Error(t, err)
	assert.Nil(t, devices)
}

func TestSelectAllPorts(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table
	tableName := "test_select_ports"
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Test empty table - should return empty map
	ports, err := selectAllPorts(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Len(t, ports, 0)

	// Insert test data with various ports
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_name, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)

	// Insert multiple entries with same port
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_1", 1, "ethernet", "down", "linkdown", 0, 0, "", "")
	require.NoError(t, err)

	// Insert different ports
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 2, "infiniband", "active", "linkup", 400, 0, "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_2", 3, "infiniband", "active", "linkup", 200, 0, "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_3", 10, "infiniband", "active", "linkup", 100, 0, "", "")
	require.NoError(t, err)

	// Get all ports - should return unique ports
	ports, err = selectAllPorts(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Len(t, ports, 4)
	assert.Contains(t, ports, uint(1))
	assert.Contains(t, ports, uint(2))
	assert.Contains(t, ports, uint(3))
	assert.Contains(t, ports, uint(10))
}

func TestSelectAllPortsWithErrors(t *testing.T) {
	ctx := context.Background()
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with non-existent table
	ports, err := selectAllPorts(ctx, dbRO, "non_existent_table")
	require.Error(t, err)
	assert.Nil(t, ports)

	// Test with closed database
	dbRO.Close()
	ports, err = selectAllPorts(ctx, dbRO, "any_table")
	require.Error(t, err)
	assert.Nil(t, ports)
}

func TestSelectAllDevicesQueryError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table with correct schema first to test Rows.Err() path
	tableName := "test_scan_error"
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Close the database after creating table to force rows.Err()
	dbRO.Close()

	// This should succeed initially but fail on query
	devices, err := selectAllDevices(ctx, dbRO, tableName)
	require.Error(t, err)
	assert.Nil(t, devices)
}

func TestSelectAllPortsRowsError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create a table with incompatible schema to trigger scan error
	tableName := "test_port_scan_error"
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %s (port TEXT)`, tableName))
	require.NoError(t, err)

	// Insert string data where we expect uint
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s (port) VALUES ('not_a_number')`, tableName))
	require.NoError(t, err)

	// This should fail when trying to scan string as uint
	ports, err := selectAllPorts(ctx, dbRO, tableName)
	require.Error(t, err)
	assert.Nil(t, ports)
}

func TestNewStoreWithSelectErrors(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table first
	err := createHistoryTable(ctx, dbRW, defaultHistoryTable)
	require.NoError(t, err)

	// Close the read-only database to force selectAllDevices/selectAllPorts to fail
	dbRO.Close()

	// New should fail when it can't query existing devices/ports
	_, err = New(ctx, dbRW, dbRO)
	require.Error(t, err)
}

func TestNewStoreWithGetLastTimestampError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table first
	err := createHistoryTable(ctx, dbRW, defaultHistoryTable)
	require.NoError(t, err)

	// Close the read-only database to force getLastTimestamp to fail
	dbRO.Close()

	// New should fail when it can't get the last timestamp
	_, err = New(ctx, dbRW, dbRO)
	require.Error(t, err)
}

func TestIBPortsEventStruct(t *testing.T) {
	// Test IBPortsEvent struct
	event := IBPortsEvent{
		Time: time.Now(),
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	// Test IBPortsEvents slice
	events := IBPortsEvents{event}
	assert.Len(t, events, 1)
	assert.Equal(t, "mlx5_0", events[0].IBPorts[0].Device)
}

func TestEntryWithValidExtraInfo(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table and insert entry with valid JSON extra_info
	tableName := "test_table_with_valid_json"
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert data with valid JSON
	extraInfo := `{"key1":"value1","key2":"value2"}`
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_name, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "test_event", extraInfo)
	require.NoError(t, err)

	// Create store and read entries
	store := &ibPortsStore{
		rootCtx:           ctx,
		historyTable:      tableName,
		dbRW:              dbRW,
		dbRO:              dbRO,
		getTimeNow:        func() time.Time { return time.Now().UTC() },
		minInsertInterval: 0,
		retention:         0,
		purgeInterval:     0,
	}

	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	require.Len(t, entryGroups[0].Entries, 1)

	entry := entryGroups[0].Entries[0]
	assert.Equal(t, "mlx5_0", entry.Device)
	assert.Equal(t, "test_event", entry.Event)
	assert.NotNil(t, entry.ExtraInfo)
	assert.Equal(t, "value1", entry.ExtraInfo["key1"])
	assert.Equal(t, "value2", entry.ExtraInfo["key2"])
}

func TestCreateTableWithIndexErrors(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create a table with an invalid name that will cause index creation errors
	// We'll use a very long table name that might cause issues
	longTableName := strings.Repeat("a", 1000)
	err := createHistoryTable(ctx, dbRW, longTableName)
	// This should succeed or fail, but we're testing error paths
	// The exact behavior depends on SQLite limits
	if err != nil {
		// Expected in some cases
		t.Logf("Create table failed as expected: %v", err)
	}
}

func TestInsertWithStmtExecError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Create an event with data that might cause execution errors
	event := &IBPortsEvent{
		Time: time.Now(),
		IBPorts: []infiniband.IBPort{
			{
				Device:          "mlx5_0",
				Port:            1,
				LinkLayer:       "InfiniBand",
				State:           "Active",
				PhysicalState:   "LinkUp",
				RateGBSec:       400,
				TotalLinkDowned: 0,
			},
		},
	}

	// Insert the first time should succeed
	err = store.Insert(event)
	require.NoError(t, err)

	// Now corrupt the database by closing it during a transaction
	dbRW.Close()

	// Try to insert again - this should hit the stmt.ExecContext error path
	err = store.Insert(event)
	require.Error(t, err)
}

func TestReadEntriesRowsScanError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table with incompatible schema
	tableName := "test_incompatible_schema"
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %s (id INTEGER PRIMARY KEY, wrong_column TEXT)`, tableName))
	require.NoError(t, err)

	// Insert some data
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s (wrong_column) VALUES (?)`, tableName), "test")
	require.NoError(t, err)

	// Create store with this incompatible table
	store := &ibPortsStore{
		rootCtx:           ctx,
		historyTable:      tableName,
		dbRW:              dbRW,
		dbRO:              dbRO,
		getTimeNow:        func() time.Time { return time.Now().UTC() },
		minInsertInterval: 0,
		retention:         0,
		purgeInterval:     0,
	}

	// This should fail when trying to scan incompatible columns
	_, err = store.Read(time.Time{})
	require.Error(t, err)
}

func TestPurgeEventsRowsAffectedError(t *testing.T) {
	// This is difficult to test directly since RowsAffected rarely fails
	// in SQLite, but we'll test the success path and document this limitation
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_purge_affected"
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert test data
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_name, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Add(-time.Hour).Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "")
	require.NoError(t, err)

	// Test successful purge
	purged, err := purge(ctx, dbRW, tableName, time.Now().Unix())
	require.NoError(t, err)
	assert.Equal(t, 1, purged)
}

func TestRunPurgeSuccessPath(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with very short intervals for quick testing
	store := &ibPortsStore{
		rootCtx:           ctx,
		historyTable:      "test_purge_success",
		dbRW:              dbRW,
		dbRO:              dbRO,
		getTimeNow:        func() time.Time { return time.Now().UTC() },
		minInsertInterval: 0,
		retention:         1 * time.Millisecond,
		purgeInterval:     5 * time.Millisecond,
	}

	// Create table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert some old data that will be purged
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_name, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, store.historyTable)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Add(-time.Hour).Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "")
	require.NoError(t, err)

	// Use a context with timeout to avoid infinite test
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	store.rootCtx = ctx

	// Run purge - this should successfully purge the old data
	store.runPurge()
}

func TestGetLastTimestampEdgeCases(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with non-existent table should return error
	_, err := getLastTimestamp(ctx, dbRO, "non_existent_table")
	require.Error(t, err)

	// Create empty table
	tableName := "test_empty_table"
	err = createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Should return zero time for empty table
	ts, err := getLastTimestamp(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.True(t, ts.IsZero())
}
func TestInsertWithEmptyIBPorts(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Test with empty IBPorts slice
	event := &IBPortsEvent{
		Time:    time.Now(),
		IBPorts: []infiniband.IBPort{}, // Empty slice
	}

	err = s.Insert(event)
	require.NoError(t, err) // Should succeed with empty slice

	// Verify no entries were added
	entryGroups, err := s.Read(time.Time{})
	require.NoError(t, err)
	assert.Len(t, entryGroups, 0)
}

func TestSetEvent(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// First, insert some test data
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return fixedTime }

	event := &IBPortsEvent{
		Time: fixedTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:          "mlx5_0",
				Port:            1,
				LinkLayer:       "InfiniBand",
				State:           "Active",
				PhysicalState:   "LinkUp",
				RateGBSec:       400,
				TotalLinkDowned: 0,
			},
			{
				Device:          "mlx5_1",
				Port:            2,
				LinkLayer:       "Ethernet",
				State:           "Down",
				PhysicalState:   "LinkDown",
				RateGBSec:       0,
				TotalLinkDowned: 5,
			},
		},
	}

	err = store.Insert(event)
	require.NoError(t, err)

	// Test successful event setting for existing row
	err = store.SetEventName(fixedTime, "mlx5_0", 1, "ib_flap_detected")
	require.NoError(t, err)

	// Verify the event was set correctly
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	require.Len(t, entryGroups[0].Entries, 2)

	// Find the mlx5_0 entry and verify the event was set
	var mlx5_0Entry *IBPortEntry
	for _, entry := range entryGroups[0].Entries {
		if entry.Device == "mlx5_0" && entry.Port == 1 {
			mlx5_0Entry = &entry
			break
		}
	}
	require.NotNil(t, mlx5_0Entry)
	assert.Equal(t, "ib_flap_detected", mlx5_0Entry.Event)

	// Verify the other entry was not affected
	var mlx5_1Entry *IBPortEntry
	for _, entry := range entryGroups[0].Entries {
		if entry.Device == "mlx5_1" && entry.Port == 2 {
			mlx5_1Entry = &entry
			break
		}
	}
	require.NotNil(t, mlx5_1Entry)
	assert.Equal(t, "", mlx5_1Entry.Event) // Should still be empty

	// Test setting event for second device
	err = store.SetEventName(fixedTime, "mlx5_1", 2, "link_down_event")
	require.NoError(t, err)

	// Verify both events are set correctly
	entryGroups, err = store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	require.Len(t, entryGroups[0].Entries, 2)

	for _, entry := range entryGroups[0].Entries {
		if entry.Device == "mlx5_0" && entry.Port == 1 {
			assert.Equal(t, "ib_flap_detected", entry.Event)
		} else if entry.Device == "mlx5_1" && entry.Port == 2 {
			assert.Equal(t, "link_down_event", entry.Event)
		}
	}
}

func TestSetEventNoMatchingRow(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Test setting event on empty database - should succeed but log warning
	err = store.SetEventName(time.Now(), "nonexistent_device", 99, "test_event")
	require.NoError(t, err) // Should not return error even when no row matches

	// Insert some data
	s := store.(*ibPortsStore)
	s.minInsertInterval = 0
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return fixedTime }

	event := &IBPortsEvent{
		Time: fixedTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	err = store.Insert(event)
	require.NoError(t, err)

	// Test with wrong timestamp - should succeed but not affect any rows
	wrongTime := fixedTime.Add(time.Hour)
	err = store.SetEventName(wrongTime, "mlx5_0", 1, "test_event")
	require.NoError(t, err)

	// Test with wrong device - should succeed but not affect any rows
	err = store.SetEventName(fixedTime, "wrong_device", 1, "test_event")
	require.NoError(t, err)

	// Test with wrong port - should succeed but not affect any rows
	err = store.SetEventName(fixedTime, "mlx5_0", 99, "test_event")
	require.NoError(t, err)

	// Verify original data is unchanged
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	require.Len(t, entryGroups[0].Entries, 1)
	assert.Equal(t, "", entryGroups[0].Entries[0].Event) // Should still be empty
}

func TestSetEventWithClosedDB(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Close the write database to force an error
	dbRW.Close()

	// SetEvent should fail due to closed database
	err = store.SetEventName(time.Now(), "mlx5_0", 1, "test_event")
	require.Error(t, err)
}

func TestSetEventWithRowsAffectedError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Insert test data first
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return fixedTime }

	event := &IBPortsEvent{
		Time: fixedTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	err = store.Insert(event)
	require.NoError(t, err)

	// Test successful update first
	err = store.SetEventName(fixedTime, "mlx5_0", 1, "test_event")
	require.NoError(t, err)

	// Verify the event was set
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	require.Len(t, entryGroups[0].Entries, 1)
	assert.Equal(t, "test_event", entryGroups[0].Entries[0].Event)

	// Close the database after successful operation to test different error scenarios
	dbRW.Close()

	// This should now fail with database error
	err = store.SetEventName(fixedTime, "mlx5_0", 1, "another_event")
	require.Error(t, err)
}

func TestSetEventWithMultipleUpdates(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Insert test data with multiple entries at same timestamp
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return fixedTime }

	event := &IBPortsEvent{
		Time: fixedTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
			{
				Device:        "mlx5_0", // Same device
				Port:          2,        // Different port
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     200,
			},
		},
	}

	err = store.Insert(event)
	require.NoError(t, err)

	// Set events for both ports
	err = store.SetEventName(fixedTime, "mlx5_0", 1, "event_port_1")
	require.NoError(t, err)

	err = store.SetEventName(fixedTime, "mlx5_0", 2, "event_port_2")
	require.NoError(t, err)

	// Verify both events were set correctly
	entryGroups, err := store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	require.Len(t, entryGroups[0].Entries, 2)

	eventsByPort := make(map[uint]string)
	for _, entry := range entryGroups[0].Entries {
		eventsByPort[entry.Port] = entry.Event
	}

	assert.Equal(t, "event_port_1", eventsByPort[1])
	assert.Equal(t, "event_port_2", eventsByPort[2])

	// Update one of the events
	err = store.SetEventName(fixedTime, "mlx5_0", 1, "updated_event_port_1")
	require.NoError(t, err)

	// Verify the update
	entryGroups, err = store.Read(time.Time{})
	require.NoError(t, err)
	require.Len(t, entryGroups, 1)
	require.Len(t, entryGroups[0].Entries, 2)

	eventsByPort = make(map[uint]string)
	for _, entry := range entryGroups[0].Entries {
		eventsByPort[entry.Port] = entry.Event
	}

	assert.Equal(t, "updated_event_port_1", eventsByPort[1])
	assert.Equal(t, "event_port_2", eventsByPort[2]) // Should remain unchanged
}

func TestSetEventWithSpecialCharacters(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Insert test data
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return fixedTime }

	event := &IBPortsEvent{
		Time: fixedTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	err = store.Insert(event)
	require.NoError(t, err)

	// Test with special characters in event string
	specialEvents := []string{
		"event with spaces",
		"event_with_underscores",
		"event-with-dashes",
		"event.with.dots",
		"event:with:colons",
		"event/with/slashes",
		"event\\with\\backslashes",
		"event'with'quotes",
		"event\"with\"double_quotes",
		"event(with)parentheses",
		"event[with]brackets",
		"event{with}braces",
		"event|with|pipes",
		"event&with&ampersands",
		"event@with@at_signs",
		"event#with#hashes",
		"event$with$dollars",
		"event%with%percents",
		"event^with^carets",
		"event*with*asterisks",
		"event+with+plus",
		"event=with=equals",
		"event~with~tildes",
		"event`with`backticks",
		"",  // Empty string
		" ", // Just space
	}

	for i, eventStr := range specialEvents {
		t.Run(fmt.Sprintf("special_event_%d", i), func(t *testing.T) {
			err = store.SetEventName(fixedTime, "mlx5_0", 1, eventStr)
			require.NoError(t, err)

			// Verify the event was set correctly
			entryGroups, err := store.Read(time.Time{})
			require.NoError(t, err)
			require.Len(t, entryGroups, 1)
			require.Len(t, entryGroups[0].Entries, 1)
			assert.Equal(t, eventStr, entryGroups[0].Entries[0].Event)
		})
	}
}

func TestLastTimestampInitialization(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with empty table - lastTimestamp should be zero
	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.lastTimestampMu.RLock()
	lastTs := s.lastTimestamp
	s.lastTimestampMu.RUnlock()
	assert.True(t, lastTs.IsZero(), "lastTimestamp should be zero for empty table")

	// Insert some data manually to test initialization with existing data
	fixedTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_name, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, defaultHistoryTable)
	_, err = dbRW.ExecContext(ctx, insertSQL, fixedTime.Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "")
	require.NoError(t, err)

	// Create new store - should initialize with the existing timestamp
	store2, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s2 := store2.(*ibPortsStore)
	s2.lastTimestampMu.RLock()
	lastTs2 := s2.lastTimestamp
	s2.lastTimestampMu.RUnlock()
	assert.Equal(t, fixedTime.Unix(), lastTs2.Unix(), "lastTimestamp should be initialized from existing data")
}

func TestLastTimestampUpdateOnInsert(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Initial lastTimestamp should be zero
	s.lastTimestampMu.RLock()
	initialLastTs := s.lastTimestamp
	s.lastTimestampMu.RUnlock()
	assert.True(t, initialLastTs.IsZero())

	// Insert first event
	firstTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return firstTime }

	event1 := &IBPortsEvent{
		Time: firstTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	err = store.Insert(event1)
	require.NoError(t, err)

	// Verify lastTimestamp was updated
	s.lastTimestampMu.RLock()
	lastTs1 := s.lastTimestamp
	s.lastTimestampMu.RUnlock()
	assert.Equal(t, firstTime.Unix(), lastTs1.Unix())

	// Insert second event with later timestamp
	secondTime := firstTime.Add(1 * time.Hour)
	s.getTimeNow = func() time.Time { return secondTime }

	event2 := &IBPortsEvent{
		Time: secondTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_1",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     200,
			},
		},
	}

	err = store.Insert(event2)
	require.NoError(t, err)

	// Verify lastTimestamp was updated to newer timestamp
	s.lastTimestampMu.RLock()
	lastTs2 := s.lastTimestamp
	s.lastTimestampMu.RUnlock()
	assert.Equal(t, secondTime.Unix(), lastTs2.Unix())
}

func TestLastTimestampThreadSafety(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	numGoroutines := 3 // Further reduced to avoid timing issues
	done := make(chan bool, numGoroutines)

	// Use sequential insertion to avoid the Prometheus metrics race condition
	// but test the mutex locking for lastTimestamp
	for i := range numGoroutines {
		eventTime := baseTime.Add(time.Duration(i) * time.Hour) // Use hours for bigger gaps
		s.getTimeNow = func() time.Time { return eventTime }

		event := &IBPortsEvent{
			Time: eventTime,
			IBPorts: []infiniband.IBPort{
				{
					Device:        fmt.Sprintf("mlx5_%d", i),
					Port:          uint(i + 1),
					LinkLayer:     "InfiniBand",
					State:         "Active",
					PhysicalState: "LinkUp",
					RateGBSec:     100 * (i + 1),
				},
			},
		}

		err = store.Insert(event)
		require.NoError(t, err)
	}

	// Now test concurrent reads of lastTimestamp
	for i := range numGoroutines {
		go func(id int) {
			defer func() { done <- true }()

			// Read lastTimestamp concurrently multiple times
			for range 10 {
				s.lastTimestampMu.RLock()
				_ = s.lastTimestamp
				s.lastTimestampMu.RUnlock()
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for range numGoroutines {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out")
		}
	}

	// Verify final lastTimestamp is the latest one
	s.lastTimestampMu.RLock()
	finalLastTs := s.lastTimestamp
	s.lastTimestampMu.RUnlock()

	expectedFinalTime := baseTime.Add(time.Duration(numGoroutines-1) * time.Hour)
	assert.Equal(t, expectedFinalTime.Unix(), finalLastTs.Unix(), "lastTimestamp should be the latest inserted timestamp")
}

func TestLastTimestampWithMinInterval(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 10 * time.Second

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return baseTime }

	// Insert first event
	event1 := &IBPortsEvent{
		Time: baseTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	err = store.Insert(event1)
	require.NoError(t, err)

	// Verify lastTimestamp was updated
	s.lastTimestampMu.RLock()
	lastTs1 := s.lastTimestamp
	s.lastTimestampMu.RUnlock()
	assert.Equal(t, baseTime.Unix(), lastTs1.Unix())

	// Try to insert second event within minInterval - should be skipped
	secondTime := baseTime.Add(5 * time.Second) // Less than minInterval
	event2 := &IBPortsEvent{
		Time: secondTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_1",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     200,
			},
		},
	}

	err = store.Insert(event2)
	require.NoError(t, err) // Should succeed but skip insert

	// Verify lastTimestamp was NOT updated (still the first timestamp)
	s.lastTimestampMu.RLock()
	lastTs2 := s.lastTimestamp
	s.lastTimestampMu.RUnlock()
	assert.Equal(t, baseTime.Unix(), lastTs2.Unix(), "lastTimestamp should not be updated when insert is skipped")

	// Insert third event after minInterval - should succeed
	thirdTime := baseTime.Add(15 * time.Second) // More than minInterval
	s.getTimeNow = func() time.Time { return thirdTime }
	event3 := &IBPortsEvent{
		Time: thirdTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_2",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     300,
			},
		},
	}

	err = store.Insert(event3)
	require.NoError(t, err)

	// Verify lastTimestamp was updated to the third timestamp
	s.lastTimestampMu.RLock()
	lastTs3 := s.lastTimestamp
	s.lastTimestampMu.RUnlock()
	assert.Equal(t, thirdTime.Unix(), lastTs3.Unix(), "lastTimestamp should be updated after successful insert")
}

func TestLastTimestampWithFailedInsert(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 0

	// Insert first event successfully
	firstTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.getTimeNow = func() time.Time { return firstTime }

	event1 := &IBPortsEvent{
		Time: firstTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_0",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     400,
			},
		},
	}

	err = store.Insert(event1)
	require.NoError(t, err)

	// Verify lastTimestamp was updated
	s.lastTimestampMu.RLock()
	lastTs1 := s.lastTimestamp
	s.lastTimestampMu.RUnlock()
	assert.Equal(t, firstTime.Unix(), lastTs1.Unix())

	// Close database to force next insert to fail
	dbRW.Close()

	secondTime := firstTime.Add(1 * time.Hour)
	s.getTimeNow = func() time.Time { return secondTime }

	event2 := &IBPortsEvent{
		Time: secondTime,
		IBPorts: []infiniband.IBPort{
			{
				Device:        "mlx5_1",
				Port:          1,
				LinkLayer:     "InfiniBand",
				State:         "Active",
				PhysicalState: "LinkUp",
				RateGBSec:     200,
			},
		},
	}

	// This should fail
	err = store.Insert(event2)
	require.Error(t, err)

	// Verify lastTimestamp was NOT updated (still the first timestamp)
	s.lastTimestampMu.RLock()
	lastTs2 := s.lastTimestamp
	s.lastTimestampMu.RUnlock()
	assert.Equal(t, firstTime.Unix(), lastTs2.Unix(), "lastTimestamp should not be updated when insert fails")
}
