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

// TestComprehensiveStickyWindowScenarios tests all combinations of the three conditions:
// 1. thresholdsFailing
// 2. dropWithinStickyWindow (drop age < sticky window)
// 3. withinRecoveryStickyWindow (recovery recent AND drop not too old)
func TestComprehensiveStickyWindowScenarios(t *testing.T) {
	tests := []struct {
		name              string
		thresholdsFailing bool
		dropAge           time.Duration
		timeSinceRecovery *time.Duration // nil means no recovery tracked
		dropStickyWindow  time.Duration
		expectProcessed   bool
		expectHealthy     bool
		description       string
	}{
		// Scenario 1: Thresholds failing - always process
		{
			name:              "thresholds_failing_recent_drop",
			thresholdsFailing: true,
			dropAge:           5 * time.Minute,
			dropStickyWindow:  10 * time.Minute,
			expectProcessed:   true,
			expectHealthy:     false,
			description:       "When thresholds fail, all drops are processed regardless of age",
		},
		{
			name:              "thresholds_failing_old_drop",
			thresholdsFailing: true,
			dropAge:           2 * time.Hour,
			dropStickyWindow:  10 * time.Minute,
			expectProcessed:   true,
			expectHealthy:     false,
			description:       "Even old drops (dormant ports) are processed when thresholds fail",
		},

		// Scenario 2: Drop within sticky window (drop age < sticky window)
		{
			name:              "recent_drop_within_sticky_no_recovery",
			thresholdsFailing: false,
			dropAge:           5 * time.Minute,
			dropStickyWindow:  10 * time.Minute,
			expectProcessed:   true,
			expectHealthy:     false,
			description:       "Recent drops within sticky window are processed even without recovery",
		},
		{
			name:              "old_drop_outside_sticky_no_recovery",
			thresholdsFailing: false,
			dropAge:           15 * time.Minute,
			dropStickyWindow:  10 * time.Minute,
			expectProcessed:   false,
			expectHealthy:     true,
			description:       "Old drops outside sticky window are ignored when thresholds pass",
		},

		// Scenario 3: Recovery sticky window
		{
			name:              "recovery_recent_drop_recent",
			thresholdsFailing: false,
			dropAge:           30 * time.Minute,
			timeSinceRecovery: ptr(5 * time.Minute),
			dropStickyWindow:  10 * time.Minute,
			expectProcessed:   true,
			expectHealthy:     false,
			description:       "Within recovery window + drop not too old (< 1hr) = processed",
		},
		{
			name:              "recovery_recent_drop_old",
			thresholdsFailing: false,
			dropAge:           2 * time.Hour,
			timeSinceRecovery: ptr(5 * time.Minute),
			dropStickyWindow:  10 * time.Minute,
			expectProcessed:   true,
			expectHealthy:     false,
			description:       "Within recovery window and port recovered even after long outage = still sticky",
		},
		{
			name:              "recovery_expired_drop_recent",
			thresholdsFailing: false,
			dropAge:           30 * time.Minute,
			timeSinceRecovery: ptr(15 * time.Minute),
			dropStickyWindow:  10 * time.Minute,
			expectProcessed:   false,
			expectHealthy:     true,
			description:       "Recovery window expired = drop ignored",
		},

		// Edge cases
		{
			name:              "zero_sticky_window",
			thresholdsFailing: false,
			dropAge:           1 * time.Minute,
			dropStickyWindow:  0,
			expectProcessed:   false,
			expectHealthy:     true,
			description:       "Zero sticky window disables sticky behavior",
		},
		{
			name:              "negative_drop_age_protection",
			thresholdsFailing: false,
			dropAge:           -5 * time.Minute, // Future event (clock issue)
			dropStickyWindow:  10 * time.Minute,
			expectProcessed:   true, // Should be treated as dropAge=0
			expectHealthy:     false,
			description:       "Negative drop age (future events) are handled safely",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			now := time.Now().UTC()
			mockStore := &mockIBPortsStoreComprehensive{
				events: []infinibandstore.Event{
					{
						Time:        now.Add(-tt.dropAge),
						Port:        types.IBPort{Device: "mlx5_0", Port: uint(1)},
						EventType:   infinibandstore.EventTypeIbPortDrop,
						EventReason: fmt.Sprintf("mlx5_0 port 1 down (test: %s)", tt.name),
					},
				},
			}

			c := &component{
				ctx:              context.Background(),
				dropStickyWindow: tt.dropStickyWindow,
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
				getClassDevicesFunc: func() (infinibandclass.Devices, error) {
					if tt.thresholdsFailing {
						return createHealthyDevices(7, 400), nil // Below threshold
					}
					return createHealthyDevices(8, 400), nil // Meets threshold
				},
			}

			// Set recovery time if specified
			if tt.timeSinceRecovery != nil {
				recoveryTime := now.Add(-*tt.timeSinceRecovery)
				c.thresholdRecoveryTime = &recoveryTime
			}

			// Execute
			cr := c.Check().(*checkResult)

			// Assert
			if tt.expectHealthy {
				assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health, tt.description)
				if tt.expectProcessed {
					t.Errorf("Test logic error: can't be both healthy and have processed drops")
				}
			} else {
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health, tt.description)
				if tt.expectProcessed {
					assert.Contains(t, cr.reason, "mlx5_0", "Drop should be included in reason")
				}
			}
		})
	}
}

// TestStickyWindowTransitions tests state transitions through the full lifecycle
func TestStickyWindowTransitions(t *testing.T) {
	baseTime := time.Now().UTC()
	currentTime := baseTime

	mockStore := &mockIBPortsStoreComprehensive{
		events: []infinibandstore.Event{},
	}

	portsHealthy := true
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
			return createHealthyDevices(7, 400), nil
		},
	}

	// Phase 1: Initial healthy state
	t.Log("Phase 1: Initial healthy state")
	cr := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)

	// Phase 2: Port fails, drop event created
	t.Log("Phase 2: Port failure with drop event")
	portsHealthy = false
	mockStore.events = []infinibandstore.Event{
		{
			Time:        currentTime,
			Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
			EventType:   infinibandstore.EventTypeIbPortDrop,
			EventReason: "mlx5_7 port 1 down",
		},
	}
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "only 7 port(s)")
	assert.Contains(t, cr.reason, "mlx5_7")

	// Phase 3: Port stays down for 30 minutes
	t.Log("Phase 3: Port down for 30 minutes")
	currentTime = currentTime.Add(30 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.True(t, cr.thresholdsFailing, "Threshold failure should be recorded while port remains down")

	// Phase 4: Port recovers - enters sticky window
	t.Log("Phase 4: Port recovers - sticky window starts")
	portsHealthy = true
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Should remain unhealthy in sticky window")
	assert.Contains(t, cr.reason, "mlx5_7")

	// Verify recovery was tracked
	require.NotNil(t, c.thresholdRecoveryTime, "Recovery time should be tracked")

	// Phase 5: Within sticky window (5 minutes after recovery)
	t.Log("Phase 5: 5 minutes after recovery - still sticky")
	currentTime = currentTime.Add(5 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Should still be unhealthy within 10-minute window")

	// Phase 6: Sticky window expires (11 minutes after recovery)
	t.Log("Phase 6: Sticky window expires")
	currentTime = currentTime.Add(6 * time.Minute)
	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"Should be healthy after sticky window")

	// Phase 7: SetHealthy clears everything
	t.Log("Phase 7: SetHealthy called")
	err := c.SetHealthy()
	require.NoError(t, err)
	assert.Nil(t, c.thresholdRecoveryTime, "Recovery time should be cleared")
}

// TestDormantPortFiltering verifies that dormant ports are correctly filtered
// based on drop age during recovery windows
func TestDormantPortFiltering(t *testing.T) {
	baseTime := time.Now().UTC()

	// Create events with different ages
	mockStore := &mockIBPortsStoreComprehensive{
		events: []infinibandstore.Event{
			// Very old drop (dormant port - 3 days old)
			{
				Time:        baseTime.Add(-72 * time.Hour),
				Port:        types.IBPort{Device: "mlx5_8", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_8 port 1 down (dormant)",
			},
			// Old drop (dormant port - 2 hours old)
			{
				Time:        baseTime.Add(-2 * time.Hour),
				Port:        types.IBPort{Device: "mlx5_9", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_9 port 1 down (dormant)",
			},
			// Recent drop (active failure - 30 minutes old)
			{
				Time:        baseTime.Add(-30 * time.Minute),
				Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_7 port 1 down (recent)",
			},
		},
	}

	c := &component{
		ctx:              context.Background(),
		dropStickyWindow: 10 * time.Minute,
		ibPortsStore:     mockStore,
		nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
		getTimeNowFunc: func() time.Time {
			return baseTime
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// All ports healthy now
			return createHealthyDevices(8, 400), nil
		},
	}

	// Scenario 1: No recovery tracked, thresholds passing
	t.Log("Scenario 1: No recovery, thresholds passing")
	cr := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"Old drops should be ignored without recovery or threshold failure")

	// Scenario 2: Recovery just happened
	t.Log("Scenario 2: Just recovered")
	recoveryTime := baseTime
	c.thresholdRecoveryTime = &recoveryTime

	cr = c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"Recent drop should make it unhealthy")
	assert.Contains(t, cr.reason, "mlx5_7", "Recent drop should be included")
	assert.NotContains(t, cr.reason, "mlx5_8", "Very old drop should be filtered")
	assert.NotContains(t, cr.reason, "mlx5_9", "Old drop should be filtered")
}

// TestEdgeCasesAndErrorConditions tests boundary conditions and error scenarios
func TestEdgeCasesAndErrorConditions(t *testing.T) {
	t.Run("future_drop_events", func(t *testing.T) {
		// Test handling of drops with future timestamps (clock sync issues)
		futureTime := time.Now().UTC().Add(1 * time.Hour)
		mockStore := &mockIBPortsStoreComprehensive{
			events: []infinibandstore.Event{
				{
					Time:        futureTime,
					Port:        types.IBPort{Device: "mlx5_0", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "future drop event",
				},
			},
		}

		c := &component{
			ctx:              context.Background(),
			dropStickyWindow: 10 * time.Minute,
			ibPortsStore:     mockStore,
			nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
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

		cr := c.Check().(*checkResult)
		// Future events should be treated as recent (dropAge would be negative -> 0)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
			"Future events should be processed as recent drops")
	})

	t.Run("simultaneous_drops_and_flaps", func(t *testing.T) {
		// Test handling of both drop and flap events together
		now := time.Now().UTC()
		mockStore := &mockIBPortsStoreComprehensive{
			events: []infinibandstore.Event{
				{
					Time:        now.Add(-5 * time.Minute),
					Port:        types.IBPort{Device: "mlx5_0", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_0 drop",
				},
				{
					Time:        now.Add(-10 * time.Minute),
					Port:        types.IBPort{Device: "mlx5_1", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortFlap,
					EventReason: "mlx5_1 flap",
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
			getClassDevicesFunc: func() (infinibandclass.Devices, error) {
				return createHealthyDevices(8, 400), nil
			},
		}

		cr := c.Check().(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "mlx5_0", "Drop should be included")
		assert.Contains(t, cr.reason, "mlx5_1", "Flap should always be included")
	})
}

// Helper functions
func ptr(d time.Duration) *time.Duration {
	return &d
}

// Mock store for comprehensive tests
type mockIBPortsStoreComprehensive struct {
	events []infinibandstore.Event
}

func (m *mockIBPortsStoreComprehensive) Insert(time.Time, []types.IBPort) error {
	return nil
}

func (m *mockIBPortsStoreComprehensive) Scan() error {
	return nil
}

func (m *mockIBPortsStoreComprehensive) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	return m.events, nil
}

func (m *mockIBPortsStoreComprehensive) SetEventType(string, uint, time.Time, string, string) error {
	return nil
}

func (m *mockIBPortsStoreComprehensive) SetHealthy() error {
	m.events = []infinibandstore.Event{}
	return nil
}

func (m *mockIBPortsStoreComprehensive) Tombstone(timestamp time.Time) error {
	return nil
}
