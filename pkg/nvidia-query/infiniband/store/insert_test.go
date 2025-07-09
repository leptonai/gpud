package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	"github.com/leptonai/gpud/pkg/sqlite"
)

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
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_type, event_reason, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, defaultHistoryTable)
	_, err = dbRW.ExecContext(ctx, insertSQL, fixedTime.Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "", "")
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

func TestInsertWithValidData(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Create test IB ports
	eventTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ibPorts := []infiniband.IBPort{
		{
			Device:          "mlx5_0",
			Port:            1,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       400,
			LinkLayer:       "Infiniband",
			TotalLinkDowned: 0,
		},
		{
			Device:          "mlx5_1",
			Port:            2,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       200,
			LinkLayer:       "Infiniband",
			TotalLinkDowned: 5,
		},
	}

	// Insert data
	err = store.Insert(eventTime, ibPorts)
	require.NoError(t, err)

	// Verify data was inserted
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE timestamp = ?`, defaultHistoryTable)
	row := dbRO.QueryRowContext(ctx, query, eventTime.Unix())
	var count int
	err = row.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "Should have inserted 2 records")

	// Verify specific data
	query = fmt.Sprintf(`SELECT device, port, state, physical_state, rate_gb_sec, total_link_downed FROM %s WHERE timestamp = ? ORDER BY device`, defaultHistoryTable)
	rows, err := dbRO.QueryContext(ctx, query, eventTime.Unix())
	require.NoError(t, err)
	defer rows.Close()

	var devices []string
	var ports []uint
	var states []string
	var physicalStates []string
	var rates []int
	var totalLinkDowned []uint64

	for rows.Next() {
		var device, state, physicalState string
		var port uint
		var rate int
		var linkDowned uint64
		err := rows.Scan(&device, &port, &state, &physicalState, &rate, &linkDowned)
		require.NoError(t, err)
		devices = append(devices, device)
		ports = append(ports, port)
		states = append(states, state)
		physicalStates = append(physicalStates, physicalState)
		rates = append(rates, rate)
		totalLinkDowned = append(totalLinkDowned, linkDowned)
	}

	assert.Equal(t, []string{"mlx5_0", "mlx5_1"}, devices)
	assert.Equal(t, []uint{1, 2}, ports)
	assert.Equal(t, []string{"active", "active"}, states)
	assert.Equal(t, []string{"linkup", "linkup"}, physicalStates)
	assert.Equal(t, []int{400, 200}, rates)
	assert.Equal(t, []uint64{0, 5}, totalLinkDowned)
}

func TestInsertWithNonIBPorts(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Create test ports with mixed link layers
	eventTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ibPorts := []infiniband.IBPort{
		{
			Device:          "mlx5_0",
			Port:            1,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       400,
			LinkLayer:       "Infiniband", // This should be inserted
			TotalLinkDowned: 0,
		},
		{
			Device:          "mlx5_1",
			Port:            2,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       200,
			LinkLayer:       "Ethernet", // This should be skipped
			TotalLinkDowned: 5,
		},
		{
			Device:          "mlx5_2",
			Port:            3,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       100,
			LinkLayer:       "Unknown", // This should be skipped
			TotalLinkDowned: 2,
		},
	}

	// Insert data
	err = store.Insert(eventTime, ibPorts)
	require.NoError(t, err)

	// Verify only IB ports were inserted
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE timestamp = ?`, defaultHistoryTable)
	row := dbRO.QueryRowContext(ctx, query, eventTime.Unix())
	var count int
	err = row.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have inserted only 1 IB port")

	// Verify it's the correct port
	query = fmt.Sprintf(`SELECT device, link_layer FROM %s WHERE timestamp = ?`, defaultHistoryTable)
	row = dbRO.QueryRowContext(ctx, query, eventTime.Unix())
	var device, linkLayer string
	err = row.Scan(&device, &linkLayer)
	require.NoError(t, err)
	assert.Equal(t, "mlx5_0", device)
	assert.Equal(t, "infiniband", linkLayer)
}

func TestInsertWithMinimumInterval(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)
	s.minInsertInterval = 1 * time.Minute // Set a longer interval for testing

	// Create test IB port
	ibPorts := []infiniband.IBPort{
		{
			Device:          "mlx5_0",
			Port:            1,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       400,
			LinkLayer:       "Infiniband",
			TotalLinkDowned: 0,
		},
	}

	// First insert should succeed
	eventTime1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	err = store.Insert(eventTime1, ibPorts)
	require.NoError(t, err)

	// Second insert within minimum interval should be skipped
	eventTime2 := eventTime1.Add(30 * time.Second) // Within the 1-minute interval
	err = store.Insert(eventTime2, ibPorts)
	require.NoError(t, err) // Should not error, but should be skipped

	// Verify only one record was inserted
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, defaultHistoryTable)
	row := dbRO.QueryRowContext(ctx, query)
	var count int
	err = row.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have only 1 record due to minimum interval")

	// Third insert after minimum interval should succeed
	eventTime3 := eventTime1.Add(2 * time.Minute) // Beyond the 1-minute interval
	err = store.Insert(eventTime3, ibPorts)
	require.NoError(t, err)

	// Verify now we have two records
	row = dbRO.QueryRowContext(ctx, query)
	err = row.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "Should have 2 records after interval passed")
}

func TestInsertWithDatabaseError(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Close the database to force an error
	dbRW.Close()

	eventTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ibPorts := []infiniband.IBPort{
		{
			Device:          "mlx5_0",
			Port:            1,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       400,
			LinkLayer:       "Infiniband",
			TotalLinkDowned: 0,
		},
	}

	// Insert should fail with database error
	err = store.Insert(eventTime, ibPorts)
	require.Error(t, err)
}

func TestInsertWithEmptyPorts(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	// Insert with empty ports slice
	eventTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	err = store.Insert(eventTime, []infiniband.IBPort{})
	require.NoError(t, err)

	// Verify no records were inserted
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, defaultHistoryTable)
	row := dbRO.QueryRowContext(ctx, query)
	var count int
	err = row.Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Should have no records for empty ports")
}

func TestUpdateAndGetLastInsertTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Initial timestamp should be zero
	lastTs := s.getLastInsertTimestamp()
	assert.True(t, lastTs.IsZero(), "Initial timestamp should be zero")

	// Update timestamp
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	s.updateLastInsertTimestamp(testTime)

	// Get timestamp should return updated value
	lastTs = s.getLastInsertTimestamp()
	assert.Equal(t, testTime.Unix(), lastTs.Unix(), "Should return updated timestamp")

	// Update with another timestamp
	testTime2 := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	s.updateLastInsertTimestamp(testTime2)

	// Get timestamp should return the newer value
	lastTs = s.getLastInsertTimestamp()
	assert.Equal(t, testTime2.Unix(), lastTs.Unix(), "Should return newer timestamp")
}

func TestReadLastTimestamp(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create table
	err := createHistoryTable(ctx, dbRW, defaultHistoryTable)
	require.NoError(t, err)

	// Test with empty table
	lastTs, err := readLastTimestamp(ctx, dbRO, defaultHistoryTable)
	require.NoError(t, err)
	assert.True(t, lastTs.IsZero(), "Should return zero time for empty table")

	// Insert test data
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	insertSQL := fmt.Sprintf(`INSERT INTO %s (timestamp, device, port, link_layer, state, physical_state, rate_gb_sec, total_link_downed, event_type, event_reason, extra_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, defaultHistoryTable)
	_, err = dbRW.ExecContext(ctx, insertSQL, testTime.Unix(), "mlx5_0", 1, "infiniband", "active", "linkup", 400, 0, "", "", "")
	require.NoError(t, err)

	// Test with data
	lastTs, err = readLastTimestamp(ctx, dbRO, defaultHistoryTable)
	require.NoError(t, err)
	assert.Equal(t, testTime.Unix(), lastTs.Unix(), "Should return the inserted timestamp")

	// Insert another record with later timestamp
	testTime2 := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	_, err = dbRW.ExecContext(ctx, insertSQL, testTime2.Unix(), "mlx5_1", 2, "infiniband", "active", "linkup", 200, 0, "", "", "")
	require.NoError(t, err)

	// Should return the latest timestamp
	lastTs, err = readLastTimestamp(ctx, dbRO, defaultHistoryTable)
	require.NoError(t, err)
	assert.Equal(t, testTime2.Unix(), lastTs.Unix(), "Should return the latest timestamp")
}

func TestReadLastTimestampWithError(t *testing.T) {
	ctx := context.Background()
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with non-existent table
	_, err := readLastTimestamp(ctx, dbRO, "non_existent_table")
	require.Error(t, err)

	// Test with closed database
	dbRO.Close()
	_, err = readLastTimestamp(ctx, dbRO, defaultHistoryTable)
	require.Error(t, err)
}

func TestInsertUpdatesDeviceAndPortValues(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(ctx, dbRW, dbRO)
	require.NoError(t, err)

	s := store.(*ibPortsStore)

	// Check initial state
	devices := s.getAllDeviceValues()
	ports := s.getAllPortValues()
	assert.Empty(t, devices, "Should start with empty devices")
	assert.Empty(t, ports, "Should start with empty ports")

	// Insert test data
	eventTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ibPorts := []infiniband.IBPort{
		{
			Device:          "mlx5_0",
			Port:            1,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       400,
			LinkLayer:       "Infiniband",
			TotalLinkDowned: 0,
		},
		{
			Device:          "mlx5_1",
			Port:            2,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       200,
			LinkLayer:       "Infiniband",
			TotalLinkDowned: 5,
		},
	}

	err = store.Insert(eventTime, ibPorts)
	require.NoError(t, err)

	// Check updated state
	devices = s.getAllDeviceValues()
	ports = s.getAllPortValues()
	assert.Contains(t, devices, "mlx5_0", "Should contain mlx5_0")
	assert.Contains(t, devices, "mlx5_1", "Should contain mlx5_1")
	assert.Contains(t, ports, uint(1), "Should contain port 1")
	assert.Contains(t, ports, uint(2), "Should contain port 2")
	assert.Len(t, devices, 2, "Should have 2 devices")
	assert.Len(t, ports, 2, "Should have 2 ports")
}

func TestInsertWithContextTimeout(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create a context that can be canceled later
	cancellableCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create a store with the cancellable context
	store, err := New(cancellableCtx, dbRW, dbRO)
	require.NoError(t, err)

	// Cancel the context after store creation to test Insert behavior with canceled context
	cancel()

	eventTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ibPorts := []infiniband.IBPort{
		{
			Device:          "mlx5_0",
			Port:            1,
			State:           "Active",
			PhysicalState:   "LinkUp",
			RateGBSec:       400,
			LinkLayer:       "Infiniband",
			TotalLinkDowned: 0,
		},
	}

	// Insert should fail due to context cancellation
	err = store.Insert(eventTime, ibPorts)
	require.Error(t, err)
}
