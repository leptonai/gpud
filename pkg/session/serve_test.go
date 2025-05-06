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

func (m *mockComponent) IsSupported() bool {
	args := m.Called()
	return args.Bool(0)
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
