package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/metrics"
)

// Mock implementations
type mockComponent struct {
	mock.Mock
}

func (m *mockComponent) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockComponent) Tags() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

func (m *mockComponent) IsSupported() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *mockComponent) Check() components.CheckResult {
	args := m.Called()
	return args.Get(0).(components.CheckResult)
}

func (m *mockComponent) LastHealthStates() apiv1.HealthStates {
	args := m.Called()
	return args.Get(0).(apiv1.HealthStates)
}

func (m *mockComponent) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	args := m.Called(ctx, since)
	return args.Get(0).(apiv1.Events), args.Error(1)
}

func (m *mockComponent) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockComponent) Start() error {
	args := m.Called()
	return args.Error(0)
}

type mockComponentRegistry struct {
	mock.Mock
}

func (m *mockComponentRegistry) MustRegister(initFunc components.InitFunc) {
	m.Called(initFunc)
}

func (m *mockComponentRegistry) Register(initFunc components.InitFunc) (components.Component, error) {
	args := m.Called(initFunc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(components.Component), args.Error(1)
}

func (m *mockComponentRegistry) Get(name string) components.Component {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(components.Component)
}

func (m *mockComponentRegistry) Deregister(name string) components.Component {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(components.Component)
}

func (m *mockComponentRegistry) All() []components.Component {
	args := m.Called()
	return args.Get(0).([]components.Component)
}

type mockDeregisterableComponent struct {
	mockComponent
}

func (m *mockDeregisterableComponent) CanDeregister() bool {
	args := m.Called()
	return args.Bool(0)
}

type mockHealthSettableComponent struct {
	mockComponent
}

func (m *mockHealthSettableComponent) SetHealthy() error {
	args := m.Called()
	return args.Error(0)
}

type mockCustomPluginRegistereeComponent struct {
	mockComponent
}

func (m *mockCustomPluginRegistereeComponent) IsCustomPlugin() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *mockCustomPluginRegistereeComponent) Spec() pkgcustomplugins.Spec {
	args := m.Called()
	return args.Get(0).(pkgcustomplugins.Spec)
}

type mockCheckResult struct {
	mock.Mock
}

func (m *mockCheckResult) ComponentName() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockCheckResult) String() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockCheckResult) Summary() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockCheckResult) HealthStateType() apiv1.HealthStateType {
	args := m.Called()
	return args.Get(0).(apiv1.HealthStateType)
}

func (m *mockCheckResult) HealthStates() apiv1.HealthStates {
	args := m.Called()
	return args.Get(0).(apiv1.HealthStates)
}

type mockMetricsStore struct {
	mock.Mock
}

func (m *mockMetricsStore) Record(ctx context.Context, ms ...metrics.Metric) error {
	args := m.Called(ctx, ms)
	return args.Error(0)
}

func (m *mockMetricsStore) Read(ctx context.Context, opts ...metrics.OpOption) (metrics.Metrics, error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(metrics.Metrics), args.Error(1)
}

func (m *mockMetricsStore) Purge(ctx context.Context, before time.Time) (int, error) {
	args := m.Called(ctx, before)
	return args.Int(0), args.Error(1)
}

type mockProcessRunner struct {
	mock.Mock
}

func (m *mockProcessRunner) RunUntilCompletion(ctx context.Context, command string) ([]byte, int32, error) {
	args := m.Called(ctx, command)
	return args.Get(0).([]byte), int32(args.Int(1)), args.Error(2)
}

// Helper functions for testing
func setupTestSession() (*Session, *mockComponentRegistry, *mockMetricsStore, *mockProcessRunner, chan Body, chan Body) {
	reader := make(chan Body, 10)
	writer := make(chan Body, 10)
	componentsRegistry := new(mockComponentRegistry)
	metricsStore := new(mockMetricsStore)
	processRunner := new(mockProcessRunner)

	session := &Session{
		reader:             reader,
		writer:             writer,
		componentsRegistry: componentsRegistry,
		metricsStore:       metricsStore,
		processRunner:      processRunner,
		components:         []string{"component1", "component2"},
		ctx:                context.Background(),
		enableAutoUpdate:   false,
		autoUpdateExitCode: -1,
	}

	return session, componentsRegistry, metricsStore, processRunner, reader, writer
}

func createMockSession(registry *mockComponentRegistry) *Session {
	metricsStore := new(mockMetricsStore)
	processRunner := new(mockProcessRunner)

	session := &Session{
		reader:             make(chan Body, 10),
		writer:             make(chan Body, 10),
		componentsRegistry: registry,
		metricsStore:       metricsStore,
		processRunner:      processRunner,
		components:         []string{"component1", "component2"},
		ctx:                context.Background(),
		enableAutoUpdate:   false,
		autoUpdateExitCode: -1,
	}

	return session
}

// Tests for getStatesFromComponent
func TestGetStatesFromComponent(t *testing.T) {
	session, registry, _, _, _, _ := setupTestSession()

	t.Run("component not found", func(t *testing.T) {
		registry.On("Get", "nonexistent").Return(nil)

		result := session.getStatesFromComponent("nonexistent", nil)

		assert.Equal(t, "nonexistent", result.Component)
		assert.Empty(t, result.States)
		registry.AssertExpectations(t)
	})

	t.Run("component returns states", func(t *testing.T) {
		comp := new(mockComponent)
		healthStates := apiv1.HealthStates{
			{Health: apiv1.HealthStateTypeHealthy, Name: "test-state"},
		}

		registry.On("Get", "component1").Return(comp)
		comp.On("LastHealthStates").Return(healthStates)

		// Pass a non-nil rebootTime to avoid calling pkghost.LastReboot
		rebootTime := time.Now().Add(-10 * time.Minute)
		lastRebootTime := &rebootTime

		result := session.getStatesFromComponent("component1", lastRebootTime)

		assert.Equal(t, "component1", result.Component)
		assert.Equal(t, healthStates, result.States)
		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})
}

// Tests for getEventsFromComponent
func TestGetEventsFromComponent(t *testing.T) {
	registry := new(mockComponentRegistry)
	session := createMockSession(registry)
	ctx := context.Background()
	startTime := time.Now().Add(-time.Hour)
	endTime := time.Now()

	t.Run("component not found", func(t *testing.T) {
		registry.On("Get", "nonexistent").Return(nil)

		result := session.getEventsFromComponent(ctx, "nonexistent", startTime, endTime)

		assert.Equal(t, "nonexistent", result.Component)
		assert.Empty(t, result.Events)
		assert.Equal(t, startTime, result.StartTime)
		assert.Equal(t, endTime, result.EndTime)
		registry.AssertExpectations(t)
	})

	t.Run("component returns events", func(t *testing.T) {
		comp := new(mockComponent)
		events := apiv1.Events{
			{Name: "event1", Message: "test event"},
		}

		registry.On("Get", "component1").Return(comp)
		comp.On("Events", ctx, startTime).Return(events, nil)

		result := session.getEventsFromComponent(ctx, "component1", startTime, endTime)

		assert.Equal(t, "component1", result.Component)
		assert.Equal(t, events, result.Events)
		assert.Equal(t, startTime, result.StartTime)
		assert.Equal(t, endTime, result.EndTime)
		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})

	t.Run("component returns error", func(t *testing.T) {
		// Create a new session and registry for this test case
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		comp := new(mockComponent)
		emptyEvents := apiv1.Events{}

		registry.On("Get", "component1").Return(comp)
		comp.On("Events", ctx, startTime).Return(emptyEvents, errors.New("test error"))

		result := session.getEventsFromComponent(ctx, "component1", startTime, endTime)

		assert.Equal(t, "component1", result.Component)
		assert.Empty(t, result.Events)
		assert.Equal(t, startTime, result.StartTime)
		assert.Equal(t, endTime, result.EndTime)
		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})
}

// Tests for getMetricsFromComponent
func TestGetMetricsFromComponent(t *testing.T) {
	registry := new(mockComponentRegistry)
	metricsStore := new(mockMetricsStore)
	session := createMockSession(registry)
	session.metricsStore = metricsStore

	ctx := context.Background()
	since := time.Now().Add(-time.Hour)

	t.Run("component not found", func(t *testing.T) {
		registry.On("Get", "nonexistent").Return(nil)

		result := session.getMetricsFromComponent(ctx, "nonexistent", since)

		assert.Equal(t, "nonexistent", result.Component)
		assert.Empty(t, result.Metrics)
		registry.AssertExpectations(t)
	})

	t.Run("metrics store returns metrics", func(t *testing.T) {
		comp := new(mockComponent)
		storeData := metrics.Metrics{
			{Name: "metric1", Value: 42, UnixMilliseconds: 1000, Labels: map[string]string{"label": "value"}},
		}

		registry.On("Get", "component1").Return(comp)
		metricsStore.On("Read", ctx, mock.Anything).Return(storeData, nil)

		result := session.getMetricsFromComponent(ctx, "component1", since)

		assert.Equal(t, "component1", result.Component)
		assert.Len(t, result.Metrics, 1)
		assert.Equal(t, "metric1", result.Metrics[0].Name)
		assert.Equal(t, float64(42), result.Metrics[0].Value)
		assert.Equal(t, int64(1000), result.Metrics[0].UnixSeconds)
		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})

	t.Run("metrics store returns error", func(t *testing.T) {
		// Create a new session and registry for this test case
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore

		comp := new(mockComponent)
		emptyMetrics := metrics.Metrics{}

		registry.On("Get", "component1").Return(comp)
		metricsStore.On("Read", ctx, mock.Anything).Return(emptyMetrics, errors.New("test error"))

		result := session.getMetricsFromComponent(ctx, "component1", since)

		assert.Equal(t, "component1", result.Component)
		assert.Empty(t, result.Metrics)
		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})
}

// Test createNeedDeleteFiles
func TestCreateNeedDeleteFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-create-need-delete-files")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test directories
	subDir1 := filepath.Join(tempDir, "subdir1")
	require.NoError(t, os.Mkdir(subDir1, 0755))

	subDir2 := filepath.Join(tempDir, "subdir2")
	require.NoError(t, os.Mkdir(subDir2, 0755))

	// Test creating needDelete files
	err = createNeedDeleteFiles(tempDir)
	require.NoError(t, err)

	// Check that needDelete files were created
	_, err = os.Stat(filepath.Join(subDir1, "needDelete"))
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(subDir2, "needDelete"))
	assert.NoError(t, err)

	// No needDelete file should be created in the root directory
	_, err = os.Stat(filepath.Join(tempDir, "needDelete"))
	assert.True(t, os.IsNotExist(err))
}

// Test getHealthStates
func TestGetHealthStates(t *testing.T) {
	session, registry, _, _, _, _ := setupTestSession()

	t.Run("with specific components", func(t *testing.T) {
		// Use a simpler implementation to avoid goroutine race conditions
		payload := Request{
			Method:     "states",
			Components: []string{"component1"},
		}

		comp := new(mockComponent)
		healthStates := apiv1.HealthStates{{Health: apiv1.HealthStateTypeHealthy, Name: "state1"}}

		registry.On("Get", "component1").Return(comp)
		comp.On("LastHealthStates").Return(healthStates)

		// Just testing the synchronous parts for simplicity
		states, err := session.getHealthStates(payload)

		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, "component1", states[0].Component)
		assert.Equal(t, healthStates, states[0].States)

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})

	t.Run("mismatched method", func(t *testing.T) {
		payload := Request{
			Method: "events", // Not "states"
		}

		states, err := session.getHealthStates(payload)

		assert.Error(t, err)
		assert.Nil(t, states)
	})
}

// Test handling bootstrap request
func TestHandleBootstrapRequest(t *testing.T) {
	session, _, _, processRunner, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader) // Ensure the goroutine exits

	// Create a bootstrap request
	script := "echo 'Hello, World!'"
	encodedScript := base64.StdEncoding.EncodeToString([]byte(script))

	req := Request{
		Method: "bootstrap",
		Bootstrap: &BootstrapRequest{
			TimeoutInSeconds: 5,
			ScriptBase64:     encodedScript,
		},
	}

	reqData, _ := json.Marshal(req)

	// Mock the process runner
	processRunner.On("RunUntilCompletion", mock.Anything, script).Return(
		[]byte("Hello, World!"), 0, nil)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
	assert.NotNil(t, response.Bootstrap)
	assert.Equal(t, "Hello, World!", response.Bootstrap.Output)
	assert.Equal(t, int32(0), response.Bootstrap.ExitCode)

	processRunner.AssertExpectations(t)
}

// Test triggerComponentCheck
func TestTriggerComponentCheck(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader) // Ensure the goroutine exits

	// Create a component and mock its behavior
	comp := new(mockComponent)
	compResults := new(mockCheckResult)
	healthStates := apiv1.HealthStates{
		{Health: apiv1.HealthStateTypeHealthy, Name: "test-state"},
	}

	registry.On("Get", "test-component").Return(comp)
	comp.On("Check").Return(compResults)
	compResults.On("HealthStates").Return(healthStates)
	compResults.On("ComponentName").Return("test-component")

	// Create the request
	req := Request{
		Method:        "triggerComponentCheck",
		ComponentName: "test-component",
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
	assert.Len(t, response.States, 1)
	assert.Equal(t, "test-component", response.States[0].Component)
	assert.Equal(t, healthStates, response.States[0].States)

	registry.AssertExpectations(t)
	comp.AssertExpectations(t)
	compResults.AssertExpectations(t)
}

// Test triggerComponentCheckByTag with non-empty TagName
func TestTriggerComponentCheckByTag(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader) // Ensure the goroutine exits

	// Create two components with different tags
	comp1 := new(mockComponent)
	comp2 := new(mockComponent)
	compResults := new(mockCheckResult)
	healthStates := apiv1.HealthStates{
		{Health: apiv1.HealthStateTypeHealthy, Name: "test-state"},
	}

	// Set up component behaviors
	comp1.On("Tags").Return([]string{"tag1", "common-tag"})
	comp1.On("Check").Return(compResults)

	comp2.On("Tags").Return([]string{"tag2"}) // This one doesn't have the matching tag
	// comp2 should not have Check called since it doesn't match the tag

	// Only comp1 should have its Check method called since only it has the matching tag
	compResults.On("HealthStates").Return(healthStates)
	compResults.On("ComponentName").Return("comp1")

	// For TagName functionality, we need to use All() to get all components and filter by tag
	registry.On("All").Return([]components.Component{comp1, comp2})

	// Test the TagName functionality by NOT setting ComponentName
	req := Request{
		Method:  "triggerComponentCheck",
		TagName: "common-tag",
		// ComponentName is intentionally not set to test TagName functionality
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
	assert.Len(t, response.States, 1)
	assert.Equal(t, "comp1", response.States[0].Component) // Should use ComponentName from checkResult
	assert.Equal(t, healthStates, response.States[0].States)

	// Verify expectations
	registry.AssertExpectations(t)
	comp1.AssertExpectations(t)
	comp2.AssertExpectations(t)
	compResults.AssertExpectations(t)
}

// Test deregisterComponent
func TestDeregisterComponent(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader) // Ensure the goroutine exits

	t.Run("successful deregistration", func(t *testing.T) {
		// Create a deregisterable component
		comp := new(mockDeregisterableComponent)
		componentName := "test-component"

		// Make comp.Name() return the component name for deregistration
		comp.On("Name").Return(componentName).Maybe()
		comp.On("CanDeregister").Return(true)
		comp.On("Close").Return(nil)

		registry.On("Get", componentName).Return(comp)
		registry.On("Deregister", componentName).Return(comp)

		// Create the request
		req := Request{
			Method:        "deregisterComponent",
			ComponentName: componentName,
		}

		reqData, _ := json.Marshal(req)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-req-id-1",
		}

		// Read the response
		var resp Body
		select {
		case resp = <-writer:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		// Parse the response
		var response Response
		err := json.Unmarshal(resp.Data, &response)
		require.NoError(t, err)

		// Check the response
		assert.Equal(t, "test-req-id-1", resp.ReqID)
		assert.Empty(t, response.Error)
		assert.Equal(t, int32(0), response.ErrorCode)

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})

	t.Run("component not found", func(t *testing.T) {
		registry.On("Get", "nonexistent").Return(nil)

		// Create the request
		req := Request{
			Method:        "deregisterComponent",
			ComponentName: "nonexistent",
		}

		reqData, _ := json.Marshal(req)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-req-id-2",
		}

		// Read the response
		var resp Body
		select {
		case resp = <-writer:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		// Parse the response
		var response Response
		err := json.Unmarshal(resp.Data, &response)
		require.NoError(t, err)

		// Check the response
		assert.Equal(t, "test-req-id-2", resp.ReqID)
		assert.Equal(t, int32(http.StatusNotFound), response.ErrorCode)

		registry.AssertExpectations(t)
	})
}

// Test for handling malformed requests
func TestMalformedRequest(t *testing.T) {
	session, _, _, _, reader, _ := setupTestSession()

	// Use a channel with buffer to ensure we don't block
	done := make(chan bool, 1)
	go func() {
		session.serve()
		done <- true
	}()

	// Send an invalid JSON payload
	reader <- Body{
		Data:  []byte("{invalid-json"),
		ReqID: "test-req-id",
	}

	// Close the channel to end the serve goroutine
	close(reader)

	// Wait for serve to complete
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for serve to exit")
	}
}

// Test metrics request handling
func TestHandleMetricsRequest(t *testing.T) {
	session, registry, metricsStore, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	comp := new(mockComponent)
	storeData := metrics.Metrics{
		{Name: "metric1", Value: 42, UnixMilliseconds: 1000, Labels: map[string]string{"label": "value"}},
	}

	registry.On("Get", "component1").Return(comp)
	metricsStore.On("Read", mock.Anything, mock.Anything).Return(storeData, nil)

	req := Request{
		Method:     "metrics",
		Components: []string{"component1"},
		Since:      time.Hour,
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
	assert.Len(t, response.Metrics, 1)
	assert.Equal(t, "component1", response.Metrics[0].Component)

	registry.AssertExpectations(t)
	metricsStore.AssertExpectations(t)
}

// Test states request handling
func TestHandleStatesRequest(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	comp := new(mockComponent)
	healthStates := apiv1.HealthStates{
		{Health: apiv1.HealthStateTypeHealthy, Name: "test-state"},
	}

	registry.On("Get", "component1").Return(comp)
	comp.On("LastHealthStates").Return(healthStates)

	req := Request{
		Method:     "states",
		Components: []string{"component1"},
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
	assert.Len(t, response.States, 1)
	assert.Equal(t, "component1", response.States[0].Component)

	registry.AssertExpectations(t)
	comp.AssertExpectations(t)
}

// Test events request handling
func TestHandleEventsRequest(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	comp := new(mockComponent)
	events := apiv1.Events{
		{Name: "event1", Message: "test event"},
	}

	registry.On("Get", "component1").Return(comp)
	comp.On("Events", mock.Anything, mock.Anything).Return(events, nil)

	req := Request{
		Method:     "events",
		Components: []string{"component1"},
		StartTime:  time.Now().Add(-time.Hour),
		EndTime:    time.Now(),
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
	assert.Len(t, response.Events, 1)
	assert.Equal(t, "component1", response.Events[0].Component)

	registry.AssertExpectations(t)
	comp.AssertExpectations(t)
}

// Test delete request handling
func TestHandleDeleteRequest(t *testing.T) {
	session, _, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	req := Request{
		Method: "delete",
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
}

// Test setHealthy request handling
func TestHandleSetHealthyRequest(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	comp := new(mockHealthSettableComponent)

	registry.On("Get", "component1").Return(comp)
	comp.On("SetHealthy").Return(nil)

	req := Request{
		Method:     "setHealthy",
		Components: []string{"component1"},
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)

	registry.AssertExpectations(t)
	comp.AssertExpectations(t)
}

// Test getPlugins request handling
func TestHandleGetPluginsRequest(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	comp := new(mockCustomPluginRegistereeComponent)
	pluginSpec := pkgcustomplugins.Spec{
		PluginName: "test-plugin",
		Type:       "component",
	}

	registry.On("All").Return([]components.Component{comp})
	comp.On("IsCustomPlugin").Return(true)
	comp.On("Name").Return("test-plugin")
	comp.On("Spec").Return(pluginSpec)

	req := Request{
		Method: "getPlugins",
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
	assert.NotEmpty(t, response.Plugins)

	registry.AssertExpectations(t)
	comp.AssertExpectations(t)
}

// Test registerPlugin request handling
func TestHandleRegisterPluginRequest(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	comp := new(mockComponent)
	pluginSpec := &pkgcustomplugins.Spec{
		PluginName: "test-plugin",
		Type:       "component",
		HealthStatePlugin: &pkgcustomplugins.Plugin{
			Steps: []pkgcustomplugins.Step{
				{
					Name: "test-step",
					RunBashScript: &pkgcustomplugins.RunBashScript{
						ContentType: "plaintext",
						Script:      "echo 'test'",
					},
				},
			},
		},
	}

	registry.On("Register", mock.Anything).Return(comp, nil)
	comp.On("Start").Return(nil)
	comp.On("Name").Return("test-plugin")

	req := Request{
		Method:     "registerPlugin",
		PluginSpec: pluginSpec,
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)

	registry.AssertExpectations(t)
	comp.AssertExpectations(t)
}

// Test updatePlugin request handling
func TestHandleUpdatePluginRequest(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	oldComp := new(mockComponent)
	newComp := new(mockComponent)
	pluginSpec := &pkgcustomplugins.Spec{
		PluginName: "test-plugin",
		Type:       "component",
		HealthStatePlugin: &pkgcustomplugins.Plugin{
			Steps: []pkgcustomplugins.Step{
				{
					Name: "test-step",
					RunBashScript: &pkgcustomplugins.RunBashScript{
						ContentType: "plaintext",
						Script:      "echo 'test updated'",
					},
				},
			},
		},
	}

	registry.On("Get", "test-plugin").Return(oldComp)
	oldComp.On("Name").Return("test-plugin")
	oldComp.On("Close").Return(nil)
	registry.On("Deregister", "test-plugin").Return(oldComp)
	registry.On("Register", mock.Anything).Return(newComp, nil)
	newComp.On("Start").Return(nil)
	newComp.On("Name").Return("test-plugin")

	req := Request{
		Method:     "updatePlugin",
		PluginSpec: pluginSpec,
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)

	registry.AssertExpectations(t)
	oldComp.AssertExpectations(t)
	newComp.AssertExpectations(t)
}

// Test setPluginSpecs request handling
func TestHandleSetPluginSpecsRequest(t *testing.T) {
	session, _, _, _, reader, writer := setupTestSession()

	// Set up the savePluginSpecsFunc
	savePluginSpecsCalled := false
	session.savePluginSpecsFunc = func(ctx context.Context, specs pkgcustomplugins.Specs) (bool, error) {
		savePluginSpecsCalled = true
		return true, nil
	}

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	pluginSpecs := pkgcustomplugins.Specs{
		{
			PluginName: "test-plugin",
			Type:       "component",
			HealthStatePlugin: &pkgcustomplugins.Plugin{
				Steps: []pkgcustomplugins.Step{
					{
						Name: "test-step",
						RunBashScript: &pkgcustomplugins.RunBashScript{
							ContentType: "plaintext",
							Script:      "echo 'test'",
						},
					},
				},
			},
		},
	}

	req := Request{
		Method:      "setPluginSpecs",
		PluginSpecs: pluginSpecs,
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
	assert.True(t, savePluginSpecsCalled)
}

// Test loadPluginSpecs request handling
func TestHandleLoadPluginSpecsRequest(t *testing.T) {
	session, _, _, _, reader, writer := setupTestSession()

	// Set up the loadPluginSpecsFunc
	pluginSpecs := pkgcustomplugins.Specs{
		{
			PluginName: "test-plugin",
			Type:       "component",
			HealthStatePlugin: &pkgcustomplugins.Plugin{
				Steps: []pkgcustomplugins.Step{
					{
						Name: "test-step",
						RunBashScript: &pkgcustomplugins.RunBashScript{
							ContentType: "plaintext",
							Script:      "echo 'test'",
						},
					},
				},
			},
		},
	}
	session.loadPluginSpecsFunc = func(ctx context.Context) (pkgcustomplugins.Specs, error) {
		return pluginSpecs, nil
	}

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	req := Request{
		Method: "loadPluginSpecs",
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
	assert.Len(t, response.PluginSpecs, 1)
	assert.Equal(t, "test-plugin", response.PluginSpecs[0].PluginName)
}

// Test error cases for various requests
func TestHandleRequestErrors(t *testing.T) {
	t.Run("events method mismatch error", func(t *testing.T) {
		session, registry, _, _, reader, writer := setupTestSession()
		go session.serve()
		defer close(reader)

		// Set up expectation for the nonexistent component
		registry.On("Get", "nonexistent").Return(nil)

		req := Request{
			Method:     "events",
			Components: []string{"nonexistent"},
		}

		reqData, _ := json.Marshal(req)
		reader <- Body{Data: reqData, ReqID: "test-req-id-1"}

		var resp Body
		select {
		case resp = <-writer:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		var response Response
		err := json.Unmarshal(resp.Data, &response)
		require.NoError(t, err)
		// Should get empty events for nonexistent component, but no error from the main function
		assert.Empty(t, response.Error)
		assert.Len(t, response.Events, 1)
		assert.Equal(t, "nonexistent", response.Events[0].Component)

		registry.AssertExpectations(t)
	})

	t.Run("states method mismatch error", func(t *testing.T) {
		session, registry, _, _, reader, writer := setupTestSession()
		go session.serve()
		defer close(reader)

		// Set up expectations for default components
		registry.On("Get", "component1").Return(nil)
		registry.On("Get", "component2").Return(nil)

		req := Request{
			Method: "events", // Wrong method for states
		}

		reqData, _ := json.Marshal(req)
		reader <- Body{Data: reqData, ReqID: "test-req-id-2"}

		var resp Body
		select {
		case resp = <-writer:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		var response Response
		err := json.Unmarshal(resp.Data, &response)
		require.NoError(t, err)
		// Should get empty events but no error since this is a valid events request
		assert.Empty(t, response.Error)

		registry.AssertExpectations(t)
	})
}

// Test reboot request handling
func TestHandleRebootRequest(t *testing.T) {
	session, _, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	req := Request{
		Method: "reboot",
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response - reboot will likely fail in test environment but that's expected
	assert.Equal(t, "test-req-id", resp.ReqID)
	// Don't assert on error since reboot may fail in test environment
}

// Test logout request handling
func TestHandleLogoutRequest(t *testing.T) {
	session, _, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	req := Request{
		Method: "logout",
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response - logout will likely fail in test environment but that's expected
	assert.Equal(t, "test-req-id", resp.ReqID)
	// Don't assert on error since logout may fail in test environment
}

// Test update request handling
func TestHandleUpdateRequest(t *testing.T) {
	session, _, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	req := Request{
		Method:        "update",
		UpdateVersion: "test-version",
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response - update will likely fail in test environment but that's expected
	assert.Equal(t, "test-req-id", resp.ReqID)
	// The update will fail because auto update is not enabled by default
	assert.NotEmpty(t, response.Error)
	assert.Contains(t, response.Error, "auto update is disabled")
}

// Test updateConfig request handling
func TestHandleUpdateConfigRequest(t *testing.T) {
	session, _, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	req := Request{
		Method: "updateConfig",
		UpdateConfig: map[string]string{
			"unsupported-component": "test-config",
		},
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)
}

// Test plugin error scenarios
func TestHandlePluginErrors(t *testing.T) {
	t.Run("registerPlugin validation error", func(t *testing.T) {
		session, _, _, _, reader, writer := setupTestSession()
		go session.serve()
		defer close(reader)

		// Invalid plugin spec without HealthStatePlugin
		pluginSpec := &pkgcustomplugins.Spec{
			PluginName: "test-plugin",
			Type:       "component",
			// Missing HealthStatePlugin
		}

		req := Request{
			Method:     "registerPlugin",
			PluginSpec: pluginSpec,
		}

		reqData, _ := json.Marshal(req)
		reader <- Body{Data: reqData, ReqID: "test-req-id"}

		var resp Body
		select {
		case resp = <-writer:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		var response Response
		err := json.Unmarshal(resp.Data, &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Error)
		assert.Contains(t, response.Error, "state plugin is required")
	})

	t.Run("updatePlugin not found", func(t *testing.T) {
		session, registry, _, _, reader, writer := setupTestSession()
		go session.serve()
		defer close(reader)

		registry.On("Get", "nonexistent-plugin").Return(nil)

		pluginSpec := &pkgcustomplugins.Spec{
			PluginName: "nonexistent-plugin",
			Type:       "component",
			HealthStatePlugin: &pkgcustomplugins.Plugin{
				Steps: []pkgcustomplugins.Step{
					{
						Name: "test-step",
						RunBashScript: &pkgcustomplugins.RunBashScript{
							ContentType: "plaintext",
							Script:      "echo 'test'",
						},
					},
				},
			},
		}

		req := Request{
			Method:     "updatePlugin",
			PluginSpec: pluginSpec,
		}

		reqData, _ := json.Marshal(req)
		reader <- Body{Data: reqData, ReqID: "test-req-id"}

		var resp Body
		select {
		case resp = <-writer:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		var response Response
		err := json.Unmarshal(resp.Data, &response)
		require.NoError(t, err)
		assert.Equal(t, int32(http.StatusNotFound), response.ErrorCode)
		assert.NotEmpty(t, response.Error)

		registry.AssertExpectations(t)
	})

	t.Run("getPlugins with specific component", func(t *testing.T) {
		session, registry, _, _, reader, writer := setupTestSession()
		go session.serve()
		defer close(reader)

		comp := new(mockCustomPluginRegistereeComponent)
		pluginSpec := pkgcustomplugins.Spec{
			PluginName: "specific-plugin",
			Type:       "component",
		}

		registry.On("Get", "specific-plugin").Return(comp)
		comp.On("IsCustomPlugin").Return(true)
		comp.On("Name").Return("specific-plugin")
		comp.On("Spec").Return(pluginSpec)

		req := Request{
			Method:        "getPlugins",
			ComponentName: "specific-plugin",
		}

		reqData, _ := json.Marshal(req)
		reader <- Body{Data: reqData, ReqID: "test-req-id"}

		var resp Body
		select {
		case resp = <-writer:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		var response Response
		err := json.Unmarshal(resp.Data, &response)
		require.NoError(t, err)
		assert.Empty(t, response.Error)
		assert.NotEmpty(t, response.Plugins)

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})
}

// Test setHealthy with component error
func TestHandleSetHealthyWithError(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSession()

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	comp := new(mockHealthSettableComponent)

	registry.On("Get", "component1").Return(comp)
	comp.On("SetHealthy").Return(errors.New("failed to set healthy"))

	req := Request{
		Method:     "setHealthy",
		Components: []string{"component1"},
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-req-id",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response - error is logged but not returned in response
	assert.Equal(t, "test-req-id", resp.ReqID)
	assert.Empty(t, response.Error)

	registry.AssertExpectations(t)
	comp.AssertExpectations(t)
}
