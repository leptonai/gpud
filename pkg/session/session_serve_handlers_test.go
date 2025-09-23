package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestProcessRequest tests the main processRequest method
func TestProcessRequest(t *testing.T) {
	t.Run("synchronous method returns false", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		ctx := context.Background()
		reqID := "test-req-sync"
		payload := Request{
			Method: "gossip", // Use gossip as it's simpler to test
		}
		response := &Response{}
		restartExitCode := -1

		// gossip is synchronous
		handledAsync := session.processRequest(ctx, reqID, payload, response, &restartExitCode)

		assert.False(t, handledAsync, "gossip should be handled synchronously")
		assert.Equal(t, -1, restartExitCode, "restartExitCode should remain unchanged")
	})

	t.Run("triggerComponent returns true for async", func(t *testing.T) {
		session, registry, _, _, _, writer := setupTestSessionWithoutFaultInjector()

		// Mock the registry to return nil so the async method will exit early
		registry.On("Get", "test-comp").Return(nil)

		ctx := context.Background()
		reqID := "test-req-async"
		payload := Request{
			Method:        "triggerComponent",
			ComponentName: "test-comp",
		}
		response := &Response{}
		restartExitCode := -1

		// triggerComponent is asynchronous
		handledAsync := session.processRequest(ctx, reqID, payload, response, &restartExitCode)

		assert.True(t, handledAsync, "triggerComponent should be handled asynchronously")
		assert.Equal(t, -1, restartExitCode, "restartExitCode should remain unchanged")

		// Wait for the async response
		select {
		case <-writer:
			// Response sent, good
		case <-time.After(200 * time.Millisecond):
			// Timeout is OK since component not found
		}

		registry.AssertExpectations(t)
	})

	t.Run("unknown method handled synchronously", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		ctx := context.Background()
		reqID := "test-req-unknown"
		payload := Request{
			Method: "unknownMethod",
		}
		response := &Response{}
		restartExitCode := -1

		// Unknown methods default to synchronous
		handledAsync := session.processRequest(ctx, reqID, payload, response, &restartExitCode)

		assert.False(t, handledAsync, "unknown method should be handled synchronously")
	})
}
