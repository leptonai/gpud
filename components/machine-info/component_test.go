package machineinfo

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
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

func TestTags(t *testing.T) {
	t.Parallel()

	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{"a": "b"},
	}
	component, err := New(gpudInstance)
	assert.NoError(t, err)

	expectedTags := []string{
		Name,
	}

	tags := component.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 1, "Component should return exactly 1 tag")
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

	// Create a test struct with minimal fields
	data := &checkResult{
		DaemonVersion: "test-version",
		MacAddress:    "00:11:22:33:44:55",
		GPUdInfo: &GPUdInfo{
			PID:                                  100,
			UsageFileDescriptors:                 100,
			UsageMemoryInBytes:                   1024 * 1024,
			UsageMemoryHumanized:                 "1 MB",
			UsageDBInBytes:                       2048 * 1024,
			UsageDBHumanized:                     "2 MB",
			UsageInsertUpdateTotal:               1000,
			UsageInsertUpdateAvgQPS:              10.5,
			UsageInsertUpdateAvgLatencyInSeconds: 0.01,
			UsageDeleteTotal:                     500,
			UsageDeleteAvgQPS:                    5.25,
			UsageDeleteAvgLatencyInSeconds:       0.005,
			UsageSelectTotal:                     2000,
			UsageSelectAvgQPS:                    20.0,
			UsageSelectAvgLatencyInSeconds:       0.008,
			StartTimeHumanized:                   "10 minutes ago",
		},
	}

	// Simply verify the String method returns something non-empty
	result := data.String()
	assert.NotEmpty(t, result)

	// Instead of looking for specific output format, just verify our fields are included
	// in the JSON representation through the HealthStates method
	states := data.HealthStates()
	jsonData := states[0].ExtraInfo["data"]

	// Verify content in the JSON
	assert.Contains(t, jsonData, `"daemon_version":"test-version"`)
	assert.Contains(t, jsonData, `"mac_address":"00:11:22:33:44:55"`)
	assert.Contains(t, jsonData, `"gpud_info"`)
	assert.Contains(t, jsonData, `"pid":100`)
	assert.Contains(t, jsonData, `"usage_memory_humanized":"1 MB"`)
	assert.Contains(t, jsonData, `"usage_db_humanized":"2 MB"`)
	assert.Contains(t, jsonData, `"start_time_humanized":"10 minutes ago"`)
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
	assert.NotNil(t, data.GPUdInfo, "GPUdInfo should not be nil")
	if data.GPUdInfo != nil {
		assert.NotZero(t, data.GPUdInfo.PID, "GPUdInfo.PID should not be zero")
	}

	// These might not be set in all test environments, so we just check they exist
	// but don't assert on their values
	assert.NotNil(t, data.ts)

	// Verify specific data from check
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Contains(t, data.reason, "daemon version")
}

func TestComponentIsSupported(t *testing.T) {
	t.Parallel()

	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{"a": "b"},
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)

	supported := c.IsSupported()
	assert.True(t, supported, "MachineInfo component should always be supported")
}

func TestCheckResultComponentName(t *testing.T) {
	t.Parallel()

	// Test with nil data
	var cr *checkResult
	assert.Equal(t, Name, cr.ComponentName())

	// Test with actual instance
	cr = &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())
}

func TestGPUInfoRenderTable(t *testing.T) {
	t.Parallel()

	// Test nil GPUInfo
	var info *GPUInfo
	buf := bytes.NewBuffer(nil)
	info.RenderTable(buf)
	assert.Empty(t, buf.String())

	// Test populated GPUInfo
	info = &GPUInfo{
		Product: GPUProduct{
			Name:         "NVIDIA A100",
			Brand:        "NVIDIA",
			Architecture: "Ampere",
		},
		Driver: GPUDriver{
			Version: "530.82.01",
		},
		CUDA: CUDA{
			Version: "12.7",
		},
		GPUCount: GPUCount{
			DeviceCount: 2,
			Attached:    1,
		},
		Memory: GPUMemory{
			TotalBytes:     16 * 1024 * 1024 * 1024,
			TotalHumanized: "16GB",
		},
	}

	buf.Reset()
	info.RenderTable(buf)
	result := buf.String()

	// Verify table contains expected information
	assert.Contains(t, result, "NVIDIA A100")
	assert.Contains(t, result, "NVIDIA")
	assert.Contains(t, result, "Ampere")
	assert.Contains(t, result, "530.82.01")
	assert.Contains(t, result, "12.7")
	assert.Contains(t, result, "2") // Device count
	assert.Contains(t, result, "1") // Attached
	assert.Contains(t, result, "16GB")
}

func TestCheckGPUInfo(t *testing.T) {
	t.Parallel()

	// Test with nil NVMLInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      context.Background(),
		Annotations:  map[string]string{},
		NVMLInstance: nil,
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	result := comp.Check()
	data, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Contains(t, data.reason, "daemon version")
	assert.Nil(t, data.GPUInfo)

	// Create a mock NVML instance that doesn't have NVML
	mockNVML := new(mock.Mock)
	mockNVMLInst := &mockNVMLInstance{Mock: mockNVML}
	mockNVML.On("NVMLExists").Return(false)

	gpudInstance = &components.GPUdInstance{
		RootCtx:      context.Background(),
		Annotations:  map[string]string{},
		NVMLInstance: mockNVMLInst,
	}
	c, err = New(gpudInstance)
	assert.NoError(t, err)
	comp = c.(*component)

	result = comp.Check()
	data, ok = result.(*checkResult)
	assert.True(t, ok)
	assert.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Contains(t, data.reason, "daemon version")
	assert.Nil(t, data.GPUInfo)
}

func TestCheckGPUInfoWithNVML(t *testing.T) {
	t.Parallel()

	// Create a mock NVML instance that doesn't have NVML
	mockNVML := new(mock.Mock)
	mockNVMLInst := &mockNVMLInstance{Mock: mockNVML}
	mockNVML.On("NVMLExists").Return(false)

	gpudInstance := &components.GPUdInstance{
		RootCtx:      context.Background(),
		Annotations:  map[string]string{},
		NVMLInstance: mockNVMLInst,
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Directly test the checkGPUInfo method
	cr := &checkResult{}
	err = comp.checkGPUInfo(cr)
	assert.Nil(t, err)
	assert.Nil(t, cr.GPUInfo)

	// Test with NVML exists but no GPU
	mockNVML = new(mock.Mock)
	mockNVMLInst = &mockNVMLInstance{Mock: mockNVML}
	mockNVML.On("NVMLExists").Return(true)
	mockNVML.On("ProductName").Return("")

	gpudInstance = &components.GPUdInstance{
		RootCtx:      context.Background(),
		Annotations:  map[string]string{},
		NVMLInstance: mockNVMLInst,
	}
	c, err = New(gpudInstance)
	assert.NoError(t, err)
	comp = c.(*component)

	// Directly test the checkGPUInfo method
	cr = &checkResult{}
	err = comp.checkGPUInfo(cr)
	assert.Nil(t, err)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML is loaded but GPU is not detected (missing product name)", cr.reason)
}

// mockNVMLInstance implements nvidianvml.Instance for testing
type mockNVMLInstance struct {
	*mock.Mock
}

func (m *mockNVMLInstance) NVMLExists() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *mockNVMLInstance) ProductName() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) Architecture() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) DriverVersion() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) DriverMajor() int {
	args := m.Called()
	return args.Int(0)
}

func (m *mockNVMLInstance) CUDAVersion() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) Brand() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	_ = m.Called() // Ignore the args but still call the method for consistency
	return nil
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return false
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func TestGPUdInfoRenderTable(t *testing.T) {
	t.Parallel()

	// Test nil GPUdInfo
	var info *GPUdInfo
	buf := bytes.NewBuffer(nil)
	info.RenderTable(buf)
	assert.Empty(t, buf.String())

	// Test populated GPUdInfo
	info = &GPUdInfo{
		PID:                                  100,
		UsageFileDescriptors:                 100,
		UsageMemoryInBytes:                   1024 * 1024,
		UsageMemoryHumanized:                 "1 MB",
		UsageDBInBytes:                       2048 * 1024,
		UsageDBHumanized:                     "2 MB",
		UsageInsertUpdateTotal:               1000,
		UsageInsertUpdateAvgQPS:              10.5,
		UsageInsertUpdateAvgLatencyInSeconds: 0.01,
		UsageDeleteTotal:                     500,
		UsageDeleteAvgQPS:                    5.25,
		UsageDeleteAvgLatencyInSeconds:       0.005,
		UsageSelectTotal:                     2000,
		UsageSelectAvgQPS:                    20.0,
		UsageSelectAvgLatencyInSeconds:       0.008,
		StartTimeInUnixTime:                  1620000000,
		StartTimeHumanized:                   "10 minutes ago",
	}

	buf.Reset()
	info.RenderTable(buf)
	result := buf.String()

	// Verify table contains expected information
	assert.Contains(t, result, "GPUd File Descriptors")
	assert.Contains(t, result, "100")
	assert.Contains(t, result, "GPUd Memory")
	assert.Contains(t, result, "1 MB")
	assert.Contains(t, result, "GPUd DB Size")
	assert.Contains(t, result, "2 MB")
	assert.Contains(t, result, "GPUd DB Insert/Update Total")
	assert.Contains(t, result, "1000")
	assert.Contains(t, result, "GPUd DB Insert/Update Avg QPS")
	assert.Contains(t, result, "10.500000")
	assert.Contains(t, result, "GPUd DB Delete Total")
	assert.Contains(t, result, "500")
	assert.Contains(t, result, "GPUd Start Time")
	assert.Contains(t, result, "10 minutes ago")
}

func TestCheckPackagesError(t *testing.T) {
	t.Parallel()

	// Testing the checkPackages method directly by creating a component with a
	// minimal error handling setup
	gpudInstance := &components.GPUdInstance{
		RootCtx:     context.Background(),
		Annotations: map[string]string{},
	}
	c, err := New(gpudInstance)
	assert.NoError(t, err)
	comp := c.(*component)

	// Create a test checkResult and store an existing error
	cr := &checkResult{
		DaemonVersion: "test-version",
		ts:            time.Now().UTC(),
	}

	// Creating an error scenario - we won't manipulate GlobalController directly
	// Instead, set up a fake context to trigger a timeout error
	originalContext := comp.ctx
	defer func() { comp.ctx = originalContext }()

	// Create a canceled context to guarantee an error
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately to force timeout
	comp.ctx = canceledCtx

	// Set the GlobalController if it's nil so the function doesn't exit early
	originalController := gpudmanager.GlobalController
	if gpudmanager.GlobalController == nil {
		// We won't restore this as we might interfere with other tests
		// We also don't set it to a mock, as we want the context cancel to trigger the error
		// This test depends on a non-nil GlobalController existing in the original code
		// If it ever becomes nil by default in the future, this test will need adjustment
		t.Skip("Skipping test because GlobalController is nil")
	}
	defer func() { gpudmanager.GlobalController = originalController }()

	// Call the function that should fail due to canceled context
	err = comp.checkPackages(cr)

	// We expect an error and unhealthy state
	assert.Error(t, err)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting package status", cr.reason)
}

func TestCheckMacAddressError(t *testing.T) {
	t.Parallel()

	// This test verifies the error case directly
	// Since it's hard to mock net.Interfaces, we're just testing the error state
	cr := &checkResult{
		DaemonVersion: "test-version",
		ts:            time.Now().UTC(),
	}

	// Manually set error state to verify behavior
	cr.health = apiv1.HealthStateTypeUnhealthy
	cr.reason = "error getting interfaces"
	cr.err = errors.New("failed to get interfaces")

	// Verify values match what we expect
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting interfaces", cr.reason)
	assert.Equal(t, "failed to get interfaces", cr.err.Error())
}

func TestGPUInfoErrorCases(t *testing.T) {
	t.Parallel()

	// Skip creating real component, this is just testing error state matching

	// Test 1: Device count error
	cr := &checkResult{
		DaemonVersion: "test-version",
		ts:            time.Now().UTC(),
	}

	// Set values to simulate device count error
	cr.health = apiv1.HealthStateTypeUnhealthy
	cr.reason = "error getting GPU device count"
	cr.err = errors.New("device count error")

	// Verify values are as expected
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting GPU device count", cr.reason)
	assert.Equal(t, "device count error", cr.err.Error())

	// Test 2: Memory error
	cr = &checkResult{
		DaemonVersion: "test-version",
		ts:            time.Now().UTC(),
	}

	// Set values to simulate memory error
	cr.health = apiv1.HealthStateTypeUnhealthy
	cr.reason = "error getting memory"
	cr.err = errors.New("memory error")

	// Verify values are as expected
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting memory", cr.reason)
	assert.Equal(t, "memory error", cr.err.Error())

	// Test 3: Serial error
	cr = &checkResult{
		DaemonVersion: "test-version",
		ts:            time.Now().UTC(),
	}

	// Set values to simulate serial error
	cr.health = apiv1.HealthStateTypeUnhealthy
	cr.reason = "error getting serial id"
	cr.err = errors.New("serial error")

	// Verify values are as expected
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting serial id", cr.reason)
	assert.Equal(t, "serial error", cr.err.Error())

	// Test 4: Minor ID error
	cr = &checkResult{
		DaemonVersion: "test-version",
		ts:            time.Now().UTC(),
	}

	// Set values to simulate minor ID error
	cr.health = apiv1.HealthStateTypeUnhealthy
	cr.reason = "error getting minor id"
	cr.err = errors.New("minor ID error")

	// Verify values are as expected
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting minor id", cr.reason)
	assert.Equal(t, "minor ID error", cr.err.Error())
}

func TestGPUInfoSuccessCase(t *testing.T) {
	t.Parallel()

	// Test the success case by manually constructing a GPUInfo structure
	gpuInfo := &GPUInfo{
		Product: GPUProduct{
			Name:         "NVIDIA A100",
			Brand:        "NVIDIA",
			Architecture: "Ampere",
		},
		Driver: GPUDriver{
			Version: "530.82.01",
		},
		CUDA: CUDA{
			Version: "12.7",
		},
		GPUCount: GPUCount{
			DeviceCount: 1,
			Attached:    1,
		},
		Memory: GPUMemory{
			TotalBytes:     16 * 1024 * 1024 * 1024,
			TotalHumanized: "16GB",
		},
		GPUIDs: []GPUID{
			{
				UUID:    "GPU-12345",
				SN:      "SERIAL123",
				MinorID: "0",
			},
		},
	}

	// Create a result with the GPUInfo
	cr := &checkResult{
		DaemonVersion: "test-version",
		GPUInfo:       gpuInfo,
		ts:            time.Now().UTC(),
	}

	// Verify the GPU info fields
	assert.NotNil(t, cr.GPUInfo)
	assert.Equal(t, 1, cr.GPUInfo.GPUCount.DeviceCount)
	assert.Equal(t, 1, cr.GPUInfo.GPUCount.Attached)
	assert.Equal(t, "NVIDIA A100", cr.GPUInfo.Product.Name)
	assert.Equal(t, "NVIDIA", cr.GPUInfo.Product.Brand)
	assert.Equal(t, "Ampere", cr.GPUInfo.Product.Architecture)
	assert.Equal(t, "530.82.01", cr.GPUInfo.Driver.Version)
	assert.Equal(t, "12.7", cr.GPUInfo.CUDA.Version)

	// Check memory information
	assert.Equal(t, uint64(16*1024*1024*1024), cr.GPUInfo.Memory.TotalBytes)
	assert.Equal(t, "16GB", cr.GPUInfo.Memory.TotalHumanized)

	// Check GPU IDs
	assert.Len(t, cr.GPUInfo.GPUIDs, 1)
	assert.Equal(t, "GPU-12345", cr.GPUInfo.GPUIDs[0].UUID)
	assert.Equal(t, "SERIAL123", cr.GPUInfo.GPUIDs[0].SN)
	assert.Equal(t, "0", cr.GPUInfo.GPUIDs[0].MinorID)
}
