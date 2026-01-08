package session

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	componentsnvidianvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	"github.com/leptonai/gpud/pkg/config"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/gpud-manager/controllers"
	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestSession_processRequest(t *testing.T) {
	t.Run("reboot method", func(t *testing.T) {
		session := &Session{
			ctx: context.Background(),
		}
		response := &Response{}
		restartExitCode := -1

		// Note: reboot calls pkghost.Reboot which requires system access
		// This would need dependency injection to test properly
		// The method should set response.Error on failure

		payload := Request{
			Method: "reboot",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "reboot should be handled synchronously")
		assert.Equal(t, -1, restartExitCode, "restart exit code should not change")
		// When not running as root, we expect an error
		if response.Error != "" {
			assert.Contains(t, response.Error, "sudo/root", "Expected permission error when not running as root")
		}
	})

	t.Run("metrics method", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore
		response := &Response{}
		restartExitCode := -1

		// Mock the components that will be queried
		comp1 := new(mockComponent)
		comp2 := new(mockComponent)
		registry.On("Get", "component1").Return(comp1)
		registry.On("Get", "component2").Return(comp2)

		// Mock the metrics store reads
		emptyMetrics := pkgmetrics.Metrics{}
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(emptyMetrics, nil)

		payload := Request{
			Method: "metrics",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "metrics should be handled synchronously")
		assert.Equal(t, -1, restartExitCode)
		assert.NotNil(t, response.Metrics)
	})

	t.Run("states method", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		response := &Response{}
		restartExitCode := -1

		// Mock the components that will be queried for states
		comp1 := new(mockComponent)
		comp2 := new(mockComponent)
		registry.On("Get", "component1").Return(comp1)
		registry.On("Get", "component2").Return(comp2)

		// Mock health states
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy},
		}
		comp1.On("LastHealthStates").Return(healthStates)
		comp2.On("LastHealthStates").Return(healthStates)

		payload := Request{
			Method: "states",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "states should be handled synchronously")
		assert.Equal(t, -1, restartExitCode)
		assert.NotNil(t, response.States)
	})

	t.Run("events method", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		response := &Response{}
		restartExitCode := -1

		// Mock the components that will be queried for events
		comp1 := new(mockComponent)
		comp2 := new(mockComponent)
		registry.On("Get", "component1").Return(comp1)
		registry.On("Get", "component2").Return(comp2)

		// Mock events
		events := apiv1.Events{}
		comp1.On("Events", mock.Anything, mock.Anything).Return(events, nil)
		comp2.On("Events", mock.Anything, mock.Anything).Return(events, nil)

		payload := Request{
			Method: "events",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "events should be handled synchronously")
		assert.Equal(t, -1, restartExitCode)
		assert.NotNil(t, response.Events)
	})

	t.Run("logout method", func(t *testing.T) {
		dataDir := t.TempDir()
		stateFile := config.StateFilePath(dataDir)

		db, err := sqlite.Open(stateFile)
		require.NoError(t, err)
		require.NoError(t, pkgmetadata.CreateTableMetadata(context.Background(), db))
		require.NoError(t, db.Close())

		session := &Session{
			ctx:     context.Background(),
			dataDir: dataDir,
		}
		response := &Response{}
		restartExitCode := -1

		payload := Request{
			Method: "logout",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "logout should be handled synchronously")
		assert.Equal(t, -1, restartExitCode)
		if response.Error != "" {
			assert.True(t, strings.Contains(response.Error, "sudo/root") || strings.Contains(response.Error, "root"), "unexpected error: %s", response.Error)
		}
	})

	t.Run("packageStatus method", func(t *testing.T) {
		prevController := gpudmanager.GlobalController
		defer func() {
			gpudmanager.GlobalController = prevController
		}()

		gpudmanager.GlobalController = controllers.NewPackageController(make(chan packages.PackageInfo))

		session := &Session{
			ctx: context.Background(),
		}
		response := &Response{}
		restartExitCode := -1

		payload := Request{
			Method: "packageStatus",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "packageStatus should be handled synchronously")
		assert.Equal(t, -1, restartExitCode)
		assert.Empty(t, response.Error)
		assert.Len(t, response.PackageStatus, 0)
	})

	t.Run("triggerComponent method returns async", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		writer := make(chan Body, 1)
		session.writer = writer

		response := &Response{}
		restartExitCode := -1

		// Mock a component that will be accessed by the async goroutine
		comp := new(mockComponent)
		registry.On("Get", "test-comp").Return(comp)

		// Mock the Check method that will be called asynchronously
		checkResult := new(mockCheckResult)
		checkResult.On("ComponentName").Return("test-comp")
		checkResult.On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy},
		})
		comp.On("Check").Return(checkResult)

		payload := Request{
			Method:        "triggerComponent",
			ComponentName: "test-comp",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.True(t, handledAsync, "triggerComponent should be handled asynchronously")
		assert.Equal(t, -1, restartExitCode)

		// Wait for async response to be sent
		select {
		case <-writer:
			// Response was sent
		case <-time.After(100 * time.Millisecond):
			// It's ok if no response within timeout in this test since we're just testing the async flag
		}

		// Let the goroutine finish
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("triggerComponentCheck method returns async", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		writer := make(chan Body, 1)
		session.writer = writer

		response := &Response{}
		restartExitCode := -1

		// Mock a component that will be accessed by the async goroutine
		comp := new(mockComponent)
		registry.On("Get", "test-comp").Return(comp)

		// Mock the Check method that will be called asynchronously
		checkResult := new(mockCheckResult)
		checkResult.On("ComponentName").Return("test-comp")
		checkResult.On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy},
		})
		comp.On("Check").Return(checkResult)

		payload := Request{
			Method:        "triggerComponentCheck",
			ComponentName: "test-comp",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.True(t, handledAsync, "triggerComponentCheck should be handled asynchronously")
		assert.Equal(t, -1, restartExitCode)

		// Wait for async response to be sent
		select {
		case <-writer:
			// Response was sent
		case <-time.After(100 * time.Millisecond):
			// It's ok if no response within timeout in this test since we're just testing the async flag
		}

		// Let the goroutine finish
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("deregisterComponent method", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		response := &Response{}
		restartExitCode := -1

		registry.On("Get", "test-comp").Return(nil)

		payload := Request{
			Method:        "deregisterComponent",
			ComponentName: "test-comp",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "deregisterComponent should be handled synchronously")
		assert.Equal(t, -1, restartExitCode)
	})

	t.Run("unknown method", func(t *testing.T) {
		session := &Session{
			ctx: context.Background(),
		}
		response := &Response{}
		restartExitCode := -1

		payload := Request{
			Method: "unknownMethod",
		}

		handledAsync := session.processRequest(context.Background(), "test-req", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "unknown method should be handled synchronously")
		assert.Equal(t, -1, restartExitCode)
	})

	t.Run("updateConfig processed when skip disabled", func(t *testing.T) {
		called := false
		session := &Session{
			ctx: context.Background(),
			setDefaultNVLinkExpectedLinkStatesFunc: func(states componentsnvidianvlink.ExpectedLinkStates) {
				called = true
			},
		}

		payload := Request{
			Method: "updateConfig",
			UpdateConfig: map[string]string{
				componentsnvidianvlink.Name: `{}`,
			},
		}

		response := &Response{}
		restartExitCode := -1

		handledAsync := session.processRequest(context.Background(), "req-update-config", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "updateConfig should be handled synchronously")
		assert.True(t, called, "updateConfig should invoke processUpdateConfig when not skipped")
		assert.Equal(t, -1, restartExitCode)
		assert.Empty(t, response.Error)
	})

	t.Run("updateConfig skipped when flag enabled", func(t *testing.T) {
		called := false
		session := &Session{
			ctx:              context.Background(),
			skipUpdateConfig: true,
			setDefaultNVLinkExpectedLinkStatesFunc: func(states componentsnvidianvlink.ExpectedLinkStates) {
				called = true
			},
		}

		payload := Request{
			Method: "updateConfig",
			UpdateConfig: map[string]string{
				componentsnvidianvlink.Name: `{}`,
			},
		}

		response := &Response{}
		restartExitCode := -1

		handledAsync := session.processRequest(context.Background(), "req-update-config-skip", payload, response, &restartExitCode)

		assert.False(t, handledAsync, "updateConfig should remain synchronous even when skipped")
		assert.False(t, called, "updateConfig should not be processed when skip flag is set")
		assert.Equal(t, -1, restartExitCode)
		assert.Empty(t, response.Error)
	})
}

func TestSession_processRequestAsync(t *testing.T) {
	t.Run("triggerComponent method component not found", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		writer := make(chan Body, 1)
		session.writer = writer

		// Mock a component that doesn't exist
		registry.On("Get", "test-comp").Return(nil)

		payload := Request{
			Method:        "triggerComponent",
			ComponentName: "test-comp",
		}

		// Call processRequestAsync directly
		session.processRequestAsync("test-req-async", "triggerComponent", payload)

		// Should send a response via writer channel
		select {
		case resp := <-writer:
			assert.Equal(t, "test-req-async", resp.ReqID)
			// Response should contain 404 error code for component not found
		default:
			t.Error("Expected response to be sent to writer channel")
		}

		registry.AssertExpectations(t)
	})

	t.Run("triggerComponent method with component", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		writer := make(chan Body, 1)
		session.writer = writer

		// Mock a component that exists
		comp := new(mockComponent)
		registry.On("Get", "existing-comp").Return(comp)

		// Mock the Check method
		checkResult := new(mockCheckResult)
		checkResult.On("ComponentName").Return("existing-comp")
		checkResult.On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy},
		})
		comp.On("Check").Return(checkResult)

		payload := Request{
			Method:        "triggerComponent",
			ComponentName: "existing-comp",
		}

		// Call processRequestAsync directly
		session.processRequestAsync("test-req-with-comp", "triggerComponent", payload)

		// Should send a response via writer channel
		select {
		case resp := <-writer:
			assert.Equal(t, "test-req-with-comp", resp.ReqID)
		default:
			t.Error("Expected response to be sent to writer channel")
		}

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
		checkResult.AssertExpectations(t)
	})

	t.Run("unsupported async method", func(t *testing.T) {
		session := createMockSession(nil)
		writer := make(chan Body, 1)
		session.writer = writer

		payload := Request{
			Method: "unsupportedAsync",
		}

		// Call processRequestAsync with unsupported method
		session.processRequestAsync("test-req-unsupported", "unsupportedAsync", payload)

		// Should send error response
		select {
		case resp := <-writer:
			assert.Equal(t, "test-req-unsupported", resp.ReqID)
		default:
			t.Error("Expected error response to be sent to writer channel")
		}
	})
}

func TestSession_sendResponse(t *testing.T) {
	t.Run("successful response send", func(t *testing.T) {
		session := createMockSession(nil)
		writer := make(chan Body, 1)
		session.writer = writer

		// Use nop audit logger for testing
		session.auditLogger = log.NewNopAuditLogger()

		response := &Response{
			Error: "",
		}

		session.sendResponse("test-req-send", "testMethod", response)

		// Should send response to writer channel
		select {
		case resp := <-writer:
			assert.Equal(t, "test-req-send", resp.ReqID)
			assert.NotNil(t, resp.Data)
		default:
			t.Error("Expected response to be sent to writer channel")
		}
	})

	t.Run("returns early when send is skipped", func(t *testing.T) {
		session := createMockSession(nil)
		writer := make(chan Body, 1)
		session.writer = writer

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		session.ctx = ctx
		session.auditLogger = log.NewNopAuditLogger()

		response := &Response{
			Error: "",
		}

		session.sendResponse("test-req-skip", "testMethod", response)

		select {
		case <-writer:
			t.Fatal("did not expect response to be sent when session context is canceled")
		default:
		}
	})

	t.Run("handles marshal error", func(t *testing.T) {
		session := createMockSession(nil)
		writer := make(chan Body, 1)
		session.writer = writer

		response := &Response{
			Metrics: apiv1.GPUdComponentMetrics{
				{
					Component: "test-comp",
					Metrics: apiv1.Metrics{
						{
							Name:  "bad-metric",
							Value: math.NaN(),
						},
					},
				},
			},
		}

		session.sendResponse("test-req-marshal", "testMethod", response)

		// Should not send anything when marshaling fails.
		select {
		case <-writer:
			t.Fatal("did not expect response to be sent when marshaling fails")
		default:
		}
	})
}

func TestSession_trySendResponse(t *testing.T) {
	t.Run("successfully sends when context is nil", func(t *testing.T) {
		session := createMockSession(nil)
		writer := make(chan Body, 1)
		session.writer = writer
		session.ctx = nil // Explicitly set ctx to nil

		body := Body{
			ReqID: "test-nil-ctx",
			Data:  []byte(`{"test": "data"}`),
		}

		sent := session.trySendResponse(body)

		assert.True(t, sent, "trySendResponse should return true when ctx is nil and write succeeds")
		select {
		case resp := <-writer:
			assert.Equal(t, "test-nil-ctx", resp.ReqID)
		default:
			t.Error("Expected response to be sent to writer channel")
		}
	})

	t.Run("recovers from panic when writer is closed", func(t *testing.T) {
		session := createMockSession(nil)
		writer := make(chan Body, 1)
		session.writer = writer
		session.ctx = nil // nil ctx to trigger the send path that could panic

		// Close the writer channel to trigger a panic when trying to send
		close(writer)

		body := Body{
			ReqID: "test-panic",
			Data:  []byte(`{"test": "data"}`),
		}

		// This should not panic - the recover should catch it
		sent := session.trySendResponse(body)

		assert.False(t, sent, "trySendResponse should return false when panic occurs")
	})

	t.Run("returns false when context is already done before select", func(t *testing.T) {
		session := createMockSession(nil)
		writer := make(chan Body, 1)
		session.writer = writer

		// Create a context that's already canceled
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		session.ctx = ctx

		body := Body{
			ReqID: "test-ctx-err",
			Data:  []byte(`{"test": "data"}`),
		}

		sent := session.trySendResponse(body)

		assert.False(t, sent, "trySendResponse should return false when context is already done")
		// Verify nothing was sent to writer
		select {
		case <-writer:
			t.Error("Should not have sent response when context is done")
		default:
			// Expected - nothing sent
		}
	})

	t.Run("successfully sends with valid context", func(t *testing.T) {
		session := createMockSession(nil)
		writer := make(chan Body, 1)
		session.writer = writer
		session.ctx = context.Background()

		body := Body{
			ReqID: "test-valid-ctx",
			Data:  []byte(`{"test": "data"}`),
		}

		sent := session.trySendResponse(body)

		assert.True(t, sent, "trySendResponse should return true with valid context")
		select {
		case resp := <-writer:
			assert.Equal(t, "test-valid-ctx", resp.ReqID)
		default:
			t.Error("Expected response to be sent to writer channel")
		}
	})
}
