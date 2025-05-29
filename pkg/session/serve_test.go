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

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	pkgkmsgwriter "github.com/leptonai/gpud/pkg/kmsg/writer"
	"github.com/leptonai/gpud/pkg/metrics"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
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

type mockFaultInjector struct {
	mock.Mock
}

func (m *mockFaultInjector) Write(kernelMessage *pkgkmsgwriter.KernelMessage) error {
	args := m.Called(kernelMessage)
	return args.Error(0)
}

func (m *mockFaultInjector) KmsgWriter() pkgkmsgwriter.KmsgWriter {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(pkgkmsgwriter.KmsgWriter)
}

type mockKmsgWriter struct {
	mock.Mock
}

func (m *mockKmsgWriter) Write(msg *pkgkmsgwriter.KernelMessage) error {
	args := m.Called(msg)
	return args.Error(0)
}

// Helper functions for testing
func setupTestSession() (*Session, *mockComponentRegistry, *mockMetricsStore, *mockProcessRunner, *mockFaultInjector, chan Body, chan Body) {
	reader := make(chan Body, 10)
	writer := make(chan Body, 10)
	componentsRegistry := new(mockComponentRegistry)
	metricsStore := new(mockMetricsStore)
	processRunner := new(mockProcessRunner)
	faultInjector := new(mockFaultInjector)

	session := &Session{
		reader:             reader,
		writer:             writer,
		componentsRegistry: componentsRegistry,
		metricsStore:       metricsStore,
		processRunner:      processRunner,
		faultInjector:      faultInjector,
		components:         []string{"component1", "component2"},
		ctx:                context.Background(),
		enableAutoUpdate:   false,
		autoUpdateExitCode: -1,
	}

	return session, componentsRegistry, metricsStore, processRunner, faultInjector, reader, writer
}

func setupTestSessionWithoutFaultInjector() (*Session, *mockComponentRegistry, *mockMetricsStore, *mockProcessRunner, chan Body, chan Body) {
	session, registry, store, runner, _, reader, writer := setupTestSession()
	return session, registry, store, runner, reader, writer
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
	session, registry, _, _, _, _ := setupTestSessionWithoutFaultInjector()

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
	session, registry, _, _, _, _ := setupTestSessionWithoutFaultInjector()

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
	session, _, _, processRunner, reader, writer := setupTestSessionWithoutFaultInjector()

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
	session, registry, _, _, reader, writer := setupTestSessionWithoutFaultInjector()

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
	session, registry, _, _, reader, writer := setupTestSessionWithoutFaultInjector()

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
	session, registry, _, _, reader, writer := setupTestSessionWithoutFaultInjector()

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
	session, _, _, _, reader, _ := setupTestSessionWithoutFaultInjector()

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

// Test handling injectFault request
func TestHandleInjectFaultRequest(t *testing.T) {
	t.Run("successful kernel message injection", func(t *testing.T) {
		session, _, _, _, faultInjector, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create an inject fault request
		kernelMessage := &pkgkmsgwriter.KernelMessage{
			Priority: "KERN_INFO",
			Message:  "test kernel message",
		}

		req := Request{
			Method: "injectFault",
			InjectFaultRequest: &pkgfaultinjector.Request{
				KernelMessage: kernelMessage,
			},
		}

		reqData, _ := json.Marshal(req)

		// Mock the fault injector to succeed
		mockWriter := new(mockKmsgWriter)
		mockWriter.On("Write", kernelMessage).Return(nil)
		faultInjector.On("KmsgWriter").Return(mockWriter)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-inject-fault-success",
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
		assert.Equal(t, "test-inject-fault-success", resp.ReqID)
		assert.Empty(t, response.Error)

		faultInjector.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("failed kernel message injection", func(t *testing.T) {
		session, _, _, _, faultInjector, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create an inject fault request
		kernelMessage := &pkgkmsgwriter.KernelMessage{
			Priority: "KERN_ERR",
			Message:  "test error message",
		}

		req := Request{
			Method: "injectFault",
			InjectFaultRequest: &pkgfaultinjector.Request{
				KernelMessage: kernelMessage,
			},
		}

		reqData, _ := json.Marshal(req)

		// Mock the fault injector to fail
		expectedError := errors.New("fault injection failed")
		mockWriter := new(mockKmsgWriter)
		mockWriter.On("Write", kernelMessage).Return(expectedError)
		faultInjector.On("KmsgWriter").Return(mockWriter)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-inject-fault-error",
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
		assert.Equal(t, "test-inject-fault-error", resp.ReqID)
		assert.Equal(t, expectedError.Error(), response.Error)

		faultInjector.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("nil fault inject request", func(t *testing.T) {
		session, _, _, _, _, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create an inject fault request with nil FaultInjectRequest
		req := Request{
			Method:             "injectFault",
			InjectFaultRequest: nil,
		}

		reqData, _ := json.Marshal(req)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-inject-fault-nil-request",
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

		// Check the response - nil request doesn't trigger validation or injection
		assert.Equal(t, "test-inject-fault-nil-request", resp.ReqID)
		assert.Empty(t, response.Error) // Should not set error for nil request
	})

	t.Run("nil kernel message validation failure", func(t *testing.T) {
		session, _, _, _, _, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create an inject fault request with nil KernelMessage
		// According to fault-injector validation logic, this should fail validation
		req := Request{
			Method: "injectFault",
			InjectFaultRequest: &pkgfaultinjector.Request{
				KernelMessage: nil,
			},
		}

		reqData, _ := json.Marshal(req)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-inject-fault-nil-kernel-message",
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

		// Check the response - should fail validation
		assert.Equal(t, "test-inject-fault-nil-kernel-message", resp.ReqID)
		assert.Equal(t, "no fault injection entry found", response.Error)
	})

	t.Run("empty fault request validation failure", func(t *testing.T) {
		session, _, _, _, _, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create an inject fault request with empty struct (no fields set)
		// This should fail validation with "no fault injection entry found"
		req := Request{
			Method:             "injectFault",
			InjectFaultRequest: &pkgfaultinjector.Request{
				// No fields set - should hit default case in validation
			},
		}

		reqData, _ := json.Marshal(req)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-inject-fault-empty-request",
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

		// Check the response - should fail validation
		assert.Equal(t, "test-inject-fault-empty-request", resp.ReqID)
		assert.Equal(t, "no fault injection entry found", response.Error)
	})
}

// Test handling injectFault request validation
func TestHandleInjectFaultRequestValidation(t *testing.T) {
	t.Run("valid fault inject request with valid kernel message", func(t *testing.T) {
		session, _, _, _, faultInjector, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create a valid inject fault request
		kernelMessage := &pkgkmsgwriter.KernelMessage{
			Priority: "KERN_INFO",
			Message:  "valid test kernel message",
		}

		req := Request{
			Method: "injectFault",
			InjectFaultRequest: &pkgfaultinjector.Request{
				KernelMessage: kernelMessage,
			},
		}

		reqData, _ := json.Marshal(req)

		// Mock the fault injector to succeed (since validation passes, injection will be attempted)
		mockWriter := new(mockKmsgWriter)
		mockWriter.On("Write", kernelMessage).Return(nil)
		faultInjector.On("KmsgWriter").Return(mockWriter)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-validate-success",
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

		// Check the response - validation should pass, no error should be set
		assert.Equal(t, "test-validate-success", resp.ReqID)
		assert.Empty(t, response.Error, "validation should pass for valid kernel message")

		faultInjector.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("invalid fault inject request with message too long", func(t *testing.T) {
		session, _, _, _, _, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// MaxPrintkRecordLength is 1024 - 48 = 976 characters
		maxLength := 976
		// Create a kernel message with message exceeding MaxPrintkRecordLength
		longMessage := make([]byte, maxLength+100) // Exceeds the limit
		for i := range longMessage {
			longMessage[i] = 'A'
		}

		kernelMessage := &pkgkmsgwriter.KernelMessage{
			Priority: "KERN_ERR",
			Message:  string(longMessage),
		}

		req := Request{
			Method: "injectFault",
			InjectFaultRequest: &pkgfaultinjector.Request{
				KernelMessage: kernelMessage,
			},
		}

		reqData, _ := json.Marshal(req)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-validate-error",
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

		// Check the response - validation should fail
		assert.Equal(t, "test-validate-error", resp.ReqID)
		assert.NotEmpty(t, response.Error, "validation should fail for message too long")
		assert.Contains(t, response.Error, "message length exceeds the maximum length", "error should mention message length limit")
		assert.Contains(t, response.Error, "976", "error should mention the specific limit")
	})

	t.Run("invalid fault inject request with nil kernel message", func(t *testing.T) {
		session, _, _, _, _, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create a fault inject request with nil KernelMessage (should fail validation)
		req := Request{
			Method: "injectFault",
			InjectFaultRequest: &pkgfaultinjector.Request{
				KernelMessage: nil,
			},
		}

		reqData, _ := json.Marshal(req)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-validate-nil-failure",
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

		// Check the response - validation should fail for nil kernel message
		assert.Equal(t, "test-validate-nil-failure", resp.ReqID)
		assert.Equal(t, "no fault injection entry found", response.Error, "validation should fail for nil kernel message")
	})

	t.Run("nil fault inject request", func(t *testing.T) {
		session, _, _, _, _, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create a request with nil FaultInjectRequest (should not call Validate)
		req := Request{
			Method:             "injectFault",
			InjectFaultRequest: nil,
		}

		reqData, _ := json.Marshal(req)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-validate-nil-request",
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

		// Check the response - should not error since Validate is not called for nil request
		assert.Equal(t, "test-validate-nil-request", resp.ReqID)
		assert.Empty(t, response.Error, "nil fault inject request should not cause validation error")
	})

	t.Run("valid XID converts to kernel message", func(t *testing.T) {
		session, _, _, _, faultInjector, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create a fault inject request with valid XID (should transform to kernel message)
		req := Request{
			Method: "injectFault",
			InjectFaultRequest: &pkgfaultinjector.Request{
				XID: &pkgfaultinjector.XIDToInject{
					ID: 63, // Valid XID that should convert to kernel message
				},
			},
		}

		reqData, _ := json.Marshal(req)

		// Mock the fault injector - after XID validation, it converts to KernelMessage
		// We need to mock with mock.Anything since we don't know the exact converted message
		mockWriter := new(mockKmsgWriter)
		mockWriter.On("Write", mock.AnythingOfType("*writer.KernelMessage")).Return(nil)
		faultInjector.On("KmsgWriter").Return(mockWriter)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-validate-xid-success",
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

		// Check the response - validation should pass and XID should be converted
		assert.Equal(t, "test-validate-xid-success", resp.ReqID)
		assert.Empty(t, response.Error, "validation should pass for valid XID")

		faultInjector.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("invalid XID with zero ID", func(t *testing.T) {
		session, _, _, _, _, reader, writer := setupTestSession()

		// Start the session in a separate goroutine
		go session.serve()
		defer close(reader) // Ensure the goroutine exits

		// Create a fault inject request with invalid XID (ID = 0)
		req := Request{
			Method: "injectFault",
			InjectFaultRequest: &pkgfaultinjector.Request{
				XID: &pkgfaultinjector.XIDToInject{
					ID: 0, // Invalid XID, should fail validation
				},
			},
		}

		reqData, _ := json.Marshal(req)

		// Send the request
		reader <- Body{
			Data:  reqData,
			ReqID: "test-validate-xid-failure",
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

		// Check the response - validation should fail for invalid XID
		assert.Equal(t, "test-validate-xid-failure", resp.ReqID)
		assert.Equal(t, "no fault injection entry found", response.Error, "validation should fail for XID with ID 0")
	})
}

// Test handling injectFault request with nil faultInjector
func TestHandleInjectFaultRequest_NilFaultInjector(t *testing.T) {
	// Create a session with nil faultInjector using createMockSession
	registry := new(mockComponentRegistry)
	session := createMockSession(registry)
	// Ensure faultInjector is nil (it should be by default from createMockSession)
	session.faultInjector = nil

	reader := make(chan Body, 10)
	writer := make(chan Body, 10)
	session.reader = reader
	session.writer = writer

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader) // Ensure the goroutine exits

	// Create an inject fault request with valid InjectFaultRequest
	kernelMessage := &pkgkmsgwriter.KernelMessage{
		Priority: "KERN_INFO",
		Message:  "test kernel message",
	}

	req := Request{
		Method: "injectFault",
		InjectFaultRequest: &pkgfaultinjector.Request{
			KernelMessage: kernelMessage,
		},
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-nil-fault-injector",
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

	// Check the response - should contain the expected error message
	assert.Equal(t, "test-nil-fault-injector", resp.ReqID)
	assert.Equal(t, "fault injector is not initialized", response.Error)
	assert.Equal(t, int32(0), response.ErrorCode) // Should be 0 as no specific error code is set
}

// Tests for processGossip
func TestProcessGossip(t *testing.T) {
	t.Run("nil createGossipRequestFunc", func(t *testing.T) {
		session := &Session{
			createGossipRequestFunc: nil,
		}
		resp := &Response{}

		session.processGossip(resp)

		// Should return early without setting anything
		assert.Nil(t, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})

	t.Run("successful gossip request creation", func(t *testing.T) {
		expectedGossipReq := &apiv1.GossipRequest{
			MachineID: "test-machine-id",
		}

		mockCreateGossipFunc := func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
			assert.Equal(t, "test-machine-id", machineID)
			return expectedGossipReq, nil
		}

		session := &Session{
			machineID:               "test-machine-id",
			token:                   "test-token",
			createGossipRequestFunc: mockCreateGossipFunc,
		}
		resp := &Response{}

		session.processGossip(resp)

		assert.Equal(t, expectedGossipReq, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})

	t.Run("error in gossip request creation", func(t *testing.T) {
		expectedError := errors.New("failed to create gossip request")

		mockCreateGossipFunc := func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
			return nil, expectedError
		}

		session := &Session{
			machineID:               "test-machine-id",
			token:                   "test-token",
			createGossipRequestFunc: mockCreateGossipFunc,
		}
		resp := &Response{}

		session.processGossip(resp)

		assert.Nil(t, resp.GossipRequest)
		assert.Equal(t, expectedError.Error(), resp.Error)
	})

	t.Run("with nvml instance", func(t *testing.T) {
		mockNvmlInstance := &mockNvmlInstance{}
		expectedGossipReq := &apiv1.GossipRequest{
			MachineID: "test-machine-id",
		}

		mockCreateGossipFunc := func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
			assert.Equal(t, "test-machine-id", machineID)
			assert.Equal(t, mockNvmlInstance, nvmlInstance)
			return expectedGossipReq, nil
		}

		session := &Session{
			machineID:               "test-machine-id",
			nvmlInstance:            mockNvmlInstance,
			token:                   "test-token",
			createGossipRequestFunc: mockCreateGossipFunc,
		}
		resp := &Response{}

		session.processGossip(resp)

		assert.Equal(t, expectedGossipReq, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})

	t.Run("empty machine ID and token", func(t *testing.T) {
		expectedGossipReq := &apiv1.GossipRequest{}

		mockCreateGossipFunc := func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error) {
			assert.Empty(t, machineID)
			return expectedGossipReq, nil
		}

		session := &Session{
			machineID:               "",
			token:                   "",
			createGossipRequestFunc: mockCreateGossipFunc,
		}
		resp := &Response{}

		session.processGossip(resp)

		assert.Equal(t, expectedGossipReq, resp.GossipRequest)
		assert.Empty(t, resp.Error)
	})
}

// Mock NVML instance for testing
type mockNvmlInstance struct{}

func (m *mockNvmlInstance) NVMLExists() bool                  { return true }
func (m *mockNvmlInstance) Library() nvmllib.Library          { return nil }
func (m *mockNvmlInstance) Devices() map[string]device.Device { return nil }
func (m *mockNvmlInstance) ProductName() string               { return "test-gpu" }
func (m *mockNvmlInstance) Architecture() string              { return "test-arch" }
func (m *mockNvmlInstance) Brand() string                     { return "test-brand" }
func (m *mockNvmlInstance) DriverVersion() string             { return "test-version" }
func (m *mockNvmlInstance) DriverMajor() int                  { return 1 }
func (m *mockNvmlInstance) CUDAVersion() string               { return "test-cuda" }
func (m *mockNvmlInstance) FabricManagerSupported() bool      { return false }
func (m *mockNvmlInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}
func (m *mockNvmlInstance) Shutdown() error { return nil }
