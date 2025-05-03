package info

import (
	"context"
	"errors"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponentName(t *testing.T) {
	t.Parallel()

	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{"a": "b"},
	}
	component, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.Equal(t, Name, component.Name())
}

func TestComponentStartAndClose(t *testing.T) {
	t.Parallel()

	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{"a": "b"},
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	err = comp.Start()
	assert.NoError(t, err)

	// Wait a bit for Check to execute
	time.Sleep(100 * time.Millisecond)

	// Verify data was collected
	comp.lastMu.RLock()
	assert.NotNil(t, comp.lastCheckResult)
	comp.lastMu.RUnlock()

	err = comp.Close()
	assert.NoError(t, err)
}

func TestComponentStates(t *testing.T) {
	t.Parallel()

	// Create component with database
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{"test": "value"},
		DBRO:        dbRO,
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Run CheckOnce to gather data
	comp.Check()

	// Check the states
	states := comp.LastHealthStates()
	require.Len(t, states, 1)
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{},
	}
	component, err := New(gpudInstance)
	assert.NoError(t, err)
	ctx := context.Background()

	events, err := component.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestComponentWithDB(t *testing.T) {
	t.Parallel()

	// Create component with database
	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{},
		DBRO:        dbRO,
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Run CheckOnce to gather database metrics
	comp.Check()

	// Verify that database metrics are collected
	comp.lastMu.RLock()
	data := comp.lastCheckResult
	comp.lastMu.RUnlock()

	assert.NotNil(t, data)
}

func TestDataGetStatesForHealth(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *checkResult
	states := nilData.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)

	// Test with error
	data := &checkResult{
		err:    assert.AnError,
		health: apiv1.HealthStateTypeUnhealthy,
	}
	states = data.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, assert.AnError.Error(), states[0].Error)

	// Test without error
	data = &checkResult{
		health: apiv1.HealthStateTypeHealthy,
	}
	states = data.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

func TestDataGetStatesForReason(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *checkResult
	states := nilData.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with error
	data := &checkResult{
		err:    assert.AnError,
		reason: "failed to get info data",
	}
	states = data.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "failed to get info data", states[0].Reason)

	// Test without error
	data = &checkResult{
		MacAddress:  "00:11:22:33:44:55",
		Annotations: map[string]string{"test": "value"},
		reason:      "daemon version: test, mac address: 00:11:22:33:44:55",
	}
	states = data.HealthStates()
	assert.Len(t, states, 1)
	assert.Contains(t, states[0].Reason, "daemon version")
	assert.Contains(t, states[0].Reason, "mac address: 00:11:22:33:44:55")
}

func TestDataGetStatesNil(t *testing.T) {
	// Test with nil data
	var cr *checkResult
	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
	assert.Empty(t, states[0].Error, "Error should be empty for nil data")
}

func TestDataGetError(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *checkResult
	errStr := nilData.getError()
	assert.Equal(t, "", errStr)

	// Test with error
	data := &checkResult{err: assert.AnError}
	errStr = data.getError()
	assert.Equal(t, assert.AnError.Error(), errStr)

	// Test without error
	data = &checkResult{}
	errStr = data.getError()
	assert.Equal(t, "", errStr)
}

func TestDataGetStatesWithExtraInfo(t *testing.T) {
	t.Parallel()

	// Test with basic data
	data := &checkResult{
		DaemonVersion: "test-version",
		MacAddress:    "00:11:22:33:44:55",
		Annotations:   map[string]string{"key": "value"},
		health:        apiv1.HealthStateTypeHealthy,
		reason:        "test reason",
	}

	states := data.HealthStates()
	assert.Len(t, states, 1)

	// Check extraInfo contains JSON data
	assert.Contains(t, states[0].ExtraInfo, "data")

	// Verify the JSON data contains our fields
	jsonData := states[0].ExtraInfo["data"]
	assert.Contains(t, jsonData, "test-version")
	assert.Contains(t, jsonData, "00:11:22:33:44:55")
	assert.Contains(t, jsonData, "key")
	assert.Contains(t, jsonData, "value")
}

// Additional tests for more coverage

func TestDataString(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *checkResult
	assert.Equal(t, "", nilData.String())

	// Test with populated data
	data := &checkResult{
		DaemonVersion:                            "test-version",
		MacAddress:                               "00:11:22:33:44:55",
		GPUdUsageFileDescriptors:                 100,
		GPUdUsageMemoryInBytes:                   1024 * 1024,
		GPUdUsageMemoryHumanized:                 "1 MB",
		GPUdUsageDBInBytes:                       2048 * 1024,
		GPUdUsageDBHumanized:                     "2 MB",
		GPUdUsageInsertUpdateTotal:               1000,
		GPUdUsageInsertUpdateAvgQPS:              10.5,
		GPUdUsageInsertUpdateAvgLatencyInSeconds: 0.01,
		GPUdUsageDeleteTotal:                     500,
		GPUdUsageDeleteAvgQPS:                    5.25,
		GPUdUsageDeleteAvgLatencyInSeconds:       0.005,
		GPUdUsageSelectTotal:                     2000,
		GPUdUsageSelectAvgQPS:                    20.0,
		GPUdUsageSelectAvgLatencyInSeconds:       0.008,
		GPUdStartTimeHumanized:                   "10 minutes ago",
	}

	result := data.String()
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "test-version")
	assert.Contains(t, result, "00:11:22:33:44:55")
	assert.Contains(t, result, "100")
	assert.Contains(t, result, "1 MB")
	assert.Contains(t, result, "2 MB")
	assert.Contains(t, result, "1000")
	assert.Contains(t, result, "10.500000")
	assert.Contains(t, result, "0.010000")
	assert.Contains(t, result, "500")
	assert.Contains(t, result, "5.250000")
	assert.Contains(t, result, "0.005000")
	assert.Contains(t, result, "2000")
	assert.Contains(t, result, "20.000000")
	assert.Contains(t, result, "0.008000")
	assert.Contains(t, result, "10 minutes ago")
}

func TestDataSummary(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *checkResult
	assert.Equal(t, "", nilData.Summary())

	// Test with reason set
	data := &checkResult{
		reason: "test summary reason",
	}
	assert.Equal(t, "test summary reason", data.Summary())
}

func TestDataHealthState(t *testing.T) {
	t.Parallel()

	// Test with nil Data
	var nilData *checkResult
	assert.Equal(t, apiv1.HealthStateType(""), nilData.HealthStateType())

	// Test with health set
	data := &checkResult{
		health: apiv1.HealthStateTypeHealthy,
	}
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.HealthStateType())

	// Test with unhealthy state
	data = &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
	}
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.HealthStateType())
}

func TestCheckWithErrors(t *testing.T) {
	t.Parallel()

	// Test error case for check with nil database
	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{},
	}
	c, err := New(gpudInstance)
	require.NoError(t, err)
	comp := c.(*component)

	// Replace gatherer with a mock that returns error
	originalGatherer := comp.gatherer
	defer func() { comp.gatherer = originalGatherer }()

	mockGatherer := &mockErrorGatherer{}
	comp.gatherer = mockGatherer

	// Run check and verify it handles metrics error correctly
	result := comp.Check()
	assert.NotNil(t, result)
	data, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "error getting SQLite metrics")
	assert.Error(t, data.err)
}

// Mock gatherer that always returns error
type mockErrorGatherer struct{}

func (m *mockErrorGatherer) Gather() ([]*dto.MetricFamily, error) {
	return nil, errors.New("mock gatherer error")
}

func TestLastHealthStatesWithNilData(t *testing.T) {
	t.Parallel()

	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{},
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Don't run Check, so lastCheckResult remains nil
	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestNewWithEmptyAnnotations(t *testing.T) {
	t.Parallel()

	gpudInstance := &components.GPUdInstance{
		RootCtx: context.Background(),
		// No annotations provided, should initialize to empty map or nil
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Test passes whether annotations is nil or empty map
	// Just ensure we can use annotations without panic
	if comp.annotations != nil {
		assert.Empty(t, comp.annotations)
	}
	// Component Check should handle nil annotations without issues
	result := comp.Check()
	assert.NotNil(t, result)
}

func TestCheckDataFieldInitialization(t *testing.T) {
	t.Parallel()

	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{"test": "value"},
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	result := comp.Check()
	data, ok := result.(*checkResult)
	assert.True(t, ok)

	// Verify that data fields are set correctly
	assert.NotEmpty(t, data.DaemonVersion)
	assert.Equal(t, gpudInstance.Annotations, data.Annotations)
	assert.NotZero(t, data.ts)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Contains(t, data.reason, "daemon version")
}

// Test more branches of Check method
func TestCheckWithMoreBranches(t *testing.T) {
	t.Parallel()

	// Create a minimal component
	gpudInstance := &components.GPUdInstance{
		RootCtx: context.Background(),
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Run check with a real dataset
	result := comp.Check()
	assert.NotNil(t, result)
	data, ok := result.(*checkResult)
	assert.True(t, ok)

	// Verify data contains minimal expected information
	// These fields should always be available
	assert.NotEmpty(t, data.DaemonVersion, "DaemonVersion should not be empty")
	assert.NotZero(t, data.GPUdPID, "GPUdPID should not be zero")

	// These might not be set in all test environments, so we just check they exist
	// but don't assert on their values
	assert.NotNil(t, data.ts)

	// Verify specific data from check
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Contains(t, data.reason, "daemon version")
}
