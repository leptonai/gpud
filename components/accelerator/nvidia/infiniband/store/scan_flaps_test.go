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

func TestFindFlaps(t *testing.T) {
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
	downIntervalThreshold := 25 * time.Second
	flapBackToActiveThreshold := 3
	device := "mlx5_0"
	port := uint(1)

	tests := []struct {
		name      string
		snapshots devPortSnapshots
		expected  int // number of flap events expected
		comment   string
	}{
		{
			name:      "empty_snapshots",
			snapshots: devPortSnapshots{},
			expected:  0,
			comment:   "Empty snapshots should return no flaps",
		},
		{
			name: "not_enough_snapshots",
			snapshots: devPortSnapshots{
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(30*time.Second), "active", 5),
			},
			expected: 0,
			comment:  "Less than 3 snapshots cannot determine flaps (need at least 3)",
		},
		{
			name: "all_active_ports",
			snapshots: devPortSnapshots{
				createSnapshot(baseTime, "active", 0),
				createSnapshot(baseTime.Add(1*time.Minute), "active", 0),
				createSnapshot(baseTime.Add(2*time.Minute), "active", 0),
				createSnapshot(baseTime.Add(3*time.Minute), "active", 0),
			},
			expected: 0,
			comment:  "All active ports should have no flaps",
		},
		{
			name: "successful_flap_detection",
			snapshots: devPortSnapshots{
				// Example: 3 flap cycles - port goes down for >25s, then back to active, 3 times
				// Cycle 1
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(30*time.Second), "down", 5),   // >25s down
				createSnapshot(baseTime.Add(40*time.Second), "active", 5), // Back to active
				// Cycle 2
				createSnapshot(baseTime.Add(1*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(1*time.Minute+30*time.Second), "down", 5),   // >25s down
				createSnapshot(baseTime.Add(1*time.Minute+40*time.Second), "active", 5), // Back to active
				// Cycle 3
				createSnapshot(baseTime.Add(2*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(2*time.Minute+30*time.Second), "down", 5),   // >25s down
				createSnapshot(baseTime.Add(2*time.Minute+40*time.Second), "active", 5), // Back to active (3rd time)
			},
			expected: 1,
			comment:  "3 flap cycles (down >25s then active) = flap detected",
		},
		{
			name: "not_enough_flap_cycles",
			snapshots: devPortSnapshots{
				// Example: Only 2 flap cycles when threshold is 3
				// Cycle 1
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(30*time.Second), "down", 5),   // >25s down
				createSnapshot(baseTime.Add(40*time.Second), "active", 5), // Back to active
				// Cycle 2
				createSnapshot(baseTime.Add(1*time.Minute), "down", 5),
				createSnapshot(baseTime.Add(1*time.Minute+30*time.Second), "down", 5),   // >25s down
				createSnapshot(baseTime.Add(1*time.Minute+40*time.Second), "active", 5), // Back to active (only 2nd time)
			},
			expected: 0,
			comment:  "Only 2 flap cycles when threshold is 3 = no flap",
		},
		{
			name: "down_interval_too_short",
			snapshots: devPortSnapshots{
				// Example: Port goes down but not long enough (only 20s, < 25s threshold)
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(20*time.Second), "down", 5),   // Only 20s down
				createSnapshot(baseTime.Add(30*time.Second), "active", 5), // Back to active
				createSnapshot(baseTime.Add(40*time.Second), "down", 5),
				createSnapshot(baseTime.Add(60*time.Second), "down", 5),   // Only 20s down
				createSnapshot(baseTime.Add(70*time.Second), "active", 5), // Back to active
				createSnapshot(baseTime.Add(80*time.Second), "down", 5),
				createSnapshot(baseTime.Add(100*time.Second), "down", 5),   // Only 20s down
				createSnapshot(baseTime.Add(110*time.Second), "active", 5), // Back to active
			},
			expected: 0,
			comment:  "Down intervals too short (< 25s threshold) = no flap",
		},
		{
			name: "non_consecutive_down_states",
			snapshots: devPortSnapshots{
				// Example: Down states interrupted by active before reaching threshold
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(10*time.Second), "active", 5), // Interrupted
				createSnapshot(baseTime.Add(20*time.Second), "down", 5),
				createSnapshot(baseTime.Add(30*time.Second), "active", 5), // Interrupted
				createSnapshot(baseTime.Add(40*time.Second), "down", 5),
				createSnapshot(baseTime.Add(50*time.Second), "active", 5), // Interrupted
			},
			expected: 0,
			comment:  "Down states not consecutive = no persistent down = no flap",
		},
		{
			name: "mixed_valid_flaps",
			snapshots: devPortSnapshots{
				// Example: Mix of valid flaps and invalid attempts
				// Valid cycle 1
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(30*time.Second), "down", 5),
				createSnapshot(baseTime.Add(40*time.Second), "active", 5),
				// Invalid attempt (too short)
				createSnapshot(baseTime.Add(50*time.Second), "down", 5),
				createSnapshot(baseTime.Add(60*time.Second), "active", 5), // Only 10s down
				// Valid cycle 2
				createSnapshot(baseTime.Add(70*time.Second), "down", 5),
				createSnapshot(baseTime.Add(100*time.Second), "down", 5),
				createSnapshot(baseTime.Add(110*time.Second), "active", 5),
				// Valid cycle 3
				createSnapshot(baseTime.Add(120*time.Second), "down", 5),
				createSnapshot(baseTime.Add(150*time.Second), "down", 5),
				createSnapshot(baseTime.Add(160*time.Second), "active", 5),
			},
			expected: 1,
			comment:  "3 valid flap cycles mixed with invalid attempts = flap detected",
		},
		{
			name: "exactly_at_threshold_count",
			snapshots: devPortSnapshots{
				// Example: Exactly 3 flap cycles (threshold = 3)
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(30*time.Second), "down", 5),
				createSnapshot(baseTime.Add(40*time.Second), "active", 5), // 1st
				createSnapshot(baseTime.Add(50*time.Second), "down", 5),
				createSnapshot(baseTime.Add(80*time.Second), "down", 5),
				createSnapshot(baseTime.Add(90*time.Second), "active", 5), // 2nd
				createSnapshot(baseTime.Add(100*time.Second), "down", 5),
				createSnapshot(baseTime.Add(130*time.Second), "down", 5),
				createSnapshot(baseTime.Add(140*time.Second), "active", 5), // 3rd (exactly at threshold)
			},
			expected: 1,
			comment:  "Exactly 3 flap cycles = flap detected",
		},
		{
			name: "more_than_threshold_flaps",
			snapshots: devPortSnapshots{
				// Example: 5 flap cycles when threshold is 3
				createSnapshot(baseTime, "down", 5),
				createSnapshot(baseTime.Add(30*time.Second), "down", 5),
				createSnapshot(baseTime.Add(40*time.Second), "active", 5), // 1st
				createSnapshot(baseTime.Add(50*time.Second), "down", 5),
				createSnapshot(baseTime.Add(80*time.Second), "down", 5),
				createSnapshot(baseTime.Add(90*time.Second), "active", 5), // 2nd
				createSnapshot(baseTime.Add(100*time.Second), "down", 5),
				createSnapshot(baseTime.Add(130*time.Second), "down", 5),
				createSnapshot(baseTime.Add(140*time.Second), "active", 5), // 3rd (threshold reached)
				createSnapshot(baseTime.Add(150*time.Second), "down", 5),
				createSnapshot(baseTime.Add(180*time.Second), "down", 5),
				createSnapshot(baseTime.Add(190*time.Second), "active", 5), // 4th
				createSnapshot(baseTime.Add(200*time.Second), "down", 5),
				createSnapshot(baseTime.Add(230*time.Second), "down", 5),
				createSnapshot(baseTime.Add(240*time.Second), "active", 5), // 5th
			},
			expected: 1,
			comment:  "5 flap cycles when threshold is 3 = returns 3rd flap event (first breach)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call findFlaps with test data
			result := tt.snapshots.findFlaps(device, port, downIntervalThreshold, flapBackToActiveThreshold)

			// Verify the number of flap events
			assert.Len(t, result, tt.expected, tt.comment)

			// If we expect a flap, verify the details
			if tt.expected > 0 && len(result) > 0 {
				flap := result[0]
				assert.Equal(t, "active", flap.state)
				assert.Contains(t, flap.reason, device)
				assert.Contains(t, flap.reason, "down since")
				assert.Contains(t, flap.reason, "flapped back to active")
			}
		})
	}
}

func TestFindFlaps_EdgeCases(t *testing.T) {
	baseTime := time.Now()
	downIntervalThreshold := 25 * time.Second
	flapBackToActiveThreshold := 3
	device := "mlx5_0"
	port := uint(1)

	t.Run("threshold_is_one", func(t *testing.T) {
		// Test with flapBackToActiveThreshold = 1
		snapshots := devPortSnapshots{
			{ts: baseTime, state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(30 * time.Second), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(40 * time.Second), state: "active", totalLinkDowned: 5}, // Only 1 flap needed
		}

		result := snapshots.findFlaps(device, port, downIntervalThreshold, 1)
		assert.Len(t, result, 1, "Single flap should be detected when threshold is 1")
	})

	t.Run("very_long_down_intervals", func(t *testing.T) {
		// Test with very long down intervals (hours)
		snapshots := devPortSnapshots{
			{ts: baseTime, state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(1 * time.Hour), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(2 * time.Hour), state: "active", totalLinkDowned: 5},
			{ts: baseTime.Add(3 * time.Hour), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(4 * time.Hour), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(5 * time.Hour), state: "active", totalLinkDowned: 5},
			{ts: baseTime.Add(6 * time.Hour), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(7 * time.Hour), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(8 * time.Hour), state: "active", totalLinkDowned: 5},
		}

		result := snapshots.findFlaps(device, port, downIntervalThreshold, flapBackToActiveThreshold)
		assert.Len(t, result, 1, "Long down intervals should still detect flaps")
	})

	t.Run("nil_handling", func(t *testing.T) {
		// Test with nil/empty scenarios
		var nilSnapshots devPortSnapshots
		result := nilSnapshots.findFlaps(device, port, downIntervalThreshold, flapBackToActiveThreshold)
		assert.Len(t, result, 0, "Nil snapshots should return empty result")
	})

	t.Run("exactly_at_down_threshold", func(t *testing.T) {
		// Test with down interval exactly at threshold (25s)
		snapshots := devPortSnapshots{
			{ts: baseTime, state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(25 * time.Second), state: "down", totalLinkDowned: 5}, // Exactly 25s
			{ts: baseTime.Add(35 * time.Second), state: "active", totalLinkDowned: 5},
			{ts: baseTime.Add(45 * time.Second), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(70 * time.Second), state: "down", totalLinkDowned: 5}, // Exactly 25s
			{ts: baseTime.Add(80 * time.Second), state: "active", totalLinkDowned: 5},
			{ts: baseTime.Add(90 * time.Second), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(115 * time.Second), state: "down", totalLinkDowned: 5}, // Exactly 25s
			{ts: baseTime.Add(125 * time.Second), state: "active", totalLinkDowned: 5},
		}

		result := snapshots.findFlaps(device, port, downIntervalThreshold, flapBackToActiveThreshold)
		assert.Len(t, result, 1, "Down intervals exactly at threshold should detect flaps")
	})
}

func TestFindFlaps_ReasonMessage(t *testing.T) {
	baseTime := time.Now()
	downIntervalThreshold := 25 * time.Second
	flapBackToActiveThreshold := 3
	device := "mlx5_0"
	port := uint(1)

	// Create a scenario with exactly 3 flaps
	snapshots := devPortSnapshots{
		// Flap 1
		{ts: baseTime, state: "down", totalLinkDowned: 5},
		{ts: baseTime.Add(30 * time.Second), state: "down", totalLinkDowned: 5},
		{ts: baseTime.Add(40 * time.Second), state: "active", totalLinkDowned: 5},
		// Flap 2
		{ts: baseTime.Add(50 * time.Second), state: "down", totalLinkDowned: 5},
		{ts: baseTime.Add(80 * time.Second), state: "down", totalLinkDowned: 5},
		{ts: baseTime.Add(90 * time.Second), state: "active", totalLinkDowned: 5},
		// Flap 3
		{ts: baseTime.Add(100 * time.Second), state: "down", totalLinkDowned: 5},
		{ts: baseTime.Add(130 * time.Second), state: "down", totalLinkDowned: 5},
		{ts: baseTime.Add(140 * time.Second), state: "active", totalLinkDowned: 5},
	}

	result := snapshots.findFlaps(device, port, downIntervalThreshold, flapBackToActiveThreshold)
	assert.Len(t, result, 1)

	// Verify the reason message format
	flap := result[0]
	assert.Contains(t, flap.reason, device)
	assert.Contains(t, flap.reason, "port 1")
	assert.Contains(t, flap.reason, "down since")
	assert.Contains(t, flap.reason, "flapped back to active")

	// The returned flap should be the 3rd active state (when threshold was breached)
	assert.Equal(t, snapshots[8].ts, flap.ts) // Index 8 is the 3rd "active" state
}

func TestFindFlaps_ComplexScenarios(t *testing.T) {
	baseTime := time.Now()
	downIntervalThreshold := 25 * time.Second
	flapBackToActiveThreshold := 2
	device := "mlx5_0"
	port := uint(1)

	t.Run("persistent_down_then_flaps", func(t *testing.T) {
		// Test scenario: Long down period, then starts flapping
		snapshots := devPortSnapshots{
			// Long persistent down (no flap)
			{ts: baseTime, state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(10 * time.Minute), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(20 * time.Minute), state: "down", totalLinkDowned: 5},
			// Now starts flapping
			{ts: baseTime.Add(21 * time.Minute), state: "active", totalLinkDowned: 5}, // Back to active
			// Flap 1
			{ts: baseTime.Add(22 * time.Minute), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(22*time.Minute + 30*time.Second), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(23 * time.Minute), state: "active", totalLinkDowned: 5}, // 1st flap
			// Flap 2
			{ts: baseTime.Add(24 * time.Minute), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(24*time.Minute + 30*time.Second), state: "down", totalLinkDowned: 5},
			{ts: baseTime.Add(25 * time.Minute), state: "active", totalLinkDowned: 5}, // 2nd flap (threshold reached)
		}

		result := snapshots.findFlaps(device, port, downIntervalThreshold, flapBackToActiveThreshold)
		assert.Len(t, result, 1, "Should detect flaps after persistent down period")
		assert.Equal(t, snapshots[6].ts, result[0].ts) // The 2nd flap back to active (when threshold reached)
	})
}

func TestScanIBPortFlaps(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create store with custom flap thresholds
	store := &ibPortsStore{
		rootCtx:                         ctx,
		historyTable:                    "test_scan_flaps_table",
		dbRW:                            dbRW,
		dbRO:                            dbRO,
		ibPortFlapDownIntervalThreshold: 20 * time.Second, // Lower threshold for testing
		ibPortFlapBackToActiveThreshold: 2,                // Only need 2 flaps for testing
		getTimeNow:                      time.Now,
	}

	// Create history table
	err := createHistoryTable(ctx, dbRW, store.historyTable)
	require.NoError(t, err)

	device := "mlx5_0"
	port := uint(1)
	currentTime := time.Now()

	t.Run("successful_flap_detection", func(t *testing.T) {
		// Insert test data that represents 2 flap cycles
		// Cycle 1: down for 25s, then active
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute+25*time.Second), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute+30*time.Second), device, port, "active", 5)

		// Cycle 2: down for 25s, then active
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute+25*time.Second), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute+30*time.Second), device, port, "active", 5)

		// Scan for flaps
		result, err := store.scanIBPortFlaps(device, port, currentTime.Add(-3*time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 1, "Should detect one flap event")

		if len(result) > 0 {
			assert.Equal(t, "active", result[0].state)
			assert.Contains(t, result[0].reason, device)
			assert.Contains(t, result[0].reason, "flapped back to active")
		}
	})

	t.Run("no_flap_when_all_active", func(t *testing.T) {
		// Clear previous data
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", store.historyTable))
		require.NoError(t, err)

		// Insert test data - all active
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-3*time.Minute), device, port, "active", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute), device, port, "active", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute), device, port, "active", 5)

		// Scan for flaps
		result, err := store.scanIBPortFlaps(device, port, currentTime.Add(-4*time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 0, "Should not detect any flap events")
	})

	t.Run("no_flap_insufficient_cycles", func(t *testing.T) {
		// Clear previous data
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", store.historyTable))
		require.NoError(t, err)

		// Insert test data - only 1 flap cycle when we need 2
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute+25*time.Second), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute+30*time.Second), device, port, "active", 5)

		// Scan for flaps
		result, err := store.scanIBPortFlaps(device, port, currentTime.Add(-2*time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 0, "Should not detect flap with insufficient cycles")
	})

	t.Run("no_flap_short_down_interval", func(t *testing.T) {
		// Clear previous data
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", store.historyTable))
		require.NoError(t, err)

		// Insert test data - down intervals too short (15s < 20s threshold)
		// Cycle 1
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute+15*time.Second), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute+20*time.Second), device, port, "active", 5)

		// Cycle 2
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute+15*time.Second), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute+20*time.Second), device, port, "active", 5)

		// Scan for flaps
		result, err := store.scanIBPortFlaps(device, port, currentTime.Add(-3*time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 0, "Should not detect flap with short down intervals")
	})

	t.Run("complex_flap_scenario", func(t *testing.T) {
		// Clear previous data
		_, err := dbRW.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", store.historyTable))
		require.NoError(t, err)

		// Insert complex scenario: persistent down, then 2 flaps
		// Long persistent down (not a flap)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-5*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-4*time.Minute), device, port, "down", 5)

		// Back to active
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-3*time.Minute+30*time.Second), device, port, "active", 5)

		// Flap 1
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-3*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-3*time.Minute+25*time.Second), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute+30*time.Second), device, port, "active", 5)

		// Flap 2
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-2*time.Minute+25*time.Second), device, port, "down", 5)
		insertSnapshotData(t, ctx, dbRW, store.historyTable, currentTime.Add(-1*time.Minute+30*time.Second), device, port, "active", 5)

		// Scan for flaps
		result, err := store.scanIBPortFlaps(device, port, currentTime.Add(-6*time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 1, "Should detect flap after persistent down")
	})

	t.Run("error_handling", func(t *testing.T) {
		// Test with non-existent device/port
		result, err := store.scanIBPortFlaps("non_existent_device", 999, currentTime)
		require.NoError(t, err) // Should not error, just return empty
		assert.Len(t, result, 0)
	})

	t.Run("database_error", func(t *testing.T) {
		// Create a store with closed database to force an error
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		cleanup() // Close the database immediately

		badStore := &ibPortsStore{
			rootCtx:                         ctx,
			historyTable:                    "test_error_table",
			dbRW:                            dbRW,
			dbRO:                            dbRO, // This is now closed
			ibPortFlapDownIntervalThreshold: 20 * time.Second,
			ibPortFlapBackToActiveThreshold: 2,
			getTimeNow:                      time.Now,
		}

		// This should return an error because the database is closed
		result, err := badStore.scanIBPortFlaps(device, port, currentTime)
		require.Error(t, err)
		assert.Nil(t, result)
	})
}
