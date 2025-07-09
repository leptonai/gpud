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

func TestFindDrops(t *testing.T) {
	// Helper function to create a snapshot
	createSnapshot := func(ts time.Time, state string, linkDowned uint64) devPortSnapshot {
		return devPortSnapshot{
			ts:              ts,
			state:           state,
			totalLinkDowned: linkDowned,
		}
	}

	// Base time for tests
	baseTime := time.Now()
	threshold := 4 * time.Minute
	device := "mlx5_0"
	port := uint(1)

	tests := []struct {
		name      string
		snapshots devPortSnapshots
		expected  int // number of drop events expected
		comment   string
	}{
		{
			name:      "empty_snapshots",
			snapshots: devPortSnapshots{},
			expected:  0,
			comment:   "Empty snapshots should return no drops",
		},
		{
			name: "single_snapshot",
			snapshots: devPortSnapshots{
				createSnapshot(baseTime, "down", 5),
			},
			expected: 0,
			comment:  "Single snapshot cannot determine a drop (need at least 2)",
		},
		{
			name: "all_active_ports",
			snapshots: devPortSnapshots{
				createSnapshot(baseTime, "active", 0),
				createSnapshot(baseTime.Add(1*time.Minute), "active", 0),
				createSnapshot(baseTime.Add(2*time.Minute), "active", 0),
			},
			expected: 0,
			comment:  "All active ports should have no drops",
		},
		{
			name: "successful_drop_detection",
			snapshots: devPortSnapshots{
				// Example: Port goes down and stays down for 5 minutes (> 4 minute threshold)
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(1*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(2*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(3*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(5*time.Minute), "down", 5), // 5 minutes elapsed
			},
			expected: 1,
			comment:  "Port down for 5 minutes with same link count = drop",
		},
		{
			name: "drop_below_threshold",
			snapshots: devPortSnapshots{
				// Example: Port down for only 3 minutes (< 4 minute threshold)
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(1*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(3*time.Minute), "down", 5),
			},
			expected: 0,
			comment:  "Port down for 3 minutes (< 4 minute threshold) = no drop",
		},
		{
			name: "interrupted_by_active",
			snapshots: devPortSnapshots{
				// Example: Port goes down, then active, then down again
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(2*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(3*time.Minute), "active", 5), // Interrupts the down sequence
				createSnapshot(baseTime.Add(4*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(6*time.Minute), "down", 5),
			},
			expected: 0,
			comment:  "Down sequence interrupted by active state = no drop (need consecutive down)",
		},
		{
			name: "changing_link_count",
			snapshots: devPortSnapshots{
				// Example: Port down but link count changes (indicates flapping)
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(1*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(2*time.Minute), "down", 6), // Link count changed!
				createSnapshot(baseTime.Add(3*time.Minute), "down", 6),
				createSnapshot(baseTime.Add(5*time.Minute), "down", 6),
			},
			expected: 0,
			comment:  "Link count changed during down period = potential flap, not drop",
		},
		{
			name: "mixed_states_ending_with_drop",
			snapshots: devPortSnapshots{
				// Example: Active, then persistent down
				createSnapshot(baseTime, "active", 5),
				createSnapshot(baseTime.Add(1*time.Minute), "active", 5),
				createSnapshot(baseTime.Add(2*time.Minute), "down", 5), // Start of down
				createSnapshot(baseTime.Add(3*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(4*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(7*time.Minute), "down", 5), // 5 minutes of down
			},
			expected: 1,
			comment:  "Active followed by persistent down = drop detected",
		},
		{
			name: "exactly_at_threshold",
			snapshots: devPortSnapshots{
				// Example: Port down for exactly 4 minutes (= threshold)
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(1*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(2*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(4*time.Minute), "down", 5), // Exactly 4 minutes
			},
			expected: 1,
			comment:  "Port down for exactly threshold duration = drop",
		},
		{
			name: "multiple_down_sequences",
			snapshots: devPortSnapshots{
				// Example: Down, active, down again (longer)
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(1*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(2*time.Minute), "active", 5),
				createSnapshot(baseTime.Add(3*time.Minute), "down", 5), // New down sequence
				createSnapshot(baseTime.Add(4*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(8*time.Minute), "down", 5), // 5 minutes in new sequence
			},
			expected: 1,
			comment:  "Multiple down sequences, only second one crosses threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call findDrops with test data
			result := tt.snapshots.findDrops(device, port, threshold)

			// Verify the number of drop events
			assert.Len(t, result, tt.expected, tt.comment)

			// If we expect a drop, verify the details
			if tt.expected > 0 && len(result) > 0 {
				drop := result[0]
				// The drop should reference the latest snapshot
				assert.Equal(t, tt.snapshots[len(tt.snapshots)-1].ts, drop.ts)
				assert.Equal(t, "down", drop.state)
				assert.Contains(t, drop.reason, device)
				assert.Contains(t, drop.reason, "down since")
			}
		})
	}
}

func TestFindDrops_EdgeCases(t *testing.T) {
	baseTime := time.Now()
	threshold := 4 * time.Minute
	device := "mlx5_0"
	port := uint(1)

	t.Run("very_long_down_period", func(t *testing.T) {
		// Test with a very long down period (hours)
		snapshots := devPortSnapshots{
			{ts: baseTime, state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(1 * time.Hour), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(2 * time.Hour), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(3 * time.Hour), state: "down", totalLinkDowned: 5},
		}

		result := snapshots.findDrops(device, port, threshold)
		assert.Len(t, result, 1, "Long down period should still detect drop")
		assert.Equal(t, snapshots[3].ts, result[0].ts)
	})

	t.Run("rapid_state_changes", func(t *testing.T) {
		// Test with rapid state changes (seconds apart)
		snapshots := devPortSnapshots{
			{ts: baseTime, state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(10 * time.Second), state: "active", totalLinkDowned: 5},
			{ts: baseTime.Add(20 * time.Second), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(30 * time.Second), state: "active", totalLinkDowned: 5},
		}

		result := snapshots.findDrops(device, port, threshold)
		assert.Len(t, result, 0, "Rapid state changes should not detect drop")
	})

	t.Run("nil_handling", func(t *testing.T) {
		// Test with nil/empty scenarios
		var nilSnapshots devPortSnapshots
		result := nilSnapshots.findDrops(device, port, threshold)
		assert.Len(t, result, 0, "Nil snapshots should return empty result")
	})
}

func TestFindDrops_ReasonMessage(t *testing.T) {
	baseTime := time.Now()
	threshold := 4 * time.Minute
	device := "mlx5_0"
	port := uint(1)

	snapshots := devPortSnapshots{
		{ts: baseTime, state: "down", totalLinkDowned: 5},
		{ts: baseTime.Add(5 * time.Minute), state: "down", totalLinkDowned: 5},
	}

	result := snapshots.findDrops(device, port, threshold)
	assert.Len(t, result, 1)

	// Verify the reason message format
	drop := result[0]
	assert.Contains(t, drop.reason, device)
	assert.Contains(t, drop.reason, "port 1")
	assert.Contains(t, drop.reason, "down since")
	assert.Contains(t, drop.reason, baseTime.UTC().Format(time.RFC3339))
}

func TestScanIBPortDrops(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with custom drop threshold
	store := &ibPortsStore{
		rootCtx:             ctx,
		historyTable:        "test_scan_drops_table",
		dbRW:                dbRW,
		dbRO:                dbRO,
		ibPortDropThreshold: 2 * time.Minute, // Lower threshold for testing
		getTimeNow:          time.Now,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	device := "mlx5_0"
	port := uint(1)
	currentTime := time.Now()

	t.Run("successful_drop_detection", func(t *testing.T) {
		// Insert test data that represents a drop scenario
		// Port down for 3 minutes (> 2 minute threshold)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-3*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime, device, port, "down", 5)

		// Scan for drops
		result, err := store.scanIBPortDrops(device, port, currentTime.Add(-4*time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 1, "Should detect one drop event")

		if len(result) > 0 {
			assert.Equal(t, "down", result[0].state)
			assert.Contains(t, result[0].reason, device)
			assert.Contains(t, result[0].reason, "down since")
		}
	})

	t.Run("no_drop_when_active", func(t *testing.T) {
		// Clear previous data
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", store.historyTable))
		require.NoError(t, err)

		// Insert test data - all active
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-3*time.Minute), device, port, "active", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute), device, port, "active", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute), device, port, "active", 5)

		// Scan for drops
		result, err := store.scanIBPortDrops(device, port, currentTime.Add(-4*time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 0, "Should not detect any drop events")
	})

	t.Run("no_drop_below_threshold", func(t *testing.T) {
		// Clear previous data
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", store.historyTable))
		require.NoError(t, err)

		// Insert test data - down for only 1 minute (< 2 minute threshold)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime, device, port, "down", 5)

		// Scan for drops
		result, err := store.scanIBPortDrops(device, port, currentTime.Add(-2*time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 0, "Should not detect drop when below threshold")
	})

	t.Run("no_drop_with_link_count_change", func(t *testing.T) {
		// Clear previous data
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", store.historyTable))
		require.NoError(t, err)

		// Insert test data - down but link count changes
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-3*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute), device, port, "down", 6) // Link count changed
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime, device, port, "down", 6)

		// Scan for drops
		result, err := store.scanIBPortDrops(device, port, currentTime.Add(-4*time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 0, "Should not detect drop when link count changes")
	})

	t.Run("error_handling", func(t *testing.T) {
		// Test with non-existent device/port
		result, err := store.scanIBPortDrops("non_existent_device", 999, currentTime)
		require.NoError(t, err) // Should not error, just return empty
		assert.Len(t, result, 0)
	})

	t.Run("database_error", func(t *testing.T) {
		// Create a store with closed database to force an error
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		cleanup() // Close the database immediately

		badStore := &ibPortsStore{
			rootCtx:             ctx,
			historyTable:        "test_error_table",
			dbRW:                dbRW,
			dbRO:                dbRO, // This is now closed
			ibPortDropThreshold: 2 * time.Minute,
			getTimeNow:          time.Now,
		}

		// This should return an error because the database is closed
		result, err := badStore.scanIBPortDrops(device, port, currentTime)
		require.Error(t, err)
		assert.Nil(t, result)
	})
}
