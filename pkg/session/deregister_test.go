package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProcessDeregisterComponent tests the processDeregisterComponent method
func TestProcessDeregisterComponent(t *testing.T) {
	t.Run("component not found", func(t *testing.T) {
		session, registry, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		registry.On("Get", "missing").Return(nil)

		payload := Request{
			ComponentName: "missing",
		}
		response := &Response{}

		session.processDeregisterComponent(payload, response)

		assert.Equal(t, int32(404), response.ErrorCode)
		assert.Empty(t, response.Error)

		registry.AssertExpectations(t)
	})

	t.Run("component not deregisterable", func(t *testing.T) {
		session, registry, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		mockComp := new(mockComponent)
		registry.On("Get", "non-deregisterable").Return(mockComp)
		mockComp.On("Name").Return("non-deregisterable")

		// Component doesn't implement Deregisterable interface
		payload := Request{
			ComponentName: "non-deregisterable",
		}
		response := &Response{}

		session.processDeregisterComponent(payload, response)

		assert.Equal(t, int32(400), response.ErrorCode)
		assert.Equal(t, "component is not deregisterable", response.Error)

		registry.AssertExpectations(t)
		mockComp.AssertExpectations(t)
	})

	t.Run("successful deregistration", func(t *testing.T) {
		session, registry, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		mockComp := new(mockDeregisterableComponent)
		registry.On("Get", "deregisterable").Return(mockComp)
		mockComp.On("CanDeregister").Return(true)
		mockComp.On("Close").Return(nil)
		registry.On("Deregister", "deregisterable").Return(nil)

		payload := Request{
			ComponentName: "deregisterable",
		}
		response := &Response{}

		session.processDeregisterComponent(payload, response)

		assert.Equal(t, int32(0), response.ErrorCode)
		assert.Empty(t, response.Error)

		registry.AssertExpectations(t)
		mockComp.AssertExpectations(t)
	})

	t.Run("empty component name", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		payload := Request{
			ComponentName: "",
		}
		response := &Response{}

		session.processDeregisterComponent(payload, response)

		// Should return early without error
		assert.Equal(t, int32(0), response.ErrorCode)
		assert.Empty(t, response.Error)
	})
}
