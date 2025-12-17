package infiniband

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	infinibandstore "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/store"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
)

// TestProductionScenarios tests the exact three node scenarios from a.md
// that motivated the sticky window implementation.
func TestProductionScenarios(t *testing.T) {
	// Parse the actual timestamps from production logs
	parseTime := func(s string) time.Time {
		t, _ := time.Parse(time.RFC3339, s)
		return t
	}

	t.Run("Node1_fargate-ip-10-0-81-136_flap", func(t *testing.T) {
		// This node had multiple flaps and correctly stayed Unhealthy (expected behavior)
		// Timeline:
		// - Multiple flap events between 2025-10-14 05:47:02Z and 2025-10-16 08:02:32Z
		// - Component correctly marked Unhealthy with "device(s) flapping between ACTIVE<>DOWN: mlx5_9"
		// - Requires SetHealthy to clear (EXPECTED AND WORKING)

		checkTime := parseTime("2025-10-16T12:06:16Z")
		mockStore := &mockIBPortsStoreProduction{
			events: []infinibandstore.Event{
				{
					Time:        parseTime("2025-10-15T23:47:32Z"),
					Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortFlap,
					EventReason: "mlx5_9 port 1 down since 2025-10-15T23:32:02Z (and flapped back to active)",
				},
			},
		}

		c := createTestComponent(checkTime, mockStore, 10*time.Minute, true)
		cr := c.Check().(*checkResult)

		// Flaps should always be sticky until SetHealthy
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
			"Node 1: Flaps correctly remain sticky")
		assert.Contains(t, cr.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_9")
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
	})

	t.Run("Node2_fargate-ip-10-0-83-166_flap", func(t *testing.T) {
		// Similar to Node1, had flaps and correctly stayed Unhealthy
		checkTime := parseTime("2025-10-16T12:28:28Z")
		mockStore := &mockIBPortsStoreProduction{
			events: []infinibandstore.Event{
				{
					Time:        parseTime("2025-10-15T23:47:31Z"),
					Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortFlap,
					EventReason: "mlx5_9 port 1 down since 2025-10-15T23:32:01Z (and flapped back to active)",
				},
			},
		}

		c := createTestComponent(checkTime, mockStore, 10*time.Minute, true)
		cr := c.Check().(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
			"Node 2: Flaps correctly remain sticky")
		assert.Contains(t, cr.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_9")
	})

	t.Run("Node3_fargate-ip-10-0-83-4_drop_problematic_recovery", func(t *testing.T) {
		// THIS IS THE KEY PROBLEMATIC CASE THAT THE STICKY WINDOW FIXES!
		// Timeline:
		// - 08:47:29Z: Control-plane recall with "only 7 port(s) are active", "device(s) down too long: mlx5_9"
		// - 08:47:29Z: Component marked Unhealthy with HARDWARE_INSPECTION
		// - Sometime before 12:51:18Z: Port recovered, thresholds now pass
		// - 12:51:18Z: Component is Healthy "ok; no infiniband port issue"
		//
		// PROBLEM: HARDWARE_INSPECTION was suggested but then immediately cleared
		// SOLUTION: With sticky window, it would stay Unhealthy for 10 minutes after recovery

		// Simulate the timeline
		recallTime := parseTime("2025-10-16T08:47:29Z")
		// checkTimeHealthy := parseTime("2025-10-16T12:51:18Z") // Much later when it was healthy (not used in test)

		// Test 1: At recall time - should be Unhealthy (thresholds failing)
		t.Run("at_recall_time", func(t *testing.T) {
			mockStore := &mockIBPortsStoreProduction{
				events: []infinibandstore.Event{
					{
						Time:        recallTime.Add(-4 * time.Minute), // Drop detected 4 min before recall
						Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)},
						EventType:   infinibandstore.EventTypeIbPortDrop,
						EventReason: "mlx5_9 port 1 down",
					},
				},
			}

			c := createTestComponent(recallTime, mockStore, 10*time.Minute, false) // Ports down
			cr := c.Check().(*checkResult)

			assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
			assert.Contains(t, cr.reason, "only 7 port(s) are active")
			assert.Contains(t, cr.reason, "device(s) down too long: mlx5_9")
			assert.NotNil(t, cr.suggestedActions)
			assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
		})

		// Test 2: Right after recovery WITHOUT sticky window (OLD BEHAVIOR - PROBLEMATIC)
		t.Run("after_recovery_without_sticky", func(t *testing.T) {
			recoveryTime := recallTime.Add(5 * time.Minute)
			mockStore := &mockIBPortsStoreProduction{
				events: []infinibandstore.Event{
					{
						Time:        recallTime.Add(-4 * time.Minute),
						Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)},
						EventType:   infinibandstore.EventTypeIbPortDrop,
						EventReason: "mlx5_9 port 1 down",
					},
				},
			}

			// Simulate OLD behavior with no sticky window
			c := createTestComponent(recoveryTime, mockStore, 0, true) // Sticky window disabled
			cr := c.Check().(*checkResult)

			// OLD BEHAVIOR: Immediately healthy (confusing!)
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
				"WITHOUT sticky window: Immediately flips to Healthy (problematic)")
		})

		// Test 3: Right after recovery WITH sticky window (NEW BEHAVIOR - FIXED)
		t.Run("after_recovery_with_sticky", func(t *testing.T) {
			recoveryTime := recallTime.Add(5 * time.Minute)
			mockStore := &mockIBPortsStoreProduction{
				events: []infinibandstore.Event{
					{
						Time:        recallTime.Add(-4 * time.Minute),
						Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)},
						EventType:   infinibandstore.EventTypeIbPortDrop,
						EventReason: "mlx5_9 port 1 down",
					},
				},
			}

			c := createTestComponent(recoveryTime, mockStore, 10*time.Minute, true) // Sticky window enabled

			// Simulate that thresholds just recovered
			recoveryTimeAdjusted := recoveryTime.Add(-30 * time.Second)
			c.thresholdRecoveryTime = &recoveryTimeAdjusted

			cr := c.Check().(*checkResult)

			// NEW BEHAVIOR: Stays unhealthy during sticky window
			assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
				"WITH sticky window: Remains Unhealthy for stabilization (fixed!)")
			assert.Contains(t, cr.reason, "device(s) down too long: mlx5_9",
				"Drop event should still be included during sticky window")
		})

		// Test 4: After sticky window expires - should be healthy
		t.Run("after_sticky_window_expires", func(t *testing.T) {
			afterStickyTime := recallTime.Add(15 * time.Minute) // Well past 10 min sticky window
			mockStore := &mockIBPortsStoreProduction{
				events: []infinibandstore.Event{
					{
						Time:        recallTime.Add(-4 * time.Minute),
						Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)},
						EventType:   infinibandstore.EventTypeIbPortDrop,
						EventReason: "mlx5_9 port 1 down",
					},
				},
			}

			c := createTestComponent(afterStickyTime, mockStore, 10*time.Minute, true)
			cr := c.Check().(*checkResult)

			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
				"After sticky window expires: Correctly becomes Healthy")
			assert.Contains(t, cr.reason, "no infiniband port issue")
		})
	})
}

// TestDormantPortHandling verifies that the sticky window doesn't break
// dormant port handling (ports beyond required thresholds).
func TestDormantPortHandling(t *testing.T) {
	now := time.Now().UTC()

	t.Run("dormant_ports_not_flagged_after_sticky_window", func(t *testing.T) {
		// Machine has 12 ports total, but only 8 are required
		// Ports 9-12 are dormant (always down)
		// These should NOT cause issues after sticky window expires

		oldDropTime := now.Add(-2 * time.Hour) // Well beyond sticky window
		mockStore := &mockIBPortsStoreProduction{
			events: []infinibandstore.Event{
				// Dormant port drops (ports beyond threshold)
				{
					Time:        oldDropTime,
					Port:        types.IBPort{Device: "mlx5_8", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_8 port 1 down (dormant port)",
				},
				{
					Time:        oldDropTime,
					Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_9 port 1 down (dormant port)",
				},
				{
					Time:        oldDropTime,
					Port:        types.IBPort{Device: "mlx5_10", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_10 port 1 down (dormant port)",
				},
				{
					Time:        oldDropTime,
					Port:        types.IBPort{Device: "mlx5_11", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_11 port 1 down (dormant port)",
				},
			},
		}

		c := &component{
			ctx:              context.Background(),
			dropStickyWindow: 10 * time.Minute,
			ibPortsStore:     mockStore,
			nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
			getTimeNowFunc: func() time.Time {
				return now
			},
			getThresholdsFunc: func() types.ExpectedPortStates {
				return types.ExpectedPortStates{
					AtLeastPorts: 8, // Only 8 required
					AtLeastRate:  400,
				}
			},
			getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
				// 8 healthy ports (meeting threshold) + 4 dormant ports
				return createMixedDevices(8, 4), nil
			},
		}

		cr := c.Check().(*checkResult)

		// CRITICAL: Dormant ports should NOT cause issues when:
		// 1. Thresholds are met (8 ports active)
		// 2. Drop events are old (beyond sticky window)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
			"Dormant ports should not cause false alerts")
		assert.Contains(t, cr.reason, "no infiniband port issue")
	})

	t.Run("dormant_ports_ARE_flagged_when_thresholds_fail", func(t *testing.T) {
		// Same scenario but now only 7 active ports (threshold failure)
		// Dormant port drops SHOULD be included in this case

		oldDropTime := now.Add(-2 * time.Hour)
		mockStore := &mockIBPortsStoreProduction{
			events: []infinibandstore.Event{
				{
					Time:        oldDropTime,
					Port:        types.IBPort{Device: "mlx5_8", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_8 port 1 down",
				},
			},
		}

		c := &component{
			ctx:              context.Background(),
			dropStickyWindow: 10 * time.Minute,
			ibPortsStore:     mockStore,
			nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
			getTimeNowFunc: func() time.Time {
				return now
			},
			getThresholdsFunc: func() types.ExpectedPortStates {
				return types.ExpectedPortStates{
					AtLeastPorts: 8,
					AtLeastRate:  400,
				}
			},
			getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
				// Only 7 healthy ports (threshold failure)
				return createMixedDevices(7, 5), nil
			},
		}

		cr := c.Check().(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
			"When thresholds fail, all drops are relevant")
		assert.Contains(t, cr.reason, "only 7 port(s) are active")
		assert.Contains(t, cr.reason, "device(s) down too long: mlx5_8")
	})
}

// Helper to create test component
func createTestComponent(checkTime time.Time, store infinibandstore.Store, stickyWindow time.Duration, portsHealthy bool) *component {
	return &component{
		ctx:              context.Background(),
		dropStickyWindow: stickyWindow,
		ibPortsStore:     store,
		nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
		getTimeNowFunc: func() time.Time {
			return checkTime
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
			if portsHealthy {
				return createHealthyDevices(8, 400), nil
			}
			return createMixedDevices(7, 1), nil // 1 port down
		},
	}
}

// Mock store for production scenarios
type mockIBPortsStoreProduction struct {
	events []infinibandstore.Event
}

func (m *mockIBPortsStoreProduction) Insert(time.Time, []types.IBPort) error {
	return nil
}

func (m *mockIBPortsStoreProduction) Scan() error {
	return nil
}

func (m *mockIBPortsStoreProduction) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	return m.events, nil
}

func (m *mockIBPortsStoreProduction) SetEventType(string, uint, time.Time, string, string) error {
	return nil
}

func (m *mockIBPortsStoreProduction) SetHealthy() error {
	return nil
}

func (m *mockIBPortsStoreProduction) Tombstone(timestamp time.Time) error {
	return nil
}

func (m *mockIBPortsStoreProduction) GetRetentionPeriod() time.Duration {
	return 24 * time.Hour
}

func (m *mockIBPortsStoreProduction) GetCheckInterval() time.Duration {
	return 30 * time.Second
}

func (m *mockIBPortsStoreProduction) GetLastScan() (time.Time, error) {
	return time.Now(), nil
}

func (m *mockIBPortsStoreProduction) GetScanWindow() time.Duration {
	return 5 * time.Minute
}
