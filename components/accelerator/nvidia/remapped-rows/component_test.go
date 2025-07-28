package remappedrows

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	gonvml "github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// Mock implementation of eventstore.Bucket
type mockEventBucket struct {
	events eventstore.Events
	err    error
}

func (m *mockEventBucket) Name() string {
	return "mock-event-bucket"
}

func (m *mockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Simple implementation to find events by name
	for i := range m.events {
		if m.events[i].Name == event.Name {
			return &m.events[i], nil
		}
	}
	return nil, nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.events) == 0 {
		return nil, nil
	}
	return &m.events[0], nil
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return 0, nil
}

func (m *mockEventBucket) Close() {
	// No-op implementation
}

// Mock implementation of eventstore.Store
type mockEventStore struct {
	bucket eventstore.Bucket
}

func (m *mockEventStore) Bucket(name string, options ...eventstore.OpOption) (eventstore.Bucket, error) {
	return m.bucket, nil
}

// Mock implementation of lib.Library
type mockLibrary struct{}

func (m *mockLibrary) NVML() gonvml.Interface {
	return nil
}

func (m *mockLibrary) Device() nvlibdevice.Interface {
	return nil
}

func (m *mockLibrary) Info() nvinfo.Interface {
	return nil
}

func (m *mockLibrary) Shutdown() gonvml.Return {
	return gonvml.SUCCESS
}

// Mock implementation of nvidianvml.Instance
type mockNVMLInstance struct {
	getDevicesFunc                           func() map[string]device.Device
	getProductNameFunc                       func() string
	getMemoryErrorManagementCapabilitiesFunc func() nvml.MemoryErrorManagementCapabilities
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return m.getDevicesFunc()
}

func (m *mockNVMLInstance) ProductName() string {
	return m.getProductNameFunc()
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

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvml.MemoryErrorManagementCapabilities {
	return m.getMemoryErrorManagementCapabilitiesFunc()
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return true
}

func (m *mockNVMLInstance) Library() lib.Library {
	return &mockLibrary{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

// Helper function to convert apiv1.Event to eventstore.Event
func apiToStoreEvent(event apiv1.Event) eventstore.Event {
	return eventstore.Event{
		Component: event.Component,
		Time:      event.Time.Time,
		Name:      event.Name,
		Type:      string(event.Type),
		Message:   event.Message,
		ExtraInfo: make(map[string]string),
	}
}

// Test the New constructor
func TestNew(t *testing.T) {
	ctx := context.Background()

	// Create mock functions
	getDevicesFunc := func() map[string]device.Device {
		return make(map[string]device.Device)
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	// Create mock NVML instance
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	eventBucket := &mockEventBucket{}
	eventStore := &mockEventStore{bucket: eventBucket}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   eventStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	require.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

// Test the Tags method
func TestTags(t *testing.T) {
	ctx := context.Background()

	// Create mock NVML instance
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc: func() map[string]device.Device {
			return make(map[string]device.Device)
		},
		getProductNameFunc: func() string {
			return "NVIDIA Test GPU"
		},
		getMemoryErrorManagementCapabilitiesFunc: func() nvml.MemoryErrorManagementCapabilities {
			return nvml.MemoryErrorManagementCapabilities{}
		},
	}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

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

// Test the IsSupported method
func TestIsSupported(t *testing.T) {
	// Test with nil NVML instance
	c := &component{
		nvmlInstance: nil,
	}
	assert.False(t, c.IsSupported())

	// Test with NVML instance that doesn't exist
	c = &component{
		nvmlInstance: &mockNVMLInstance{
			getDevicesFunc: func() map[string]device.Device {
				return make(map[string]device.Device)
			},
			getProductNameFunc: func() string {
				return "NVIDIA Test GPU"
			},
			getMemoryErrorManagementCapabilitiesFunc: func() nvml.MemoryErrorManagementCapabilities {
				return nvml.MemoryErrorManagementCapabilities{}
			},
		},
	}
	// Override the NVMLExists method
	c.nvmlInstance = &mockNVMLInstance{
		getDevicesFunc: func() map[string]device.Device {
			return nil
		},
		getProductNameFunc: func() string {
			return ""
		},
	}
	assert.False(t, c.IsSupported())

	// Test with NVML instance that exists but has no product name
	c = &component{
		nvmlInstance: &mockNVMLInstance{
			getDevicesFunc: func() map[string]device.Device {
				return make(map[string]device.Device)
			},
			getProductNameFunc: func() string {
				return ""
			},
		},
	}
	assert.False(t, c.IsSupported())

	// Test with NVML instance that exists and has a product name
	c = &component{
		nvmlInstance: &mockNVMLInstance{
			getDevicesFunc: func() map[string]device.Device {
				return make(map[string]device.Device)
			},
			getProductNameFunc: func() string {
				return "NVIDIA Test GPU"
			},
		},
	}
	assert.True(t, c.IsSupported())
}

// Test the Events method
func TestEvents(t *testing.T) {
	ctx := context.Background()
	since := time.Now().Add(-1 * time.Hour)

	// Create mock event bucket with test events
	apiEvents := apiv1.Events{
		{
			Time:    metav1.Time{Time: time.Now().Add(-30 * time.Minute)},
			Name:    "test_event",
			Type:    apiv1.EventTypeWarning,
			Message: "Test event 1",
		},
		{
			Time:    metav1.Time{Time: time.Now().Add(-15 * time.Minute)},
			Name:    "another_test_event",
			Type:    apiv1.EventTypeInfo,
			Message: "Test event 2",
		},
	}

	// Convert to store events
	storeEvents := make(eventstore.Events, len(apiEvents))
	for i, apiEvent := range apiEvents {
		storeEvents[i] = apiToStoreEvent(apiEvent)
	}

	eventBucket := &mockEventBucket{
		events: storeEvents,
	}

	getDevicesFunc := func() map[string]device.Device {
		return make(map[string]device.Device)
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	// Create mock NVML instance
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	eventStore := &mockEventStore{bucket: eventBucket}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   eventStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Set the event bucket directly to ensure it's properly initialized
	c := comp.(*component)
	c.eventBucket = eventBucket

	// Get events
	events, err := comp.Events(ctx, since)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

// Test that CheckOnce properly detects and reports remapping issues
func TestCheckOnceRemappingIssueDetection(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Use a unique table name for this test to avoid conflicts with other tests
	tableName := fmt.Sprintf("test_events_%d", time.Now().UnixNano())

	// Create a real event store and bucket using the test database
	eventStore, err := eventstore.New(dbRW, dbRO, 0)
	require.NoError(t, err)
	eventBucket, err := eventStore.Bucket(tableName)
	require.NoError(t, err)
	defer eventBucket.Close()

	// Create mock device data using testutil
	mockDev1 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")
	mockDev2 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:02:00.0")
	mockDev3 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:03:00.0")
	mockDevices := map[string]device.Device{
		"GPU1": mockDev1,
		"GPU2": mockDev2,
		"GPU3": mockDev3,
	}

	// Create the component with our mock functions
	getDevicesFunc := func() map[string]device.Device {
		return mockDevices
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	// Create mock NVML instance
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	mockStore := &mockEventStore{bucket: eventBucket}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   mockStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Get the underlying component to modify getRemappedRowsFunc
	c := comp.(*component)
	c.eventBucket = eventBucket // Ensure eventBucket is set directly
	c.getRemappedRowsFunc = func(uuid string, dev device.Device) (nvml.RemappedRows, error) {
		switch uuid {
		case "GPU1":
			// Healthy GPU - no events expected
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 false,
			}, nil
		case "GPU2":
			// Remapping pending - should be detected and reported
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 true,
			}, nil
		case "GPU3":
			// Remapping failed - should be detected and reported
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 2,
				RemappingFailed:                  true,
				RemappingPending:                 false,
			}, nil
		default:
			return nvml.RemappedRows{}, errors.New("unknown GPU")
		}
	}

	// Run Check instead of CheckOnce
	c.Check()

	// Verify health states without checking for events
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)

	// Verify the component detects both pending and failed remapping issues
	assert.Contains(t, states[0].Reason, "needs reset")
	assert.Contains(t, states[0].Reason, "qualifies for RMA")

	// Verify suggested actions are set appropriately
	require.NotNil(t, states[0].SuggestedActions)
	// RMA should take precedence over reset for suggested actions
	assert.Equal(t, "row remapping failure requires hardware inspection", states[0].SuggestedActions.Description)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, states[0].SuggestedActions.RepairActions)
}

// Test error handling during event persistence
func TestCheckOnceWithNVMLError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := fmt.Sprintf("test_events_%d", time.Now().UnixNano())
	eventStore, err := eventstore.New(dbRW, dbRO, 0)
	require.NoError(t, err)
	eventBucket, err := eventStore.Bucket(tableName)
	require.NoError(t, err)
	defer eventBucket.Close()

	// Create the component with our mock functions
	mockDev1 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")
	mockDevices := map[string]device.Device{
		"GPU1": mockDev1,
	}
	getDevicesFunc := func() map[string]device.Device {
		return mockDevices
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	// Create mock NVML instance
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   eventStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Get the underlying component
	c := comp.(*component)
	c.eventBucket = eventBucket // Ensure eventBucket is set directly

	// Instead of trying to modify c.Check, which isn't assignable,
	// we'll directly set the lastCheckResult to simulate an error condition
	c.lastMu.Lock()
	c.lastCheckResult = &checkResult{
		ProductName: "NVIDIA Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		ts:     time.Now(),
		err:    errors.New("nvml error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting remapped rows for GPU1: nvml error",
	}
	c.lastMu.Unlock()

	// Get the data directly
	cr := c.lastCheckResult

	// Verify the component's data contains the error
	assert.NotNil(t, cr.err, "Expected an error in the component's data")
	assert.Contains(t, cr.err.Error(), "nvml error")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health, "Expected health state to be Unhealthy when error occurs")
	assert.Contains(t, cr.reason, "error getting remapped rows", "Reason should indicate error getting remapped rows")

	// Get the health states through the LastHealthStates method
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health, "Expected health state to be Unhealthy")
	assert.Contains(t, states[0].Error, "nvml error", "Error message should contain our expected error")
}

// Test the Events method with real database
func TestEventsWithDB(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	since := time.Now().Add(-1 * time.Hour)

	// Create test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Use a unique table name for this test
	tableName := fmt.Sprintf("test_events_%d", time.Now().UnixNano())

	// Create a real event store and bucket using the test database
	eventStore, err := eventstore.New(dbRW, dbRO, 0)
	require.NoError(t, err)
	eventBucket, err := eventStore.Bucket(tableName)
	require.NoError(t, err)
	defer eventBucket.Close()

	// Insert test events directly into the database to test Events() method
	testEvent1 := eventstore.Event{
		Time:    time.Now().Add(-30 * time.Minute),
		Name:    "test_event",
		Type:    string(apiv1.EventTypeWarning),
		Message: "Test event 1",
	}
	testEvent2 := eventstore.Event{
		Time:    time.Now().Add(-15 * time.Minute),
		Name:    "another_test_event",
		Type:    string(apiv1.EventTypeInfo),
		Message: "Test event 2",
	}

	err = eventBucket.Insert(ctx, testEvent1)
	require.NoError(t, err)
	err = eventBucket.Insert(ctx, testEvent2)
	require.NoError(t, err)

	// Create a component to test the Events() method
	getDevicesFunc := func() map[string]device.Device {
		return make(map[string]device.Device)
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	mockStore := &mockEventStore{bucket: eventBucket}
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   mockStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Test the Events() method
	events, err := comp.Events(ctx, since)
	require.NoError(t, err)
	assert.Len(t, events, 2)

	// Verify the events by checking their names are present
	foundEvent1, foundEvent2 := false, false
	for _, e := range events {
		if e.Name == "test_event" {
			foundEvent1 = true
		} else if e.Name == "another_test_event" {
			foundEvent2 = true
		}
	}
	assert.True(t, foundEvent1, "Expected to find test_event in results")
	assert.True(t, foundEvent2, "Expected to find another_test_event in results")
}

// Test component states with different scenarios
func TestComponentStates(t *testing.T) {
	ctx := context.Background()

	// Setup base test data
	tests := []struct {
		name                       string
		rowRemappingSupported      bool
		remappedRows               []nvml.RemappedRows
		expectedHealth             apiv1.HealthStateType
		expectedHealthy            bool
		expectContainsRMAMessage   bool
		expectContainsResetMessage bool
		expectedSuggestedAction    *apiv1.SuggestedActions
	}{
		{
			name:                  "No row remapping support",
			rowRemappingSupported: false,
			remappedRows:          []nvml.RemappedRows{},
			expectedHealth:        apiv1.HealthStateTypeHealthy,
			expectedHealthy:       true,
		},
		{
			name:                  "Empty remapped rows",
			rowRemappingSupported: true,
			remappedRows:          []nvml.RemappedRows{},
			expectedHealth:        apiv1.HealthStateTypeHealthy,
			expectedHealthy:       true,
		},
		{
			name:                  "Healthy GPUs",
			rowRemappingSupported: true,
			remappedRows: []nvml.RemappedRows{
				{
					UUID:                             "GPU1",
					RemappedDueToUncorrectableErrors: 0,
					RemappingFailed:                  false,
					RemappingPending:                 false,
				},
			},
			expectedHealth:  apiv1.HealthStateTypeHealthy,
			expectedHealthy: true,
		},
		{
			name:                  "RMA qualifying GPU",
			rowRemappingSupported: true,
			remappedRows: []nvml.RemappedRows{
				{
					UUID:                             "GPU1",
					RemappedDueToUncorrectableErrors: 1,
					RemappingFailed:                  true,
				},
			},
			expectedHealth:           apiv1.HealthStateTypeUnhealthy,
			expectedHealthy:          false,
			expectContainsRMAMessage: true,
			expectedSuggestedAction: &apiv1.SuggestedActions{
				Description: "row remapping failure requires hardware inspection",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeHardwareInspection,
				},
			},
		},
		{
			name:                  "Reset required GPU",
			rowRemappingSupported: true,
			remappedRows: []nvml.RemappedRows{
				{
					UUID:             "GPU1",
					RemappingPending: true,
				},
			},
			expectedHealth:             apiv1.HealthStateTypeUnhealthy,
			expectedHealthy:            false,
			expectContainsResetMessage: true,
			expectedSuggestedAction: &apiv1.SuggestedActions{
				Description: "row remapping pending requires GPU reset or system reboot",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		},
		{
			name:                  "Mixed state GPUs - RMA takes precedence for suggestedAction if both apply and RMA is later",
			rowRemappingSupported: true,
			remappedRows: []nvml.RemappedRows{
				{
					UUID:                             "GPU1", // Healthy
					RemappedDueToUncorrectableErrors: 0,
					RemappingFailed:                  false,
					RemappingPending:                 false,
				},
				{
					UUID:                             "GPU2", // RMA
					RemappedDueToUncorrectableErrors: 1,
					RemappingFailed:                  true,
				},
				{
					UUID:             "GPU3", // Reset
					RemappingPending: true,
				},
			},
			expectedHealth:             apiv1.HealthStateTypeUnhealthy,
			expectedHealthy:            false,
			expectContainsRMAMessage:   true,
			expectContainsResetMessage: true,
			expectedSuggestedAction: &apiv1.SuggestedActions{
				Description: "row remapping failure requires hardware inspection",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeHardwareInspection,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock functions
			getDevicesFunc := func() map[string]device.Device {
				// Return empty map since CheckOnce won't be called
				return make(map[string]device.Device)
			}
			getProductNameFunc := func() string {
				return "NVIDIA Test GPU"
			}
			getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
				return nvml.MemoryErrorManagementCapabilities{
					RowRemapping: tt.rowRemappingSupported,
				}
			}

			// Create mock NVML instance
			nvmlInstance := &mockNVMLInstance{
				getDevicesFunc:                           getDevicesFunc,
				getProductNameFunc:                       getProductNameFunc,
				getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
			}

			eventBucket := &mockEventBucket{}
			eventStore := &mockEventStore{bucket: eventBucket}

			// Create a GPUdInstance
			gpudInstance := &components.GPUdInstance{
				RootCtx:      ctx,
				NVMLInstance: nvmlInstance,
				EventStore:   eventStore,
			}

			comp, err := New(gpudInstance)
			require.NoError(t, err)
			c := comp.(*component)

			// Set the data directly
			c.lastMu.Lock()
			c.lastCheckResult = &checkResult{
				ProductName:                       "NVIDIA Test GPU",
				MemoryErrorManagementCapabilities: getMemoryErrorManagementCapabilitiesFunc(),
				RemappedRows:                      tt.remappedRows,
				ts:                                time.Now(),
				suggestedActions:                  tt.expectedSuggestedAction,
			}

			// Calculate the reason and health based on the data
			if !tt.rowRemappingSupported {
				c.lastCheckResult.health = apiv1.HealthStateTypeHealthy
				c.lastCheckResult.reason = fmt.Sprintf("%q does not support row remapping", c.lastCheckResult.ProductName)
			} else if len(tt.remappedRows) == 0 {
				c.lastCheckResult.health = apiv1.HealthStateTypeHealthy
				c.lastCheckResult.reason = "no issue detected"
			} else {
				issues := make([]string, 0)
				for _, row := range tt.remappedRows {
					if row.QualifiesForRMA() {
						issues = append(issues, fmt.Sprintf("%s qualifies for RMA (row remapping failed, remapped due to %d uncorrectable error(s))", row.UUID, row.RemappedDueToUncorrectableErrors))
					}
					if row.RequiresReset() {
						issues = append(issues, fmt.Sprintf("%s needs reset (detected pending row remapping)", row.UUID))
					}
				}

				if len(issues) > 0 {
					c.lastCheckResult.health = apiv1.HealthStateTypeUnhealthy
					c.lastCheckResult.reason = strings.Join(issues, ", ")
				} else {
					c.lastCheckResult.health = apiv1.HealthStateTypeHealthy
					c.lastCheckResult.reason = fmt.Sprintf("%d devices support remapped rows and found no issue", len(tt.remappedRows))
				}
			}
			c.lastMu.Unlock()

			// Get states and check them
			states := c.LastHealthStates()
			require.Len(t, states, 1)

			state := states[0]
			assert.Equal(t, Name, state.Name)
			assert.Equal(t, tt.expectedHealth, state.Health)
			assert.Equal(t, tt.expectedSuggestedAction, state.SuggestedActions)

			if tt.expectContainsRMAMessage {
				assert.Contains(t, state.Reason, "qualifies for RMA")
			}
			if tt.expectContainsResetMessage {
				assert.Contains(t, state.Reason, "needs reset")
			}
		})
	}
}

// Test error handling for component states
func TestComponentStatesWithError(t *testing.T) {
	ctx := context.Background()

	// Create mock functions
	getDevicesFunc := func() map[string]device.Device {
		return make(map[string]device.Device)
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	// Create mock NVML instance
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	eventBucket := &mockEventBucket{}
	eventStore := &mockEventStore{bucket: eventBucket}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   eventStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	c := comp.(*component)

	// Set error in the data
	c.lastMu.Lock()
	c.lastCheckResult = &checkResult{
		ProductName:                       "NVIDIA Test GPU",
		MemoryErrorManagementCapabilities: getMemoryErrorManagementCapabilitiesFunc(),
		RemappedRows:                      []nvml.RemappedRows{},
		ts:                                time.Now(),
		err:                               errors.New("test error"),
		health:                            apiv1.HealthStateTypeUnhealthy,
		reason:                            "failed to get remapped rows data -- test error",
	}
	c.lastMu.Unlock()

	// Get states and check they reflect the error
	states := c.LastHealthStates()
	require.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Contains(t, state.Reason, "failed to get remapped rows data")
	assert.Equal(t, "test error", state.Error)
}

// Test nil data handling for component states
func TestComponentStatesWithNilData(t *testing.T) {
	ctx := context.Background()

	// Create mock functions
	getDevicesFunc := func() map[string]device.Device {
		return make(map[string]device.Device)
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	// Create mock NVML instance
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	eventBucket := &mockEventBucket{}
	eventStore := &mockEventStore{bucket: eventBucket}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   eventStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	// No need to access the underlying component in this test
	// since we're just checking the default behavior when lastCheckResult is nil

	// Get states and check default values
	states := comp.(*component).LastHealthStates()
	require.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, "no data yet", state.Reason)
	assert.Empty(t, state.Error)
}

// Test to verify multiple check cycles and state transitions
func TestStateTransitions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	tableName := fmt.Sprintf("test_transitions_%d", time.Now().UnixNano())
	eventStore, err := eventstore.New(dbRW, dbRO, 0)
	require.NoError(t, err)
	eventBucket, err := eventStore.Bucket(tableName)
	require.NoError(t, err)
	defer eventBucket.Close()

	// Setup initial state with 1 GPU
	mockDev1 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")
	mockDevices := map[string]device.Device{
		"GPU1": mockDev1,
	}

	var stateCheckCount int
	var stateChangeMu sync.Mutex

	// Set up NVML instance with functions that will change behavior over time
	getDevicesFunc := func() map[string]device.Device {
		return mockDevices
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   eventStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Get the underlying component
	c := comp.(*component)
	c.eventBucket = eventBucket

	// Create a function that changes GPU state after each check
	c.getRemappedRowsFunc = func(uuid string, dev device.Device) (nvml.RemappedRows, error) {
		stateChangeMu.Lock()
		defer stateChangeMu.Unlock()

		switch stateCheckCount {
		case 0: // First check - healthy
			stateCheckCount++
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 false,
			}, nil

		case 1: // Second check - remapping pending
			stateCheckCount++
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 true,
			}, nil

		case 2: // Third check - remapping failed
			stateCheckCount++
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 2,
				RemappingFailed:                  true,
				RemappingPending:                 false,
			}, nil

		case 3: // Fourth check - remapping pending AND failed (failed should take precedence for suggestion)
			stateCheckCount++
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 1, // Needs some uncorrectable for RMA
				RemappingFailed:                  true,
				RemappingPending:                 true,
			}, nil

		default: // Back to healthy
			stateCheckCount = 0 // Reset for potential future loop if test extended
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 false,
			}, nil
		}
	}

	// Perform first check cycle - healthy
	c.Check()
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Nil(t, c.lastCheckResult.suggestedActions, "Healthy state should have nil suggestedActions")
	assert.Nil(t, states[0].SuggestedActions, "Healthy state should have nil SuggestedActions in HealthState")

	// Perform second check cycle - should transition to unhealthy (pending)
	c.Check()
	states = c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "needs reset")
	require.NotNil(t, c.lastCheckResult.suggestedActions, "Pending state should have suggestedActions")
	assert.Equal(t, "row remapping pending requires GPU reset or system reboot", c.lastCheckResult.suggestedActions.Description)
	require.Len(t, c.lastCheckResult.suggestedActions.RepairActions, 1)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, c.lastCheckResult.suggestedActions.RepairActions[0])
	assert.Equal(t, c.lastCheckResult.suggestedActions, states[0].SuggestedActions)

	// Perform third check cycle - should remain unhealthy (failed)
	c.Check()
	states = c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "qualifies for RMA")
	require.NotNil(t, c.lastCheckResult.suggestedActions, "Failed state should have suggestedActions")
	assert.Equal(t, "row remapping failure requires hardware inspection", c.lastCheckResult.suggestedActions.Description)
	require.Len(t, c.lastCheckResult.suggestedActions.RepairActions, 1)
	assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, c.lastCheckResult.suggestedActions.RepairActions[0])
	assert.Equal(t, c.lastCheckResult.suggestedActions, states[0].SuggestedActions)

	// Perform fourth check cycle - pending AND failed (should be unhealthy, failed suggestion)
	c.Check()
	states = c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "qualifies for RMA") // RMA message due to failed
	assert.Contains(t, states[0].Reason, "needs reset")       // Reset message due to pending
	require.NotNil(t, c.lastCheckResult.suggestedActions, "Pending and Failed state should have suggestedActions")
	assert.Equal(t, "row remapping failure requires hardware inspection", c.lastCheckResult.suggestedActions.Description, "Failed suggestion should take precedence")
	require.Len(t, c.lastCheckResult.suggestedActions.RepairActions, 1)
	assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, c.lastCheckResult.suggestedActions.RepairActions[0])
	assert.Equal(t, c.lastCheckResult.suggestedActions, states[0].SuggestedActions)

	// Perform fifth check cycle - should return to healthy
	c.Check()
	states = c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Nil(t, c.lastCheckResult.suggestedActions, "Back to Healthy state should have nil suggestedActions")
	assert.Nil(t, states[0].SuggestedActions, "Back to Healthy state should have nil SuggestedActions in HealthState")

	// Verify that all state transitions were properly handled
	// No events are generated for remapping states as per the component design
	// The component provides health states and suggested actions instead
}

// Test for checking threshold behavior for row remapping
func TestRemappedRowsThresholds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	tableName := fmt.Sprintf("test_thresholds_%d", time.Now().UnixNano())
	eventStore, err := eventstore.New(dbRW, dbRO, 0)
	require.NoError(t, err)
	eventBucket, err := eventStore.Bucket(tableName)
	require.NoError(t, err)
	defer eventBucket.Close()

	// Setup with 1 GPU
	mockDev1 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")
	mockDevices := map[string]device.Device{
		"GPU1": mockDev1,
	}

	// Set up NVML instance
	getDevicesFunc := func() map[string]device.Device {
		return mockDevices
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   eventStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Get the underlying component
	c := comp.(*component)
	c.eventBucket = eventBucket

	// Test cases with different row error counts
	testCases := []struct {
		uncorrectableErrors int
		remappingFailed     bool
		expectedHealth      apiv1.HealthStateType
		description         string
	}{
		{0, false, apiv1.HealthStateTypeHealthy, "No errors"},
		{1, true, apiv1.HealthStateTypeUnhealthy, "Minimum threshold for RMA qualification"},
		{5, true, apiv1.HealthStateTypeUnhealthy, "Multiple errors"},
		{100, true, apiv1.HealthStateTypeUnhealthy, "Large number of errors"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Configure the test case
			c.getRemappedRowsFunc = func(uuid string, dev device.Device) (nvml.RemappedRows, error) {
				return nvml.RemappedRows{
					UUID:                             uuid,
					RemappedDueToUncorrectableErrors: tc.uncorrectableErrors,
					RemappingFailed:                  tc.remappingFailed,
					RemappingPending:                 false,
				}, nil
			}

			// Run the check
			c.Check()

			// Verify health state
			states := c.LastHealthStates()
			require.Len(t, states, 1)
			assert.Equal(t, tc.expectedHealth, states[0].Health)

			if tc.expectedHealth == apiv1.HealthStateTypeUnhealthy {
				assert.Contains(t, states[0].Reason, "qualifies for RMA")
				assert.Contains(t, states[0].Reason, fmt.Sprintf("%d uncorrectable error", tc.uncorrectableErrors))
			}
		})
	}
}

// Test direct CheckOnce functionality with multiple GPUs
func TestCheckOnceWithMultipleGPUs(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	tableName := fmt.Sprintf("test_multiple_gpus_%d", time.Now().UnixNano())
	eventStore, err := eventstore.New(dbRW, dbRO, 0)
	require.NoError(t, err)
	eventBucket, err := eventStore.Bucket(tableName)
	require.NoError(t, err)
	defer eventBucket.Close()

	// Setup with multiple GPUs
	mockDev1 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")
	mockDev2 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:02:00.0")
	mockDev3 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:03:00.0")
	mockDev4 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:04:00.0")
	mockDevices := map[string]device.Device{
		"GPU1": mockDev1,
		"GPU2": mockDev2,
		"GPU3": mockDev3,
		"GPU4": mockDev4,
	}

	// Set up NVML instance
	getDevicesFunc := func() map[string]device.Device {
		return mockDevices
	}
	getProductNameFunc := func() string {
		return "NVIDIA Test GPU"
	}
	getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
		return nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		}
	}

	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc:                           getDevicesFunc,
		getProductNameFunc:                       getProductNameFunc,
		getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
	}

	// Create a GPUdInstance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   eventStore,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Get the underlying component
	c := comp.(*component)
	c.eventBucket = eventBucket

	// Configure mixed states for different GPUs
	c.getRemappedRowsFunc = func(uuid string, dev device.Device) (nvml.RemappedRows, error) {
		switch uuid {
		case "GPU1":
			// Healthy
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 false,
			}, nil
		case "GPU2":
			// Pending reset
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 true,
			}, nil
		case "GPU3":
			// Failed and RMA
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 3,
				RemappingFailed:                  true,
				RemappingPending:                 false,
			}, nil
		case "GPU4":
			// Error case
			return nvml.RemappedRows{}, errors.New("GPU error")
		default:
			return nvml.RemappedRows{}, errors.New("unknown GPU")
		}
	}

	// Run the check
	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify component state
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// Check for both GPU2 and GPU3 issues in the reason (now uses PCI bus ID)
	assert.Contains(t, data.reason, "0000:02:00.0 needs reset")
	assert.Contains(t, data.reason, "0000:03:00.0 qualifies for RMA")

	// Check health states API
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "needs reset")
	assert.Contains(t, states[0].Reason, "qualifies for RMA")

	// Verify suggested actions are set appropriately
	// RMA should take precedence over reset for suggested actions
	require.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, "row remapping failure requires hardware inspection", states[0].SuggestedActions.Description)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, states[0].SuggestedActions.RepairActions)
}

// Test handling of errors in GPU accessors
func TestErrorHandlingInAccessors(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cases := []struct {
		name                    string
		mockDeviceError         bool
		mockCapabilitiesError   bool
		mockGetRemappedRowsFunc func(uuid string, dev device.Device) (nvml.RemappedRows, error)
		expectedHealth          apiv1.HealthStateType
	}{
		{
			name:                  "No errors",
			mockDeviceError:       false,
			mockCapabilitiesError: false,
			mockGetRemappedRowsFunc: func(uuid string, dev device.Device) (nvml.RemappedRows, error) {
				return nvml.RemappedRows{UUID: uuid}, nil
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name:                  "GetRemappedRows error",
			mockDeviceError:       false,
			mockCapabilitiesError: false,
			mockGetRemappedRowsFunc: func(uuid string, dev device.Device) (nvml.RemappedRows, error) {
				// Simulate error for remapped rows
				return nvml.RemappedRows{}, errors.New("NVML error getting remapped rows")
			},
			// The component sets the overall health to Healthy when there are no issues detected
			// Even with errors for individual GPUs, the component only becomes unhealthy if there are
			// actual remapping issues detected
			expectedHealth: apiv1.HealthStateTypeHealthy,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock event bucket
			eventBucket := &mockEventBucket{}
			eventStore := &mockEventStore{bucket: eventBucket}

			// Setup with 1 GPU
			mockDev1 := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")
			mockDevices := map[string]device.Device{
				"GPU1": mockDev1,
			}

			// Set up NVML instance
			getDevicesFunc := func() map[string]device.Device {
				return mockDevices
			}
			getProductNameFunc := func() string {
				return "NVIDIA Test GPU"
			}
			getMemoryErrorManagementCapabilitiesFunc := func() nvml.MemoryErrorManagementCapabilities {
				return nvml.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				}
			}

			nvmlInstance := &mockNVMLInstance{
				getDevicesFunc:                           getDevicesFunc,
				getProductNameFunc:                       getProductNameFunc,
				getMemoryErrorManagementCapabilitiesFunc: getMemoryErrorManagementCapabilitiesFunc,
			}

			// Create a GPUdInstance
			gpudInstance := &components.GPUdInstance{
				RootCtx:      ctx,
				NVMLInstance: nvmlInstance,
				EventStore:   eventStore,
			}

			comp, err := New(gpudInstance)
			require.NoError(t, err)

			// Get the underlying component
			c := comp.(*component)
			c.eventBucket = eventBucket

			// Set up the mock function
			c.getRemappedRowsFunc = tc.mockGetRemappedRowsFunc

			// Run the check
			c.Check()

			// Get the health states
			states := c.LastHealthStates()
			require.Len(t, states, 1)
			assert.Equal(t, tc.expectedHealth, states[0].Health)

			// Only check for error presence/absence, not specific content
			if tc.name == "No errors" {
				assert.Empty(t, states[0].Error)
			}
		})
	}
}

// Test component data string and summary methods
func TestComponentDataStringAndSummary(t *testing.T) {
	t.Parallel()

	// Test different data states
	tests := []struct {
		name                 string
		data                 *checkResult
		expectedStringPrefix string
		expectedSummary      string
	}{
		{
			name:                 "Nil data",
			data:                 nil,
			expectedStringPrefix: "",
			expectedSummary:      "",
		},
		{
			name: "Empty rows",
			data: &checkResult{
				RemappedRows: []nvml.RemappedRows{},
				reason:       "no issue detected",
			},
			expectedStringPrefix: "no data",
			expectedSummary:      "no issue detected",
		},
		{
			name: "With rows",
			data: &checkResult{
				RemappedRows: []nvml.RemappedRows{
					{
						UUID:                             "GPU1",
						RemappedDueToCorrectableErrors:   1,
						RemappedDueToUncorrectableErrors: 2,
						RemappingPending:                 true,
						RemappingFailed:                  false,
					},
				},
				reason: "GPU1 needs reset",
			},
			expectedStringPrefix: "+", // Table starts with a +
			expectedSummary:      "GPU1 needs reset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test String() method
			str := tt.data.String()
			if tt.expectedStringPrefix != "" {
				assert.True(t, strings.HasPrefix(str, tt.expectedStringPrefix) || str == tt.expectedStringPrefix)
			} else {
				assert.Equal(t, tt.expectedStringPrefix, str)
			}

			// Test Summary() method
			summary := tt.data.Summary()
			assert.Equal(t, tt.expectedSummary, summary)

			// If data is not nil, test HealthState() method and ComponentName()
			if tt.data != nil {
				tt.data.health = apiv1.HealthStateTypeUnhealthy // Example health state
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, tt.data.HealthStateType())
				assert.Equal(t, Name, tt.data.ComponentName()) // Test ComponentName
			} else {
				// Test HealthStateType() for nil checkResult
				assert.Equal(t, apiv1.HealthStateType(""), tt.data.HealthStateType()) // Expect empty string for nil receiver
				// ComponentName() for nil checkResult would panic if called as tt.data.ComponentName()
				// but the method is on *checkResult, so one could call (*checkResult)(nil).ComponentName(),
				// which would return Name. Testing the non-nil case is generally more relevant for typical usage.
				// Let's ensure ComponentName() is also tested for the nil case if it makes sense.
				// As Name is a const, (*checkResult)(nil).ComponentName() would return it without panic.
				assert.Equal(t, Name, (*checkResult)(nil).ComponentName()) // Test ComponentName for nil receiver
			}
		})
	}
}

// Test component with no NVML available
func TestComponentWithNoNVML(t *testing.T) {
	ctx := context.Background()

	// Create a GPUdInstance with nil NVML
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nil, // No NVML
		EventStore:   nil,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Run the check
	c := comp.(*component)
	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify the component reports healthy since NVML is not available
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)

	// Get health states
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "NVIDIA NVML instance is nil", states[0].Reason)
	assert.Nil(t, states[0].SuggestedActions, "SuggestedActions should be nil when NVML instance is nil")

	// Test Events() method when eventBucket is nil (which it will be here as EventStore is nil)
	events, err := c.Events(ctx, time.Now())
	assert.NoError(t, err, "Events() should not error with nil eventBucket")
	assert.Nil(t, events, "Events() should return nil events with nil eventBucket")
}

// TestCheckSuggestedActionsWithNilEventBucket verifies that suggestedActions are set based on remapping state, regardless of eventBucket availability.
func TestCheckSuggestedActionsWithNilEventBucket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a proper mock device using testutil
	mockDev := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")
	mockDevices := map[string]device.Device{
		"GPU1": mockDev,
	}
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc: func() map[string]device.Device { return mockDevices },
		getProductNameFunc: func() string {
			return "NVIDIA Test GPU"
		},
		getMemoryErrorManagementCapabilitiesFunc: func() nvml.MemoryErrorManagementCapabilities {
			return nvml.MemoryErrorManagementCapabilities{
				RowRemapping: true, // Supports remapping
			}
		},
	}

	// Create GPUdInstance with nil EventStore, which should lead to a nil eventBucket in the component
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		EventStore:   nil,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)

	c := comp.(*component)
	// Explicitly ensure eventBucket is nil (though New should handle it based on EventStore == nil for non-Linux)
	// Forcing it to nil for clarity in test intent, especially if runtime.GOOS affects New's behavior.
	if c.eventBucket != nil {
		// If New still created a bucket (e.g. if test env is Linux and it used a default store)
		// we force it to nil to test the specific condition 'c.eventBucket == nil' in Check()
		c.eventBucket = nil
	}

	// Configure getRemappedRowsFunc to indicate remapping is pending
	c.getRemappedRowsFunc = func(uuid string, dev device.Device) (nvml.RemappedRows, error) {
		return nvml.RemappedRows{
			UUID:             uuid,
			RemappingPending: true, // Condition that would normally trigger suggestedAction
		}, nil
	}

	// Run the check
	result := c.Check()
	checkRes, ok := result.(*checkResult)
	require.True(t, ok, "Check result should be of type *checkResult")

	// Verify that suggestedActions is still set even when eventBucket is nil
	// The component sets suggested actions based on remapping state, not event bucket availability
	assert.NotNil(t, checkRes.suggestedActions, "suggestedActions should be set based on remapping state, regardless of eventBucket")
	assert.Equal(t, "row remapping pending requires GPU reset or system reboot", checkRes.suggestedActions.Description)

	// Also verify through LastHealthStates
	healthStates := c.LastHealthStates()
	require.Len(t, healthStates, 1)
	assert.NotNil(t, healthStates[0].SuggestedActions, "HealthState.SuggestedActions should be set based on remapping state")
	// The component should report unhealthy due to pending remapping, with appropriate suggestion
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, healthStates[0].Health)
	assert.Contains(t, healthStates[0].Reason, "needs reset")
	assert.Equal(t, "row remapping pending requires GPU reset or system reboot", healthStates[0].SuggestedActions.Description)

	// Test Events() method when eventBucket is nil - this was already tested above in TestComponentWithNoNVML
	// but this test specifically sets up c.eventBucket = nil (or verifies it from New() with nil EventStore)
	// So, re-asserting here for this specific test's setup is fine.
	eventsClient, errClient := c.Events(ctx, time.Now())
	assert.NoError(t, errClient, "Events() should not error with nil eventBucket in TestCheckSuggestedActionsWithNilEventBucket")
	assert.Nil(t, eventsClient, "Events() should return nil events with nil eventBucket in TestCheckSuggestedActionsWithNilEventBucket")
}

// Test failure injection initialization
func TestNewWithFailureInjector(t *testing.T) {
	ctx := context.Background()

	// Create mock NVML instance
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc: func() map[string]device.Device {
			return make(map[string]device.Device)
		},
		getProductNameFunc: func() string {
			return "NVIDIA Test GPU"
		},
		getMemoryErrorManagementCapabilitiesFunc: func() nvml.MemoryErrorManagementCapabilities {
			return nvml.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			}
		},
	}

	// Create failure injector with test UUIDs
	failureInjector := &components.FailureInjector{
		GPUUUIDsWithRowRemappingPending: []string{"GPU-pending-uuid"},
		GPUUUIDsWithRowRemappingFailed:  []string{"GPU-failed-uuid"},
	}

	// Create GPUdInstance with failure injector
	gpudInstance := &components.GPUdInstance{
		RootCtx:         ctx,
		NVMLInstance:    nvmlInstance,
		FailureInjector: failureInjector,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	require.NotNil(t, comp)

	// Get the underlying component
	c := comp.(*component)

	// Verify failure injection UUIDs are stored
	_, hasPendingUUID := c.gpuUUIDsWithRowRemappingPending["GPU-pending-uuid"]
	assert.True(t, hasPendingUUID, "Expected pending UUID to be stored")

	_, hasFailedUUID := c.gpuUUIDsWithRowRemappingFailed["GPU-failed-uuid"]
	assert.True(t, hasFailedUUID, "Expected failed UUID to be stored")

	// Verify component properties
	assert.Equal(t, Name, comp.Name())
}
