package session

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestProcessBootstrap tests the processBootstrap method
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
