package session

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
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

// TestProcessDeregisterComponent tests the extracted deregisterComponent handler
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

// TestProcessBootstrap tests the extracted bootstrap handler
func TestProcessBootstrap(t *testing.T) {
	t.Run("nil bootstrap request", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		ctx := context.Background()
		payload := Request{
			Bootstrap: nil,
		}
		response := &Response{}

		session.processBootstrap(ctx, payload, response)

		// Should return early without error
		assert.Nil(t, response.Bootstrap)
		assert.Empty(t, response.Error)
	})

	t.Run("invalid base64 script", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		ctx := context.Background()
		payload := Request{
			Bootstrap: &BootstrapRequest{
				ScriptBase64: "invalid-base64!@#",
			},
		}
		response := &Response{}

		session.processBootstrap(ctx, payload, response)

		assert.Nil(t, response.Bootstrap)
		assert.NotEmpty(t, response.Error)
		assert.Contains(t, response.Error, "illegal base64")
	})

	t.Run("successful script execution", func(t *testing.T) {
		session, _, _, runner, _, _ := setupTestSessionWithoutFaultInjector()

		script := "echo hello"
		scriptBase64 := base64.StdEncoding.EncodeToString([]byte(script))

		runner.On("RunUntilCompletion", mock.Anything, script).Return([]byte("hello\n"), 0, nil)

		ctx := context.Background()
		payload := Request{
			Bootstrap: &BootstrapRequest{
				ScriptBase64:     scriptBase64,
				TimeoutInSeconds: 5,
			},
		}
		response := &Response{}

		session.processBootstrap(ctx, payload, response)

		assert.NotNil(t, response.Bootstrap)
		assert.Equal(t, "hello\n", response.Bootstrap.Output)
		assert.Equal(t, int32(0), response.Bootstrap.ExitCode)
		assert.Empty(t, response.Error)

		runner.AssertExpectations(t)
	})

	t.Run("default timeout applied", func(t *testing.T) {
		session, _, _, runner, _, _ := setupTestSessionWithoutFaultInjector()

		script := "sleep 1"
		scriptBase64 := base64.StdEncoding.EncodeToString([]byte(script))

		runner.On("RunUntilCompletion", mock.Anything, script).Return([]byte(""), 0, nil)

		ctx := context.Background()
		payload := Request{
			Bootstrap: &BootstrapRequest{
				ScriptBase64:     scriptBase64,
				TimeoutInSeconds: 0, // Should default to 10 seconds
			},
		}
		response := &Response{}

		session.processBootstrap(ctx, payload, response)

		assert.NotNil(t, response.Bootstrap)
		runner.AssertExpectations(t)
	})
}

// TestProcessInjectFault tests the extracted fault injection handler
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

// TestProcessUpdate tests the extracted update handler
func TestProcessUpdate(t *testing.T) {
	t.Run("auto update disabled", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()
		session.enableAutoUpdate = false

		ctx := context.Background()
		payload := Request{
			UpdateVersion: "v1.2.3",
		}
		response := &Response{}
		restartExitCode := -1

		session.processUpdate(ctx, payload, response, &restartExitCode)

		assert.Equal(t, "auto update is disabled", response.Error)
		assert.Equal(t, -1, restartExitCode)
	})

	t.Run("empty update version", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()
		session.enableAutoUpdate = true
		session.autoUpdateExitCode = 0 // Set to bypass systemd check

		ctx := context.Background()
		payload := Request{
			UpdateVersion: "",
		}
		response := &Response{}
		restartExitCode := -1

		session.processUpdate(ctx, payload, response, &restartExitCode)

		assert.Equal(t, "update_version is empty", response.Error)
		assert.Equal(t, -1, restartExitCode)
	})
}
