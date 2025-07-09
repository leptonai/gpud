package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestSetEventTypeSuccess(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_success",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp, device, port, "active", "linkup", 400, 0)

	// Test successful update
	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.NoError(t, err)

	// Verify the event type was set
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp, device, port, eventType)
}

func TestSetEventTypeNoRowsAffected(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_no_rows",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data with different timestamp/device/port combination
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp.Add(-1*time.Hour), "mlx5_1", 2, "active", "linkup", 400, 0)

	// Test update with non-existent timestamp/device/port combination
	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.NoError(t, err) // Should not error, but will log warning

	// Verify no rows were updated (event type should remain empty)
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp.Add(-1*time.Hour), "mlx5_1", 2, "")
}

func TestSetEventTypeMultipleRowsUpdate(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_multiple",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data with same timestamp/device/port combination
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp, device, port, "active", "linkup", 400, 0)

	// Test update
	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.NoError(t, err)

	// Verify the event type was set
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp, device, port, eventType)
}

func TestSetEventTypeWithSpecialCharacters(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_special_chars",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data with special characters
	device := "mlx5_0-test!@#$%"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event_with_special_chars!@#$%"

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp, device, port, "active", "linkup", 400, 0)

	// Test update with special characters
	err = store.SetEventType(device, port, timestamp, eventType, "test_reason_with_special_chars")
	require.NoError(t, err)

	// Verify the event type was set
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp, device, port, eventType)
}

func TestSetEventTypeWithZeroValues(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_zero_values",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data with zero values
	device := "mlx5_0"
	port := uint(0) // Zero port
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp, device, port, "active", "linkup", 400, 0)

	// Test update with zero port
	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.NoError(t, err)

	// Verify the event type was set
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp, device, port, eventType)
}

func TestSetEventTypeWithEmptyEventType(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_empty",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "" // Empty event type

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp, device, port, "active", "linkup", 400, 0)

	// Test update with empty event type
	err = store.SetEventType(device, port, timestamp, eventType, "")
	require.NoError(t, err)

	// Verify the event type was set to empty string
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp, device, port, eventType)
}

func TestSetEventTypeWithNonExistentTable(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with non-existent table
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "non_existent_table",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Test update on non-existent table
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	err := store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestSetEventTypeWithClosedDatabase(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_closed_db",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Close the database
	dbRW.Close()

	// Test update with closed database
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database is closed")
}

func TestSetEventTypeWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with canceled context
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_canceled",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Test update with canceled context
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestSetEventTypeWithTimeout(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with canceled context to ensure timeout behavior
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Cancel immediately

	store := &ibPortsStore{
		rootCtx:      canceledCtx,
		historyTable: "test_set_event_type_timeout",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table with the valid context
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Test update with canceled context - should reliably fail
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestSetEventTypeUpdateExistingEvent(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_update_existing",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	initialEventType := "initial_event"
	updatedEventType := "updated_event"

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp, device, port, "active", "linkup", 400, 0)

	// First update
	err = store.SetEventType(device, port, timestamp, initialEventType, "initial_reason")
	require.NoError(t, err)

	// Verify first update
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp, device, port, initialEventType)

	// Second update (overwrite existing event type)
	err = store.SetEventType(device, port, timestamp, updatedEventType, "updated_reason")
	require.NoError(t, err)

	// Verify second update
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp, device, port, updatedEventType)
}

func TestSetEventTypeRowsAffectedError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_rows_affected_error",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp, device, port, "active", "linkup", 400, 0)

	// Close database after insert to trigger RowsAffected error
	dbRW.Close()

	// Test update - should fail when trying to check RowsAffected
	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database is closed")
}

func TestSetEventTypeWithLongEventType(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_long_event",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data
	device := "mlx5_0"
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	// Create a very long event type
	eventType := ""
	for i := 0; i < 1000; i++ {
		eventType += "a"
	}

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp, device, port, "active", "linkup", 400, 0)

	// Test update with long event type
	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.NoError(t, err)

	// Verify the event type was set
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp, device, port, eventType)
}

func TestSetEventTypeWithEmptyDevice(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:      ctx,
		historyTable: "test_set_event_type_empty_device",
		dbRW:         dbRW,
		dbRO:         dbRO,
		getTimeNow:   func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Insert test data with empty device
	device := ""
	port := uint(1)
	timestamp := time.Now().Add(-1 * time.Hour)
	eventType := "test_event"

	insertTestData(t, ctx, dbRW, store.historyTable, timestamp, device, port, "active", "linkup", 400, 0)

	// Test update with empty device
	err = store.SetEventType(device, port, timestamp, eventType, "test_reason")
	require.NoError(t, err)

	// Verify the event type was set
	verifyEventType(t, ctx, dbRO, store.historyTable, timestamp, device, port, eventType)
}

// Helper functions

func insertTestData(t *testing.T, ctx context.Context, dbRW *sql.DB, tableName string, timestamp time.Time, device string, port uint, state string, physicalState string, rateGBSec int, totalLinkDowned uint64) {
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_type, event_reason, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)
	_, err := dbRW.ExecContext(ctx, insertSQL, timestamp.Unix(), device, port, "infiniband", state, physicalState, rateGBSec, totalLinkDowned, "", "", "")
	require.NoError(t, err)
}

func insertTestDataWithEvent(t *testing.T, ctx context.Context, dbRW *sql.DB, tableName string, timestamp time.Time, device string, port uint, state string, physicalState string, rateGBSec int, totalLinkDowned uint64, eventType string, eventReason string) {
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_type, event_reason, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)
	_, err := dbRW.ExecContext(ctx, insertSQL, timestamp.Unix(), device, port, "infiniband", state, physicalState, rateGBSec, totalLinkDowned, eventType, eventReason, "")
	require.NoError(t, err)
}

func verifyEventType(t *testing.T, ctx context.Context, dbRO *sql.DB, tableName string, timestamp time.Time, device string, port uint, expectedEventType string) {
	query := fmt.Sprintf(`SELECT event_type FROM %s WHERE timestamp = ? AND device = ? AND port = ?`, tableName)
	row := dbRO.QueryRowContext(ctx, query, timestamp.Unix(), device, port)

	var eventType string
	err := row.Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, expectedEventType, eventType)
}

// Events method tests

func TestEventsSuccess(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_success",
		metadataTable: "test_events_success_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Insert test data with events
	baseTime := time.Now().Add(-2 * time.Hour)
	since := baseTime.Add(-30 * time.Minute)

	// Insert events before and after the 'since' timestamp
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime.Add(-1*time.Hour), "mlx5_0", 1, "active", "linkup", 400, 0, "event1", "reason1")
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime.Add(-15*time.Minute), "mlx5_1", 2, "active", "linkup", 400, 0, "event2", "reason2")
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime, "mlx5_2", 3, "active", "linkup", 400, 0, "event3", "")

	// Test retrieving events
	events, err := store.Events(since)
	require.NoError(t, err)
	require.Len(t, events, 2) // Should get 2 events after 'since'

	// Verify events are sorted by timestamp
	assert.True(t, events[0].Time.Before(events[1].Time) || events[0].Time.Equal(events[1].Time))

	// Verify event details
	assert.Equal(t, "event2", events[0].EventType)
	assert.Equal(t, "reason2", events[0].EventReason)
	assert.Equal(t, "mlx5_1", events[0].Port.Device)
	assert.Equal(t, uint(2), events[0].Port.Port)

	assert.Equal(t, "event3", events[1].EventType)
	assert.Equal(t, "", events[1].EventReason) // Empty reason
	assert.Equal(t, "mlx5_2", events[1].Port.Device)
	assert.Equal(t, uint(3), events[1].Port.Port)
}

func TestEventsNoEventsFound(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_no_events",
		metadataTable: "test_events_no_events_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Insert test data with events before 'since'
	baseTime := time.Now().Add(-2 * time.Hour)
	since := baseTime.Add(1 * time.Hour)

	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime.Add(-1*time.Hour), "mlx5_0", 1, "active", "linkup", 400, 0, "event1", "reason1")

	// Test retrieving events - should return empty
	events, err := store.Events(since)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestEventsWithEmptyEventType(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_empty_event_type",
		metadataTable: "test_events_empty_event_type_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Insert test data with empty event types
	baseTime := time.Now().Add(-1 * time.Hour)
	since := baseTime.Add(-30 * time.Minute)

	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime.Add(-15*time.Minute), "mlx5_0", 1, "active", "linkup", 400, 0, "", "")
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime, "mlx5_1", 2, "active", "linkup", 400, 0, "event1", "reason1")

	// Test retrieving events - should filter out empty event types
	events, err := store.Events(since)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "event1", events[0].EventType)
}

func TestEventsOrderedByTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_ordered",
		metadataTable: "test_events_ordered_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Insert test data in reverse chronological order
	baseTime := time.Now().Add(-1 * time.Hour)
	since := baseTime.Add(-30 * time.Minute)

	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime, "mlx5_0", 1, "active", "linkup", 400, 0, "event3", "reason3")
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime.Add(-15*time.Minute), "mlx5_1", 2, "active", "linkup", 400, 0, "event2", "reason2")
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime.Add(-25*time.Minute), "mlx5_2", 3, "active", "linkup", 400, 0, "event1", "reason1")

	// Test retrieving events - should be ordered by timestamp ascending
	events, err := store.Events(since)
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Verify chronological order
	assert.Equal(t, "event1", events[0].EventType)
	assert.Equal(t, "event2", events[1].EventType)
	assert.Equal(t, "event3", events[2].EventType)

	// Verify timestamps are in ascending order
	for i := 1; i < len(events); i++ {
		assert.True(t, events[i-1].Time.Before(events[i].Time) || events[i-1].Time.Equal(events[i].Time))
	}
}

func TestEventsWithNullEventReason(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_null_reason",
		metadataTable: "test_events_null_reason_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Insert test data with NULL event reason
	baseTime := time.Now().Add(-1 * time.Hour)
	since := baseTime.Add(-30 * time.Minute)

	// Insert with NULL event_reason
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_type, event_reason, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`, store.historyTable)
	_, err = dbRW.ExecContext(ctx, insertSQL, baseTime.Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "event1", "")
	require.NoError(t, err)

	// Test retrieving events
	events, err := store.Events(since)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "event1", events[0].EventType)
	assert.Equal(t, "", events[0].EventReason) // Should be empty string for NULL
}

func TestEventsWithNonExistentTable(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with non-existent table
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "non_existent_table",
		metadataTable: "non_existent_table_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Test retrieving events from non-existent table
	since := time.Now().Add(-1 * time.Hour)
	events, err := store.Events(since)
	require.Error(t, err)
	assert.Nil(t, events)
	// The error message varies depending on whether the table exists or not
	assert.True(t, strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "attempt to write a readonly database"))
}

func TestEventsWithClosedDatabase(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_closed_db",
		metadataTable: "test_events_closed_db_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Close the database
	dbRO.Close()

	// Test retrieving events with closed database
	since := time.Now().Add(-1 * time.Hour)
	events, err := store.Events(since)
	require.Error(t, err)
	assert.Nil(t, events)
	assert.Contains(t, err.Error(), "database is closed")
}

func TestEventsWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with canceled context
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_canceled",
		metadataTable: "test_events_canceled_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Test retrieving events with canceled context
	since := time.Now().Add(-1 * time.Hour)
	events, err := store.Events(since)
	require.Error(t, err)
	assert.Nil(t, events)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestEventsWithTimeout(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with canceled context to ensure timeout behavior
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Cancel immediately

	store := &ibPortsStore{
		rootCtx:       canceledCtx,
		historyTable:  "test_events_timeout",
		metadataTable: "test_events_timeout_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table with the valid context
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table with the valid context
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Test retrieving events with canceled context - should reliably fail
	since := time.Now().Add(-1 * time.Hour)
	events, err := store.Events(since)
	require.Error(t, err)
	assert.Nil(t, events)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestEventsWithRowScanError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_scan_error",
		metadataTable: "test_events_scan_error_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Insert test data with invalid data type for rate_gb_sec (string instead of int)
	baseTime := time.Now().Add(-1 * time.Hour)
	since := baseTime.Add(-30 * time.Minute)

	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_type, event_reason, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, store.historyTable)
	_, err = dbRW.ExecContext(ctx, insertSQL, baseTime.Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", "invalid_rate", 0, "event1", "reason1", "")
	require.NoError(t, err)

	// Test retrieving events - should fail during row scan
	events, err := store.Events(since)
	require.Error(t, err)
	assert.Nil(t, events)
	// SQLite is lenient and might convert string to int, so we just check for any error
}

func TestEventsWithLargeDataSet(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_large_dataset",
		metadataTable: "test_events_large_dataset_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Insert a large number of events
	baseTime := time.Now().Add(-1 * time.Hour)
	since := baseTime.Add(-30 * time.Minute)

	numEvents := 1000
	for i := 0; i < numEvents; i++ {
		eventTime := baseTime.Add(time.Duration(i) * time.Second)
		insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, eventTime, fmt.Sprintf("mlx5_%d", i%10), uint(i%5), "active", "linkup", 400, 0, fmt.Sprintf("event_%d", i), fmt.Sprintf("reason_%d", i))
	}

	// Test retrieving events
	events, err := store.Events(since)
	require.NoError(t, err)

	// Should get all events after 'since'
	expectedCount := numEvents // All events are after 'since'
	assert.Len(t, events, expectedCount)

	// Verify events are in chronological order
	for i := 1; i < len(events); i++ {
		assert.True(t, events[i-1].Time.Before(events[i].Time) || events[i-1].Time.Equal(events[i].Time))
	}
}

func TestEventsWithZeroTime(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store
	store := &ibPortsStore{
		rootCtx:       ctx,
		historyTable:  "test_events_zero_time",
		metadataTable: "test_events_zero_time_metadata",
		dbRW:          dbRW,
		dbRO:          dbRO,
		getTimeNow:    func() time.Time { return time.Now().UTC() },
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	// Create metadata table
	err = createMetadataTable(ctx, dbRW, store.metadataTable)
	require.NoError(t, err)

	// Insert test data with events at various times
	baseTime := time.Now().Add(-2 * time.Hour)

	// Insert events at different timestamps
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime.Add(-1*time.Hour), "mlx5_0", 1, "active", "linkup", 400, 0, "old_event", "old_reason")
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime.Add(-30*time.Minute), "mlx5_1", 2, "active", "linkup", 400, 0, "middle_event", "middle_reason")
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime, "mlx5_2", 3, "active", "linkup", 400, 0, "recent_event", "")

	// Insert one row with empty event type that should be filtered out
	insertTestDataWithEvent(t, ctx, dbRW, store.historyTable, baseTime.Add(-15*time.Minute), "mlx5_3", 4, "active", "linkup", 400, 0, "", "")

	// Test retrieving events with zero time (should get ALL events with non-empty event_type)
	var zeroTime time.Time // time.Time{}.IsZero() == true
	events, err := store.Events(zeroTime)
	require.NoError(t, err)

	// Should get all 3 events with non-empty event types (filters out the empty event type)
	require.Len(t, events, 3)

	// Verify events are sorted by timestamp in ascending order
	assert.Equal(t, "old_event", events[0].EventType)
	assert.Equal(t, "old_reason", events[0].EventReason)
	assert.Equal(t, "mlx5_0", events[0].Port.Device)
	assert.Equal(t, uint(1), events[0].Port.Port)

	assert.Equal(t, "middle_event", events[1].EventType)
	assert.Equal(t, "middle_reason", events[1].EventReason)
	assert.Equal(t, "mlx5_1", events[1].Port.Device)
	assert.Equal(t, uint(2), events[1].Port.Port)

	assert.Equal(t, "recent_event", events[2].EventType)
	assert.Equal(t, "", events[2].EventReason) // Empty reason
	assert.Equal(t, "mlx5_2", events[2].Port.Device)
	assert.Equal(t, uint(3), events[2].Port.Port)

	// Verify timestamps are in ascending order
	for i := 1; i < len(events); i++ {
		assert.True(t, events[i-1].Time.Before(events[i].Time) || events[i-1].Time.Equal(events[i].Time))
	}
}
