package infiniband

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	infinibandstore "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/store"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
)

// TestDebugThresholdFailingOldDrop specifically tests why old drops aren't included
// when thresholds are failing
func TestDebugThresholdFailingOldDrop(t *testing.T) {
	now := time.Now().UTC()

	// Create a 2-hour old drop event
	mockStore := &mockIBPortsStoreDebug{
		events: []infinibandstore.Event{
			{
				Time:        now.Add(-2 * time.Hour),
				Port:        types.IBPort{Device: "mlx5_0", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_0 port 1 down (old drop)",
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
			// Return 7 healthy ports (below threshold)
			return createHealthyDevices(7, 400), nil
		},
	}

	// Check if ibPortsStore has events
	events, err := mockStore.LastEvents(time.Time{})
	require.NoError(t, err)
	require.Len(t, events, 1, "Store should have 1 event")
	t.Logf("Event in store: %+v", events[0])

	// Execute the check
	cr := c.Check().(*checkResult)

	// Debug output
	t.Logf("Health: %s", cr.health)
	t.Logf("Reason: %s", cr.reason)
	t.Logf("Unhealthy IB Ports: %d", len(cr.unhealthyIBPorts))

	// Check if events were retrieved
	eventsRetrieved, _ := c.ibPortsStore.LastEvents(time.Time{})
	t.Logf("Events retrieved from store: %d", len(eventsRetrieved))

	// Assertions
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Should be unhealthy when thresholds are failing")

	// The reason should contain both threshold failure AND drop device
	assert.Contains(t, cr.reason, "only 7 port(s)",
		"Should mention threshold failure")
	assert.Contains(t, cr.reason, "mlx5_0",
		"Should include the drop device even though it's old because thresholds are failing")
}

// TestThreeConditionsIndependently tests each of the three conditions separately
func TestThreeConditionsIndependently(t *testing.T) {
	now := time.Now().UTC()

	testCases := []struct {
		name            string
		setupThresholds func() (int, int) // returns (healthy ports, required ports)
		dropAge         time.Duration
		recoveryTime    *time.Time
		expectProcessed bool
		description     string
	}{
		{
			name: "condition1_thresholds_failing",
			setupThresholds: func() (int, int) {
				return 7, 8 // 7 healthy, need 8 = failing
			},
			dropAge:         3 * time.Hour, // Very old drop
			recoveryTime:    nil,
			expectProcessed: true,
			description:     "Condition 1: thresholdsFailing = true should process ANY drop",
		},
		{
			name: "condition2_drop_within_sticky",
			setupThresholds: func() (int, int) {
				return 8, 8 // Meets threshold
			},
			dropAge:         5 * time.Minute, // Within 10 min sticky window
			recoveryTime:    nil,
			expectProcessed: true,
			description:     "Condition 2: dropWithinStickyWindow = true should process",
		},
		{
			name: "condition3_recovery_sticky_recent_drop",
			setupThresholds: func() (int, int) {
				return 8, 8 // Meets threshold
			},
			dropAge:         30 * time.Minute,
			recoveryTime:    ptrTime(now.Add(-5 * time.Minute)), // Recovered 5 min ago
			expectProcessed: true,
			description:     "Condition 3: withinRecoveryStickyWindow processes recent drops",
		},
		{
			name: "condition3_recovery_sticky_old_drop",
			setupThresholds: func() (int, int) {
				return 8, 8 // Meets threshold
			},
			dropAge:         2 * time.Hour,
			recoveryTime:    ptrTime(now.Add(-5 * time.Minute)), // Recovered 5 min ago
			expectProcessed: true,
			description:     "Condition 3: long outages remain sticky during recovery window",
		},
		{
			name: "none_of_conditions_met",
			setupThresholds: func() (int, int) {
				return 8, 8 // Meets threshold
			},
			dropAge:         2 * time.Hour, // Old drop
			recoveryTime:    nil,
			expectProcessed: false,
			description:     "No conditions met: should NOT process",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			healthyPorts, requiredPorts := tc.setupThresholds()

			mockStore := &mockIBPortsStoreDebug{
				events: []infinibandstore.Event{
					{
						Time:        now.Add(-tc.dropAge),
						Port:        types.IBPort{Device: "mlx5_test", Port: uint(1)},
						EventType:   infinibandstore.EventTypeIbPortDrop,
						EventReason: "test drop event",
					},
				},
			}

			c := &component{
				ctx:                   context.Background(),
				dropStickyWindow:      10 * time.Minute,
				ibPortsStore:          mockStore,
				nvmlInstance:          &mockNVMLInstance{exists: true, productName: "Test GPU"},
				thresholdRecoveryTime: tc.recoveryTime,
				getTimeNowFunc: func() time.Time {
					return now
				},
				getThresholdsFunc: func() types.ExpectedPortStates {
					return types.ExpectedPortStates{
						AtLeastPorts: requiredPorts,
						AtLeastRate:  400,
					}
				},
				getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
					devices := createHealthyDevices(healthyPorts, 400)
					if healthyPorts > 0 {
						devices[healthyPorts-1].Name = "mlx5_test"
					}
					return devices, nil
				},
			}

			cr := c.Check().(*checkResult)

			t.Logf("%s", tc.description)
			t.Logf("  Health: %s, Reason: %s", cr.health, cr.reason)

			if tc.expectProcessed {
				assert.Contains(t, cr.reason, "mlx5_test",
					"%s - drop should be in reason", tc.description)
			} else {
				assert.NotContains(t, cr.reason, "mlx5_test",
					"%s - drop should NOT be in reason", tc.description)
			}
		})
	}
}

// Helper function to get pointer to time.Time
func ptrTime(t time.Time) *time.Time {
	return &t
}

// Mock store for debug tests
type mockIBPortsStoreDebug struct {
	events []infinibandstore.Event
}

func (m *mockIBPortsStoreDebug) Insert(time.Time, []types.IBPort) error {
	println("Insert called")
	return nil
}

func (m *mockIBPortsStoreDebug) Scan() error {
	println("Scan called")
	return nil
}

func (m *mockIBPortsStoreDebug) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	// Debug log
	if len(m.events) > 0 {
		println("LastEvents called, returning", len(m.events), "events")
	}
	return m.events, nil
}

func (m *mockIBPortsStoreDebug) SetEventType(string, uint, time.Time, string, string) error {
	return nil
}

func (m *mockIBPortsStoreDebug) SetHealthy() error {
	m.events = []infinibandstore.Event{}
	return nil
}

func (m *mockIBPortsStoreDebug) Tombstone(timestamp time.Time) error {
	return nil
}
