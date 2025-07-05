package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	_, err := readLastTimestamp(ctx, dbRO, "non_existent_table")
	require.Error(t, err)
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
	devices, err := readAllDevices(ctx, dbRO, tableName)
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
	devices, err = readAllDevices(ctx, dbRO, tableName)
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
	devices, err := readAllDevices(ctx, dbRO, "non_existent_table")
	require.Error(t, err)
	assert.Nil(t, devices)

	// Test with closed database
	dbRO.Close()
	devices, err = readAllDevices(ctx, dbRO, "any_table")
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
	ports, err := readAllPorts(ctx, dbRO, tableName)
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
	ports, err = readAllPorts(ctx, dbRO, tableName)
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
	ports, err := readAllPorts(ctx, dbRO, "non_existent_table")
	require.Error(t, err)
	assert.Nil(t, ports)

	// Test with closed database
	dbRO.Close()
	ports, err = readAllPorts(ctx, dbRO, "any_table")
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
	devices, err := readAllDevices(ctx, dbRO, tableName)
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
	ports, err := readAllPorts(ctx, dbRO, tableName)
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

func TestLastTimestampInitialization(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with empty table - lastTimestamp should be zero
	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.lastInsertedTsMu.RLock()
	lastTs := s.lastInsertedTs
	s.lastInsertedTsMu.RUnlock()
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
	s2.lastInsertedTsMu.RLock()
	lastTs2 := s2.lastInsertedTs
	s2.lastInsertedTsMu.RUnlock()
	assert.Equal(t, fixedTime.Unix(), lastTs2.Unix(), "lastTimestamp should be initialized from existing data")
}
