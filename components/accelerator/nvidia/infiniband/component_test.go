package infiniband

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	infinibandstore "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/store"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

func TestComponentCheck(t *testing.T) {
	t.Parallel()

	// Create a component with mocked functions
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Apply fixes to ensure context is properly set
	mockBucket := createMockEventBucket()
	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: mockGetThresholds,
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
	}

	// Case 1: No NVML
	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)

	// Case 2: With NVML but missing product name
	nvmlMock := &mockNVMLInstance{exists: true, productName: ""}
	c.nvmlInstance = nvmlMock
	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML is loaded but GPU is not detected (missing product name)", data.reason)

	// Case 3: With NVML and valid product name but zero threshold
	nvmlMock.productName = "Tesla V100"
	c.getThresholdsFunc = func() types.ExpectedPortStates {
		return types.ExpectedPortStates{
			AtLeastPorts: 0,
			AtLeastRate:  0,
		}
	}
	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoThreshold, data.reason)
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	// Setup test event bucket
	mockBucket := createMockEventBucket()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := &component{
		ctx:         ctx,
		cancel:      cancel,
		eventBucket: mockBucket,
	}

	now := time.Now().UTC()

	// Insert various test events
	// 1. PCI power insufficient event (should be included)
	pciPowerEvent := eventstore.Event{
		Time:    now.Add(-8 * time.Second),
		Name:    eventPCIPowerInsufficient,
		Type:    string(apiv1.EventTypeWarning),
		Message: "Insufficient power on MLX5 PCIe slot",
	}
	err := mockBucket.Insert(ctx, pciPowerEvent)
	require.NoError(t, err)

	// 2. Port module high temperature event (should be included)
	tempEvent := eventstore.Event{
		Time:    now.Add(-6 * time.Second),
		Name:    eventPortModuleHighTemperature,
		Type:    string(apiv1.EventTypeWarning),
		Message: "Overheated MLX5 adapter",
	}
	err = mockBucket.Insert(ctx, tempEvent)
	require.NoError(t, err)

	// 3. Other event (should be filtered out)
	otherEvent := eventstore.Event{
		Time:      now.Add(-5 * time.Second),
		Name:      "test_event",
		Type:      string(apiv1.EventTypeWarning),
		Message:   "test message",
		Component: Name, // Even with component set, should be filtered
	}
	err = mockBucket.Insert(ctx, otherEvent)
	require.NoError(t, err)

	// 4. IB port event (should be filtered out)
	ibPortEvent := eventstore.Event{
		Time:      now.Add(-4 * time.Second),
		Name:      infinibandstore.EventTypeIbPortFlap,
		Type:      string(apiv1.EventTypeWarning),
		Message:   "IB port flap",
		Component: Name,
	}
	err = mockBucket.Insert(ctx, ibPortEvent)
	require.NoError(t, err)

	// Test Events method - should only return the two kmsg events
	events, err := c.Events(ctx, now.Add(-10*time.Second))
	require.NoError(t, err)
	require.Len(t, events, 2, "Should only return PCI power and temperature events")

	// Verify the events are the correct ones
	eventNames := make(map[string]bool)
	for _, ev := range events {
		eventNames[ev.Name] = true
	}
	assert.True(t, eventNames[eventPCIPowerInsufficient], "Should include PCI power event")
	assert.True(t, eventNames[eventPortModuleHighTemperature], "Should include temperature event")

	// Test with more recent time filter
	events, err = c.Events(ctx, now.Add(-7*time.Second))
	require.NoError(t, err)
	assert.Len(t, events, 1, "Should only return temperature event")
	assert.Equal(t, eventPortModuleHighTemperature, events[0].Name)

	// Test with time filter that excludes all events
	events, err = c.Events(ctx, now)
	require.NoError(t, err)
	assert.Empty(t, events)

	// Test with canceled context
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Cancel immediately

	events, err = c.Events(canceledCtx, now)
	assert.Error(t, err)
	assert.Nil(t, events)
}

func TestComponentEventsFiltersKmsgEvents(t *testing.T) {
	t.Parallel()

	// Setup test event bucket
	mockBucket := createMockEventBucket()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := &component{
		ctx:         ctx,
		cancel:      cancel,
		eventBucket: mockBucket,
	}

	now := time.Now().UTC()

	// Insert kmsg events without Component field (as kmsg syncer would)
	kmsgPciPowerEvent := eventstore.Event{
		Time:    now.Add(-5 * time.Second),
		Name:    eventPCIPowerInsufficient,
		Type:    string(apiv1.EventTypeWarning),
		Message: "Detected insufficient power on the PCIe slot (27W)",
		// Component field is intentionally not set, as kmsg syncer doesn't set it
	}
	err := mockBucket.Insert(ctx, kmsgPciPowerEvent)
	require.NoError(t, err)

	kmsgTempEvent := eventstore.Event{
		Time:    now.Add(-3 * time.Second),
		Name:    eventPortModuleHighTemperature,
		Type:    string(apiv1.EventTypeWarning),
		Message: "Port module event[error]: module 0, Cable error, High Temperature",
		// Component field is intentionally not set
	}
	err = mockBucket.Insert(ctx, kmsgTempEvent)
	require.NoError(t, err)

	// Insert other kmsg event that should be filtered out
	otherKmsgEvent := eventstore.Event{
		Time:    now.Add(-2 * time.Second),
		Name:    "some_other_kmsg_event",
		Type:    string(apiv1.EventTypeWarning),
		Message: "Some other kernel message",
		// Component field not set
	}
	err = mockBucket.Insert(ctx, otherKmsgEvent)
	require.NoError(t, err)

	// Test Events method - should return only the specific kmsg events
	events, err := c.Events(ctx, now.Add(-10*time.Second))
	require.NoError(t, err)
	require.Len(t, events, 2, "Should return exactly 2 kmsg events")

	// Verify both events are present and in the correct order
	foundPciPower := false
	foundTemp := false
	for _, ev := range events {
		if ev.Name == eventPCIPowerInsufficient {
			foundPciPower = true
			assert.Contains(t, ev.Message, "insufficient power")
		}
		if ev.Name == eventPortModuleHighTemperature {
			foundTemp = true
			assert.Contains(t, ev.Message, "High Temperature")
		}
	}
	assert.True(t, foundPciPower, "Should find PCI power event")
	assert.True(t, foundTemp, "Should find temperature event")
}

func TestComponentClose(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	mockBucket := createMockEventBucket()

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
	}

	// Add a real kmsgSyncer only if on linux to test the path where it's not nil
	// Note: This part might still not run in all test environments
	if runtime.GOOS == "linux" {
		// Attempt to create, ignore error for this test focused on Close()
		kmsgSyncer, _ := kmsg.NewSyncer(cctx, func(line string) (string, string) {
			return "test", "test"
		}, mockBucket)
		c.kmsgSyncer = kmsgSyncer // Assign if created
	}

	err := c.Close()
	assert.NoError(t, err)

	// Verify context is canceled
	select {
	case <-cctx.Done():
		// Success - context should be canceled
	default:
		t.Fatal("Context not canceled after Close()")
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

func TestTags(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	tags := comp.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 4, "Component should return exactly 4 tags")
}

// mockEventStore for testing New errors
type mockEventStore struct {
	bucketErr error
}

func (m *mockEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	if m.bucketErr != nil {
		return nil, m.bucketErr
	}
	// Return a simple mock bucket if no error
	return createMockEventBucket(), nil
}

// mockEventBucket implements the events_db.Store interface for testing
type mockEventBucket struct {
	events    eventstore.Events
	mu        sync.Mutex
	findErr   error // Added for testing Find errors
	insertErr error // Added for testing Insert errors
}

func createMockEventBucket() *mockEventBucket {
	return &mockEventBucket{
		events: eventstore.Events{},
	}
}

func (m *mockEventBucket) Name() string {
	return "mock"
}

func (m *mockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result eventstore.Events
	for _, event := range m.events {
		if !event.Time.Before(since) {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *mockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, e := range m.events {
		if e.Name == event.Name && e.Type == event.Type && e.Message == event.Message {
			return &m.events[i], nil
		}
	}
	return nil, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.events) == 0 {
		return nil, nil
	}

	latest := m.events[0]
	for _, e := range m.events[1:] {
		if e.Time.After(latest.Time) {
			latest = e
		}
	}
	return &latest, nil
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var newEvents eventstore.Events
	var purgedCount int

	for _, event := range m.events {
		if event.Time.Unix() >= beforeTimestamp {
			newEvents = append(newEvents, event)
		} else {
			purgedCount++
		}
	}

	m.events = newEvents
	return purgedCount, nil
}

func (m *mockEventBucket) Close() {
	// No-op for mock
}

// GetEvents returns a copy of the stored events as apiv1.Events for assertion convenience.
func (m *mockEventBucket) GetAPIEvents() apiv1.Events {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(apiv1.Events, len(m.events))
	for i, ev := range m.events {
		result[i] = ev.ToEvent()
	}
	return result
}

// Test helpers for mocking NVML
type mockNVMLInstance struct {
	exists      bool
	productName string
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.exists
}

// Simple mock implementation of required InstanceV2 interface methods
func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return nil
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) ProductName() string {
	if m.productName == "" {
		return "" // Empty string for testing
	}
	return m.productName // Return custom value for testing
}

func (m *mockNVMLInstance) Architecture() string {
	return ""
}

func (m *mockNVMLInstance) Brand() string {
	return ""
}

func (m *mockNVMLInstance) DriverVersion() string {
	return ""
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 0
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return ""
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return true
}

func (m *mockNVMLInstance) FabricStateSupported() bool {
	return false
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func mockGetThresholds() types.ExpectedPortStates {
	return types.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	}
}

func TestLastHealthStates(t *testing.T) {
	t.Parallel()

	// Test with nil data
	c := &component{}
	states := c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with data
	mockData := &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "test reason",
		err:    fmt.Errorf("test error"),
	}
	c.lastMu.Lock()
	c.lastCheckResult = mockData
	c.lastMu.Unlock()

	states = c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "test reason", states[0].Reason)
	assert.Equal(t, "test error", states[0].Error)
}

func TestDataString(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.String())

	// Test with no data
	cr = &checkResult{}
	assert.Equal(t, "", cr.String())

	// Test with class devices
	cr = &checkResult{
		ClassDevices: infinibandclass.Devices{
			{
				Name:            "mlx5_0",
				BoardID:         "MT_0000000838",
				FirmwareVersion: "28.41.1000",
				HCAType:         "MT4129",
			},
		},
	}
	result := cr.String()
	assert.Contains(t, result, "mlx5_0")
	assert.Contains(t, result, "MT_0000000838")
}

func TestDataSummary(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.Summary())

	// Test with reason
	cr = &checkResult{reason: "test reason"}
	assert.Equal(t, "test reason", cr.Summary())
}

func TestDataHealthState(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())

	// Test with health state
	cr = &checkResult{health: apiv1.HealthStateTypeUnhealthy}
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
}

func TestDataGetError(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.getError())

	// Test with nil error
	cr = &checkResult{}
	assert.Equal(t, "", cr.getError())

	// Test with error
	cr = &checkResult{err: errors.New("test error")}
	assert.Equal(t, "test error", cr.getError())
}

func TestComponentCheckErrorCases(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Test case: with NVML but no threshold
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			}
		},
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoThreshold, data.reason)
}

func TestEventsWithContextCanceled(t *testing.T) {
	t.Parallel()

	mockBucket := createMockEventBucket()

	// Create component with the mock bucket
	c := &component{
		eventBucket: mockBucket,
	}

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Test Events with canceled context
	since := time.Now().Add(-time.Hour)
	events, err := c.Events(ctx, since)
	assert.Error(t, err)
	assert.Nil(t, events)
	assert.Equal(t, context.Canceled, err)
}

func TestEventsWithNoEventBucket(t *testing.T) {
	t.Parallel()

	// Create component with nil eventBucket
	c := &component{
		eventBucket: nil,
	}

	// Test Events with nil eventBucket
	since := time.Now().Add(-time.Hour)
	events, err := c.Events(context.Background(), since)
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestCloseWithNilComponents(t *testing.T) {
	t.Parallel()

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {}, // no-op cancel
		eventBucket: nil,
		kmsgSyncer:  nil,
	}

	err := c.Close()
	assert.NoError(t, err)
}

func TestIsSupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nvml     nvidianvml.Instance
		expected bool
	}{
		{
			name:     "nil nvml instance",
			nvml:     nil,
			expected: false,
		},
		{
			name:     "nvml exists false",
			nvml:     &mockNVMLInstance{exists: false, productName: ""},
			expected: false,
		},
		{
			name:     "nvml exists but no product name",
			nvml:     &mockNVMLInstance{exists: true, productName: ""},
			expected: false,
		},
		{
			name:     "nvml exists with product name",
			nvml:     &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &component{
				nvmlInstance: tt.nvml,
			}
			assert.Equal(t, tt.expected, c.IsSupported())
		})
	}
}

func TestComponentName(t *testing.T) {
	t.Parallel()

	cr := &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())
}

func TestGetSuggestedActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cr       *checkResult
		expected *apiv1.SuggestedActions
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: nil,
		},
		{
			name:     "no suggested actions",
			cr:       &checkResult{},
			expected: nil,
		},
		{
			name: "with suggested actions",
			cr: &checkResult{
				suggestedActions: &apiv1.SuggestedActions{
					RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
				},
			},
			expected: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cr.getSuggestedActions())
		})
	}
}

func TestNewWithEventStoreError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockStore := &mockEventStore{
		bucketErr: errors.New("bucket creation error"),
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
		EventStore:           mockStore,
	}

	comp, err := New(gpudInstance)
	assert.Error(t, err)
	assert.Nil(t, comp)
	assert.Contains(t, err.Error(), "bucket creation error")
}

func TestNewWithNilEventStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
		EventStore:           nil, // Explicitly nil
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Verify the component was created properly without event store
	casted := comp.(*component)
	assert.Nil(t, casted.eventBucket)
	assert.Nil(t, casted.kmsgSyncer)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

func TestNewWithCustomToolOverwrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	customToolOverwrites := pkgconfigcommon.ToolOverwrites{
		InfinibandClassRootDir: "/custom/infiniband",
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: customToolOverwrites,
		EventStore:           nil,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Verify the custom tool overwrites are set
	casted := comp.(*component)
	assert.Equal(t, "/custom/infiniband", casted.toolOverwrites.InfinibandClassRootDir)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

func TestCheckWithClassDevicesSuccess(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Mock successful class devices
	mockDevices := infinibandclass.Devices{
		{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
		},
	}

	// Create a component with mocked functions where getClassDevicesFunc succeeds
	mockBucket := createMockEventBucket()
	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return mockDevices, nil
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify the success case continues processing
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortData, data.reason)
	assert.Equal(t, mockDevices, data.ClassDevices, "ClassDevices should be populated when successful")
	assert.Len(t, data.ClassDevices, 1, "Should have one device")
	assert.Equal(t, "mlx5_0", data.ClassDevices[0].Name)
}

func TestComponentCheckWithUnknownEventType(t *testing.T) {
	t.Parallel()

	// Create a component with mocked functions and ibPortsStore
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with unknown event type
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   "unknown_event_type",
				EventReason: "unknown event reason",
			},
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_1",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_1 port 1 drop since " + time.Now().UTC().Format(time.RFC3339),
			},
		},
	}

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		eventBucket:  mockBucket,
		ibPortsStore: mockStore,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return mock devices with IB ports - some unhealthy to trigger event processing
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down",
							PhysState: "Disabled",
							RateGBSec: 0,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				// Add more disabled ports to ensure unhealthyIBPorts is populated
				{
					Name: "mlx5_2",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down",
							PhysState: "Polling",
							RateGBSec: 0,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to the known event (drop)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// Should contain only the known event reason, not the unknown one
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_1")
	assert.NotContains(t, data.reason, "unknown event reason")
}

func TestComponentCheckWithNoEvents(t *testing.T) {
	t.Parallel()

	// Create a component with mocked functions and ibPortsStore with no events
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with no events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{}, // No events
	}

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		eventBucket:  mockBucket,
		ibPortsStore: mockStore,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be healthy since no events and no IB port data
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortData, data.reason)
}

func TestComponentCheckWithUnhealthyIBPortsAndEvents(t *testing.T) {
	t.Parallel()

	// Test case: unhealthyIBPorts exist AND there are events
	// This tests the logic: if len(cr.unhealthyIBPorts) > 0 { cr.reason = "" }
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event",
			},
		},
	}

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		eventBucket:  mockBucket,
		ibPortsStore: mockStore,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return devices that will be detected as unhealthy by threshold evaluation
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down", // This should make it unhealthy
							PhysState: "Disabled",
							RateGBSec: 100, // Below threshold
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to both threshold and events
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// The reason should contain both threshold failure and event reason
	// because both thresholds fail and there are events
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
	assert.Contains(t, data.reason, "port(s) are active")
	assert.Contains(t, data.reason, "physical state Disabled")
	assert.NotNil(t, data.suggestedActions)
	assert.Contains(t, data.suggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
}

func TestComponentCheckWithExistingReasonAndEvents(t *testing.T) {
	t.Parallel()

	// Test case: cr.reason already exists AND there are events
	// This tests the logic: if cr.reason != "" { cr.reason += "; " }
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_0 port 1 drop event",
			},
		},
	}

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		eventBucket:  mockBucket,
		ibPortsStore: mockStore,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return devices that will be detected as unhealthy by threshold evaluation
			// This will trigger unhealthyIBPorts to be populated, which allows event processing
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down",     // Unhealthy state to trigger threshold breach
							PhysState: "Disabled", // Unhealthy physical state
							RateGBSec: 100,        // Below threshold
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
						{
							Port:      2,
							Name:      "2",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400, // Only 2 ports total, need 8
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to both threshold breach and events
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// The reason should contain both threshold reason and event reason separated by "; "
	assert.Contains(t, data.reason, "; ")
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_0")
	// Should also contain threshold breach reasons
	assert.Contains(t, data.reason, "port(s) are active")
}

func TestComponentCheckWithEmptyReasonAndEvents(t *testing.T) {
	t.Parallel()

	// Test case: unhealthyIBPorts exist AND there are events
	// This tests the event processing logic when there's a threshold breach
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with multiple events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event",
			},
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_1",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_1 port 1 drop event",
			},
		},
	}

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		eventBucket:  mockBucket,
		ibPortsStore: mockStore,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 4, // Need 4 ports, will only have 2
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return devices that fail port count threshold
			// Some ports have disabled/polling physical state to populate unhealthyIBPorts
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400, // Meets rate threshold
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400, // Meets rate threshold
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_2",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down",
							PhysState: "Disabled", // This will be included in unhealthyIBPorts
							RateGBSec: 0,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_3",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Polling",
							PhysState: "Polling", // This will be included in unhealthyIBPorts
							RateGBSec: 0,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to both threshold breach and events
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// The reason should contain both threshold failure and event reasons
	assert.Contains(t, data.reason, "only 2 port(s) are active and >=400 Gb/s, expect >=4 port(s)")
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_1")
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
	assert.Contains(t, data.reason, "; ")
}

func TestComponentCheckWithTrueEmptyReasonAndEvents(t *testing.T) {
	t.Parallel()

	// Test case: cr.reason is truly empty because len(cr.unhealthyIBPorts) > 0 clears it
	// This tests the logic where cr.reason = "" happens, then cr.reason += strings.Join(reasons, ", ")
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with multiple events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event",
			},
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_1",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_1 port 1 drop event",
			},
		},
	}

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		eventBucket:  mockBucket,
		ibPortsStore: mockStore,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8, // Set high to trigger threshold failure
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return devices that will trigger unhealthyIBPorts (down/disabled state)
			// This should cause len(cr.unhealthyIBPorts) > 0, which clears cr.reason
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down",     // This makes it unhealthy
							PhysState: "Disabled", // This makes it unhealthy
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down",     // This makes it unhealthy
							PhysState: "Disabled", // This makes it unhealthy
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to both thresholds and events
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// The reason should contain threshold reason + event reasons
	// because len(cr.unhealthyIBPorts) > 0 but cr.reason is not cleared for threshold failures
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_1")
	assert.Contains(t, data.reason, "; ")                 // Should contain semicolon separator
	assert.Contains(t, data.reason, "port(s) are active") // Should contain threshold reason
}

// Local mock store for component_test.go
type testIBPortsStore struct {
	events          []infinibandstore.Event
	lastEventsError error
	insertError     error
	scanError       error
}

func (m *testIBPortsStore) Insert(eventTime time.Time, ibPorts []types.IBPort) error {
	return m.insertError
}

func (m *testIBPortsStore) SetEventType(device string, port uint, timestamp time.Time, eventType string, eventReason string) error {
	return nil
}

func (m *testIBPortsStore) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	return m.events, m.lastEventsError
}

func (m *testIBPortsStore) Tombstone(timestamp time.Time) error {
	return nil
}

func (m *testIBPortsStore) Scan() error {
	return m.scanError
}

// Tests for LastEvents error handling
func TestComponentCheckWithLastEventsError(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store that returns error on LastEvents
	mockStore := &testIBPortsStore{
		events:          []infinibandstore.Event{},
		lastEventsError: errors.New("database connection failed"),
	}

	c := &component{
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    mockBucket,
		ibPortsStore:   mockStore,
		requestTimeout: 5 * time.Second,
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return devices so we reach the LastEvents section
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy when LastEvents fails
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Equal(t, "error getting ib flap/drop events", data.reason)
	assert.NotNil(t, data.err)
	assert.Equal(t, "database connection failed", data.err.Error())
}

func TestComponentCheckWithNilIBPortsStore(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    mockBucket,
		ibPortsStore:   nil, // Nil store should skip LastEvents processing
		requestTimeout: 5 * time.Second,
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return devices so we reach the LastEvents section
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should complete successfully, skip LastEvents processing
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
}

func TestComponentCheckWithMultipleEventTypes(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with multiple event types
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event",
			},
			{
				Time: time.Now().UTC().Add(time.Second),
				Port: types.IBPort{
					Device: "mlx5_1",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_1 port 1 drop event",
			},
		},
	}

	c := &component{
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    mockBucket,
		ibPortsStore:   mockStore,
		requestTimeout: 5 * time.Second,
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 4, // Expect 4 ports but only provide 2 with 1 unhealthy
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down",
							PhysState: "Disabled",
							RateGBSec: 0,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to events
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_1")
	assert.Contains(t, data.reason, "; ")

	// Verify no IB port drop/flap events were inserted into gpud event store
	// These events are handled through health state evaluation instead
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 0)
}

func TestComponentCheckWithEmptyEvents(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with empty events list
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{}, // Empty events
	}

	c := &component{
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    mockBucket,
		ibPortsStore:   mockStore,
		requestTimeout: 5 * time.Second,
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be healthy since no events and thresholds are met
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)

	// Verify no events were inserted
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 0)
}

// Tests that IB port events are still processed through health state evaluation
// even when event bucket operations fail
func TestComponentCheckWithEventFindError(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create a mock bucket that returns error on Find
	mockBucket := createMockEventBucket()
	mockBucket.findErr = errors.New("find operation failed")

	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event",
			},
		},
	}

	c := &component{
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    mockBucket,
		ibPortsStore:   mockStore,
		requestTimeout: 5 * time.Second,
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 2, // Expect 2 ports but only provide 1
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down",
							PhysState: "Disabled",
							RateGBSec: 0,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to the event, despite Find error
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")

	// No IB port drop/flap events should be inserted into gpud event store
	// These events are handled through health state evaluation instead
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 0)
}

// Tests for different event types (Drop/Flap)
func TestComponentCheckHealthyPortsWithHistoricalEvents(t *testing.T) {
	t.Parallel()

	// Test case: All ports are currently healthy (meeting thresholds)
	// BUT there are historical flap/drop events that haven't been cleared
	// Expected: Component should be unhealthy due to historical events
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with historical events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC().Add(-time.Hour), // Event from 1 hour ago
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event from 1 hour ago",
			},
			{
				Time: time.Now().UTC().Add(-30 * time.Minute), // Event from 30 min ago
				Port: types.IBPort{
					Device: "mlx5_1",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_1 port 1 drop event from 30 minutes ago",
			},
		},
	}

	c := &component{
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    mockBucket,
		ibPortsStore:   mockStore,
		requestTimeout: 5 * time.Second,
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 2,   // We have 2 healthy ports
				AtLeastRate:  400, // Both ports meet this rate
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return all healthy ports that meet thresholds
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to historical flap event, even though current ports are healthy
	// Drop events should NOT be processed since there are no unhealthy IB ports
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
	// Drop event should NOT be processed since len(cr.unhealthyIBPorts) == 0
	assert.NotContains(t, data.reason, "device(s) down too long: mlx5_1")

	// Suggested actions should include hardware inspection
	assert.NotNil(t, data.suggestedActions)
	assert.Contains(t, data.suggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
}

func TestComponentCheckWithUnknownEventTypeDefaultCase(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with unknown event type
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   "unknown_event_type",
				EventReason: "unknown event reason",
			},
		},
	}

	c := &component{
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    mockBucket,
		ibPortsStore:   mockStore,
		requestTimeout: 5 * time.Second,
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be healthy since unknown event type is not processed
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)

	// Unknown event type should not be inserted
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 0)
}

func TestComponentCheckDropEventsIgnoredWhenHealthy(t *testing.T) {
	t.Parallel()

	// Test case: All ports are healthy but there are drop events
	// Expected: Drop events should be ignored, component should be healthy
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with drop events only
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC().Add(-time.Hour),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_0 port 1 drop event",
			},
			{
				Time: time.Now().UTC().Add(-30 * time.Minute),
				Port: types.IBPort{
					Device: "mlx5_1",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_1 port 1 drop event",
			},
		},
	}

	c := &component{
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    mockBucket,
		ibPortsStore:   mockStore,
		requestTimeout: 5 * time.Second,
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return all healthy ports
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be healthy since drop events are ignored when no unhealthy ports
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
	assert.NotContains(t, data.reason, "device(s) down too long")

	// No events should be inserted since drop events are ignored
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 0)
}

func TestComponentCheckThresholdMismatchNoHardwareInspection(t *testing.T) {
	t.Parallel()

	// Test case: Ports don't meet threshold requirements (count or rate)
	// Expected: Component should be unhealthy but NOT suggest hardware inspection
	// since port mismatches often self-recover
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		eventBucket:  mockBucket,
		ibPortsStore: nil, // No events store, pure threshold evaluation
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 8, // Require 8 ports
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return only 4 ports, not meeting the 8 port requirement
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_2",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_3",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to insufficient ports
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "only 4 port(s) are active")
	assert.Contains(t, data.reason, "expect >=8 port(s)")

	// IMPORTANT: Should NOT suggest hardware inspection for threshold mismatches
	// as these often self-recover
	assert.Nil(t, data.suggestedActions, "Should NOT suggest hardware inspection for port/rate threshold mismatches")
}

func TestComponentCheckRateMismatchNoHardwareInspection(t *testing.T) {
	t.Parallel()

	// Test case: Ports don't meet rate requirements
	// Expected: Component should be unhealthy but NOT suggest hardware inspection
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		eventBucket:  mockBucket,
		ibPortsStore: nil, // No events store, pure threshold evaluation
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  400, // Require 400 Gb/s
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return ports with insufficient rate
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 200, // Only 200 Gb/s, not meeting 400 requirement
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 200, // Only 200 Gb/s, not meeting 400 requirement
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to insufficient rate
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "only 0 port(s) are active and >=400 Gb/s")

	// IMPORTANT: Should NOT suggest hardware inspection for rate mismatches
	assert.Nil(t, data.suggestedActions, "Should NOT suggest hardware inspection for rate threshold mismatches")
}

func TestComponentCheckDropEventsProcessedWhenUnhealthy(t *testing.T) {
	t.Parallel()

	// Test case: Some ports are unhealthy AND there are drop events
	// Expected: Drop events should be processed, component should be unhealthy
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with drop events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC().Add(-time.Hour),
				Port: types.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_0 port 1 drop event",
			},
		},
	}

	c := &component{
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    mockBucket,
		ibPortsStore:   mockStore,
		requestTimeout: 5 * time.Second,
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return one healthy and one unhealthy port
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Down",     // Unhealthy
							PhysState: "Disabled", // Unhealthy
							RateGBSec: 0,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
				{
					Name: "mlx5_1",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400,
							LinkLayer: "Infiniband",
							Counters: infinibandclass.Counters{
								LinkDowned: new(uint64),
							},
						},
					},
				},
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to threshold breach and drop events
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "only 1 port(s) are active")
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_0")

	// No IB port drop/flap events should be inserted into gpud event store
	// These events are handled through health state evaluation instead
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 0)
}
