package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
)

func TestGetStatesFromComponentWithDeps(t *testing.T) {
	tests := []struct {
		name                string
		componentName       string
		lastRebootTime      time.Time
		mockHealthStates    apiv1.HealthStates
		componentExists     bool
		expectedHealthTypes []apiv1.HealthStateType
		expectInitializing  bool
	}{
		{
			name:                "component_not_found",
			componentName:       "nonexistent",
			lastRebootTime:      time.Now(),
			componentExists:     false,
			expectedHealthTypes: nil,
		},
		{
			name:            "healthy_component_no_grace_period",
			componentName:   "healthy-comp",
			lastRebootTime:  time.Now().Add(-10 * time.Minute), // Reboot was 10 minutes ago
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeHealthy, Reason: "All good"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeHealthy,
			},
		},
		{
			name:            "unhealthy_within_grace_period",
			componentName:   "unhealthy-comp",
			lastRebootTime:  time.Now().Add(-2 * time.Minute), // Within 5 min grace
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeInitializing,
			},
			expectInitializing: true,
		},
		{
			name:            "unhealthy_after_grace_period",
			componentName:   "unhealthy-comp",
			lastRebootTime:  time.Now().Add(-10 * time.Minute), // Beyond 5 min grace
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeUnhealthy,
			},
		},
		{
			name:            "zero_reboot_time",
			componentName:   "comp-zero-reboot",
			lastRebootTime:  time.Time{}, // Zero time
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeUnhealthy, // Should remain unhealthy when reboot time is zero
			},
		},
		{
			name:            "multiple_health_states",
			componentName:   "multi-state-comp",
			lastRebootTime:  time.Now().Add(-2 * time.Minute), // Within grace period
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeHealthy, Reason: "First check OK"},
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Second check failed"},
				{Health: apiv1.HealthStateTypeDegraded, Reason: "Third check degraded"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeHealthy,
				apiv1.HealthStateTypeInitializing,
				apiv1.HealthStateTypeInitializing,
			},
		},
		{
			name:            "degraded_within_grace_period",
			componentName:   "degraded-comp",
			lastRebootTime:  time.Now().Add(-2 * time.Minute), // Within grace period
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeDegraded, Reason: "Component degraded"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeInitializing,
			},
			expectInitializing: true,
		},
		{
			name:            "degraded_after_grace_period",
			componentName:   "degraded-comp",
			lastRebootTime:  time.Now().Add(-10 * time.Minute), // After grace period
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeDegraded, Reason: "Component degraded"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeDegraded,
			},
		},
		{
			name:            "exactly_at_grace_period_boundary",
			componentName:   "boundary-comp",
			lastRebootTime:  time.Now().Add(-5 * time.Minute), // Exactly at 5 minutes
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeUnhealthy, // Should NOT be initializing at boundary
			},
		},
		{
			name:            "future_reboot_time_protection",
			componentName:   "future-reboot-comp",
			lastRebootTime:  time.Now().Add(1 * time.Hour), // Future time (should be protected)
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeUnhealthy, // Should NOT be initializing with future time
			},
		},
		{
			name:            "unix_epoch_protection",
			componentName:   "epoch-comp",
			lastRebootTime:  time.Unix(1, 0), // Near Unix epoch (1970)
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeUnhealthy, // Should NOT be initializing with ancient time
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock component function
			getComponentFunc := func(name string) components.Component {
				if !tt.componentExists {
					return nil
				}
				comp := &mockComponent{}
				comp.On("LastHealthStates").Return(tt.mockHealthStates)
				return comp
			}

			// Call the function under test
			result := getHealthStatesFromComponentWithDeps(
				tt.componentName,
				tt.lastRebootTime,
				getComponentFunc,
			)

			// Verify component name
			assert.Equal(t, tt.componentName, result.Component)

			// Verify health states
			if tt.expectedHealthTypes == nil {
				assert.Nil(t, result.States)
			} else {
				assert.Equal(t, len(tt.expectedHealthTypes), len(result.States))
				for i, expectedType := range tt.expectedHealthTypes {
					assert.Equal(t, expectedType, result.States[i].Health,
						"State %d: expected %v, got %v", i, expectedType, result.States[i].Health)
				}
			}
		})
	}
}

// TestRebootTimeScenarios tests various reboot time scenarios
func TestRebootTimeScenarios(t *testing.T) {
	tests := []struct {
		name               string
		rebootMinutesAgo   int
		expectInitializing bool
		description        string
	}{
		{
			name:               "very_recent_reboot",
			rebootMinutesAgo:   1,
			expectInitializing: true,
			description:        "1 minute after reboot should be initializing",
		},
		{
			name:               "middle_of_grace_period",
			rebootMinutesAgo:   3,
			expectInitializing: true,
			description:        "3 minutes after reboot should be initializing",
		},
		{
			name:               "just_before_grace_expires",
			rebootMinutesAgo:   4,
			expectInitializing: true,
			description:        "4 minutes after reboot should still be initializing",
		},
		{
			name:               "exactly_at_grace_period",
			rebootMinutesAgo:   5,
			expectInitializing: false,
			description:        "Exactly 5 minutes should NOT be initializing",
		},
		{
			name:               "after_grace_period",
			rebootMinutesAgo:   10,
			expectInitializing: false,
			description:        "10 minutes after reboot should NOT be initializing",
		},
		{
			name:               "long_after_reboot",
			rebootMinutesAgo:   60,
			expectInitializing: false,
			description:        "1 hour after reboot should NOT be initializing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentName := "test-component"
			lastRebootTime := time.Now().Add(time.Duration(-tt.rebootMinutesAgo) * time.Minute)

			getComponentFunc := func(name string) components.Component {
				comp := &mockComponent{}
				comp.On("LastHealthStates").Return(apiv1.HealthStates{
					{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Test failure"},
				})
				return comp
			}

			result := getHealthStatesFromComponentWithDeps(
				componentName,
				lastRebootTime,
				getComponentFunc,
			)

			expectedHealth := apiv1.HealthStateTypeUnhealthy
			if tt.expectInitializing {
				expectedHealth = apiv1.HealthStateTypeInitializing
			}

			assert.Equal(t, expectedHealth, result.States[0].Health, tt.description)
		})
	}
}

// TestBootTimeUnixSecondsIntegration tests that the actual implementation
// using BootTimeUnixSeconds works correctly
func TestBootTimeUnixSecondsIntegration(t *testing.T) {
	// This test ensures that using Unix timestamp for boot time works correctly
	// simulating what happens in the actual getHealthStates function

	tests := []struct {
		name               string
		bootTimeUnix       uint64
		expectInitializing bool
	}{
		{
			name:               "recent_boot_unix",
			bootTimeUnix:       uint64(time.Now().Add(-2 * time.Minute).Unix()),
			expectInitializing: true,
		},
		{
			name:               "old_boot_unix",
			bootTimeUnix:       uint64(time.Now().Add(-10 * time.Minute).Unix()),
			expectInitializing: false,
		},
		{
			name:               "zero_boot_unix",
			bootTimeUnix:       0,
			expectInitializing: false, // Zero time should not trigger initializing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate what happens in getHealthStates
			rebootTime := time.Unix(int64(tt.bootTimeUnix), 0)

			getComponentFunc := func(name string) components.Component {
				comp := &mockComponent{}
				comp.On("LastHealthStates").Return(apiv1.HealthStates{
					{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Test"},
				})
				return comp
			}

			result := getHealthStatesFromComponentWithDeps(
				"test-component",
				rebootTime,
				getComponentFunc,
			)

			expectedHealth := apiv1.HealthStateTypeUnhealthy
			if tt.expectInitializing {
				expectedHealth = apiv1.HealthStateTypeInitializing
			}

			assert.Equal(t, expectedHealth, result.States[0].Health)
		})
	}
}
