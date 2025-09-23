package session

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
)

// TestProcessInjectFault tests the processInjectFault method
func TestProcessInjectFault(t *testing.T) {
	t.Run("nil fault request", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		payload := Request{
			InjectFaultRequest: nil,
		}
		response := &Response{}

		session.processInjectFault(payload, response)

		// Should return early without error but log warning
		assert.Empty(t, response.Error)
		assert.Equal(t, int32(0), response.ErrorCode)
	})

	t.Run("fault injector not initialized", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()
		// faultInjector is nil in this setup

		payload := Request{
			InjectFaultRequest: &pkgfaultinjector.Request{},
		}
		response := &Response{}

		session.processInjectFault(payload, response)

		// The actual error comes from Validate() being called on an empty request
		assert.Equal(t, "no fault injection entry found", response.Error)
	})
}
