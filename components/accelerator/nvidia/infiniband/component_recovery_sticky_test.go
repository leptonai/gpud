package infiniband

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	infinibandstore "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/store"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
)

// TestRecoveryStickyWindow tests the ACTUAL scenario from a.md where a port
// has been down for a long time, recovers, and should remain unhealthy for
// the sticky window period AFTER recovery.
func TestRecoveryStickyWindow(t *testing.T) {
	// This test simulates the exact timeline from a.md:
	// 08:00:00 - Port goes DOWN
	// 08:04:00 - Drop event created (after 4 min threshold)
	// 08:47:00 - Port recovers to ACTIVE
	// 08:47:30 - Next check runs
	// Expected: Should be unhealthy until 08:57:00 (recovery + 10 min)

	baseTime := time.Now().UTC()
	portDownTime := baseTime
	dropEventTime := baseTime.Add(4 * time.Minute)     // Drop detected after 4 minutes
	portRecoveryTime := baseTime.Add(47 * time.Minute) // Port recovers after 47 minutes
	firstCheckAfterRecovery := portRecoveryTime.Add(30 * time.Second)

	mockStore := &mockIBPortsStoreForRecovery{
		events: []infinibandstore.Event{},
	}

	currentTime := portDownTime
	c := &component{
		ctx:              context.Background(),
		dropStickyWindow: 10 * time.Minute,
		ibPortsStore:     mockStore,
		nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
		getTimeNowFunc: func() time.Time {
			return currentTime
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Initially, port mlx5_7 is down
			if currentTime.Before(portRecoveryTime) {
				return createMixedDevices(7, 1), nil // 7 healthy, 1 down (mlx5_7)
			}
			// After recovery, all 8 ports are healthy
			return createHealthyDevices(8, 400), nil
		},
	}

	// Step 1: Initial state with port down (simulating 08:00:00)
	t.Log("Step 1: Port goes down at 08:00:00")
	cr := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "only 7 port(s) are active")

	// Step 2: Drop event created after 4 minutes (simulating 08:04:00)
	t.Log("Step 2: Drop event created at 08:04:00")
	currentTime = dropEventTime
	mockStore.events = []infinibandstore.Event{
		{
			Time:        dropEventTime,
			Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
			EventType:   infinibandstore.EventTypeIbPortDrop,
			EventReason: fmt.Sprintf("mlx5_7 port 1 down since %s", portDownTime.Format(time.RFC3339)),
		},
	}
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "device(s) down too long: mlx5_7")

	// Step 3: Port recovers (simulating 08:47:00)
	t.Log("Step 3: Port recovers at 08:47:00")
	currentTime = portRecoveryTime

	// Step 4: First check after recovery (simulating 08:47:30)
	t.Log("Step 4: First check at 08:47:30 - should STILL be unhealthy")
	currentTime = firstCheckAfterRecovery
	cr = c.Check().(*checkResult)

	// CRITICAL TEST: Even though thresholds are now met (all 8 ports healthy),
	// the component should STILL be unhealthy because we're within the sticky window
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Component should remain unhealthy within sticky window after recovery")
	assert.Contains(t, cr.reason, "device(s) down too long: mlx5_7",
		"Drop event should still be included in reason during sticky window")

	// Verify recovery time was tracked
	c.thresholdRecoveryTimeMu.RLock()
	assert.NotNil(t, c.thresholdRecoveryTime, "Recovery time should be tracked")
	c.thresholdRecoveryTimeMu.RUnlock()

	// Step 5: Check 5 minutes after recovery - still within sticky window
	t.Log("Step 5: Check at 08:52:00 (5 min after recovery) - should STILL be unhealthy")
	currentTime = portRecoveryTime.Add(5 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Should still be unhealthy 5 minutes after recovery")

	// Step 6: Check 11 minutes after recovery - outside sticky window
	t.Log("Step 6: Check at 08:58:00 (11 min after recovery) - should be healthy")
	currentTime = portRecoveryTime.Add(11 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"Should be healthy after sticky window expires")
	assert.Contains(t, cr.reason, "no infiniband port issue")
}

// TestMultipleRecoveries tests multiple recovery cycles to ensure
// sticky window resets properly.
func TestMultipleRecoveries(t *testing.T) {
	baseTime := time.Now().UTC()
	currentTime := baseTime

	mockStore := &mockIBPortsStoreForRecovery{
		events: []infinibandstore.Event{},
	}

	portsHealthy := true
	c := &component{
		ctx:              context.Background(),
		dropStickyWindow: 5 * time.Minute, // Shorter window for faster test
		ibPortsStore:     mockStore,
		nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
		getTimeNowFunc: func() time.Time {
			return currentTime
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			if portsHealthy {
				return createHealthyDevices(8, 400), nil
			}
			return createMixedDevices(7, 1), nil // 1 port down
		},
	}

	// Cycle 1: Port failure and recovery
	t.Log("Cycle 1: Initial port failure")
	portsHealthy = false
	mockStore.events = []infinibandstore.Event{
		{
			Time:        currentTime,
			Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
			EventType:   infinibandstore.EventTypeIbPortDrop,
			EventReason: "mlx5_7 port 1 down",
		},
	}
	cr := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)

	t.Log("Cycle 1: Port recovers")
	portsHealthy = true
	currentTime = currentTime.Add(30 * time.Second)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Should be unhealthy immediately after recovery")

	t.Log("Cycle 1: Sticky window expires")
	currentTime = currentTime.Add(6 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"Should be healthy after sticky window")

	// Cycle 2: Another failure and recovery
	t.Log("Cycle 2: Another port failure")
	portsHealthy = false
	currentTime = currentTime.Add(10 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)

	t.Log("Cycle 2: Port recovers again")
	portsHealthy = true
	currentTime = currentTime.Add(1 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Should be unhealthy in new sticky window")

	// Verify recovery time was updated
	c.thresholdRecoveryTimeMu.RLock()
	assert.NotNil(t, c.thresholdRecoveryTime, "Recovery time should be tracked")
	c.thresholdRecoveryTimeMu.RUnlock()
}

// TestDormantPortsWithRecovery ensures dormant ports don't interfere
// with the recovery sticky window logic.
func TestDormantPortsWithRecovery(t *testing.T) {
	baseTime := time.Now().UTC()
	currentTime := baseTime

	// Machine has 12 ports, but only 8 are required
	// Ports mlx5_8 through mlx5_11 are dormant (always down)
	mockStore := &mockIBPortsStoreForRecovery{
		events: []infinibandstore.Event{
			// Old drop events for dormant ports
			{
				Time:        baseTime.Add(-1 * time.Hour),
				Port:        types.IBPort{Device: "mlx5_8", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_8 port 1 down (dormant)",
			},
			{
				Time:        baseTime.Add(-1 * time.Hour),
				Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_9 port 1 down (dormant)",
			},
		},
	}

	activePortHealthy := true
	c := &component{
		ctx:              context.Background(),
		dropStickyWindow: 10 * time.Minute,
		ibPortsStore:     mockStore,
		nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
		getTimeNowFunc: func() time.Time {
			return currentTime
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			if activePortHealthy {
				// 8 active healthy ports + 4 dormant ports
				return createMixedDevices(8, 4), nil
			}
			// 7 active healthy ports + 1 active down + 4 dormant ports
			devices := createMixedDevices(7, 5)
			// Mark mlx5_7 as the active port that failed (not dormant)
			for i := range devices {
				if devices[i].Name == "mlx5_7" {
					devices[i].Ports[0].State = "DOWN"
					devices[i].Ports[0].PhysState = "Polling"
					devices[i].Ports[0].RateGBSec = 0
				}
			}
			return devices, nil
		},
	}

	// Initial state: dormant ports exist but thresholds are met
	t.Log("Initial: Dormant ports exist but shouldn't cause issues")
	cr := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"Dormant ports beyond threshold should not cause unhealthy state")

	// Active port failure
	t.Log("Active port mlx5_7 fails")
	activePortHealthy = false
	// Add drop event for the active port
	mockStore.events = append(mockStore.events, infinibandstore.Event{
		Time:        currentTime,
		Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
		EventType:   infinibandstore.EventTypeIbPortDrop,
		EventReason: "mlx5_7 port 1 down (active port)",
	})

	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	// Should include all drop events when thresholds are failing
	assert.Contains(t, cr.reason, "mlx5_7")
	assert.Contains(t, cr.reason, "mlx5_8") // Dormant ports also included when thresholds fail
	assert.Contains(t, cr.reason, "mlx5_9")

	// Active port recovers
	t.Log("Active port recovers - within sticky window")
	activePortHealthy = true
	currentTime = currentTime.Add(30 * time.Second)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Should remain unhealthy within sticky window")
	// Only the recent active port drop should be included
	assert.Contains(t, cr.reason, "mlx5_7")
	// Dormant ports should NOT be included when thresholds pass
	assert.NotContains(t, cr.reason, "mlx5_8",
		"Dormant ports should not be included when thresholds pass")

	// After sticky window expires
	t.Log("Sticky window expires")
	currentTime = currentTime.Add(11 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"Should be healthy after sticky window, ignoring dormant ports")
}

// Mock implementation for recovery tests
type mockIBPortsStoreForRecovery struct {
	events []infinibandstore.Event
}

func (m *mockIBPortsStoreForRecovery) Insert(time.Time, []types.IBPort) error {
	return nil
}

func (m *mockIBPortsStoreForRecovery) Scan() error {
	return nil
}

func (m *mockIBPortsStoreForRecovery) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	return m.events, nil
}

func (m *mockIBPortsStoreForRecovery) SetEventType(string, uint, time.Time, string, string) error {
	return nil
}

func (m *mockIBPortsStoreForRecovery) SetHealthy() error {
	m.events = []infinibandstore.Event{} // Clear events
	return nil
}

func (m *mockIBPortsStoreForRecovery) Tombstone(timestamp time.Time) error {
	return nil
}
