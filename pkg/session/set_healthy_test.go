package session

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/leptonai/gpud/components"
)

// mockHealthSettableComponent is a mock that implements both Component and HealthSettable
type mockHealthSettableComponent struct {
	mockComponent
	mock.Mock
}

func (m *mockHealthSettableComponent) SetHealthy() error {
	args := m.Called()
	return args.Error(0)
}

// Ensure mockHealthSettableComponent implements HealthSettable
var _ components.HealthSettable = (*mockHealthSettableComponent)(nil)

func TestSession_processSetHealthy(t *testing.T) {
	t.Run("component not found", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		registry.On("Get", "nonexistent").Return(nil)

		payload := Request{
			Components: []string{"nonexistent"},
		}

		// Should log error but not panic
		session.processSetHealthy(payload)

		registry.AssertExpectations(t)
	})

	t.Run("component does not implement HealthSettable", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		// Regular component that doesn't implement HealthSettable
		comp := new(mockComponent)
		registry.On("Get", "non-settable").Return(comp)

		payload := Request{
			Components: []string{"non-settable"},
		}

		// Should log warning but not panic
		session.processSetHealthy(payload)

		registry.AssertExpectations(t)
	})

	t.Run("successful SetHealthy call", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		// Component that implements HealthSettable
		comp := new(mockHealthSettableComponent)
		registry.On("Get", "settable").Return(comp)
		comp.On("SetHealthy").Return(nil)

		payload := Request{
			Components: []string{"settable"},
		}

		session.processSetHealthy(payload)

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})

	t.Run("SetHealthy returns error", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		// Component that implements HealthSettable but returns error
		comp := new(mockHealthSettableComponent)
		registry.On("Get", "error-comp").Return(comp)
		comp.On("SetHealthy").Return(errors.New("set healthy failed"))

		payload := Request{
			Components: []string{"error-comp"},
		}

		// Should log error but not panic
		session.processSetHealthy(payload)

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})

	t.Run("multiple components with mixed results", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		// First component: not found
		registry.On("Get", "missing").Return(nil)

		// Second component: doesn't implement HealthSettable
		nonSettableComp := new(mockComponent)
		registry.On("Get", "non-settable").Return(nonSettableComp)

		// Third component: implements HealthSettable successfully
		successComp := new(mockHealthSettableComponent)
		registry.On("Get", "success").Return(successComp)
		successComp.On("SetHealthy").Return(nil)

		// Fourth component: implements HealthSettable but fails
		errorComp := new(mockHealthSettableComponent)
		registry.On("Get", "error").Return(errorComp)
		errorComp.On("SetHealthy").Return(errors.New("failed"))

		payload := Request{
			Components: []string{"missing", "non-settable", "success", "error"},
		}

		// Should process all components despite individual failures
		session.processSetHealthy(payload)

		registry.AssertExpectations(t)
		successComp.AssertExpectations(t)
		errorComp.AssertExpectations(t)
	})

	t.Run("empty components list", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		payload := Request{
			Components: []string{},
		}

		// Should handle empty list gracefully
		session.processSetHealthy(payload)

		// No calls to registry expected
		registry.AssertNotCalled(t, "Get")
	})
}
