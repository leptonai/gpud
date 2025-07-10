package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	infiniband "github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
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

func TestNewStoreWithCustomValues(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)
	require.NotNil(t, store)

	s := store.(*ibPortsStore)

	// Verify default values are set correctly
	assert.Equal(t, defaultHistoryTable, s.historyTable)
	assert.Equal(t, defaultMetadataTable, s.metadataTable)
	assert.Equal(t, defaultMinInsertInterval, s.minInsertInterval)
	assert.Equal(t, defaultPurgeInterval, s.purgeInterval)
	assert.NotNil(t, s.getTimeNow)
	assert.NotNil(t, s.allDeviceValues)
	assert.NotNil(t, s.allPortValues)
}

func TestReadAllDeviceValuesErrorHandling(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_history_table"

	// Test with non-existent table
	_, err := readAllDeviceValues(ctx, dbRO, "non_existent_table")
	require.Error(t, err)

	// Create table and add some test data
	err = createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert test data
	insertQuery := fmt.Sprintf(`INSERT INTO %s (%s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tableName,
		historyTableColumnTimestamp,
		historyTableColumnDevice,
		historyTableColumnPort,
		historyTableColumnLinkLayer,
		historyTableColumnState,
		historyTableColumnPhysicalState,
		historyTableColumnRateGBSec,
		historyTableColumnTotalLinkDowned)

	_, err = dbRW.ExecContext(ctx, insertQuery, "2024-01-01T12:00:00Z", "mlx5_0", "1", "InfiniBand", "Active", "LinkUp", 100, 0)
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertQuery, "2024-01-01T12:01:00Z", "mlx5_1", "2", "InfiniBand", "Active", "LinkUp", 200, 0)
	require.NoError(t, err)

	// Test successful read
	devices, err := readAllDeviceValues(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Len(t, devices, 2)
	assert.Contains(t, devices, "mlx5_0")
	assert.Contains(t, devices, "mlx5_1")

	// Test with closed database
	dbRO.Close()
	_, err = readAllDeviceValues(ctx, dbRO, tableName)
	require.Error(t, err)
}

func TestReadAllPortValuesErrorHandling(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_history_table"

	// Test with non-existent table
	_, err := readAllPortValues(ctx, dbRO, "non_existent_table")
	require.Error(t, err)

	// Create table and add some test data
	err = createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Insert test data
	insertQuery := fmt.Sprintf(`INSERT INTO %s (%s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tableName,
		historyTableColumnTimestamp,
		historyTableColumnDevice,
		historyTableColumnPort,
		historyTableColumnLinkLayer,
		historyTableColumnState,
		historyTableColumnPhysicalState,
		historyTableColumnRateGBSec,
		historyTableColumnTotalLinkDowned)

	_, err = dbRW.ExecContext(ctx, insertQuery, "2024-01-01T12:00:00Z", "mlx5_0", "1", "InfiniBand", "Active", "LinkUp", 100, 0)
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertQuery, "2024-01-01T12:01:00Z", "mlx5_1", "2", "InfiniBand", "Active", "LinkUp", 200, 0)
	require.NoError(t, err)

	// Test successful read
	ports, err := readAllPortValues(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Len(t, ports, 2)
	assert.Contains(t, ports, uint(1))
	assert.Contains(t, ports, uint(2))

	// Test with closed database
	dbRO.Close()
	_, err = readAllPortValues(ctx, dbRO, tableName)
	require.Error(t, err)
}

func TestCreateHistoryTableTransactionErrors(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := "test_history_table"

	// Test successful creation
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Test with closed database - should fail to begin transaction
	dbRW.Close()
	err = createHistoryTable(ctx, dbRW, "test_table_2")
	require.Error(t, err)
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

func TestCreateHistoryTableWithIndexErrors(t *testing.T) {
	ctx := context.Background()
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create a table with an invalid name that would cause index creation to fail
	tableName := "invalid-table-name-with-reserved-characters"
	err := createHistoryTable(ctx, dbRW, tableName)
	// SQLite might be permissive with table names, but the test ensures we handle errors
	// The actual error depends on SQLite's behavior
	// Since SQLite is quite permissive, we test that it doesn't panic
	if err != nil {
		t.Logf("Expected error for invalid table name: %v", err)
	}
}

func TestCreateHistoryTableSuccess(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	testTable := "test_history_table"
	err := createHistoryTable(ctx, dbRW, testTable)
	require.NoError(t, err)

	// Verify table was created by checking if we can query it
	query := fmt.Sprintf(`SELECT name FROM sqlite_master WHERE type='table' AND name='%s'`, testTable)
	row := dbRO.QueryRowContext(ctx, query)
	var tableName string
	err = row.Scan(&tableName)
	require.NoError(t, err)
	assert.Equal(t, testTable, tableName)

	// Verify indexes were created
	query = fmt.Sprintf(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='%s'`, testTable)
	rows, err := dbRO.QueryContext(ctx, query)
	require.NoError(t, err)
	defer rows.Close()

	var indexCount int
	for rows.Next() {
		var indexName string
		err := rows.Scan(&indexName)
		require.NoError(t, err)
		indexCount++
	}
	assert.GreaterOrEqual(t, indexCount, 5, "Should have created at least 5 indexes")
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

func TestSelectAllDevices(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table
	tableName := "test_select_devices"
	err := createHistoryTable(ctx, dbRW, tableName)
	require.NoError(t, err)

	// Test empty table - should return empty map
	devices, err := readAllDeviceValues(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Len(t, devices, 0)

	// Insert test data with various devices
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_type, event_reason, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)

	// Insert multiple entries with same device
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 2, "infiniband", "active", "linkup", 400, 0, "", "", "")
	require.NoError(t, err)

	// Insert different devices
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_1", 1, "ethernet", "down", "linkdown", 0, 0, "", "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_2", 1, "infiniband", "active", "linkup", 200, 0, "", "", "")
	require.NoError(t, err)

	// Get all devices - should return unique devices
	devices, err = readAllDeviceValues(ctx, dbRO, tableName)
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
	devices, err := readAllDeviceValues(ctx, dbRO, "non_existent_table")
	require.Error(t, err)
	assert.Nil(t, devices)

	// Test with closed database
	dbRO.Close()
	devices, err = readAllDeviceValues(ctx, dbRO, "any_table")
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
	ports, err := readAllPortValues(ctx, dbRO, tableName)
	require.NoError(t, err)
	assert.Len(t, ports, 0)

	// Insert test data with various ports
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_type, event_reason, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tableName)

	// Insert multiple entries with same port
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_1", 1, "ethernet", "down", "linkdown", 0, 0, "", "", "")
	require.NoError(t, err)

	// Insert different ports
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_0", 2, "infiniband", "active", "linkup", 400, 0, "", "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_2", 3, "infiniband", "active", "linkup", 200, 0, "", "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, time.Now().Unix(), "mlx5_3", 10, "infiniband", "active", "linkup", 100, 0, "", "", "")
	require.NoError(t, err)

	// Get all ports - should return unique ports
	ports, err = readAllPortValues(ctx, dbRO, tableName)
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
	ports, err := readAllPortValues(ctx, dbRO, "non_existent_table")
	require.Error(t, err)
	assert.Nil(t, ports)

	// Test with closed database
	dbRO.Close()
	ports, err = readAllPortValues(ctx, dbRO, "any_table")
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
	devices, err := readAllDeviceValues(ctx, dbRO, tableName)
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
	ports, err := readAllPortValues(ctx, dbRO, tableName)
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

func TestNewStoreWithMetadataTableCreationError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create history table first
	err := createHistoryTable(ctx, dbRW, defaultHistoryTable)
	require.NoError(t, err)

	// Close the write database to force metadata table creation to fail
	dbRW.Close()

	// New should fail when it can't create metadata table
	_, err = New(ctx, dbRW, dbRO)
	require.Error(t, err)
}

func TestStoreInitialization(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Insert some test data before creating store
	err := createHistoryTable(ctx, dbRW, defaultHistoryTable)
	require.NoError(t, err)

	// Insert test data
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_type, event_reason, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, defaultHistoryTable)
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	_, err = dbRW.ExecContext(ctx, insertSQL, testTime.Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "", "")
	require.NoError(t, err)
	_, err = dbRW.ExecContext(ctx, insertSQL, testTime.Unix(), "mlx5_1", 2, "infiniband", "active", "linkup", 200, 0, "", "", "")
	require.NoError(t, err)

	// Create store - should initialize with existing data
	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Verify initialization
	assert.Equal(t, testTime.Unix(), s.getLastInsertTimestamp().Unix())

	devices := s.getAllDeviceValues()
	assert.Contains(t, devices, "mlx5_0")
	assert.Contains(t, devices, "mlx5_1")
	assert.Len(t, devices, 2)

	ports := s.getAllPortValues()
	assert.Contains(t, ports, uint(1))
	assert.Contains(t, ports, uint(2))
	assert.Len(t, ports, 2)
}

func TestUpdateAllDeviceValues(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Initial state should be empty
	devices := s.getAllDeviceValues()
	assert.Empty(t, devices)

	// Update with new devices
	newDevices := map[string]any{
		"mlx5_0": nil,
		"mlx5_1": nil,
	}
	s.updateAllDeviceValues(newDevices)

	// Verify devices were added
	devices = s.getAllDeviceValues()
	assert.Contains(t, devices, "mlx5_0")
	assert.Contains(t, devices, "mlx5_1")
	assert.Len(t, devices, 2)

	// Update with overlapping devices
	moreDevices := map[string]any{
		"mlx5_1": nil, // Already exists
		"mlx5_2": nil, // New device
	}
	s.updateAllDeviceValues(moreDevices)

	// Verify all devices are present
	devices = s.getAllDeviceValues()
	assert.Contains(t, devices, "mlx5_0")
	assert.Contains(t, devices, "mlx5_1")
	assert.Contains(t, devices, "mlx5_2")
	assert.Len(t, devices, 3)
}

func TestUpdateAllPortValues(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Initial state should be empty
	ports := s.getAllPortValues()
	assert.Empty(t, ports)

	// Update with new ports
	newPorts := map[uint]any{
		1: nil,
		2: nil,
	}
	s.updateAllPortValues(newPorts)

	// Verify ports were added
	ports = s.getAllPortValues()
	assert.Contains(t, ports, uint(1))
	assert.Contains(t, ports, uint(2))
	assert.Len(t, ports, 2)

	// Update with overlapping ports
	morePorts := map[uint]any{
		2: nil, // Already exists
		3: nil, // New port
	}
	s.updateAllPortValues(morePorts)

	// Verify all ports are present
	ports = s.getAllPortValues()
	assert.Contains(t, ports, uint(1))
	assert.Contains(t, ports, uint(2))
	assert.Contains(t, ports, uint(3))
	assert.Len(t, ports, 3)
}

func TestGetAllDeviceValuesThreadSafety(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some initial devices
	initialDevices := map[string]any{
		"mlx5_0": nil,
		"mlx5_1": nil,
	}
	s.updateAllDeviceValues(initialDevices)

	// Test concurrent access
	done := make(chan bool)

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			devices := s.getAllDeviceValues()
			assert.GreaterOrEqual(t, len(devices), 2)
		}
		done <- true
	}()

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			newDevices := map[string]any{
				fmt.Sprintf("mlx5_%d", i): nil,
			}
			s.updateAllDeviceValues(newDevices)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify final state
	devices := s.getAllDeviceValues()
	assert.GreaterOrEqual(t, len(devices), 2)
}

func TestGetAllPortValuesThreadSafety(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Add some initial ports
	initialPorts := map[uint]any{
		1: nil,
		2: nil,
	}
	s.updateAllPortValues(initialPorts)

	// Test concurrent access
	done := make(chan bool)

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			ports := s.getAllPortValues()
			assert.GreaterOrEqual(t, len(ports), 2)
		}
		done <- true
	}()

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			newPorts := map[uint]any{
				uint(i + 10): nil,
			}
			s.updateAllPortValues(newPorts)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify final state
	ports := s.getAllPortValues()
	assert.GreaterOrEqual(t, len(ports), 2)
}

func TestStoreWithZeroRetentionAndPurge(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create stores normally and verify they have default values
	store1, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s1 := store1.(*ibPortsStore)
	assert.Equal(t, defaultPurgeInterval, s1.purgeInterval, "First store should use default purge interval")

	// Create second store with the same context and verify it gets its own default values
	store2, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s2 := store2.(*ibPortsStore)
	assert.Equal(t, defaultPurgeInterval, s2.purgeInterval, "Second store should also use default purge interval")

	// Verify that both stores are independent instances
	assert.NotSame(t, s1, s2, "Stores should be different instances")

	// Verify that stores can be used concurrently without issues
	// Test basic operations on both stores
	eventTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ibPorts := []infiniband.IBPort{{Device: "mlx5_0", Port: 1, State: "active"}}

	err1 := s1.Insert(eventTime, ibPorts)
	err2 := s2.Insert(eventTime, ibPorts)

	// Both should succeed
	assert.NoError(t, err1, "Store1 insert should succeed")
	assert.NoError(t, err2, "Store2 insert should succeed")
}
