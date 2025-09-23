package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

// TestProcessRequestAsync tests the async processing of requests
func TestProcessRequestAsync(t *testing.T) {
	t.Run("single component check", func(t *testing.T) {
		// Setup
		session, registry, _, _, _, writer := setupTestSessionWithoutFaultInjector()

		// Create mock component and check result
		mockComp := new(mockComponent)
		mockResult := new(mockCheckResult)

		// Setup expectations
		registry.On("Get", "test-component").Return(mockComp)
		mockComp.On("Check").Return(mockResult)
		mockResult.On("ComponentName").Return("test-component")
		mockResult.On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Name: "test-state"},
		})

		// Execute async request
		reqID := "test-req-123"
		method := "triggerComponent"
		payload := Request{
			Method:        method,
			ComponentName: "test-component",
		}

		// Run in background
		go session.processRequestAsync(reqID, method, payload)

		// Wait for response with timeout
		select {
		case body := <-writer:
			assert.Equal(t, reqID, body.ReqID)

			var response Response
			err := json.Unmarshal(body.Data, &response)
			require.NoError(t, err)

			assert.Len(t, response.States, 1)
			assert.Equal(t, "test-component", response.States[0].Component)
			assert.Equal(t, apiv1.HealthStateTypeHealthy, response.States[0].States[0].Health)

		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for response")
		}

		registry.AssertExpectations(t)
		mockComp.AssertExpectations(t)
		mockResult.AssertExpectations(t)
	})

	t.Run("component not found", func(t *testing.T) {
		// Setup
		session, registry, _, _, _, writer := setupTestSessionWithoutFaultInjector()

		// Setup expectations - component not found
		registry.On("Get", "missing-component").Return(nil)

		// Execute async request
		reqID := "test-req-456"
		method := "triggerComponent"
		payload := Request{
			Method:        method,
			ComponentName: "missing-component",
		}

		// Run in background
		go session.processRequestAsync(reqID, method, payload)

		// Wait for response
		select {
		case body := <-writer:
			assert.Equal(t, reqID, body.ReqID)

			var response Response
			err := json.Unmarshal(body.Data, &response)
			require.NoError(t, err)

			assert.Equal(t, int32(404), response.ErrorCode)

		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for response")
		}

		registry.AssertExpectations(t)
	})

	t.Run("tag-based component checks", func(t *testing.T) {
		// Setup
		session, registry, _, _, _, writer := setupTestSessionWithoutFaultInjector()

		// Create mock components
		mockComp1 := new(mockComponent)
		mockComp2 := new(mockComponent)
		mockComp3 := new(mockComponent)

		mockResult1 := new(mockCheckResult)
		mockResult2 := new(mockCheckResult)

		// Setup registry to return all components
		registry.On("All").Return([]components.Component{mockComp1, mockComp2, mockComp3})

		// Setup component tags
		mockComp1.On("Tags").Return([]string{"gpu", "nvidia"})
		mockComp2.On("Tags").Return([]string{"disk", "storage"})
		mockComp3.On("Tags").Return([]string{"gpu", "amd"})

		// Only comp1 and comp3 have "gpu" tag, so only they should be checked
		mockComp1.On("Check").Return(mockResult1)
		mockComp3.On("Check").Return(mockResult2)

		mockResult1.On("ComponentName").Return("nvidia-gpu")
		mockResult1.On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Name: "nvidia-state"},
		})

		mockResult2.On("ComponentName").Return("amd-gpu")
		mockResult2.On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Name: "amd-state"},
		})

		// Execute async request with tag
		reqID := "test-req-789"
		method := "triggerComponent"
		payload := Request{
			Method:  method,
			TagName: "gpu",
		}

		// Run in background
		go session.processRequestAsync(reqID, method, payload)

		// Wait for response
		select {
		case body := <-writer:
			assert.Equal(t, reqID, body.ReqID)

			var response Response
			err := json.Unmarshal(body.Data, &response)
			require.NoError(t, err)

			assert.Len(t, response.States, 2)

			// Check both components are in the response
			componentNames := []string{}
			for _, state := range response.States {
				componentNames = append(componentNames, state.Component)
			}
			assert.Contains(t, componentNames, "nvidia-gpu")
			assert.Contains(t, componentNames, "amd-gpu")

		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for response")
		}

		registry.AssertExpectations(t)
		mockComp1.AssertExpectations(t)
		mockComp2.AssertExpectations(t)
		mockComp3.AssertExpectations(t)
		mockResult1.AssertExpectations(t)
		mockResult2.AssertExpectations(t)
	})
}

// TestTriggerComponentAsync tests that triggerComponent runs asynchronously
func TestTriggerComponentAsync(t *testing.T) {
	t.Run("async execution with slow component", func(t *testing.T) {
		// Setup
		session, registry, _, _, _, writer := setupTestSessionWithoutFaultInjector()

		// Create a slow component that takes time to check
		slowComp := new(mockComponent)
		slowCheckResult := new(mockCheckResult)

		registry.On("Get", "slow-component").Return(slowComp)

		checkStarted := make(chan struct{})
		checkCompleted := make(chan struct{})

		// Make the Check() method slow but observable
		slowComp.On("Check").Run(func(args mock.Arguments) {
			close(checkStarted)
			time.Sleep(200 * time.Millisecond) // Simulate slow check
			close(checkCompleted)
		}).Return(slowCheckResult)

		slowCheckResult.On("ComponentName").Return("slow-component")
		slowCheckResult.On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Name: "slow-state"},
		})

		// Execute the async request
		reqID := "test-slow-req"
		method := "triggerComponent"
		payload := Request{
			Method:        method,
			ComponentName: "slow-component",
		}

		// Run in background - this should return immediately
		go session.processRequestAsync(reqID, method, payload)

		// Wait for check to start
		select {
		case <-checkStarted:
			// Good, check has started
		case <-time.After(100 * time.Millisecond):
			t.Fatal("check did not start in time")
		}

		// At this point, the check is running but not complete
		// The function should have returned already (async)

		// Now wait for the response
		select {
		case body := <-writer:
			// Should only arrive after check completes
			select {
			case <-checkCompleted:
				// Good, check completed before response
			default:
				t.Fatal("response sent before check completed")
			}

			assert.Equal(t, reqID, body.ReqID)

			var response Response
			err := json.Unmarshal(body.Data, &response)
			require.NoError(t, err)
			assert.Len(t, response.States, 1)

		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		registry.AssertExpectations(t)
		slowComp.AssertExpectations(t)
		slowCheckResult.AssertExpectations(t)
	})
}

// TestTrySendResponse tests the safe response sending with panic recovery
func TestTrySendResponse(t *testing.T) {
	t.Run("successful send", func(t *testing.T) {
		session := &Session{
			ctx:         context.Background(),
			writer:      make(chan Body, 1),
			auditLogger: log.NewNopAuditLogger(),
		}

		body := Body{
			ReqID: "test-123",
			Data:  []byte("test data"),
		}

		sent := session.trySendResponse(body)
		assert.True(t, sent)

		// Verify the body was sent
		select {
		case received := <-session.writer:
			assert.Equal(t, body.ReqID, received.ReqID)
		default:
			t.Fatal("body was not sent to writer")
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		session := &Session{
			ctx:         ctx,
			writer:      make(chan Body), // No buffer to ensure blocking
			auditLogger: log.NewNopAuditLogger(),
		}

		body := Body{
			ReqID: "test-456",
			Data:  []byte("test data"),
		}

		sent := session.trySendResponse(body)
		assert.False(t, sent)
	})

	t.Run("closed writer channel panic recovery", func(t *testing.T) {
		session := &Session{
			ctx:         context.Background(),
			writer:      make(chan Body),
			auditLogger: log.NewNopAuditLogger(),
		}

		// Close the writer to trigger panic on send
		close(session.writer)

		body := Body{
			ReqID: "test-789",
			Data:  []byte("test data"),
		}

		// This should not panic due to recovery
		sent := session.trySendResponse(body)
		assert.False(t, sent)
	})
}

// TestSendResponse tests the centralized response sending
func TestSendResponse(t *testing.T) {
	t.Run("successful response marshaling and sending", func(t *testing.T) {
		session := &Session{
			ctx:            context.Background(),
			writer:         make(chan Body, 1),
			auditLogger:    log.NewNopAuditLogger(),
			machineID:      "test-machine",
			epControlPlane: "http://control-plane",
		}

		response := &Response{
			States: apiv1.GPUdComponentHealthStates{
				{
					Component: "test-comp",
					States: apiv1.HealthStates{
						{Health: apiv1.HealthStateTypeHealthy, Name: "test"},
					},
				},
			},
		}

		session.sendResponse("req-123", "triggerComponent", response)

		// Verify response was sent
		select {
		case body := <-session.writer:
			assert.Equal(t, "req-123", body.ReqID)

			var unmarshaled Response
			err := json.Unmarshal(body.Data, &unmarshaled)
			require.NoError(t, err)
			assert.Equal(t, response.States, unmarshaled.States)

		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}
	})

	t.Run("marshal error handling", func(t *testing.T) {
		session := &Session{
			ctx:         context.Background(),
			writer:      make(chan Body, 1),
			auditLogger: log.NewNopAuditLogger(),
		}

		// This test can't easily trigger a marshal error with Response type,
		// but the code path exists for safety
		session.sendResponse("req-456", "test", nil)

		// Should handle gracefully without panic
		// No response should be sent for nil
	})
}

// TestProcessRequestAsyncGenericFunctionality tests the generic async request handling
func TestProcessRequestAsyncGenericFunctionality(t *testing.T) {
	t.Run("unsupported async method", func(t *testing.T) {
		// Setup
		session, _, _, _, _, writer := setupTestSessionWithoutFaultInjector()

		// Execute async request with unsupported method
		reqID := "test-unsupported-req"
		method := "unsupportedMethod"
		payload := Request{
			Method: method,
		}

		// Run in background
		go session.processRequestAsync(reqID, method, payload)

		// Wait for response
		select {
		case body := <-writer:
			assert.Equal(t, reqID, body.ReqID)

			var response Response
			err := json.Unmarshal(body.Data, &response)
			require.NoError(t, err)

			assert.Equal(t, int32(400), response.ErrorCode)
			assert.Contains(t, response.Error, "unsupported async method")
			assert.Contains(t, response.Error, method)

		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for response")
		}
	})

	t.Run("triggerComponentCheck backward compatibility", func(t *testing.T) {
		// Setup
		session, registry, _, _, _, writer := setupTestSessionWithoutFaultInjector()

		// Create mock component
		mockComp := new(mockComponent)
		mockResult := new(mockCheckResult)

		registry.On("Get", "test-component").Return(mockComp)
		mockComp.On("Check").Return(mockResult)
		mockResult.On("ComponentName").Return("test-component")
		mockResult.On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Name: "test-state"},
		})

		// Execute with legacy "triggerComponentCheck" method
		reqID := "test-legacy-req"
		method := "triggerComponentCheck"
		payload := Request{
			Method:        method,
			ComponentName: "test-component",
		}

		// Run in background
		go session.processRequestAsync(reqID, method, payload)

		// Wait for response
		select {
		case body := <-writer:
			assert.Equal(t, reqID, body.ReqID)

			var response Response
			err := json.Unmarshal(body.Data, &response)
			require.NoError(t, err)

			// Should work the same as triggerComponent
			assert.Len(t, response.States, 1)
			assert.Equal(t, "test-component", response.States[0].Component)
			assert.Equal(t, apiv1.HealthStateTypeHealthy, response.States[0].States[0].Health)

		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for response")
		}

		registry.AssertExpectations(t)
		mockComp.AssertExpectations(t)
		mockResult.AssertExpectations(t)
	})
}

// TestProcessTriggerComponent tests the extracted processTriggerComponent helper method
func TestProcessTriggerComponent(t *testing.T) {
	t.Run("component name and tag both empty", func(t *testing.T) {
		session, _, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		payload := Request{
			Method: "triggerComponent",
			// Both ComponentName and TagName are empty
		}
		response := &Response{}

		session.processTriggerComponent(payload, response)

		// Should return empty states when no component or tag specified
		assert.NotNil(t, response.States)
		assert.Len(t, response.States, 0)
		assert.Empty(t, response.Error)
		assert.Equal(t, int32(0), response.ErrorCode)
	})

	t.Run("direct method call with component not found", func(t *testing.T) {
		session, registry, _, _, _, _ := setupTestSessionWithoutFaultInjector()

		registry.On("Get", "missing").Return(nil)

		payload := Request{
			Method:        "triggerComponent",
			ComponentName: "missing",
		}
		response := &Response{}

		session.processTriggerComponent(payload, response)

		// Should set error code but not error message (handled in processRequestAsync)
		assert.Equal(t, int32(404), response.ErrorCode)
		assert.Empty(t, response.States)

		registry.AssertExpectations(t)
	})
}

// TestConcurrentTriggerComponentRequests tests multiple concurrent trigger requests
func TestConcurrentTriggerComponentRequests(t *testing.T) {
	session, registry, _, _, _, writer := setupTestSessionWithoutFaultInjector()

	// Create multiple mock components
	numComponents := 5
	components := make([]*mockComponent, numComponents)
	results := make([]*mockCheckResult, numComponents)

	for i := range numComponents {
		components[i] = new(mockComponent)
		results[i] = new(mockCheckResult)

		componentName := fmt.Sprintf("component-%d", i)
		registry.On("Get", componentName).Return(components[i])

		// Each component takes different time to check
		delay := time.Duration(i*100) * time.Millisecond
		components[i].On("Check").Run(func(args mock.Arguments) {
			time.Sleep(delay)
		}).Return(results[i])

		results[i].On("ComponentName").Return(componentName)
		results[i].On("HealthStates").Return(apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Name: componentName},
		})
	}

	// Launch multiple concurrent requests
	var wg sync.WaitGroup
	for i := range numComponents {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			reqID := fmt.Sprintf("req-%d", idx)
			componentName := fmt.Sprintf("component-%d", idx)
			payload := Request{
				Method:        "triggerComponent",
				ComponentName: componentName,
			}

			session.processRequestAsync(reqID, "triggerComponent", payload)
		}(i)
	}

	// Collect all responses
	responses := make(map[string]*Response)
	for range numComponents {
		select {
		case body := <-writer:
			var response Response
			err := json.Unmarshal(body.Data, &response)
			require.NoError(t, err)
			responses[body.ReqID] = &response

		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for responses")
		}
	}

	// Wait for all goroutines
	wg.Wait()

	// Verify all responses were received
	assert.Len(t, responses, numComponents)
	for i := range numComponents {
		reqID := fmt.Sprintf("req-%d", i)
		assert.Contains(t, responses, reqID)

		response := responses[reqID]
		assert.Len(t, response.States, 1)
		assert.Equal(t, fmt.Sprintf("component-%d", i), response.States[0].Component)
	}

	// Verify all mocks
	registry.AssertExpectations(t)
	for i := range numComponents {
		components[i].AssertExpectations(t)
		results[i].AssertExpectations(t)
	}
}
