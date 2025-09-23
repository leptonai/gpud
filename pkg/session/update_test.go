package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProcessUpdate tests the processUpdate method
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
