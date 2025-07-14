package infiniband

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
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
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_1")

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
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_1")
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
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 8,
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return devices that will be detected as unhealthy by threshold evaluation
			// but won't trigger the cr.reason = "" logic (no unhealthyIBPorts)
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active", // Healthy state
							PhysState: "LinkUp", // Healthy physical state
							RateGBSec: 100,      // Below threshold but state is healthy
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

	// The reason should contain both threshold reason and event reason separated by "; "
	assert.Contains(t, data.reason, "; ")
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_0")
}

func TestComponentCheckWithEmptyReasonAndEvents(t *testing.T) {
	t.Parallel()

	// Test case: cr.reason is empty AND there are events
	// This tests the logic: cr.reason += strings.Join(reasons, ", ")
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with multiple events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event",
			},
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 2, // Set to match number of ports to avoid threshold failure
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return devices that meet thresholds exactly (no unhealthyIBPorts, reason will be "ok; no infiniband port issue")
			return infinibandclass.Devices{
				{
					Name: "mlx5_0",
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active",
							PhysState: "LinkUp",
							RateGBSec: 400, // Meets threshold
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
							RateGBSec: 400, // Meets threshold
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

	// After the fix, when thresholds are met but there are events,
	// the reason should contain ONLY the event reasons, not the threshold reason
	// This avoids confusing messages like "ok; no infiniband port issue; event1, event2"
	assert.Equal(t, "device(s) down too long: mlx5_1; device(s) flapping between ACTIVE<>DOWN: mlx5_0", data.reason)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_1")
	assert.Contains(t, data.reason, "; ")
	assert.NotContains(t, data.reason, "ok; no infiniband port issue") // Should NOT contain threshold reason
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
				Port: infiniband.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event",
			},
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

func TestComponentCheckShouldNotHaveConfusingReasonMessage(t *testing.T) {
	t.Parallel()

	// Test case: Ensure we don't get confusing messages like:
	// "ok; no infiniband port issue; mlx5_1 port 1 down since..."
	// This happens when threshold evaluation passes but there are historical events
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with port down events
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
					Device: "mlx5_1",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_1 port 1 down since 2025-07-10T15:16:35Z",
			},
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
					Device: "mlx5_2",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_2 port 1 down since 2025-07-10T15:16:35Z",
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
			// Set thresholds that will be met by the active ports
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 2, // We'll have 2 active ports
				AtLeastRate:  400,
			}
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			// Return devices where some ports are active (meet threshold) but others have events
			return infinibandclass.Devices{
				{
					Name: "mlx5_0", // Active port, no events
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
					Name: "mlx5_1", // Port with events but currently showing as active
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active", // Currently active but has historical events
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
					Name: "mlx5_2", // Port with events but currently showing as active
					Ports: []infinibandclass.Port{
						{
							Port:      1,
							Name:      "1",
							State:     "Active", // Currently active but has historical events
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

	// The problematic message we want to avoid:
	// "ok; no infiniband port issue; mlx5_1 port 1 down since..., mlx5_2 port 1 down since..."
	problematicPattern := "ok; no infiniband port issue;"

	// This test documents the current behavior - if this fails, it means the logic has changed
	// and we need to update our understanding
	if strings.Contains(data.reason, problematicPattern) {
		t.Logf("DETECTED CONFUSING MESSAGE: %s", data.reason)

		// The message is confusing because it says "no infiniband port issue"
		// but then lists port issues. This should not happen.
		assert.Fail(t, "Confusing reason message detected",
			"Reason contains 'ok; no infiniband port issue' followed by port issues: %s", data.reason)
	}

	// Verify that we get the expected behavior instead
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_1, mlx5_2")
}

// Local mock store for component_test.go
type testIBPortsStore struct {
	events          []infinibandstore.Event
	lastEventsError error
	insertError     error
	scanError       error
}

func (m *testIBPortsStore) Insert(eventTime time.Time, ibPorts []infiniband.IBPort) error {
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Should still complete successfully despite LastEvents error
	// The error is logged but doesn't fail the check
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

// Tests for event processing and conversion to gpud events
func TestComponentCheckWithEventProcessingAndConversion(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with single event for conversion testing
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event for conversion test",
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Should be unhealthy due to flap event
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
	assert.NotNil(t, data.suggestedActions)
	assert.Contains(t, data.suggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)

	// Verify event was inserted into bucket
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, Name, events[0].Component)
	assert.Equal(t, infinibandstore.EventTypeIbPortFlap, events[0].Name)
	assert.Equal(t, apiv1.EventTypeWarning, events[0].Type)
	assert.Equal(t, "mlx5_0 port 1 flap event for conversion test", events[0].Message)
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
				Port: infiniband.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event",
			},
			{
				Time: time.Now().UTC().Add(time.Second),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
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
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_1")
	assert.Contains(t, data.reason, "; ")

	// Verify both events were inserted
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 2)

	// Events should be sorted by time
	assert.Equal(t, infinibandstore.EventTypeIbPortFlap, events[0].Name)
	assert.Equal(t, infinibandstore.EventTypeIbPortDrop, events[1].Name)
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

// Tests for event insertion logic with Find/Insert pattern
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
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Should be unhealthy due to the event, despite Find error
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")

	// Find error should be logged but not prevent processing
	// No events should be inserted due to Find error
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 0)
}

func TestComponentCheckWithExistingEvent(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Pre-insert an event to simulate existing event
	existingEvent := eventstore.Event{
		Component: Name,
		Time:      time.Now().UTC(),
		Name:      infinibandstore.EventTypeIbPortFlap,
		Type:      string(apiv1.EventTypeWarning),
		Message:   "mlx5_0 port 1 flap event",
	}
	err := mockBucket.Insert(context.Background(), existingEvent)
	require.NoError(t, err)

	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Should be unhealthy due to the event
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")

	// Should still have only one event (existing event not duplicated)
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, existingEvent.Name, events[0].Name)
	assert.Equal(t, existingEvent.Message, events[0].Message)
}

func TestComponentCheckWithNewEvent(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Should be unhealthy due to the event
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_0")

	// Should have inserted new event
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, Name, events[0].Component)
	assert.Equal(t, infinibandstore.EventTypeIbPortDrop, events[0].Name)
	assert.Equal(t, apiv1.EventTypeWarning, events[0].Type)
	assert.Equal(t, "mlx5_0 port 1 drop event", events[0].Message)
}

func TestComponentCheckWithEventInsertError(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create a mock bucket that returns error on Insert
	mockBucket := createMockEventBucket()
	mockBucket.insertErr = errors.New("insert operation failed")

	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Should be unhealthy due to the event, despite Insert error
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")

	// Insert error should be logged but not prevent processing
	// No events should be inserted due to Insert error
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 0)
}

// Tests for different event types (Drop/Flap)
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
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

func TestComponentCheckWithDropEventType(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with drop event type
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Should be unhealthy due to drop event
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_0")

	// Drop event should be inserted
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, infinibandstore.EventTypeIbPortDrop, events[0].Name)
	assert.Equal(t, "mlx5_0 port 1 drop event", events[0].Message)
}

func TestComponentCheckWithFlapEventType(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with flap event type
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Should be unhealthy due to flap event
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")

	// Flap event should be inserted
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, infinibandstore.EventTypeIbPortFlap, events[0].Name)
	assert.Equal(t, "mlx5_0 port 1 flap event", events[0].Message)
}

func TestComponentCheckWithMixedEventTypes(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	// Create a mock store with mixed event types including unknown
	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortDrop,
				EventReason: "mlx5_0 port 1 drop event",
			},
			{
				Time: time.Now().UTC().Add(time.Second),
				Port: infiniband.IBPort{
					Device: "mlx5_1",
					Port:   1,
				},
				EventType:   "unknown_event_type",
				EventReason: "unknown event reason",
			},
			{
				Time: time.Now().UTC().Add(2 * time.Second),
				Port: infiniband.IBPort{
					Device: "mlx5_2",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_2 port 1 flap event",
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 3,
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
			}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to known events (drop and flap)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_0")
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_2")
	assert.NotContains(t, data.reason, "unknown event reason")

	// Only known events should be inserted (drop and flap)
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 2)
	assert.Equal(t, infinibandstore.EventTypeIbPortDrop, events[0].Name)
	assert.Equal(t, infinibandstore.EventTypeIbPortFlap, events[1].Name)
}

// Tests for context timeout scenarios
func TestComponentCheckWithContextTimeoutDuringFind(t *testing.T) {
	t.Parallel()

	// Create a context that will timeout quickly
	cctx, ccancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer ccancel()

	// Create a mock bucket that will delay on Find to trigger timeout
	mockBucket := &mockEventBucket{
		events: eventstore.Events{},
	}

	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		requestTimeout: 10 * time.Millisecond, // Short timeout
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Add a delay to trigger timeout
	time.Sleep(2 * time.Millisecond)

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)

	// Should be unhealthy due to the event, despite potential timeout
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")
}

func TestComponentCheckWithContextTimeoutDuringInsert(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create a mock bucket that will delay on Insert to trigger timeout
	mockBucket := &mockEventBucket{
		events: eventstore.Events{},
	}

	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		requestTimeout: 1 * time.Millisecond, // Very short timeout
		nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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

	// Should be unhealthy due to the event, despite potential timeout
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "device(s) down too long: mlx5_0")
}

func TestComponentCheckWithRequestTimeoutConfiguration(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
					Device: "mlx5_0",
					Port:   1,
				},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_0 port 1 flap event",
			},
		},
	}

	// Test with different request timeout values
	timeouts := []time.Duration{
		1 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
	}

	for _, timeout := range timeouts {
		c := &component{
			ctx:            cctx,
			cancel:         ccancel,
			eventBucket:    mockBucket,
			ibPortsStore:   mockStore,
			requestTimeout: timeout,
			nvmlInstance:   &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			getThresholdsFunc: func() infiniband.ExpectedPortStates {
				return infiniband.ExpectedPortStates{
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
		require.True(t, ok, "timeout: %v", timeout)
		require.NotNil(t, data, "timeout: %v", timeout)

		// Should be unhealthy due to the event regardless of timeout
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health, "timeout: %v", timeout)
		assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0", "timeout: %v", timeout)
	}
}

func TestComponentCheckWithExistingEventSkipInsertion(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create a mock bucket that already has an event
	mockBucket := createMockEventBucket()
	existingEvent := eventstore.Event{
		Component: Name,
		Time:      time.Now().UTC(),
		Name:      "ib_port_flap",
		Type:      string(apiv1.EventTypeWarning),
		Message:   "mlx5_0 port 1 flap event",
	}
	mockBucket.events = append(mockBucket.events, existingEvent)

	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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
							LinkLayer: "InfiniBand",
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
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")

	// Verify the event was found (not inserted again)
	// The mock bucket should still have exactly one event
	assert.Equal(t, 1, len(mockBucket.events))
	assert.Equal(t, existingEvent.Name, mockBucket.events[0].Name)
	assert.Equal(t, existingEvent.Message, mockBucket.events[0].Message)
}

func TestComponentCheckWithSuccessfulEventInsertion(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create a mock bucket that starts empty
	mockBucket := createMockEventBucket()

	mockStore := &testIBPortsStore{
		events: []infinibandstore.Event{
			{
				Time: time.Now().UTC(),
				Port: infiniband.IBPort{
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
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
							LinkLayer: "InfiniBand",
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
	assert.Contains(t, data.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_0")

	// Verify the event was inserted successfully
	// The mock bucket should now have exactly one event
	assert.Equal(t, 1, len(mockBucket.events))
	assert.Equal(t, "ib_port_flap", mockBucket.events[0].Name)
	assert.Equal(t, "mlx5_0 port 1 flap event", mockBucket.events[0].Message)
	assert.Equal(t, string(apiv1.EventTypeWarning), mockBucket.events[0].Type)
}
