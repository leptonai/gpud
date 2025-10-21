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
			Method: "setHealthy", // Use setHealthy as it's synchronous
		}
		response := &Response{}
		restartExitCode := -1

		// setHealthy is synchronous
		handledAsync := session.processRequest(ctx, reqID, payload, response, &restartExitCode)

		assert.False(t, handledAsync, "setHealthy should be handled synchronously")
		assert.Equal(t, -1, restartExitCode, "restartExitCode should remain unchanged")
	})

	t.Run("gossip returns true for async", func(t *testing.T) {
		session, _, _, _, _, writer := setupTestSessionWithoutFaultInjector()

		ctx := context.Background()
		reqID := "test-req-gossip"
		payload := Request{
			Method: "gossip",
		}
		response := &Response{}
		restartExitCode := -1

		// gossip is now asynchronous to prevent blocking on disk I/O
		handledAsync := session.processRequest(ctx, reqID, payload, response, &restartExitCode)

		assert.True(t, handledAsync, "gossip should be handled asynchronously")
		assert.Equal(t, -1, restartExitCode, "restartExitCode should remain unchanged")

		// Wait for the async response
		select {
		case <-writer:
			// Response sent, good
		case <-time.After(200 * time.Millisecond):
			// Timeout is OK for this test
		}
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
