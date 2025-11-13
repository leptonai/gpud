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

// TestDropStickyWindow tests that IB port drop events remain sticky for the configured window
// even after thresholds recover, preventing immediate Healthy flips.
func TestDropStickyWindow(t *testing.T) {
	mockTime := time.Now().UTC()
	mockStore := &mockIBPortsStoreForStickyDrop{
		events: []infinibandstore.Event{},
	}

	c := &component{
		ctx:              context.Background(),
		dropStickyWindow: 10 * time.Minute, // 10 minute sticky window
		ibPortsStore:     mockStore,
		nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
		getTimeNowFunc: func() time.Time {
			return mockTime
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return 8 healthy ports (meeting threshold)
			return createHealthyDevices(8, 400), nil
		},
	}

	// Test 1: No events, should be healthy
	cr := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "no infiniband port issue")

	// Test 2: Add a recent drop event (within sticky window)
	// Even though thresholds are met, should be unhealthy
	dropTime := mockTime.Add(-5 * time.Minute) // 5 minutes ago
	mockStore.events = []infinibandstore.Event{
		{
			Time:        dropTime,
			Port:        types.IBPort{Device: "mlx5_0", Port: uint(1)},
			EventType:   infinibandstore.EventTypeIbPortDrop,
			EventReason: "mlx5_0 port 1 down since " + dropTime.Format(time.RFC3339),
		},
	}

	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "device(s) down too long: mlx5_0")
	require.NotNil(t, cr.suggestedActions)
	assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
	require.Empty(t, cr.unhealthyIBPorts,
		"Drop-only unhealthy state should not mark thresholds as failing")

	// Test 3: Move time forward past sticky window (11 minutes total)
	// Drop event should no longer be considered recent
	mockTime = mockTime.Add(11 * time.Minute)
	c.getTimeNowFunc = func() time.Time {
		return mockTime
	}

	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "no infiniband port issue")

	// Test 4: Drop event with threshold failure
	// When thresholds fail, drop events for unhealthy ports should be included
	// Update the drop event to be for mlx5_7 which will be unhealthy
	mockStore.events = []infinibandstore.Event{
		{
			Time:        dropTime, // Still 5 minutes ago (now 16 minutes old after time advance)
			Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
			EventType:   infinibandstore.EventTypeIbPortDrop,
			EventReason: "mlx5_7 port 1 down since " + dropTime.Format(time.RFC3339),
		},
	}
	c.getClassDevicesFunc = func() (infinibandclass.Devices, error) {
		// Return 7 healthy ports (mlx5_0 to mlx5_6) and 1 down port (mlx5_7)
		devices := createHealthyDevices(7, 400) // mlx5_0 to mlx5_6
		// Add mlx5_7 as a down port
		devices = append(devices, infinibandclass.Device{
			Name: "mlx5_7",
			Ports: []infinibandclass.Port{
				{
					Port:      uint(1),
					State:     "DOWN",
					PhysState: "Polling",
					RateGBSec: 0,
					LinkLayer: "InfiniBand",
				},
			},
		})
		return devices, nil
	}

	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	// Should have both threshold violation and drop event in reason
	assert.Contains(t, cr.reason, "only 7 port(s) are active")
	assert.Contains(t, cr.reason, "device(s) down too long: mlx5_7")

	// Test 5: Flap events should always be processed (unchanged behavior)
	mockStore.events = []infinibandstore.Event{
		{
			Time:        mockTime.Add(-1 * time.Hour), // Old event
			Port:        types.IBPort{Device: "mlx5_1", Port: uint(1)},
			EventType:   infinibandstore.EventTypeIbPortFlap,
			EventReason: "mlx5_1 port 1 flapping",
		},
	}
	c.getClassDevicesFunc = func() (infinibandclass.Devices, error) {
		return createHealthyDevices(8, 400), nil // Thresholds met
	}

	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_1")
}

// TestDropStickyWindowEdgeCases tests edge cases for the sticky window implementation
func TestDropStickyWindowEdgeCases(t *testing.T) {
	mockTime := time.Now().UTC()
	mockStore := &mockIBPortsStoreForStickyDrop{
		events: []infinibandstore.Event{},
	}

	c := &component{
		ctx:              context.Background(),
		dropStickyWindow: 5 * time.Minute, // 5 minute sticky window
		ibPortsStore:     mockStore,
		nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
		getTimeNowFunc: func() time.Time {
			return mockTime
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Machine has 12 ports total, but only 8 are required
			// 4 dormant ports should not trigger alerts when healthy
			return createMixedDevices(8, 4), nil // 8 healthy, 4 dormant
		},
	}

	// Test: Dormant ports beyond threshold should not trigger drop alerts
	// when the drop event is old (outside sticky window)
	oldDropTime := mockTime.Add(-10 * time.Minute) // 10 minutes ago (outside 5 min window)
	mockStore.events = []infinibandstore.Event{
		{
			Time:        oldDropTime,
			Port:        types.IBPort{Device: "mlx5_8", Port: uint(1)}, // Dormant port
			EventType:   infinibandstore.EventTypeIbPortDrop,
			EventReason: "mlx5_8 port 1 down since " + oldDropTime.Format(time.RFC3339),
		},
		{
			Time:        oldDropTime,
			Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)}, // Another dormant port
			EventType:   infinibandstore.EventTypeIbPortDrop,
			EventReason: "mlx5_9 port 1 down since " + oldDropTime.Format(time.RFC3339),
		},
	}

	cr := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "no infiniband port issue")
	// Dormant ports should not cause issues when outside sticky window

	// Now make one of the required ports fail (threshold violation)
	c.getClassDevicesFunc = func() (infinibandclass.Devices, error) {
		return createMixedDevices(7, 5), nil // Only 7 healthy, 5 dormant
	}

	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	// Now the old drop events should be included because thresholds are violated
	assert.Contains(t, cr.reason, "device(s) down too long")
	assert.Contains(t, cr.reason, "mlx5_8")
	assert.Contains(t, cr.reason, "mlx5_9")
}

// TestDropStickyWindowDisabled verifies that setting the sticky window to zero
// restores the legacy behavior (no stickiness once thresholds recover).
func TestDropStickyWindowDisabled(t *testing.T) {
	mockTime := time.Now().UTC()
	mockStore := &mockIBPortsStoreForStickyDrop{
		events: []infinibandstore.Event{},
	}

	c := &component{
		ctx:              context.Background(),
		dropStickyWindow: 0,
		ibPortsStore:     mockStore,
		nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
		getTimeNowFunc: func() time.Time {
			return mockTime
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return createHealthyDevices(8, 400), nil
		},
	}

	// Inject a recent drop event. With sticky window disabled, the component
	// should remain healthy when thresholds are satisfied.
	mockStore.events = []infinibandstore.Event{
		{
			Time:        mockTime.Add(-5 * time.Minute),
			Port:        types.IBPort{Device: "mlx5_0", Port: uint(1)},
			EventType:   infinibandstore.EventTypeIbPortDrop,
			EventReason: "mlx5_0 port 1 down",
		},
	}

	cr := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"Sticky window disabled should not hold component unhealthy")
	assert.Contains(t, cr.reason, "no infiniband port issue")
}

// TestDropStickyWindowRecoveryLongOutage ensures long outages remain sticky after
// thresholds recover, even though the drop event itself is old.
func TestDropStickyWindowRecoveryLongOutage(t *testing.T) {
	baseTime := time.Now().UTC()
	currentTime := baseTime

	mockStore := &mockIBPortsStoreForStickyDrop{
		events: []infinibandstore.Event{},
	}

	portsHealthy := false
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
			if portsHealthy {
				return createHealthyDevices(8, 400), nil
			}
			return createMixedDevices(7, 1), nil
		},
	}

	// Initial unhealthy state with drop recorded over an hour ago.
	oldDrop := baseTime.Add(-2 * time.Hour)
	portsHealthy = false
	mockStore.events = []infinibandstore.Event{
		{
			Time:        oldDrop,
			Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
			EventType:   infinibandstore.EventTypeIbPortDrop,
			EventReason: "mlx5_7 port 1 down",
		},
	}

	cr := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)

	// Thresholds recover shortly after; even though the drop is older than the
	// sticky window, we should remain unhealthy during the recovery window.
	portsHealthy = true
	currentTime = baseTime.Add(30 * time.Second)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Long outage should remain sticky during recovery window")
	assert.Contains(t, cr.reason, "device(s) down too long: mlx5_7")

	// Once the recovery window passes, the component should return to healthy.
	currentTime = baseTime.Add(12 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"Long outage should clear after recovery sticky window expires")
	assert.Contains(t, cr.reason, "no infiniband port issue")
}

// Helper function to create healthy devices for testing
func createHealthyDevices(count int, rate int) infinibandclass.Devices {
	devices := make(infinibandclass.Devices, 0, count)
	for i := 0; i < count; i++ {
		devices = append(devices, infinibandclass.Device{
			Name: fmt.Sprintf("mlx5_%d", i),
			Ports: []infinibandclass.Port{
				{
					Port:      uint(1),
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: float64(rate),
					LinkLayer: "InfiniBand",
				},
			},
		})
	}
	return devices
}

// Helper function to create mixed healthy and dormant devices for testing
func createMixedDevices(healthyCount, dormantCount int) infinibandclass.Devices {
	devices := make(infinibandclass.Devices, 0, healthyCount+dormantCount)

	// Add healthy devices
	for i := 0; i < healthyCount; i++ {
		devices = append(devices, infinibandclass.Device{
			Name: fmt.Sprintf("mlx5_%d", i),
			Ports: []infinibandclass.Port{
				{
					Port:      uint(1),
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: 400,
					LinkLayer: "InfiniBand",
				},
			},
		})
	}

	// Add dormant devices
	for i := 0; i < dormantCount; i++ {
		devices = append(devices, infinibandclass.Device{
			Name: fmt.Sprintf("mlx5_%d", healthyCount+i),
			Ports: []infinibandclass.Port{
				{
					Port:      uint(1),
					State:     "DOWN",
					PhysState: "Polling",
					RateGBSec: 0,
					LinkLayer: "InfiniBand",
				},
			},
		})
	}

	return devices
}

// Mock implementation of the IBPortsStore for testing sticky drops
type mockIBPortsStoreForStickyDrop struct {
	events []infinibandstore.Event
}

func (m *mockIBPortsStoreForStickyDrop) Insert(time.Time, []types.IBPort) error {
	return nil
}

func (m *mockIBPortsStoreForStickyDrop) Scan() error {
	return nil
}

func (m *mockIBPortsStoreForStickyDrop) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	return m.events, nil
}

func (m *mockIBPortsStoreForStickyDrop) SetEventType(string, uint, time.Time, string, string) error {
	return nil
}

func (m *mockIBPortsStoreForStickyDrop) SetHealthy() error {
	return nil
}

func (m *mockIBPortsStoreForStickyDrop) Tombstone(timestamp time.Time) error {
	return nil
}

// Implement remaining required methods for the Store interface
func (m *mockIBPortsStoreForStickyDrop) GetRetentionPeriod() time.Duration {
	return 24 * time.Hour
}

func (m *mockIBPortsStoreForStickyDrop) GetCheckInterval() time.Duration {
	return 30 * time.Second
}

func (m *mockIBPortsStoreForStickyDrop) GetLastScan() (time.Time, error) {
	return time.Now(), nil
}

func (m *mockIBPortsStoreForStickyDrop) GetScanWindow() time.Duration {
	return 5 * time.Minute
}
