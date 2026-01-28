package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/metrics"
)

// TestTriggerComponentCheckWithTagName tests triggerComponentCheck when tagName is used
func TestTriggerComponentCheckWithTagName(t *testing.T) {
	mockey.PatchConvey("trigger component check with tagName", t, func() {
		// Create components with tags
		healthStates := apiv1.HealthStates{
			{
				Health: apiv1.HealthStateTypeHealthy,
				Reason: "Component is healthy",
			},
		}

		mockCheck := &mockCheckResult{
			healthStateType: apiv1.HealthStateTypeHealthy,
			summary:         "Component is healthy",
			healthStates:    healthStates,
			componentName:   "tagged-component",
		}

		comp1 := &mockComponent{
			name:        "tagged-component",
			tags:        []string{"gpu", "nvidia"},
			isSupported: true,
			checkResult: mockCheck,
		}

		comp2 := &mockComponent{
			name:        "untagged-component",
			tags:        []string{"cpu"},
			isSupported: true,
			checkResult: &mockCheckResult{componentName: "untagged-component"},
		}

		handler, _, _ := setupTestHandler([]components.Component{comp1, comp2})
		_, c, w := setupTestRouter()

		// Use tagName instead of componentName
		c.Request = httptest.NewRequest("GET", "/v1/components/trigger-check?tagName=gpu", nil)
		handler.triggerComponentCheck(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentHealthStates
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Should only have the tagged component
		assert.Len(t, response, 1)
		assert.Equal(t, "tagged-component", response[0].Component)
	})
}

// TestTriggerComponentsByTagSuccess tests successful tag-based triggering
func TestTriggerComponentsByTagSuccess(t *testing.T) {
	mockey.PatchConvey("trigger components by tag success", t, func() {
		comp1 := &mockComponent{
			name:        "gpu-comp-1",
			tags:        []string{"gpu", "nvidia"},
			isSupported: true,
			checkResult: &mockCheckResult{
				healthStateType: apiv1.HealthStateTypeHealthy,
				componentName:   "gpu-comp-1",
			},
		}

		comp2 := &mockComponent{
			name:        "gpu-comp-2",
			tags:        []string{"gpu", "amd"},
			isSupported: true,
			checkResult: &mockCheckResult{
				healthStateType: apiv1.HealthStateTypeHealthy,
				componentName:   "gpu-comp-2",
			},
		}

		comp3 := &mockComponent{
			name:        "cpu-comp",
			tags:        []string{"cpu"},
			isSupported: true,
			checkResult: &mockCheckResult{componentName: "cpu-comp"},
		}

		handler, _, _ := setupTestHandler([]components.Component{comp1, comp2, comp3})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/components/trigger-tag?tagName=gpu", nil)
		handler.triggerComponentsByTag(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		components := response["components"].([]interface{})
		assert.Len(t, components, 2)
		assert.Equal(t, true, response["success"])
		assert.Equal(t, float64(0), response["exit"])
	})
}

// TestGetEventsInvalidContentType tests getEvents with invalid content type
func TestGetEventsInvalidContentType(t *testing.T) {
	mockey.PatchConvey("get events invalid content type", t, func() {
		handler, _, _ := setupTestHandler([]components.Component{})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/events", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, "application/xml")
		handler.getEvents(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "invalid content type", response["message"])
	})
}

// TestGetEventsInvalidTimeParams tests getEvents with invalid time parameters
func TestGetEventsInvalidTimeParams(t *testing.T) {
	mockey.PatchConvey("get events invalid time params", t, func() {
		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		// Invalid startTime
		c.Request = httptest.NewRequest("GET", "/v1/events?startTime=invalid", nil)
		handler.getEvents(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "failed to parse time")
	})
}

// TestGetEventsInvalidEndTime tests getEvents with invalid end time
func TestGetEventsInvalidEndTime(t *testing.T) {
	mockey.PatchConvey("get events invalid end time", t, func() {
		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		// Valid startTime but invalid endTime
		c.Request = httptest.NewRequest("GET", "/v1/events?startTime=1234567890&endTime=invalid", nil)
		handler.getEvents(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "failed to parse time")
	})
}

// TestGetEventsWithEventsError tests getEvents when component returns error
func TestGetEventsWithEventsError(t *testing.T) {
	mockey.PatchConvey("get events with events error", t, func() {
		comp := &mockComponent{
			name:        "error-comp",
			isSupported: true,
			eventsError: errors.New("failed to get events"),
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/events", nil)
		handler.getEvents(c)

		// Should still return 200 OK as errors are logged, not returned
		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentEvents
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Component should still be in response, just with empty events
		assert.Len(t, response, 1)
		assert.Equal(t, "error-comp", response[0].Component)
		assert.Empty(t, response[0].Events)
	})
}

// TestGetEventsIndentedJSON tests getEvents with indented JSON
func TestGetEventsIndentedJSON(t *testing.T) {
	mockey.PatchConvey("get events indented JSON", t, func() {
		now := time.Now()
		events := apiv1.Events{
			{
				Time:    metav1.NewTime(now),
				Message: "Test event",
				Type:    apiv1.EventTypeInfo,
			},
		}

		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
			events:      events,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/events", nil)
		c.Request.Header.Set(httputil.RequestHeaderJSONIndent, "true")
		handler.getEvents(c)

		assert.Equal(t, http.StatusOK, w.Code)

		// Check that the response is indented
		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "\n")
		assert.Contains(t, responseBody, "  ")
	})
}

// TestGetInfoInvalidContentType tests getInfo with invalid content type
func TestGetInfoInvalidContentType(t *testing.T) {
	mockey.PatchConvey("get info invalid content type", t, func() {
		handler, _, _ := setupTestHandler([]components.Component{})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, "application/xml")
		handler.getInfo(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "invalid content type", response["message"])
	})
}

// TestGetInfoInvalidSinceParam tests getInfo with invalid since parameter
func TestGetInfoInvalidSinceParam(t *testing.T) {
	mockey.PatchConvey("get info invalid since param", t, func() {
		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info?since=invalid", nil)
		handler.getInfo(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "failed to parse duration")
	})
}

// TestGetInfoInvalidTimeParams tests getInfo with invalid time parameters
func TestGetInfoInvalidTimeParams(t *testing.T) {
	mockey.PatchConvey("get info invalid time params", t, func() {
		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info?startTime=invalid", nil)
		handler.getInfo(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "failed to parse time")
	})
}

// TestGetInfoWithEventsError tests getInfo when component.Events returns error
func TestGetInfoWithEventsError(t *testing.T) {
	mockey.PatchConvey("get info with events error", t, func() {
		healthStates := apiv1.HealthStates{
			{
				Health: apiv1.HealthStateTypeHealthy,
				Reason: "OK",
			},
		}

		comp := &mockComponent{
			name:         "error-comp",
			isSupported:  true,
			healthStates: healthStates,
			eventsError:  errors.New("events error"),
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info", nil)
		handler.getInfo(c)

		// Should still return 200 OK as events error is logged
		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentInfos
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Component should still be in response with states but empty events
		assert.Len(t, response, 1)
		assert.Equal(t, "error-comp", response[0].Component)
		assert.Len(t, response[0].Info.States, 1)
		assert.Empty(t, response[0].Info.Events)
	})
}

// TestGetInfoIndentedJSON tests getInfo with indented JSON
func TestGetInfoIndentedJSON(t *testing.T) {
	mockey.PatchConvey("get info indented JSON", t, func() {
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Reason: "OK"},
		}

		comp := &mockComponent{
			name:         "test-comp",
			isSupported:  true,
			healthStates: healthStates,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info", nil)
		c.Request.Header.Set(httputil.RequestHeaderJSONIndent, "true")
		handler.getInfo(c)

		assert.Equal(t, http.StatusOK, w.Code)

		// Check that the response is indented
		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "\n")
		assert.Contains(t, responseBody, "  ")
	})
}

// TestGetMetricsInvalidContentType tests getMetrics with invalid content type
func TestGetMetricsInvalidContentType(t *testing.T) {
	mockey.PatchConvey("get metrics invalid content type", t, func() {
		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		cfg := &config.Config{}
		store := &mockMetricsStore{metrics: []metrics.Metric{}}

		handler := newGlobalHandler(cfg, registry, store, nil, nil)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/metrics", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, "application/xml")
		handler.getMetrics(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "invalid content type", response["message"])
	})
}

// TestGetMetricsIndentedJSON tests getMetrics with indented JSON
func TestGetMetricsIndentedJSON(t *testing.T) {
	mockey.PatchConvey("get metrics indented JSON", t, func() {
		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
		}

		metricsData := []metrics.Metric{
			{
				UnixMilliseconds: 1234567890000,
				Component:        "test-comp",
				Name:             "test-metric",
				Value:            42.0,
			},
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		cfg := &config.Config{}
		store := &mockMetricsStore{metrics: metricsData}

		handler := newGlobalHandler(cfg, registry, store, nil, nil)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/metrics", nil)
		c.Request.Header.Set(httputil.RequestHeaderJSONIndent, "true")
		handler.getMetrics(c)

		assert.Equal(t, http.StatusOK, w.Code)

		// Check that the response is indented
		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "\n")
		assert.Contains(t, responseBody, "  ")
	})
}

// TestGetHealthStatesIndentedJSON tests getHealthStates with indented JSON
func TestGetHealthStatesIndentedJSON(t *testing.T) {
	mockey.PatchConvey("get health states indented JSON", t, func() {
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Reason: "OK"},
		}

		comp := &mockComponent{
			name:         "test-comp",
			isSupported:  true,
			healthStates: healthStates,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/states", nil)
		c.Request.Header.Set(httputil.RequestHeaderJSONIndent, "true")
		handler.getHealthStates(c)

		assert.Equal(t, http.StatusOK, w.Code)

		// Check that the response is indented
		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "\n")
		assert.Contains(t, responseBody, "  ")
	})
}

// TestGetHealthStatesComponentNotSupported tests getHealthStates when component is not supported
func TestGetHealthStatesComponentNotSupported(t *testing.T) {
	mockey.PatchConvey("get health states component not supported", t, func() {
		// Create a component that is not supported
		comp := &mockComponent{
			name:        "unsupported-comp",
			isSupported: false,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/states", nil)
		handler.getHealthStates(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentHealthStates
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Unsupported component should not be in response
		assert.Len(t, response, 0)
	})
}

// TestGetHealthStatesComponentNotFoundInRegistry tests getHealthStates when component name is in list but not found in registry
func TestGetHealthStatesComponentNotFoundInRegistry(t *testing.T) {
	mockey.PatchConvey("get health states component not found in registry", t, func() {
		// Create handler with a component but then remove it from registry
		comp := &mockComponent{
			name:        "existing-comp",
			isSupported: true,
			healthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeHealthy, Reason: "OK"},
			},
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		cfg := &config.Config{}
		store := &mockMetricsStore{}

		// Create handler - this will capture component names
		handler := newGlobalHandler(cfg, registry, store, nil, nil)

		// Remove the component from registry after handler creation
		registry.Deregister("existing-comp")

		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/states", nil)
		handler.getHealthStates(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentHealthStates
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Component should be in response but with empty states
		assert.Len(t, response, 1)
		assert.Equal(t, "existing-comp", response[0].Component)
		assert.Empty(t, response[0].States)
	})
}

// TestGetHealthStatesWithComponentsParam tests getHealthStates with specific components parameter
func TestGetHealthStatesWithComponentsParam(t *testing.T) {
	mockey.PatchConvey("get health states with components param", t, func() {
		comp1 := &mockComponent{
			name:        "comp1",
			isSupported: true,
			healthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeHealthy, Reason: "Comp1 OK"},
			},
		}

		comp2 := &mockComponent{
			name:        "comp2",
			isSupported: true,
			healthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeUnhealthy, Reason: "Comp2 error"},
			},
		}

		comp3 := &mockComponent{
			name:        "comp3",
			isSupported: true,
			healthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeHealthy, Reason: "Comp3 OK"},
			},
		}

		handler, _, _ := setupTestHandler([]components.Component{comp1, comp2, comp3})
		_, c, w := setupTestRouter()

		// Request only comp1 and comp3
		c.Request = httptest.NewRequest("GET", "/v1/states?components=comp1,comp3", nil)
		handler.getHealthStates(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentHealthStates
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Should have only 2 components
		assert.Len(t, response, 2)

		componentNames := make([]string, 0)
		for _, state := range response {
			componentNames = append(componentNames, state.Component)
		}
		assert.Contains(t, componentNames, "comp1")
		assert.Contains(t, componentNames, "comp3")
		assert.NotContains(t, componentNames, "comp2")
	})
}

// TestGetHealthStatesComponentNotFound tests getHealthStates when specified component not found
func TestGetHealthStatesComponentNotFound(t *testing.T) {
	mockey.PatchConvey("get health states component not found", t, func() {
		comp := &mockComponent{
			name:        "existing-comp",
			isSupported: true,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/states?components=nonexistent", nil)
		handler.getHealthStates(c)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "not found")
	})
}

// TestGetEventsComponentNotFound tests getEvents when specified component not found
func TestGetEventsComponentNotFound(t *testing.T) {
	mockey.PatchConvey("get events component not found", t, func() {
		comp := &mockComponent{
			name:        "existing-comp",
			isSupported: true,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/events?components=nonexistent", nil)
		handler.getEvents(c)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "not found")
	})
}

// TestGetInfoComponentNotFound tests getInfo when specified component not found
func TestGetInfoComponentNotFound(t *testing.T) {
	mockey.PatchConvey("get info component not found", t, func() {
		comp := &mockComponent{
			name:        "existing-comp",
			isSupported: true,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info?components=nonexistent", nil)
		handler.getInfo(c)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "not found")
	})
}

// TestGetMetricsComponentNotFound tests getMetrics when specified component not found
func TestGetMetricsComponentNotFound(t *testing.T) {
	mockey.PatchConvey("get metrics component not found", t, func() {
		comp := &mockComponent{
			name:        "existing-comp",
			isSupported: true,
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		cfg := &config.Config{}
		store := &mockMetricsStore{metrics: []metrics.Metric{}}

		handler := newGlobalHandler(cfg, registry, store, nil, nil)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/metrics?components=nonexistent", nil)
		handler.getMetrics(c)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "not found")
	})
}

// TestDeregisterCanDeregisterFalse tests deregistering when CanDeregister returns false
func TestDeregisterCanDeregisterFalse(t *testing.T) {
	mockey.PatchConvey("deregister can deregister false", t, func() {
		// Component implements Deregisterable but CanDeregister returns false
		comp := &mockComponent{
			name:          "cannot-deregister",
			isSupported:   true,
			canDeregister: false,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("DELETE", "/v1/components?componentName=cannot-deregister", nil)
		handler.deregisterComponent(c)

		// Component can be "retrieved" but CanDeregister returns false
		// The handler will check CanDeregister and return OK since component doesn't implement interface
		// Actually, mockComponent doesn't implement Deregisterable unless canDeregister is checked
		// Let me check the actual behavior - the mockComponent DOES implement CanDeregister()

		// Based on the handler code:
		// 1. It casts to Deregisterable interface
		// 2. If cast fails, returns "component is not deregisterable"
		// 3. If cast succeeds but CanDeregister() returns false, returns same message

		// mockComponent implements CanDeregister() which returns m.canDeregister
		// So it DOES implement the interface, but CanDeregister() returns false
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "component is not deregisterable", response["message"])
	})
}

// TestInfoWithSinceParam tests getInfo with the since parameter
func TestInfoWithSinceParam(t *testing.T) {
	mockey.PatchConvey("get info with since param", t, func() {
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Reason: "OK"},
		}

		comp := &mockComponent{
			name:         "test-comp",
			isSupported:  true,
			healthStates: healthStates,
		}

		metricsData := []metrics.Metric{
			{
				UnixMilliseconds: time.Now().UnixMilli(),
				Component:        "test-comp",
				Name:             "test-metric",
				Value:            42.0,
			},
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		cfg := &config.Config{}
		store := &mockMetricsStore{metrics: metricsData}

		handler := newGlobalHandler(cfg, registry, store, nil, nil)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info?since=1h", nil)
		handler.getInfo(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentInfos
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Len(t, response, 1)
		assert.Equal(t, "test-comp", response[0].Component)
	})
}

// TestGetEventsComponentNotSupported tests getEvents when component is not supported
func TestGetEventsComponentNotSupported(t *testing.T) {
	mockey.PatchConvey("get events component not supported", t, func() {
		comp := &mockComponent{
			name:        "unsupported-comp",
			isSupported: false,
			events: apiv1.Events{
				{Time: metav1.NewTime(time.Now()), Message: "Should not appear", Type: apiv1.EventTypeInfo},
			},
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/events", nil)
		handler.getEvents(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentEvents
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Unsupported component should not be in response
		assert.Len(t, response, 0)
	})
}

// TestGetInfoComponentNotSupported tests getInfo when component is not supported
func TestGetInfoComponentNotSupported(t *testing.T) {
	mockey.PatchConvey("get info component not supported", t, func() {
		comp := &mockComponent{
			name:        "unsupported-comp",
			isSupported: false,
			healthStates: apiv1.HealthStates{
				{Health: apiv1.HealthStateTypeHealthy, Reason: "Should not appear"},
			},
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info", nil)
		handler.getInfo(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentInfos
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Unsupported component should not be in response
		assert.Len(t, response, 0)
	})
}

// TestGetInfoMetricsStoreError tests getInfo when metrics store returns error
func TestGetInfoMetricsStoreError(t *testing.T) {
	mockey.PatchConvey("get info metrics store error", t, func() {
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Reason: "OK"},
		}

		comp := &mockComponent{
			name:         "test-comp",
			isSupported:  true,
			healthStates: healthStates,
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		cfg := &config.Config{}
		store := &mockMetricsStore{err: errors.New("metrics store error")}

		handler := newGlobalHandler(cfg, registry, store, nil, nil)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info", nil)
		handler.getInfo(c)

		// Should still return 200 OK as metrics error is logged
		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentInfos
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Component should be in response but with empty metrics
		assert.Len(t, response, 1)
		assert.Empty(t, response[0].Info.Metrics)
	})
}

// TestYAMLMarshalError tests YAML marshal error handling using mockey
func TestYAMLMarshalErrorInGetStates(t *testing.T) {
	mockey.PatchConvey("yaml marshal error in get states", t, func() {
		mockey.Mock(yaml.Marshal).To(func(v interface{}) ([]byte, error) {
			return nil, errors.New("yaml marshal error")
		}).Build()

		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Reason: "OK"},
		}

		comp := &mockComponent{
			name:         "test-comp",
			isSupported:  true,
			healthStates: healthStates,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/states", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)
		handler.getHealthStates(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "failed to marshal states")
	})
}

// TestYAMLMarshalErrorInGetEvents tests YAML marshal error in getEvents
func TestYAMLMarshalErrorInGetEvents(t *testing.T) {
	mockey.PatchConvey("yaml marshal error in get events", t, func() {
		mockey.Mock(yaml.Marshal).To(func(v interface{}) ([]byte, error) {
			return nil, errors.New("yaml marshal error")
		}).Build()

		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
			events: apiv1.Events{
				{Time: metav1.NewTime(time.Now()), Message: "Test", Type: apiv1.EventTypeInfo},
			},
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/events", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)
		handler.getEvents(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "failed to marshal events")
	})
}

// TestYAMLMarshalErrorInGetInfo tests YAML marshal error in getInfo
func TestYAMLMarshalErrorInGetInfo(t *testing.T) {
	mockey.PatchConvey("yaml marshal error in get info", t, func() {
		mockey.Mock(yaml.Marshal).To(func(v interface{}) ([]byte, error) {
			return nil, errors.New("yaml marshal error")
		}).Build()

		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)
		handler.getInfo(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "failed to marshal infos")
	})
}

// TestYAMLMarshalErrorInGetMetrics tests YAML marshal error in getMetrics
func TestYAMLMarshalErrorInGetMetrics(t *testing.T) {
	mockey.PatchConvey("yaml marshal error in get metrics", t, func() {
		mockey.Mock(yaml.Marshal).To(func(v interface{}) ([]byte, error) {
			return nil, errors.New("yaml marshal error")
		}).Build()

		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
		}

		metricsData := []metrics.Metric{
			{
				UnixMilliseconds: 1234567890000,
				Component:        "test-comp",
				Name:             "test-metric",
				Value:            42.0,
			},
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		cfg := &config.Config{}
		store := &mockMetricsStore{metrics: metricsData}

		handler := newGlobalHandler(cfg, registry, store, nil, nil)
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/metrics", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)
		handler.getMetrics(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "failed to marshal metrics")
	})
}

// TestYAMLMarshalErrorInGetComponents tests YAML marshal error in getComponents
func TestYAMLMarshalErrorInGetComponents(t *testing.T) {
	mockey.PatchConvey("yaml marshal error in get components", t, func() {
		mockey.Mock(yaml.Marshal).To(func(v interface{}) ([]byte, error) {
			return nil, errors.New("yaml marshal error")
		}).Build()

		comp := &mockComponent{
			name:        "test-comp",
			isSupported: true,
		}

		handler, _, _ := setupTestHandler([]components.Component{comp})
		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/components", nil)
		c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)
		handler.getComponents(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "failed to marshal components")
	})
}

// TestGetEventsComponentNotFoundInRegistry tests when component is in list but removed from registry
func TestGetEventsComponentNotFoundInRegistry(t *testing.T) {
	mockey.PatchConvey("get events component not found in registry", t, func() {
		comp := &mockComponent{
			name:        "existing-comp",
			isSupported: true,
			events: apiv1.Events{
				{Time: metav1.NewTime(time.Now()), Message: "Test", Type: apiv1.EventTypeInfo},
			},
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		cfg := &config.Config{}
		store := &mockMetricsStore{}

		handler := newGlobalHandler(cfg, registry, store, nil, nil)

		// Remove component after handler creation
		registry.Deregister("existing-comp")

		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/events", nil)
		handler.getEvents(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentEvents
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Component should be in response but with empty events
		assert.Len(t, response, 1)
		assert.Equal(t, "existing-comp", response[0].Component)
		assert.Empty(t, response[0].Events)
	})
}

// TestGetInfoComponentNotFoundInRegistry tests when component is in list but removed from registry
func TestGetInfoComponentNotFoundInRegistry(t *testing.T) {
	mockey.PatchConvey("get info component not found in registry", t, func() {
		comp := &mockComponent{
			name:         "existing-comp",
			isSupported:  true,
			healthStates: apiv1.HealthStates{{Health: apiv1.HealthStateTypeHealthy, Reason: "OK"}},
		}

		registry := newMockRegistry()
		registry.AddMockComponent(comp)

		cfg := &config.Config{}
		store := &mockMetricsStore{}

		handler := newGlobalHandler(cfg, registry, store, nil, nil)

		// Remove component after handler creation
		registry.Deregister("existing-comp")

		_, c, w := setupTestRouter()

		c.Request = httptest.NewRequest("GET", "/v1/info", nil)
		handler.getInfo(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response apiv1.GPUdComponentInfos
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Component should be in response but with empty info
		assert.Len(t, response, 1)
		assert.Equal(t, "existing-comp", response[0].Component)
		assert.Empty(t, response[0].Info.States)
		assert.Empty(t, response[0].Info.Events)
	})
}
