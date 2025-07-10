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
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
	infinibandstore "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/store"
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
	c.getThresholdsFunc = func() infiniband.ExpectedPortStates {
		return infiniband.ExpectedPortStates{
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

	// Insert test event using eventstore.Event
	testEvent := eventstore.Event{
		Time:    now.Add(-5 * time.Second),
		Name:    "test_event",
		Type:    string(apiv1.EventTypeWarning),
		Message: "test message",
	}
	err := mockBucket.Insert(ctx, testEvent)
	require.NoError(t, err)

	// Test Events method
	events, err := c.Events(ctx, now.Add(-10*time.Second))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, testEvent.Name, events[0].Name)
	assert.Equal(t, testEvent.Message, events[0].Message)   // Check message too
	assert.Equal(t, testEvent.Type, string(events[0].Type)) // Check type too (cast to string)

	// Test with more recent time filter
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

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func mockGetThresholds() infiniband.ExpectedPortStates {
	return infiniband.ExpectedPortStates{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

func TestSetHealthy(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:    cctx,
		cancel: ccancel,
	}

	err := c.SetHealthy()
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

func TestComponentCheckWithIBPortsStore(t *testing.T) {
	t.Parallel()

	// Create a component with mocked functions and ibPortsStore
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap since " + time.Now().UTC().Format(time.RFC3339),
			},
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return mock devices with IB ports so the component proceeds to event handling
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

	// Should be unhealthy due to events
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// Should contain both event reasons
	assert.Contains(t, data.reason, "mlx5_0 port 1 flap since")
	assert.Contains(t, data.reason, "mlx5_1 port 1 drop since")

	// Should have hardware inspection suggested action
	assert.NotNil(t, data.suggestedActions)
	assert.Contains(t, data.suggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
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
				Port: infiniband.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   "unknown_event_type",
				EventReason: "unknown event reason",
			},
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return mock devices with IB ports so the component proceeds to event handling
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

	// Should be unhealthy due to the known event (drop)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// Should contain only the known event reason, not the unknown one
	assert.Contains(t, data.reason, "mlx5_1 port 1 drop since")
	assert.NotContains(t, data.reason, "unknown event reason")
}

func TestComponentCheckWithEmptyEventReason(t *testing.T) {
	t.Parallel()

	// Create a component with mocked functions and ibPortsStore
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with empty event reason
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "", // Empty reason
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return mock devices with IB ports so the component proceeds to event handling
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

	// Should be unhealthy due to the event
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// Should handle empty reason gracefully
	assert.Contains(t, data.reason, ";")
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

// Local mock store for component_test.go
type testIBPortsStore struct {
	events []infinibandstore.Event
}

func (m *testIBPortsStore) Insert(eventTime time.Time, ibPorts []infiniband.IBPort) error {
	return nil
}

func (m *testIBPortsStore) SetEventType(device string, port uint, timestamp time.Time, eventType string, eventReason string) error {
	return nil
}

func (m *testIBPortsStore) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	return m.events, nil
}

func (m *testIBPortsStore) Tombstone(timestamp time.Time) error {
	return nil
}

func (m *testIBPortsStore) Scan() error {
	return nil
}
