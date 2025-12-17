package gpucounts

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// mockNVMLInstance implements the nvidianvml.Instance interface for testing
type mockNVMLInstance struct {
	devices      map[string]device.Device
	nvmlExists   bool
	productName  string
	architecture string
	brand        string
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.nvmlExists
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockNVMLInstance) ProductName() string {
	return m.productName
}

func (m *mockNVMLInstance) Architecture() string {
	return m.architecture
}

func (m *mockNVMLInstance) Brand() string {
	return m.brand
}

func (m *mockNVMLInstance) DriverVersion() string {
	return "test-driver-version"
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 1
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return "test-cuda-version"
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return true
}

func (m *mockNVMLInstance) FabricStateSupported() bool {
	return false
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func (m *mockNVMLInstance) InitError() error {
	return nil
}

// mockRebootEventStore implements pkghost.RebootEventStore for testing
type mockRebootEventStore struct {
	events eventstore.Events
	err    error
}

// Ensure mockRebootEventStore implements pkghost.RebootEventStore
var _ pkghost.RebootEventStore = (*mockRebootEventStore)(nil)

func (m *mockRebootEventStore) RecordReboot(ctx context.Context) error {
	return nil
}

func (m *mockRebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

// mockEventBucket implements eventstore.Bucket for testing
type mockEventBucket struct {
	name                 string
	events               []eventstore.Event
	insertErr            error
	findErr              error
	getErr               error
	latestErr            error
	purgeErr             error
	foundEvent           *eventstore.Event
	latestEvent          *eventstore.Event
	insertCalled         bool
	findCalled           bool
	purgeCalled          bool
	purgeBeforeTimestamp int64
	purgeReturnCount     int
}

func (m *mockEventBucket) Name() string {
	return m.name
}

func (m *mockEventBucket) Insert(ctx context.Context, ev eventstore.Event) error {
	m.insertCalled = true
	if m.insertErr != nil {
		return m.insertErr
	}
	m.events = append(m.events, ev)
	return nil
}

func (m *mockEventBucket) Find(ctx context.Context, ev eventstore.Event) (*eventstore.Event, error) {
	m.findCalled = true
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.foundEvent, nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	var result eventstore.Events
	// Include all events in the bucket (including newly inserted ones)
	for _, ev := range m.events {
		if ev.Time.After(since) || ev.Time.Equal(since) {
			result = append(result, ev)
		}
	}
	return result, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	if m.latestErr != nil {
		return nil, m.latestErr
	}
	return m.latestEvent, nil
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	m.purgeCalled = true
	m.purgeBeforeTimestamp = beforeTimestamp
	if m.purgeErr != nil {
		return 0, m.purgeErr
	}
	return m.purgeReturnCount, nil
}

func (m *mockEventBucket) Close() {}

// TestNew tests the New function
func TestNew(t *testing.T) {
	ctx := context.Background()
	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}
	mockInstance := &mockNVMLInstance{
		devices:     mockDevices,
		nvmlExists:  true,
		productName: "test-product",
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockInstance,
	}
	comp, err := New(gpudInstance)
	require.NoError(t, err)

	c, ok := comp.(*component)
	require.True(t, ok)
	assert.Equal(t, mockInstance, c.nvmlInstance)
	assert.NotNil(t, c.getCountLspci)
	assert.NotNil(t, c.getThresholdsFunc)
	assert.NotNil(t, c.ctx)
	assert.NotNil(t, c.cancel)

	// Test that getCountLspci works correctly
	count, err := c.getCountLspci(ctx)
	// It may fail if lspci is not available on the test system
	if err == nil {
		assert.GreaterOrEqual(t, count, 0)
	}
}

// TestComponent_Name tests the Name method
func TestComponent_Name(t *testing.T) {
	c := &component{}
	assert.Equal(t, Name, c.Name())
}

// TestComponent_Tags tests the Tags method
func TestComponent_Tags(t *testing.T) {
	c := &component{}

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags)
	assert.Len(t, tags, 4)
}

// TestComponent_IsSupported tests the IsSupported method
func TestComponent_IsSupported(t *testing.T) {
	tests := []struct {
		name      string
		component *component
		expected  bool
	}{
		{
			name: "nil nvml instance",
			component: &component{
				nvmlInstance: nil,
			},
			expected: false,
		},
		{
			name: "nvml not exists",
			component: &component{
				nvmlInstance: &mockNVMLInstance{
					nvmlExists: false,
				},
			},
			expected: false,
		},
		{
			name: "empty product name",
			component: &component{
				nvmlInstance: &mockNVMLInstance{
					nvmlExists:  true,
					productName: "",
				},
			},
			expected: false,
		},
		{
			name: "supported",
			component: &component{
				nvmlInstance: &mockNVMLInstance{
					nvmlExists:  true,
					productName: "test-product",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.component.IsSupported()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestComponent_Start tests the Start method
func TestComponent_Start(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}
	mockInstance := &mockNVMLInstance{
		devices:     mockDevices,
		nvmlExists:  true,
		productName: "test-product",
	}

	c := &component{
		ctx:          ctx,
		cancel:       cancel,
		nvmlInstance: mockInstance,
		getCountLspci: func(ctx context.Context) (int, error) {
			return 1, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 0}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	err := c.Start()
	assert.NoError(t, err)

	// Allow the goroutine time to initialize
	time.Sleep(100 * time.Millisecond)
}

// TestComponent_LastHealthStates tests the LastHealthStates method
func TestComponent_LastHealthStates(t *testing.T) {
	ctx := context.Background()

	// Test when lastCheckResult is nil
	c := &component{
		ctx: ctx,
	}

	states := c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with valid data
	c.lastCheckResult = &checkResult{
		ProductName: "test-product",
		CountLspci:  2,
		CountNVML:   2,
		ts:          time.Now(),
		health:      apiv1.HealthStateTypeHealthy,
		reason:      "all good",
	}

	states = c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "all good", states[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

// TestComponent_Events tests the Events method
func TestComponent_Events(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx: ctx,
	}

	events, err := c.Events(ctx, time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)
}

// TestComponent_Events_Simple tests the Events method with different time values
func TestComponent_Events_Simple(t *testing.T) {
	tests := []struct {
		name  string
		since time.Time
	}{
		{
			name:  "events since 1 hour ago",
			since: time.Now().Add(-time.Hour),
		},
		{
			name:  "events since 1 day ago",
			since: time.Now().Add(-24 * time.Hour),
		},
		{
			name:  "events since epoch",
			since: time.Unix(0, 0),
		},
		{
			name:  "events since future time",
			since: time.Now().Add(time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			c := &component{}

			events, err := c.Events(ctx, tt.since)
			assert.NoError(t, err)
			assert.Nil(t, events)
		})
	}
}

// TestComponent_Close tests the Close method
func TestComponent_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,
	}

	err := c.Close()
	assert.NoError(t, err)
}

// TestComponent_Check_NilNVML tests the Check method with nil NVML instance
func TestComponent_Check_NilNVML(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx:          ctx,
		nvmlInstance: nil,
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", cr.reason)
	assert.Nil(t, cr.err)
}

// TestComponent_Check_NVMLNotLoaded tests the Check method when NVML is not loaded
func TestComponent_Check_NVMLNotLoaded(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			nvmlExists: false,
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", cr.reason)
	assert.Nil(t, cr.err)
}

// TestComponent_Check_EmptyProductName tests the Check method when product name is empty
func TestComponent_Check_EmptyProductName(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			nvmlExists:  true,
			productName: "",
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML is loaded but GPU is not detected (missing product name)", cr.reason)
	assert.Nil(t, cr.err)
}

// TestComponent_Check_Success tests the Check method for successful case
func TestComponent_Check_Success(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid-1": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		"test-uuid-2": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:01.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, "test-product", cr.ProductName)
	assert.Equal(t, 2, cr.CountLspci)
	assert.Equal(t, 2, cr.CountNVML)
	assert.Nil(t, cr.err)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "nvidia gpu count matching thresholds (2)", cr.reason)

	// Check that lastCheckResult was updated
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, cr, c.lastCheckResult)
}

// TestComponent_Check_LspciError tests the Check method when lspci fails
func TestComponent_Check_LspciError(t *testing.T) {
	ctx := context.Background()
	testErr := errors.New("lspci error")

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 0, testErr
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// With the new behavior, lspci errors don't cause health failures
	// Only NVML count is checked against threshold
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "nvidia gpu count matching thresholds (1)", cr.reason)
	assert.Nil(t, cr.err)             // No error stored in checkResult anymore
	assert.Equal(t, 0, cr.CountLspci) // Still set to 0 due to error
	assert.Equal(t, 1, cr.CountNVML)  // NVML count matches threshold
}

// TestComponent_Check_ThresholdNotSet tests the Check method when threshold is not set
func TestComponent_Check_ThresholdNotSet(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 1, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 0} // Zero threshold
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, reasonThresholdNotSetSkipped, cr.reason)
}

// TestComponent_Check_LspciCountMismatch tests when lspci count doesn't match threshold
func TestComponent_Check_LspciCountMismatch(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid-1": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		"test-uuid-2": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:01.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 1, nil // lspci reports 1 GPU
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2} // Expecting 2 GPUs
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// With the new behavior, lspci count mismatches don't cause health failures
	// Only NVML count is checked against threshold, and NVML count (2) matches threshold (2)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "nvidia gpu count matching thresholds (2)", cr.reason)
	assert.Nil(t, cr.err)
	assert.Equal(t, 1, cr.CountLspci) // lspci count still recorded
	assert.Equal(t, 2, cr.CountNVML)  // NVML count matches threshold
}

// TestComponent_Check_NVMLCountMismatch tests when NVML count doesn't match threshold
func TestComponent_Check_NVMLCountMismatch(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil // lspci reports 2 GPUs
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2} // Expecting 2 GPUs
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (found 1, expected 2)", cr.reason)
	assert.Nil(t, cr.err)
	assert.Equal(t, 2, cr.CountLspci)
	assert.Equal(t, 1, cr.CountNVML) // Only 1 device in mockDevices
}

// TestComponent_Check_NVMLZeroDevices tests when NVML reports zero devices
func TestComponent_Check_NVMLZeroDevices(t *testing.T) {
	ctx := context.Background()

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     map[string]device.Device{}, // Empty devices map
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil // lspci reports 2 GPUs
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2} // Expecting 2 GPUs
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (found 0, expected 2)", cr.reason)
	assert.Nil(t, cr.err)
	assert.Equal(t, 2, cr.CountLspci)
	assert.Equal(t, 0, cr.CountNVML)
}

// TestCheckResult_String tests the String method of checkResult
func TestCheckResult_String(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	str := nilCR.String()
	assert.Empty(t, str)

	// Test empty checkResult
	emptyCR := &checkResult{}
	str = emptyCR.String()
	assert.Equal(t, "no data", str)

	// Test with data
	cr := &checkResult{
		ProductName: "test-product",
		CountLspci:  2,
		CountNVML:   3,
	}

	str = cr.String()
	assert.Contains(t, str, "test-product")
	assert.Contains(t, str, "2")
	assert.Contains(t, str, "3")
}

// TestCheckResult_Summary tests the Summary method of checkResult
func TestCheckResult_Summary(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	summary := nilCR.Summary()
	assert.Empty(t, summary)

	// Test with reason
	cr := &checkResult{
		reason: "test reason",
	}
	summary = cr.Summary()
	assert.Equal(t, "test reason", summary)
}

// TestCheckResult_HealthStateType tests the HealthStateType method of checkResult
func TestCheckResult_HealthStateType(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	health := nilCR.HealthStateType()
	assert.Empty(t, health)

	// Test with health state
	cr := &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
	}
	health = cr.HealthStateType()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, health)
}

// TestCheckResult_GetSuggestedActions tests the getSuggestedActions method
func TestCheckResult_GetSuggestedActions(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	actions := nilCR.getSuggestedActions()
	assert.Nil(t, actions)

	// Test with no suggested actions
	cr := &checkResult{}
	actions = cr.getSuggestedActions()
	assert.Nil(t, actions)

	// Test with suggested actions
	expectedActions := &apiv1.SuggestedActions{
		Description:   "Test issue description",
		RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem, apiv1.RepairActionTypeHardwareInspection},
	}
	cr = &checkResult{
		suggestedActions: expectedActions,
	}
	actions = cr.getSuggestedActions()
	assert.Equal(t, expectedActions, actions)
}

// TestCheckResult_GetError tests the getError method
func TestCheckResult_GetError(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	errStr := nilCR.getError()
	assert.Empty(t, errStr)

	// Test with no error
	cr := &checkResult{}
	errStr = cr.getError()
	assert.Empty(t, errStr)

	// Test with error
	testErr := errors.New("test error")
	cr = &checkResult{
		err: testErr,
	}
	errStr = cr.getError()
	assert.Equal(t, "test error", errStr)
}

// TestCheckResult_ComponentName tests the ComponentName method
func TestCheckResult_ComponentName(t *testing.T) {
	cr := &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())
}

// TestCheckResult_HealthStates tests the HealthStates method
func TestCheckResult_HealthStates(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	states := nilCR.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with data
	testTime := time.Now()
	testErr := errors.New("test error")
	suggestedActions := &apiv1.SuggestedActions{
		Description:   "Test issue description",
		RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeCheckUserAppAndGPU},
	}

	cr := &checkResult{
		ProductName:      "test-product",
		CountLspci:       2,
		CountNVML:        3,
		ts:               testTime,
		health:           apiv1.HealthStateTypeUnhealthy,
		reason:           "test reason",
		err:              testErr,
		suggestedActions: suggestedActions,
	}

	states = cr.HealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "test reason", state.Reason)
	assert.Equal(t, "test error", state.Error)
	assert.Equal(t, suggestedActions, state.SuggestedActions)
}

// TestExpectedGPUCounts_IsZero tests the IsZero method from threshold.go
func TestExpectedGPUCounts_IsZero(t *testing.T) {
	// Test nil
	var nilEC *ExpectedGPUCounts
	assert.True(t, nilEC.IsZero())

	// Test zero count
	ec := &ExpectedGPUCounts{Count: 0}
	assert.True(t, ec.IsZero())

	// Test negative count
	ec = &ExpectedGPUCounts{Count: -1}
	assert.True(t, ec.IsZero())

	// Test positive count
	ec = &ExpectedGPUCounts{Count: 1}
	assert.False(t, ec.IsZero())
}

// TestDefaultExpectedGPUCounts tests the Get/Set functions for default thresholds
func TestDefaultExpectedGPUCounts(t *testing.T) {
	// Test initial default
	initial := GetDefaultExpectedGPUCounts()
	assert.Equal(t, 0, initial.Count)

	// Test setting new default
	newCounts := ExpectedGPUCounts{Count: 4}
	SetDefaultExpectedGPUCounts(newCounts)

	retrieved := GetDefaultExpectedGPUCounts()
	assert.Equal(t, 4, retrieved.Count)

	// Reset to original
	SetDefaultExpectedGPUCounts(ExpectedGPUCounts{Count: 0})
}

// TestComponent_Integration tests the full lifecycle of the component
func TestComponent_Integration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock devices
	mockDevices := map[string]device.Device{
		"test-uuid-1": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		"test-uuid-2": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:01.0"),
	}

	mockInstance := &mockNVMLInstance{
		devices:     mockDevices,
		nvmlExists:  true,
		productName: "test-product",
	}

	// Create GPUd instance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockInstance,
	}

	// Create component
	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Test component properties
	assert.Equal(t, Name, comp.Name())
	assert.True(t, comp.IsSupported())
	assert.Len(t, comp.Tags(), 4)

	// Start the component
	err = comp.Start()
	assert.NoError(t, err)

	// Allow the goroutine to run at least one check
	time.Sleep(200 * time.Millisecond)

	// Check health states
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)

	// Ensure the check has run
	if states[0].Reason != "no data yet" {
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, reasonThresholdNotSetSkipped, states[0].Reason)
	}

	// Check events (should be nil for this component)
	events, err := comp.Events(ctx, time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)

	// Close the component
	err = comp.Close()
	assert.NoError(t, err)
}

// TestComponent_ConcurrentAccess tests concurrent access to the component
func TestComponent_ConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 1, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	// Run multiple checks concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Check()
			_ = c.LastHealthStates()
		}()
	}

	wg.Wait()

	// Verify the final state
	states := c.LastHealthStates()
	assert.Len(t, states, 1)
}

// TestComponent_Check_NilLspciFunc tests when getCountLspci is nil
func TestComponent_Check_NilLspciFunc(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: nil, // Explicitly set to nil
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// With the new behavior, nil lspci function doesn't cause health failures
	// Only NVML count is checked against threshold, and NVML count (1) matches threshold (1)
	assert.Equal(t, "test-product", cr.ProductName)
	assert.Equal(t, 0, cr.CountLspci) // Should be zero since getCountLspci is nil
	assert.Equal(t, 1, cr.CountNVML)
	assert.Nil(t, cr.err)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "nvidia gpu count matching thresholds (1)", cr.reason)
}

// TestComponent_Check_ContextCancellation tests behavior when context is canceled during lspci call
func TestComponent_Check_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context immediately

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			// Return context canceled error
			return 0, ctx.Err()
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// With the new behavior, lspci context cancellation doesn't cause health failures
	// Only NVML count is checked against threshold, and NVML count (1) matches threshold (1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "nvidia gpu count matching thresholds (1)", cr.reason)
	assert.Nil(t, cr.err) // No error stored in checkResult
}

// TestComponent_Check_LargeGPUCount tests with a large number of GPUs
func TestComponent_Check_LargeGPUCount(t *testing.T) {
	ctx := context.Background()

	// Create many mock devices
	mockDevices := make(map[string]device.Device)
	for i := 0; i < 16; i++ {
		uuid := fmt.Sprintf("test-uuid-%d", i)
		pciAddress := fmt.Sprintf("0000:00:%02x.0", i)
		mockDevices[uuid] = testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", pciAddress)
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product-multi-gpu",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 16, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 16}
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, "test-product-multi-gpu", cr.ProductName)
	assert.Equal(t, 16, cr.CountLspci)
	assert.Equal(t, 16, cr.CountNVML)
	assert.Nil(t, cr.err)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "nvidia gpu count matching thresholds (16)", cr.reason)
}

// TestComponent_Check_NegativeThreshold tests with negative threshold values
func TestComponent_Check_NegativeThreshold(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 1, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: -5} // Negative threshold
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Negative threshold should be treated as zero (IsZero returns true)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, reasonThresholdNotSetSkipped, cr.reason)
}

// TestComponent_Check_ProductNameEdgeCases tests various product name scenarios
func TestComponent_Check_ProductNameEdgeCases(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name        string
		productName string
		expected    string
	}{
		{
			name:        "whitespace product name",
			productName: "   ",
			expected:    "   ",
		},
		{
			name:        "special characters",
			productName: "NVIDIA-GeForce-RTX-4090/Ti",
			expected:    "NVIDIA-GeForce-RTX-4090/Ti",
		},
		{
			name:        "very long product name",
			productName: "Very-Long-Product-Name-That-Exceeds-Normal-Length-NVIDIA-GeForce-RTX-4090-Super-Ti-Ultra-Max",
			expected:    "Very-Long-Product-Name-That-Exceeds-Normal-Length-NVIDIA-GeForce-RTX-4090-Super-Ti-Ultra-Max",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevices := map[string]device.Device{
				"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
			}

			c := &component{
				ctx: ctx,
				nvmlInstance: &mockNVMLInstance{
					devices:     mockDevices,
					nvmlExists:  true,
					productName: tc.productName,
				},
				getCountLspci: func(ctx context.Context) (int, error) {
					return 1, nil
				},
				getThresholdsFunc: func() ExpectedGPUCounts {
					return ExpectedGPUCounts{Count: 1}
				},
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
			}

			result := c.Check()
			cr, ok := result.(*checkResult)
			require.True(t, ok)

			assert.Equal(t, tc.expected, cr.ProductName)
			assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		})
	}
}

// TestComponent_Check_BoundaryValues tests boundary value scenarios
func TestComponent_Check_BoundaryValues(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name           string
		lspciCount     int
		nvmlDevices    int
		thresholdCount int
		expectedHealth apiv1.HealthStateType
		expectedReason string
	}{
		{
			name:           "all zero counts",
			lspciCount:     0,
			nvmlDevices:    0,
			thresholdCount: 0,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonThresholdNotSetSkipped,
		},
		{
			name:           "threshold 1, all others 0",
			lspciCount:     0,
			nvmlDevices:    0,
			thresholdCount: 1,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedReason: "nvidia gpu count mismatch (found 0, expected 1)",
		},
		{
			name:           "edge case with large numbers",
			lspciCount:     1000,
			nvmlDevices:    1000,
			thresholdCount: 1000,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "nvidia gpu count matching thresholds (1000)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the number of devices specified
			mockDevices := make(map[string]device.Device)
			for i := 0; i < tc.nvmlDevices; i++ {
				uuid := fmt.Sprintf("test-uuid-%d", i)
				pciAddress := fmt.Sprintf("0000:%02x:00.0", i%256)
				mockDevices[uuid] = testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", pciAddress)
			}

			c := &component{
				ctx: ctx,
				nvmlInstance: &mockNVMLInstance{
					devices:     mockDevices,
					nvmlExists:  true,
					productName: "test-product",
				},
				getCountLspci: func(ctx context.Context) (int, error) {
					return tc.lspciCount, nil
				},
				getThresholdsFunc: func() ExpectedGPUCounts {
					return ExpectedGPUCounts{Count: tc.thresholdCount}
				},
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
			}

			result := c.Check()
			cr, ok := result.(*checkResult)
			require.True(t, ok)

			assert.Equal(t, tc.expectedHealth, cr.health)
			assert.Equal(t, tc.expectedReason, cr.reason)
			assert.Equal(t, tc.lspciCount, cr.CountLspci)
			assert.Equal(t, tc.nvmlDevices, cr.CountNVML)
		})
	}
}

// TestComponent_Check_ErrorMessageValidation tests that error messages contain expected values
func TestComponent_Check_ErrorMessageValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("nvml mismatch error message", func(t *testing.T) {
		mockDevices := map[string]device.Device{
			"test-uuid-1": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
			"test-uuid-2": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:01.0"),
			"test-uuid-3": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:02.0"),
		}

		c := &component{
			ctx: ctx,
			nvmlInstance: &mockNVMLInstance{
				devices:     mockDevices,
				nvmlExists:  true,
				productName: "test-product",
			},
			getCountLspci: func(ctx context.Context) (int, error) {
				return 5, nil // This doesn't matter for health check anymore
			},
			getThresholdsFunc: func() ExpectedGPUCounts {
				return ExpectedGPUCounts{Count: 8} // NVML count (3) != threshold (8)
			},
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Only NVML count is checked against threshold now
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "found 3")
		assert.Contains(t, cr.reason, "expected 8")
		assert.Contains(t, cr.reason, "nvidia gpu count mismatch")
	})

	t.Run("nvml mismatch error message - updated test", func(t *testing.T) {
		mockDevices := map[string]device.Device{
			"test-uuid-1": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
			"test-uuid-2": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:01.0"),
			"test-uuid-3": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:02.0"),
		}

		c := &component{
			ctx: ctx,
			nvmlInstance: &mockNVMLInstance{
				devices:     mockDevices,
				nvmlExists:  true,
				productName: "test-product",
			},
			getCountLspci: func(ctx context.Context) (int, error) {
				return 5, nil // This doesn't matter for health check anymore
			},
			getThresholdsFunc: func() ExpectedGPUCounts {
				return ExpectedGPUCounts{Count: 8} // NVML count (3) != threshold (8)
			},
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Only NVML count is checked against threshold now
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "found 3")
		assert.Contains(t, cr.reason, "expected 8")
		assert.Contains(t, cr.reason, "nvidia gpu count mismatch")
	})
}

// TestComponent_Check_LspciLoggingBehavior tests that lspci results are logged but don't affect health
func TestComponent_Check_LspciLoggingBehavior(t *testing.T) {
	ctx := context.Background()

	t.Run("lspci success logging", func(t *testing.T) {
		mockDevices := map[string]device.Device{
			"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		}

		c := &component{
			ctx: ctx,
			nvmlInstance: &mockNVMLInstance{
				devices:     mockDevices,
				nvmlExists:  true,
				productName: "test-product",
			},
			getCountLspci: func(ctx context.Context) (int, error) {
				return 8, nil // Successfully returns count
			},
			getThresholdsFunc: func() ExpectedGPUCounts {
				return ExpectedGPUCounts{Count: 1} // NVML count (1) matches threshold (1)
			},
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Health status is determined only by NVML count vs threshold
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "nvidia gpu count matching thresholds (1)", cr.reason)
		assert.Equal(t, 8, cr.CountLspci) // lspci count is still recorded
		assert.Equal(t, 1, cr.CountNVML)  // NVML count matches threshold
		assert.Nil(t, cr.err)
	})

	t.Run("lspci error logging", func(t *testing.T) {
		mockDevices := map[string]device.Device{
			"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		}

		c := &component{
			ctx: ctx,
			nvmlInstance: &mockNVMLInstance{
				devices:     mockDevices,
				nvmlExists:  true,
				productName: "test-product",
			},
			getCountLspci: func(ctx context.Context) (int, error) {
				return 0, errors.New("lspci command failed")
			},
			getThresholdsFunc: func() ExpectedGPUCounts {
				return ExpectedGPUCounts{Count: 1} // NVML count (1) matches threshold (1)
			},
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Health status is still determined only by NVML count vs threshold
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "nvidia gpu count matching thresholds (1)", cr.reason)
		assert.Equal(t, 0, cr.CountLspci) // lspci count is 0 due to error
		assert.Equal(t, 1, cr.CountNVML)  // NVML count matches threshold
		assert.Nil(t, cr.err)             // No error stored in checkResult
	})
}

// TestComponent_GetTimeNowFunc tests the getTimeNowFunc functionality
func TestComponent_GetTimeNowFunc(t *testing.T) {
	ctx := context.Background()

	t.Run("default time function", func(t *testing.T) {
		mockDevices := map[string]device.Device{
			"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		}
		mockInstance := &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		}

		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockInstance,
		}

		comp, err := New(gpudInstance)
		require.NoError(t, err)

		c, ok := comp.(*component)
		require.True(t, ok)

		// Test the default time function
		before := time.Now().UTC()
		actualTime := c.getTimeNowFunc()
		after := time.Now().UTC()

		assert.True(t, actualTime.After(before) || actualTime.Equal(before))
		assert.True(t, actualTime.Before(after) || actualTime.Equal(after))
	})

	t.Run("custom time function", func(t *testing.T) {
		fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

		mockDevices := map[string]device.Device{
			"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		}

		c := &component{
			ctx: ctx,
			nvmlInstance: &mockNVMLInstance{
				devices:     mockDevices,
				nvmlExists:  true,
				productName: "test-product",
			},
			getCountLspci: func(ctx context.Context) (int, error) {
				return 1, nil
			},
			getThresholdsFunc: func() ExpectedGPUCounts {
				return ExpectedGPUCounts{Count: 1}
			},
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, fixedTime, cr.ts)
	})
}

// TestComponent_Check_OnlyNVMLMatters tests that only NVML count affects health status
func TestComponent_Check_OnlyNVMLMatters(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name           string
		lspciCount     int
		lspciError     error
		nvmlDevices    int
		thresholdCount int
		expectedHealth apiv1.HealthStateType
		expectedReason string
	}{
		{
			name:           "lspci high, nvml matches threshold",
			lspciCount:     100,
			lspciError:     nil,
			nvmlDevices:    2,
			thresholdCount: 2,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "nvidia gpu count matching thresholds (2)",
		},
		{
			name:           "lspci error, nvml matches threshold",
			lspciCount:     0,
			lspciError:     errors.New("lspci failed"),
			nvmlDevices:    4,
			thresholdCount: 4,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: "nvidia gpu count matching thresholds (4)",
		},
		{
			name:           "lspci matches threshold, nvml doesn't",
			lspciCount:     8,
			lspciError:     nil,
			nvmlDevices:    2,
			thresholdCount: 8,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedReason: "nvidia gpu count mismatch (found 2, expected 8)",
		},
		{
			name:           "lspci low, nvml doesn't match threshold",
			lspciCount:     1,
			lspciError:     nil,
			nvmlDevices:    3,
			thresholdCount: 6,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedReason: "nvidia gpu count mismatch (found 3, expected 6)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the number of devices specified for NVML
			mockDevices := make(map[string]device.Device)
			for i := 0; i < tc.nvmlDevices; i++ {
				uuid := fmt.Sprintf("test-uuid-%d", i)
				pciAddress := fmt.Sprintf("0000:%02x:00.0", i%256)
				mockDevices[uuid] = testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", pciAddress)
			}

			c := &component{
				ctx: ctx,
				nvmlInstance: &mockNVMLInstance{
					devices:     mockDevices,
					nvmlExists:  true,
					productName: "test-product",
				},
				getCountLspci: func(ctx context.Context) (int, error) {
					return tc.lspciCount, tc.lspciError
				},
				getThresholdsFunc: func() ExpectedGPUCounts {
					return ExpectedGPUCounts{Count: tc.thresholdCount}
				},
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
			}

			result := c.Check()
			cr, ok := result.(*checkResult)
			require.True(t, ok)

			assert.Equal(t, tc.expectedHealth, cr.health, "Test case: %s", tc.name)
			assert.Equal(t, tc.expectedReason, cr.reason, "Test case: %s", tc.name)
			assert.Equal(t, tc.nvmlDevices, cr.CountNVML, "Test case: %s", tc.name)

			// lspci count should be recorded regardless of errors
			if tc.lspciError == nil {
				assert.Equal(t, tc.lspciCount, cr.CountLspci, "Test case: %s", tc.name)
			} else {
				assert.Equal(t, 0, cr.CountLspci, "Test case: %s", tc.name)
			}

			// No error should be stored in checkResult (lspci errors are just logged)
			assert.Nil(t, cr.err, "Test case: %s", tc.name)
		})
	}
}

// TestComponent_Check_SuggestedActions_NoReboot tests Case 1: no reboot -> GPU mismatch; suggest reboot
func TestComponent_Check_SuggestedActions_NoReboot(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{}, // No reboot events
		err:    nil,
	}

	mockBucket := &mockEventBucket{
		name:       Name,
		events:     []eventstore.Event{},
		foundEvent: nil, // Event not found, will be inserted
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2} // Expected 2, but only 1 found
		},
		rebootEventStore: mockRebootStore,
		eventBucket:      mockBucket,
		lookbackPeriod:   72 * time.Hour, // 3 days
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (found 1, expected 2)", cr.reason)
	assert.NotNil(t, cr.suggestedActions)
	assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	assert.True(t, mockBucket.insertCalled, "GPU mismatch event should be inserted")
	assert.True(t, mockBucket.findCalled, "Should check if event already exists")
}

// TestComponent_Check_SuggestedActions_OneSequence tests Case 2: GPU mismatch -> reboot -> GPU mismatch; suggest second reboot
func TestComponent_Check_SuggestedActions_OneSequence(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	// Reboot happened 24 hours ago BEFORE the first mismatch
	rebootTime := now.Add(-48 * time.Hour)
	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{
			{
				Time:    rebootTime,
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		},
		err: nil,
	}

	// GPU mismatch happened AFTER reboot (1 day ago)
	previousMismatchTime := now.Add(-24 * time.Hour)
	mockBucket := &mockEventBucket{
		name: Name,
		events: []eventstore.Event{
			{
				Component: Name,
				Time:      previousMismatchTime,
				Name:      EventNameMisMatch,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "nvidia gpu count mismatch (found 1, expected 2)",
			},
		},
		foundEvent: nil, // Current event not found, will be inserted
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2} // Expected 2, but only 1 found
		},
		rebootEventStore: mockRebootStore,
		eventBucket:      mockBucket,
		lookbackPeriod:   72 * time.Hour, // 3 days
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (found 1, expected 2)", cr.reason)
	assert.NotNil(t, cr.suggestedActions)
	assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem) // Should suggest reboot, not hardware inspection
	assert.True(t, mockBucket.insertCalled, "GPU mismatch event should be inserted")
}

// TestComponent_Check_SuggestedActions_RebootEventsError tests error handling when getting reboot events fails
func TestComponent_Check_SuggestedActions_RebootEventsError(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	testErr := errors.New("failed to get reboot events")
	mockRebootStore := &mockRebootEventStore{
		events: nil,
		err:    testErr,
	}

	mockBucket := &mockEventBucket{
		name:       Name,
		events:     []eventstore.Event{},
		foundEvent: nil,
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2}
		},
		rebootEventStore: mockRebootStore,
		eventBucket:      mockBucket,
		lookbackPeriod:   72 * time.Hour, // 3 days
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (found 1, expected 2)", cr.reason)
	assert.Equal(t, testErr, cr.err)
	assert.Nil(t, cr.suggestedActions)
}

// TestComponent_Check_SuggestedActions_GPUMismatchEventsError tests error handling when getting GPU mismatch events fails
func TestComponent_Check_SuggestedActions_GPUMismatchEventsError(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{
			{
				Time:    time.Now().Add(-12 * time.Hour), // 12 hours ago
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		},
		err: nil,
	}

	testErr := errors.New("failed to get GPU mismatch events")
	mockBucket := &mockEventBucket{
		name:       Name,
		events:     []eventstore.Event{},
		foundEvent: nil,
		getErr:     testErr,
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2}
		},
		rebootEventStore: mockRebootStore,
		eventBucket:      mockBucket,
		lookbackPeriod:   72 * time.Hour, // 3 days
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (found 1, expected 2)", cr.reason)
	assert.Equal(t, testErr, cr.err)
	assert.Nil(t, cr.suggestedActions)
}

// TestComponent_Check_SuggestedActions_NoEventBucket tests when eventBucket is nil
func TestComponent_Check_SuggestedActions_NoEventBucket(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2}
		},
		eventBucket: nil, // No event bucket
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (found 1, expected 2)", cr.reason)
	assert.Nil(t, cr.suggestedActions) // No suggested actions when event bucket is nil
}

// TestComponent_recordGPUMismatchEvent tests the recordGPUMismatchEvent method
func TestComponent_recordGPUMismatchEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("successful insert", func(t *testing.T) {
		mockBucket := &mockEventBucket{
			name:       Name,
			events:     []eventstore.Event{},
			foundEvent: nil, // Event not found
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
		}

		cr := &checkResult{
			ts:     time.Now(),
			reason: "test mismatch",
		}

		err := c.recordMismatchEvent(cr)
		assert.NoError(t, err)
		assert.True(t, mockBucket.insertCalled)
		assert.True(t, mockBucket.findCalled)
		assert.Len(t, mockBucket.events, 1)
		assert.Equal(t, EventNameMisMatch, mockBucket.events[0].Name)
	})

	t.Run("event already exists", func(t *testing.T) {
		existingEvent := &eventstore.Event{
			Component: Name,
			Time:      time.Now(),
			Name:      EventNameMisMatch,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "existing event",
		}

		mockBucket := &mockEventBucket{
			name:       Name,
			events:     []eventstore.Event{},
			foundEvent: existingEvent, // Event already exists
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
		}

		cr := &checkResult{
			ts:     time.Now(),
			reason: "test mismatch",
		}

		err := c.recordMismatchEvent(cr)
		assert.NoError(t, err)
		assert.True(t, mockBucket.findCalled)
		assert.False(t, mockBucket.insertCalled) // Should not insert if already exists
	})

	t.Run("find error", func(t *testing.T) {
		testErr := errors.New("find error")
		mockBucket := &mockEventBucket{
			name:    Name,
			findErr: testErr,
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
		}

		cr := &checkResult{
			ts:     time.Now(),
			reason: "test mismatch",
		}

		err := c.recordMismatchEvent(cr)
		assert.Error(t, err)
		assert.Equal(t, testErr, err)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error finding gpu count mismatch event", cr.reason)
	})

	t.Run("insert error", func(t *testing.T) {
		testErr := errors.New("insert error")
		mockBucket := &mockEventBucket{
			name:       Name,
			foundEvent: nil,
			insertErr:  testErr,
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
		}

		cr := &checkResult{
			ts:     time.Now(),
			reason: "test mismatch",
		}

		err := c.recordMismatchEvent(cr)
		assert.Error(t, err)
		assert.Equal(t, testErr, err)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error inserting gpu count mismatch event", cr.reason)
	})
}

// TestComponent_Check_SuggestedActions_TwoSequences tests Case 3: Multiple sequences of GPU mismatch -> reboot -> GPU mismatch; suggest hardware inspection
func TestComponent_Check_SuggestedActions_TwoSequences(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	// Create a scenario with an initial reboot, then mismatches and reboots
	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{
			{
				Time:    now.Add(-72 * time.Hour), // Initial reboot (oldest)
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    now.Add(-48 * time.Hour), // Second reboot
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    now.Add(-24 * time.Hour), // Third reboot
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		},
		err: nil,
	}

	// GPU mismatches that happen after reboots
	mockBucket := &mockEventBucket{
		name: Name,
		events: []eventstore.Event{
			{
				Component: Name,
				Time:      now.Add(-60 * time.Hour), // First mismatch (after initial reboot)
				Name:      EventNameMisMatch,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "nvidia gpu count mismatch",
			},
			{
				Component: Name,
				Time:      now.Add(-36 * time.Hour), // Second mismatch (after second reboot)
				Name:      EventNameMisMatch,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "nvidia gpu count mismatch",
			},
			{
				Component: Name,
				Time:      now.Add(-12 * time.Hour), // Third mismatch (after third reboot)
				Name:      EventNameMisMatch,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "nvidia gpu count mismatch",
			},
		},
		foundEvent: nil, // Current mismatch not found, will be inserted
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2}
		},
		rebootEventStore: mockRebootStore,
		eventBucket:      mockBucket,
		lookbackPeriod:   96 * time.Hour, // 4 days to cover all events
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.NotNil(t, cr.suggestedActions)
	// Should suggest hardware inspection since there are 2+ mismatch->reboot mappings
	assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
}

// TestComponent_Check_SuggestedActions_NoActionAfterReboot tests when the first mismatch happened after the most recent reboot
func TestComponent_Check_SuggestedActions_NoActionAfterReboot(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	// Reboot happened before any GPU mismatches
	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{
			{
				Time:    now.Add(-48 * time.Hour), // Reboot 48h ago
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		},
		err: nil,
	}

	// GPU mismatch happened after reboot
	mockBucket := &mockEventBucket{
		name: Name,
		events: []eventstore.Event{
			{
				Component: Name,
				Time:      now.Add(-24 * time.Hour), // Mismatch 24h ago (after reboot)
				Name:      EventNameMisMatch,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "nvidia gpu count mismatch",
			},
		},
		foundEvent: nil,
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2}
		},
		rebootEventStore: mockRebootStore,
		eventBucket:      mockBucket,
		lookbackPeriod:   72 * time.Hour, // 3 days
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	// When there's 1 reboot and 2 mismatches (including current), it should suggest reboot
	assert.NotNil(t, cr.suggestedActions)
	assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
}

// Note: The evaluateSuggestedActions function has been moved to pkg/eventstore package
// These tests have been migrated to pkg/eventstore/suggested_actions_test.go

// TestComponent_GetTimeNowFunc_Integration tests that getTimeNowFunc is used correctly throughout the component
func TestComponent_GetTimeNowFunc_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("Check method uses getTimeNowFunc for timestamp", func(t *testing.T) {
		fixedTime := time.Date(2024, 12, 25, 15, 30, 45, 0, time.UTC)

		mockDevices := map[string]device.Device{
			"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		}

		mockBucket := &mockEventBucket{
			name:       Name,
			foundEvent: nil, // Event not found
		}

		mockRebootStore := &mockRebootEventStore{
			events: eventstore.Events{},
			err:    nil,
		}

		c := &component{
			ctx: ctx,
			nvmlInstance: &mockNVMLInstance{
				devices:     mockDevices,
				nvmlExists:  true,
				productName: "test-product",
			},
			getCountLspci: func(ctx context.Context) (int, error) {
				return 2, nil
			},
			getThresholdsFunc: func() ExpectedGPUCounts {
				return ExpectedGPUCounts{Count: 2} // Mismatch to trigger event recording
			},
			eventBucket:      mockBucket,
			rebootEventStore: mockRebootStore,
			lookbackPeriod:   72 * time.Hour,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Verify the timestamp is the fixed time
		assert.Equal(t, fixedTime, cr.ts)

		// Check that the event was created with the correct timestamp
		assert.True(t, mockBucket.insertCalled)
		assert.Len(t, mockBucket.events, 1)
		assert.Equal(t, fixedTime, mockBucket.events[0].Time)
	})

	t.Run("SetHealthy uses getTimeNowFunc for purge timestamp", func(t *testing.T) {
		fixedTime := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)

		mockBucket := &mockEventBucket{
			name:             Name,
			purgeReturnCount: 3,
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)

		// Verify purge was called with the correct timestamp
		assert.True(t, mockBucket.purgeCalled)
		assert.Equal(t, fixedTime.Unix(), mockBucket.purgeBeforeTimestamp)
	})

	t.Run("recordMismatchEvent uses timestamp from checkResult", func(t *testing.T) {
		eventTime := time.Date(2024, 3, 10, 8, 45, 30, 0, time.UTC)

		mockBucket := &mockEventBucket{
			name:       Name,
			foundEvent: nil, // Event not found
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
		}

		cr := &checkResult{
			ts:     eventTime,
			reason: "test mismatch reason",
		}

		err := c.recordMismatchEvent(cr)
		assert.NoError(t, err)

		// Verify the event was recorded with the checkResult's timestamp
		assert.True(t, mockBucket.insertCalled)
		assert.Len(t, mockBucket.events, 1)
		assert.Equal(t, eventTime, mockBucket.events[0].Time)
		assert.Equal(t, "test mismatch reason", mockBucket.events[0].Message)
	})

	t.Run("concurrent Check calls use independent timestamps", func(t *testing.T) {
		var timestamps []time.Time
		var mu sync.Mutex

		mockDevices := map[string]device.Device{
			"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		}

		timeCounter := 0
		c := &component{
			ctx: ctx,
			nvmlInstance: &mockNVMLInstance{
				devices:     mockDevices,
				nvmlExists:  true,
				productName: "test-product",
			},
			getCountLspci: func(ctx context.Context) (int, error) {
				return 1, nil
			},
			getThresholdsFunc: func() ExpectedGPUCounts {
				return ExpectedGPUCounts{Count: 1}
			},
			getTimeNowFunc: func() time.Time {
				mu.Lock()
				defer mu.Unlock()
				timeCounter++
				// Return different times for each call
				return time.Date(2024, 1, 1, 0, 0, timeCounter, 0, time.UTC)
			},
		}

		// Run multiple checks concurrently
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result := c.Check()
				cr, ok := result.(*checkResult)
				if ok {
					mu.Lock()
					timestamps = append(timestamps, cr.ts)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		// Verify all timestamps are different
		assert.Len(t, timestamps, 5)
		uniqueTimestamps := make(map[time.Time]bool)
		for _, ts := range timestamps {
			uniqueTimestamps[ts] = true
		}
		assert.Len(t, uniqueTimestamps, 5, "All timestamps should be unique")
	})
}
