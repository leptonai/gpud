package gpudstate

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTableMachineMetadata(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	err := CreateTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Verify table exists
	var name string
	err = dbRO.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", TableNameMachineMetadata).Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, TableNameMachineMetadata, name)

	// Verify columns exist
	columns := []string{ColumnMachineID, ColumnToken, ColumnComponents}
	for _, col := range columns {
		var columnName string
		err = dbRO.QueryRow("SELECT name FROM pragma_table_info(?) WHERE name=?", TableNameMachineMetadata, col).Scan(&columnName)
		require.NoError(t, err)
		assert.Equal(t, col, columnName)
	}
}

func TestMachineIDOperations(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table
	err := CreateTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Test getting non-existent machine ID
	_, err = GetMachineID(ctx, dbRO)
	assert.Error(t, err)

	// Test creating machine ID without UID
	id1, err := CreateMachineIDIfNotExist(ctx, dbRW, dbRW, "")
	require.NoError(t, err)
	assert.NotEmpty(t, id1)

	// Test getting existing machine ID
	id2, err := GetMachineID(ctx, dbRO)
	require.NoError(t, err)
	assert.Equal(t, id1, id2)

	// Test creating machine ID with specific UID
	specificUID := "test-machine-123"
	id3, err := CreateMachineIDIfNotExist(ctx, dbRW, dbRW, specificUID)
	require.NoError(t, err)
	assert.Equal(t, id1, id3) // Should return existing ID
}

func TestLoginInfoOperations(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table and machine ID
	err := CreateTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)
	machineID, err := CreateMachineIDIfNotExist(ctx, dbRW, dbRW, "")
	require.NoError(t, err)

	// Test getting non-existent login info
	var token string
	err = dbRO.QueryRowContext(ctx, "SELECT COALESCE(token, '') FROM machine_metadata WHERE machine_id = ?", machineID).Scan(&token)
	assert.NoError(t, err)
	assert.Empty(t, token)

	// Test updating login info
	testToken := "test-token-123"
	err = UpdateLoginInfo(ctx, dbRW, machineID, testToken)
	require.NoError(t, err)

	// Test getting updated login info
	err = dbRO.QueryRowContext(ctx, "SELECT COALESCE(token, '') FROM machine_metadata WHERE machine_id = ?", machineID).Scan(&token)
	require.NoError(t, err)
	assert.Equal(t, testToken, token)
}

func TestComponentsOperations(t *testing.T) {
	t.Parallel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table and machine ID
	err := CreateTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)
	machineID, err := CreateMachineIDIfNotExist(ctx, dbRW, dbRW, "")
	require.NoError(t, err)

	// Test getting non-existent components
	var components string
	err = dbRO.QueryRowContext(ctx, "SELECT COALESCE(components, '') FROM machine_metadata WHERE machine_id = ?", machineID).Scan(&components)
	assert.NoError(t, err)
	assert.Empty(t, components)

	// Test updating components
	testComponents := `{"gpu": {"vendor": "nvidia", "count": 4}}`
	err = UpdateComponents(ctx, dbRW, machineID, testComponents)
	require.NoError(t, err)

	// Test getting updated components
	err = dbRO.QueryRowContext(ctx, "SELECT COALESCE(components, '') FROM machine_metadata WHERE machine_id = ?", machineID).Scan(&components)
	require.NoError(t, err)
	assert.Equal(t, testComponents, components)
}

func TestRecordMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatal("failed to register metrics:", err)
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create table and machine ID
	err := CreateTableMachineMetadata(ctx, dbRW)
	require.NoError(t, err)
	machineID, err := CreateMachineIDIfNotExist(ctx, dbRW, dbRW, "")
	require.NoError(t, err)

	// Update components
	testComponents := `{"gpu": {"vendor": "nvidia", "count": 4}}`
	err = UpdateComponents(ctx, dbRW, machineID, testComponents)
	require.NoError(t, err)

	// Record metrics
	err = RecordMetrics(ctx, dbRO)
	require.NoError(t, err)

	// Verify metrics were recorded
	metrics, err := reg.Gather()
	require.NoError(t, err)
	assert.NotEmpty(t, metrics)
}
