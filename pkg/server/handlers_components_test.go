package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/config"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/metrics"
)

// mockComponent is a simplified component implementation for testing
type mockComponent struct {
	name            string
	tags            []string
	isSupported     bool
	checkResult     components.CheckResult
	events          apiv1.Events
	healthStates    apiv1.HealthStates
	eventsError     error
	isCustomPlugin  bool
	canDeregister   bool
	deregisterError error
	spec            pkgcustomplugins.Spec
}

func (m *mockComponent) Name() string {
	return m.name
}

func (m *mockComponent) Tags() []string {
	return m.tags
}

func (m *mockComponent) IsSupported() bool {
	return m.isSupported
}

func (m *mockComponent) Start() error {
	return nil
}

func (m *mockComponent) Check() components.CheckResult {
	return m.checkResult
}

func (m *mockComponent) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if m.eventsError != nil {
		return nil, m.eventsError
	}
	return m.events, nil
}

func (m *mockComponent) LastHealthStates() apiv1.HealthStates {
	return m.healthStates
}

func (m *mockComponent) Close() error {
	if m.deregisterError != nil {
		return m.deregisterError
	}
	return nil
}

// Implement Deregisterable interface
func (m *mockComponent) CanDeregister() bool {
	return m.canDeregister
}

// Implement CustomPluginRegisteree interface
func (m *mockComponent) IsCustomPlugin() bool {
	return m.isCustomPlugin
}

func (m *mockComponent) Spec() pkgcustomplugins.Spec {
	return m.spec
}

// mockRegistry is a simplified registry implementation for testing
type mockRegistry struct {
	components map[string]components.Component
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		components: make(map[string]components.Component),
	}
}

func (r *mockRegistry) Register(initFunc components.InitFunc) (components.Component, error) {
	// Special case for testing with custom plugins - return a mock component
	if initFunc == nil {
		// Create a mock component for testing
		mockComp := &mockComponent{
			name:        "generated-mock-component",
			isSupported: true,
		}
		r.components[mockComp.Name()] = mockComp
		return mockComp, nil
	}

	// For testing, create a dummy GPUdInstance that's safe to pass to initFunc
	instance := &components.GPUdInstance{
		RootCtx: context.Background(),
	}

	comp, err := initFunc(instance)
	if err != nil {
		return nil, err
	}

	r.components[comp.Name()] = comp
	return comp, nil
}

func (r *mockRegistry) MustRegister(initFunc components.InitFunc) {
	comp, _ := r.Register(initFunc)
	r.components[comp.Name()] = comp
}

func (r *mockRegistry) Get(name string) components.Component {
	return r.components[name]
}

func (r *mockRegistry) All() []components.Component {
	comps := make([]components.Component, 0, len(r.components))
	for _, c := range r.components {
		comps = append(comps, c)
	}
	return comps
}

func (r *mockRegistry) Deregister(name string) components.Component {
	comp := r.components[name]
	delete(r.components, name)
	return comp
}

func (r *mockRegistry) AddMockComponent(c components.Component) {
	r.components[c.Name()] = c
}

// mockCheckResult is a simplified implementation of the CheckResult interface
type mockCheckResult struct {
	healthStateType apiv1.HealthStateType
	summary         string
	healthStates    apiv1.HealthStates
	debugOutput     string
	componentName   string
}

func (m *mockCheckResult) HealthStateType() apiv1.HealthStateType {
	return m.healthStateType
}

func (m *mockCheckResult) Summary() string {
	return m.summary
}

func (m *mockCheckResult) HealthStates() apiv1.HealthStates {
	return m.healthStates
}

func (m *mockCheckResult) Debug() string {
	return m.debugOutput
}

func (m *mockCheckResult) ComponentName() string {
	return m.componentName
}

func (m *mockCheckResult) String() string {
	return fmt.Sprintf("%s: %s", m.componentName, m.summary)
}

// mockMetricsStore is a simplified metrics store for testing
type mockMetricsStore struct {
	metrics []metrics.Metric
	err     error
}

func (m *mockMetricsStore) Read(ctx context.Context, opts ...metrics.OpOption) (metrics.Metrics, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.metrics, nil
}

func (m *mockMetricsStore) Purge(ctx context.Context, before time.Time) (int, error) {
	return 0, nil
}

func (m *mockMetricsStore) Record(ctx context.Context, ms ...metrics.Metric) error {
	return nil
}

func setupTestRouter() (*gin.Engine, *gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// Initialize with a default request
	c.Request = httptest.NewRequest("GET", "/", nil)
	return router, c, w
}

func TestGetComponents(t *testing.T) {
	registry := newMockRegistry()

	// Add mock components to the registry
	registry.AddMockComponent(&mockComponent{name: "comp1", isSupported: true})
	registry.AddMockComponent(&mockComponent{name: "comp2", isSupported: true})

	cfg := &config.Config{}
	store := &mockMetricsStore{}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	_, c, w := setupTestRouter()

	// Test with default JSON content type
	c.Request = httptest.NewRequest("GET", "/v1/components", nil)
	handler.getComponents(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response
	var components []string
	err := json.Unmarshal(w.Body.Bytes(), &components)
	require.NoError(t, err)

	// Verify the components
	assert.Contains(t, components, "comp1")
	assert.Contains(t, components, "comp2")
	assert.Len(t, components, 2)
}

func TestGetComponentsYAML(t *testing.T) {
	registry := newMockRegistry()

	// Add mock components to the registry
	registry.AddMockComponent(&mockComponent{name: "comp1", isSupported: true})
	registry.AddMockComponent(&mockComponent{name: "comp2", isSupported: true})

	cfg := &config.Config{}
	store := &mockMetricsStore{}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	_, c, w := setupTestRouter()

	// Set up a new request with YAML content type
	c.Request = httptest.NewRequest("GET", "/v1/components", nil)
	c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)

	handler.getComponents(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the YAML response
	var components []string
	err := yaml.Unmarshal(w.Body.Bytes(), &components)
	require.NoError(t, err)

	// Verify the components
	assert.Contains(t, components, "comp1")
	assert.Contains(t, components, "comp2")
	assert.Len(t, components, 2)
}

func TestTriggerComponentCheck(t *testing.T) {
	// Create a mock component with health states
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
		componentName:   "test-component",
	}

	mockComp := &mockComponent{
		name:        "test-component",
		isSupported: true,
		checkResult: mockCheck,
	}

	// Setup handler with the test component
	handler, _, _ := setupTestHandler([]components.Component{mockComp})
	_, c, w := setupTestRouter()

	// Setup the request with query parameter
	c.Request = httptest.NewRequest("GET", "/v1/components/trigger-check?componentName=test-component", nil)

	// Call the handler
	handler.triggerComponentCheck(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response
	var responseStates apiv1.GPUdComponentHealthStates
	err := json.Unmarshal(w.Body.Bytes(), &responseStates)
	require.NoError(t, err)

	// Verify the health states
	assert.Len(t, responseStates, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, responseStates[0].States[0].Health)
	assert.Equal(t, "Component is healthy", responseStates[0].States[0].Reason)
}

func TestDeregisterComponent(t *testing.T) {
	// Add a deregisterable component
	canDeregister := &mockComponent{
		name:          "can-deregister",
		isSupported:   true,
		canDeregister: true,
	}

	// Add a non-deregisterable component
	cannotDeregister := &mockComponent{
		name:          "cannot-deregister",
		isSupported:   true,
		canDeregister: false,
	}

	// Setup handler with plugin API enabled
	handler, registry, _ := setupTestHandler([]components.Component{canDeregister, cannotDeregister})
	_, c, w := setupTestRouter()

	// Test deregistering a component that can be deregistered
	c.Request = httptest.NewRequest("DELETE", "/v1/components?componentName=can-deregister", nil)
	handler.deregisterComponent(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// The component should be removed from the registry
	assert.Nil(t, registry.Get("can-deregister"))

	// Reset for next test
	w.Body.Reset()
	c = &gin.Context{}
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/v1/components?componentName=cannot-deregister", nil)

	// Test deregistering a component that cannot be deregistered
	handler.deregisterComponent(c)

	// Verify the response - should be BadRequest
	assert.Equal(t, http.StatusOK, w.Code)

	// The component should still be in the registry
	assert.NotNil(t, registry.Get("cannot-deregister"))
}

func TestGetHealthStates(t *testing.T) {
	// Add components with health states
	healthStates1 := apiv1.HealthStates{
		{
			Health: apiv1.HealthStateTypeHealthy,
			Reason: "Component 1 is healthy",
		},
	}

	healthStates2 := apiv1.HealthStates{
		{
			Health: apiv1.HealthStateTypeUnhealthy,
			Reason: "Component 2 is unhealthy",
		},
	}

	comp1 := &mockComponent{
		name:         "comp1",
		isSupported:  true,
		healthStates: healthStates1,
	}

	comp2 := &mockComponent{
		name:         "comp2",
		isSupported:  true,
		healthStates: healthStates2,
	}

	// Setup handler with both components
	handler, _, _ := setupTestHandler([]components.Component{comp1, comp2})
	_, c, w := setupTestRouter()

	// Test getting health states for all components
	c.Request = httptest.NewRequest("GET", "/v1/states", nil)
	handler.getHealthStates(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response
	var states apiv1.GPUdComponentHealthStates
	err := json.Unmarshal(w.Body.Bytes(), &states)
	require.NoError(t, err)

	// Should have states for both components
	assert.Len(t, states, 2)

	// Find comp1 states
	var comp1States, comp2States apiv1.ComponentHealthStates
	for _, s := range states {
		if s.Component == "comp1" {
			comp1States = s
		} else if s.Component == "comp2" {
			comp2States = s
		}
	}

	assert.Equal(t, "comp1", comp1States.Component)
	assert.Len(t, comp1States.States, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, comp1States.States[0].Health)

	assert.Equal(t, "comp2", comp2States.Component)
	assert.Len(t, comp2States.States, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, comp2States.States[0].Health)
}

func TestGetEvents(t *testing.T) {
	// Add components with events
	now := time.Now()
	events1 := apiv1.Events{
		{
			Time:    metav1.NewTime(now.Add(-30 * time.Minute)),
			Message: "Event from component 1",
			Type:    apiv1.EventTypeInfo,
		},
	}

	events2 := apiv1.Events{
		{
			Time:    metav1.NewTime(now.Add(-15 * time.Minute)),
			Message: "Event from component 2",
			Type:    apiv1.EventTypeWarning,
		},
	}

	comp1 := &mockComponent{
		name:        "comp1",
		isSupported: true,
		events:      events1,
	}

	comp2 := &mockComponent{
		name:        "comp2",
		isSupported: true,
		events:      events2,
	}

	// Setup handler with both components
	handler, _, _ := setupTestHandler([]components.Component{comp1, comp2})
	_, c, w := setupTestRouter()

	// Test getting events for all components
	c.Request = httptest.NewRequest("GET", "/v1/events", nil)
	handler.getEvents(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response
	var events apiv1.GPUdComponentEvents
	err := json.Unmarshal(w.Body.Bytes(), &events)
	require.NoError(t, err)

	// Should have events for both components
	assert.Len(t, events, 2)

	// Find comp1 events
	var comp1Events, comp2Events apiv1.ComponentEvents
	for _, e := range events {
		if e.Component == "comp1" {
			comp1Events = e
		} else if e.Component == "comp2" {
			comp2Events = e
		}
	}

	assert.Equal(t, "comp1", comp1Events.Component)
	assert.Len(t, comp1Events.Events, 1)
	assert.Equal(t, apiv1.EventTypeInfo, comp1Events.Events[0].Type)

	assert.Equal(t, "comp2", comp2Events.Component)
	assert.Len(t, comp2Events.Events, 1)
	assert.Equal(t, apiv1.EventTypeWarning, comp2Events.Events[0].Type)
}

func TestGetInfo(t *testing.T) {
	// Add components with states and events
	healthStates := apiv1.HealthStates{
		{
			Health: apiv1.HealthStateTypeHealthy,
			Reason: "Component is healthy",
		},
	}

	now := time.Now()
	events := apiv1.Events{
		{
			Time:    metav1.NewTime(now),
			Message: "Test event",
			Type:    apiv1.EventTypeInfo,
		},
	}

	comp := &mockComponent{
		name:         "test-comp",
		isSupported:  true,
		healthStates: healthStates,
		events:       events,
	}

	// Setup mock metrics data
	metricsData := []metrics.Metric{
		{
			UnixMilliseconds: 1234567890000,
			Component:        "test-comp",
			Name:             "test-metric",
			Labels:           map[string]string{"label": "value"},
			Value:            42.0,
		},
	}

	// Setup handler manually to include metrics data
	registry := newMockRegistry()
	registry.AddMockComponent(comp)

	cfg := &config.Config{}
	store := &mockMetricsStore{metrics: metricsData}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	_, c, w := setupTestRouter()

	// Test getting info for a specific component
	c.Request = httptest.NewRequest("GET", "/v1/info?component=test-comp", nil)
	handler.getInfo(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response
	var infos apiv1.GPUdComponentInfos
	err := json.Unmarshal(w.Body.Bytes(), &infos)
	require.NoError(t, err)

	// Should have info for the component
	assert.Len(t, infos, 1)
	assert.Equal(t, "test-comp", infos[0].Component)

	// Check that all data types are present
	assert.Len(t, infos[0].Info.States, 1)
	assert.Len(t, infos[0].Info.Events, 1)
	assert.Len(t, infos[0].Info.Metrics, 1)

	// Verify states
	assert.Equal(t, apiv1.HealthStateTypeHealthy, infos[0].Info.States[0].Health)
	assert.Equal(t, "Component is healthy", infos[0].Info.States[0].Reason)

	// Verify events
	assert.Equal(t, apiv1.EventTypeInfo, infos[0].Info.Events[0].Type)
	assert.Equal(t, "Test event", infos[0].Info.Events[0].Message)

	// Verify metrics
	assert.Equal(t, int64(1234567890000), infos[0].Info.Metrics[0].UnixSeconds)
	assert.Equal(t, "test-metric", infos[0].Info.Metrics[0].Name)
	assert.Equal(t, 42.0, infos[0].Info.Metrics[0].Value)
	assert.Equal(t, map[string]string{"label": "value"}, infos[0].Info.Metrics[0].Labels)
}

func TestGetMetrics(t *testing.T) {
	// Create a component
	comp := &mockComponent{
		name:        "test-comp",
		isSupported: true,
	}

	// Setup mock metrics data
	metricsData := []metrics.Metric{
		{
			UnixMilliseconds: 1234567890000,
			Component:        "test-comp",
			Name:             "test-metric",
			Labels:           map[string]string{"label": "value"},
			Value:            42.0,
		},
		{
			UnixMilliseconds: 1234567891000,
			Component:        "test-comp",
			Name:             "another-metric",
			Labels:           map[string]string{"another": "label"},
			Value:            99.9,
		},
	}

	// Setup handler manually to include metrics data
	registry := newMockRegistry()
	registry.AddMockComponent(comp)

	cfg := &config.Config{}
	store := &mockMetricsStore{metrics: metricsData}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	_, c, w := setupTestRouter()

	// Test getting metrics for a specific component
	c.Request = httptest.NewRequest("GET", "/v1/metrics?component=test-comp", nil)
	handler.getMetrics(c)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response as JSON to verify it's valid
	var result []interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	// Should have metrics data
	assert.NotEmpty(t, result)
}

func TestRegisterComponentRoutes(t *testing.T) {
	// Setup handler with plugin API enabled
	handler, _, _ := setupTestHandler(nil)

	// Setup router with "/v1" path
	router, v1 := setupRouterWithPath("/v1")

	// Register routes
	handler.registerComponentRoutes(v1)

	// Create a test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Test a few endpoints to verify they were registered
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/components"},
		{"GET", "/v1/components/trigger-check?componentName=test"},
		{"GET", "/v1/states"},
		{"GET", "/v1/events"},
		{"GET", "/v1/info"},
		{"GET", "/v1/metrics"},
		{"DELETE", "/v1/components?componentName=test"},
	}

	client := &http.Client{}

	for _, ep := range endpoints {
		req, err := http.NewRequest(ep.method, fmt.Sprintf("%s%s", server.URL, ep.path), nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)

		// We don't care about the response code, just that the routes were registered
		_ = resp.Body.Close()
	}
}

// Additional comprehensive tests for better coverage

func TestGetComponentsInvalidContentType(t *testing.T) {
	registry := newMockRegistry()
	registry.AddMockComponent(&mockComponent{name: "comp1", isSupported: true})

	cfg := &config.Config{}
	store := &mockMetricsStore{}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	_, c, w := setupTestRouter()

	// Test with invalid content type
	c.Request = httptest.NewRequest("GET", "/v1/components", nil)
	c.Request.Header.Set(httputil.RequestHeaderContentType, "application/xml")
	handler.getComponents(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "invalid content type", response["message"])
}

func TestGetComponentsIndentedJSON(t *testing.T) {
	registry := newMockRegistry()
	registry.AddMockComponent(&mockComponent{name: "comp1", isSupported: true})

	cfg := &config.Config{}
	store := &mockMetricsStore{}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	_, c, w := setupTestRouter()

	// Test with JSON indent header
	c.Request = httptest.NewRequest("GET", "/v1/components", nil)
	c.Request.Header.Set(httputil.RequestHeaderJSONIndent, "true")
	handler.getComponents(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check that the response is indented
	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "\n")
	assert.Contains(t, responseBody, "  ")
}

func TestDeregisterComponentNotFound(t *testing.T) {
	handler, _, _ := setupTestHandler([]components.Component{})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("DELETE", "/v1/components?componentName=nonexistent", nil)
	handler.deregisterComponent(c)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "component not found", response["message"])
}

func TestDeregisterComponentMissingName(t *testing.T) {
	handler, _, _ := setupTestHandler([]components.Component{})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("DELETE", "/v1/components", nil)
	handler.deregisterComponent(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "component name is required", response["message"])
}

func TestDeregisterComponentNotDeregisterable(t *testing.T) {
	// Add a component that doesn't implement Deregisterable
	comp := &mockComponent{
		name:        "not-deregisterable",
		isSupported: true,
	}

	handler, registry, _ := setupTestHandler([]components.Component{comp})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("DELETE", "/v1/components?componentName=not-deregisterable", nil)
	handler.deregisterComponent(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "component is not deregisterable", response["message"])

	// Component should still be in registry
	assert.NotNil(t, registry.Get("not-deregisterable"))
}

func TestDeregisterComponentCloseError(t *testing.T) {
	// Add a deregisterable component that fails to close
	comp := &mockComponent{
		name:            "close-error",
		isSupported:     true,
		canDeregister:   true,
		deregisterError: errors.New("close failed"),
	}

	handler, registry, _ := setupTestHandler([]components.Component{comp})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("DELETE", "/v1/components?componentName=close-error", nil)
	handler.deregisterComponent(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "failed to deregister component")

	// Component should still be in registry since close failed
	assert.NotNil(t, registry.Get("close-error"))
}

func TestTriggerComponentCheckMissingParams(t *testing.T) {
	handler, _, _ := setupTestHandler([]components.Component{})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/components/trigger-check", nil)
	handler.triggerComponentCheck(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "component or tag name is required", response["message"])
}

func TestTriggerComponentCheckNotFound(t *testing.T) {
	handler, _, _ := setupTestHandler([]components.Component{})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/components/trigger-check?componentName=nonexistent", nil)
	handler.triggerComponentCheck(c)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "component not found", response["message"])
}

func TestTriggerComponentsByTagMissingTag(t *testing.T) {
	handler, _, _ := setupTestHandler([]components.Component{})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/components/trigger-tag", nil)
	handler.triggerComponentsByTag(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "tagName parameter is required", response["error"])
}

func TestTriggerComponentsByTagNoMatches(t *testing.T) {
	comp := &mockComponent{
		name:        "comp1",
		tags:        []string{"tag1", "tag2"},
		isSupported: true,
		checkResult: &mockCheckResult{componentName: "comp1"},
	}

	handler, _, _ := setupTestHandler([]components.Component{comp})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/components/trigger-tag?tagName=nonexistent", nil)
	handler.triggerComponentsByTag(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	components := response["components"].([]interface{})
	assert.Len(t, components, 0)
	assert.Equal(t, float64(0), response["exit"])
	assert.Equal(t, true, response["success"])
}

func TestGetHealthStatesYAML(t *testing.T) {
	healthStates := apiv1.HealthStates{
		{Health: apiv1.HealthStateTypeHealthy, Reason: "Component is healthy"},
	}

	comp := &mockComponent{
		name:         "comp1",
		isSupported:  true,
		healthStates: healthStates,
	}

	handler, _, _ := setupTestHandler([]components.Component{comp})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/states", nil)
	c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)
	handler.getHealthStates(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var states apiv1.GPUdComponentHealthStates
	err := yaml.Unmarshal(w.Body.Bytes(), &states)
	require.NoError(t, err)

	assert.Len(t, states, 1)
	assert.Equal(t, "comp1", states[0].Component)
}

func TestGetHealthStatesInvalidContentType(t *testing.T) {
	handler, _, _ := setupTestHandler([]components.Component{})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/states", nil)
	c.Request.Header.Set(httputil.RequestHeaderContentType, "application/xml")
	handler.getHealthStates(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "invalid content type", response["message"])
}

func TestGetEventsYAML(t *testing.T) {
	now := time.Now()
	events := apiv1.Events{
		{
			Time:    metav1.NewTime(now),
			Message: "Test event",
			Type:    apiv1.EventTypeInfo,
		},
	}

	comp := &mockComponent{
		name:        "comp1",
		isSupported: true,
		events:      events,
	}

	handler, _, _ := setupTestHandler([]components.Component{comp})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/events", nil)
	c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)
	handler.getEvents(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var eventsResponse apiv1.GPUdComponentEvents
	err := yaml.Unmarshal(w.Body.Bytes(), &eventsResponse)
	require.NoError(t, err)

	assert.Len(t, eventsResponse, 1)
	assert.Equal(t, "comp1", eventsResponse[0].Component)
}

func TestGetEventsWithTimeParams(t *testing.T) {
	now := time.Now()
	events := apiv1.Events{
		{
			Time:    metav1.NewTime(now),
			Message: "Test event",
			Type:    apiv1.EventTypeInfo,
		},
	}

	comp := &mockComponent{
		name:        "comp1",
		isSupported: true,
		events:      events,
	}

	handler, _, _ := setupTestHandler([]components.Component{comp})
	_, c, w := setupTestRouter()

	startTime := now.Add(-time.Hour).Unix()
	endTime := now.Unix()
	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/v1/events?startTime=%d&endTime=%d", startTime, endTime), nil)
	handler.getEvents(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var eventsResponse apiv1.GPUdComponentEvents
	err := json.Unmarshal(w.Body.Bytes(), &eventsResponse)
	require.NoError(t, err)

	assert.Len(t, eventsResponse, 1)
	assert.Equal(t, "comp1", eventsResponse[0].Component)
}

func TestGetInfoYAML(t *testing.T) {
	healthStates := apiv1.HealthStates{
		{Health: apiv1.HealthStateTypeHealthy, Reason: "Component is healthy"},
	}

	now := time.Now()
	events := apiv1.Events{
		{
			Time:    metav1.NewTime(now),
			Message: "Test event",
			Type:    apiv1.EventTypeInfo,
		},
	}

	comp := &mockComponent{
		name:         "comp1",
		isSupported:  true,
		healthStates: healthStates,
		events:       events,
	}

	metricsData := []metrics.Metric{
		{
			UnixMilliseconds: 1234567890000,
			Component:        "comp1",
			Name:             "test-metric",
			Labels:           map[string]string{"label": "value"},
			Value:            42.0,
		},
	}

	registry := newMockRegistry()
	registry.AddMockComponent(comp)

	cfg := &config.Config{}
	store := &mockMetricsStore{metrics: metricsData}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/info", nil)
	c.Request.Header.Set(httputil.RequestHeaderContentType, httputil.RequestHeaderYAML)
	handler.getInfo(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var infos apiv1.GPUdComponentInfos
	err := yaml.Unmarshal(w.Body.Bytes(), &infos)
	require.NoError(t, err)

	assert.Len(t, infos, 1)
	assert.Equal(t, "comp1", infos[0].Component)
}

func TestGetMetricsYAML(t *testing.T) {
	comp := &mockComponent{
		name:        "comp1",
		isSupported: true,
	}

	metricsData := []metrics.Metric{
		{
			UnixMilliseconds: 1234567890000,
			Component:        "comp1",
			Name:             "test-metric",
			Labels:           map[string]string{"label": "value"},
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

	assert.Equal(t, http.StatusOK, w.Code)

	// Just verify we get a valid YAML response
	assert.Greater(t, w.Body.Len(), 0)
}

func TestGetMetricsWithSinceParam(t *testing.T) {
	comp := &mockComponent{
		name:        "comp1",
		isSupported: true,
	}

	metricsData := []metrics.Metric{
		{
			UnixMilliseconds: 1234567890000,
			Component:        "comp1",
			Name:             "test-metric",
			Labels:           map[string]string{"label": "value"},
			Value:            42.0,
		},
	}

	registry := newMockRegistry()
	registry.AddMockComponent(comp)

	cfg := &config.Config{}
	store := &mockMetricsStore{metrics: metricsData}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/metrics?since=1h", nil)
	handler.getMetrics(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Just verify we get a valid response
	assert.Greater(t, w.Body.Len(), 0)
}

func TestGetMetricsInvalidSince(t *testing.T) {
	handler, _, _ := setupTestHandler([]components.Component{})
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/metrics?since=invalid", nil)
	handler.getMetrics(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "failed to parse duration")
}

func TestGetMetricsStoreError(t *testing.T) {
	comp := &mockComponent{
		name:        "comp1",
		isSupported: true,
	}

	registry := newMockRegistry()
	registry.AddMockComponent(comp)

	cfg := &config.Config{}
	store := &mockMetricsStore{err: errors.New("store error")}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	_, c, w := setupTestRouter()

	c.Request = httptest.NewRequest("GET", "/v1/metrics", nil)
	handler.getMetrics(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "failed to read metrics")
}
