package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
)

// MockComponent is a mock implementation of components.Component
type MockComponent struct {
	mock.Mock
}

func (m *MockComponent) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockComponent) Tags() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

func (m *MockComponent) IsSupported() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockComponent) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockComponent) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockComponent) LastHealthStates() apiv1.HealthStates {
	args := m.Called()
	return args.Get(0).(apiv1.HealthStates)
}

func (m *MockComponent) Check() components.CheckResult {
	args := m.Called()
	return args.Get(0).(components.CheckResult)
}

func (m *MockComponent) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	args := m.Called(ctx, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(apiv1.Events), args.Error(1)
}

func TestGetStatesFromComponentWithDeps(t *testing.T) {
	tests := []struct {
		name                   string
		componentName          string
		lastRebootTime         time.Time
		mockHealthStates       apiv1.HealthStates
		componentExists        bool
		processStartError      error
		processStartMinutesAgo int
		expectedHealthTypes    []apiv1.HealthStateType
		expectInitializing     bool
		expectOverride         bool
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
			processStartMinutesAgo: 10,
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
			processStartMinutesAgo: 2,
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
			processStartMinutesAgo: 10,
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeUnhealthy,
			},
		},
		{
			name:            "future_reboot_time_with_process_override",
			componentName:   "comp-with-future-reboot",
			lastRebootTime:  time.Now().Add(1 * time.Hour), // Future (timezone bug)
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			processStartMinutesAgo: 10, // Process has been up 10 minutes
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeUnhealthy, // Should NOT be initializing
			},
			expectOverride: true,
		},
		{
			name:            "zero_reboot_time",
			componentName:   "comp-zero-reboot",
			lastRebootTime:  time.Time{}, // Zero time
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			processStartMinutesAgo: 0,
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeUnhealthy, // Should remain unhealthy
			},
		},
		{
			name:            "process_start_time_error",
			componentName:   "comp-process-error",
			lastRebootTime:  time.Now().Add(-2 * time.Minute), // Within grace period
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			processStartError: errors.New("failed to get process start time"),
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeInitializing, // Falls back to initializing
			},
			expectInitializing: true,
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
			processStartMinutesAgo: 2,
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeHealthy,
				apiv1.HealthStateTypeInitializing,
				apiv1.HealthStateTypeInitializing,
			},
		},
		{
			name:            "process_uptime_exactly_grace_period",
			componentName:   "exact-grace-comp",
			lastRebootTime:  time.Now().Add(-2 * time.Minute), // Recent reboot
			componentExists: true,
			mockHealthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Component failing"},
			},
			processStartMinutesAgo: 5, // Exactly at grace period boundary
			expectedHealthTypes: []apiv1.HealthStateType{
				apiv1.HealthStateTypeUnhealthy, // Should NOT be initializing
			},
			expectOverride: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock component function
			getComponentFunc := func(name string) components.Component {
				if !tt.componentExists {
					return nil
				}
				comp := &MockComponent{}
				comp.On("LastHealthStates").Return(tt.mockHealthStates)
				return comp
			}

			// Setup mock process start time function
			getProcessStartTimeFunc := func() (uint64, error) {
				if tt.processStartError != nil {
					return 0, tt.processStartError
				}
				if tt.processStartMinutesAgo == 0 {
					return uint64(time.Now().Unix()), nil
				}
				return uint64(time.Now().Add(time.Duration(-tt.processStartMinutesAgo) * time.Minute).Unix()), nil
			}

			// Call the function under test
			result := getStatesFromComponentWithDeps(
				tt.componentName,
				tt.lastRebootTime,
				getComponentFunc,
				getProcessStartTimeFunc,
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

// TestProcessUptimeOverride specifically tests the process uptime override logic
func TestProcessUptimeOverride(t *testing.T) {
	tests := []struct {
		name                   string
		systemRebootMinutesAgo int
		processStartMinutesAgo int
		expectInitializing     bool
		description            string
	}{
		{
			name:                   "process_longer_than_grace_overrides",
			systemRebootMinutesAgo: 1,  // System shows recent reboot (bug)
			processStartMinutesAgo: 10, // But process has been up 10 minutes
			expectInitializing:     false,
			description:            "Process uptime should override incorrect system reboot time",
		},
		{
			name:                   "both_within_grace_period",
			systemRebootMinutesAgo: 2,
			processStartMinutesAgo: 2,
			expectInitializing:     true,
			description:            "Both times within grace period, should be initializing",
		},
		{
			name:                   "system_future_process_old",
			systemRebootMinutesAgo: -60, // System reboot time is in the future
			processStartMinutesAgo: 10,  // Process has been up 10 minutes
			expectInitializing:     false,
			description:            "Future system time should be overridden by process uptime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentName := "test-component"
			lastRebootTime := time.Now().Add(time.Duration(-tt.systemRebootMinutesAgo) * time.Minute)

			getComponentFunc := func(name string) components.Component {
				comp := &MockComponent{}
				comp.On("LastHealthStates").Return(apiv1.HealthStates{
					{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Test failure"},
				})
				return comp
			}

			getProcessStartTimeFunc := func() (uint64, error) {
				return uint64(time.Now().Add(time.Duration(-tt.processStartMinutesAgo) * time.Minute).Unix()), nil
			}

			result := getStatesFromComponentWithDeps(
				componentName,
				lastRebootTime,
				getComponentFunc,
				getProcessStartTimeFunc,
			)

			expectedHealth := apiv1.HealthStateTypeUnhealthy
			if tt.expectInitializing {
				expectedHealth = apiv1.HealthStateTypeInitializing
			}

			assert.Equal(t, expectedHealth, result.States[0].Health, tt.description)
		})
	}
}

// TestFutureRebootTimeScenario tests the specific bug scenario where
// uptime -s returns a future time due to timezone issues
func TestFutureRebootTimeScenario(t *testing.T) {
	// Simulate the bug: system reports reboot in the future
	// but process has been running for 8 hours
	componentName := "accelerator-nvidia-error-xid"
	futureRebootTime := time.Now().Add(4 * time.Hour) // UTC+4 timezone bug

	getComponentFunc := func(name string) components.Component {
		comp := &MockComponent{}
		comp.On("LastHealthStates").Return(apiv1.HealthStates{
			{
				Health: apiv1.HealthStateTypeUnhealthy,
				Reason: "set unhealthy state initializing due to recent reboot",
			},
		})
		return comp
	}

	// Process has been running for 8 hours
	processStartTime := time.Now().Add(-8 * time.Hour)
	getProcessStartTimeFunc := func() (uint64, error) {
		return uint64(processStartTime.Unix()), nil
	}

	result := getStatesFromComponentWithDeps(
		componentName,
		futureRebootTime,
		getComponentFunc,
		getProcessStartTimeFunc,
	)

	// With the fix, the state should remain UNHEALTHY, not INITIALIZING
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.States[0].Health,
		"Process running for 8 hours should override future reboot time")
}
