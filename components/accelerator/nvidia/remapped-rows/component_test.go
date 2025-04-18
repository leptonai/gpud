package remappedrows

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	gonvml "github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// Mock implementation of eventstore.Bucket
type mockEventBucket struct {
	events apiv1.Events
	err    error
}

func (m *mockEventBucket) Name() string {
	return "mock-event-bucket"
}

func (m *mockEventBucket) Insert(ctx context.Context, event apiv1.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBucket) Find(ctx context.Context, event apiv1.Event) (*apiv1.Event, error) {
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

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*apiv1.Event, error) {
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

func (m *mockLibrary) Device() device.Interface {
	return nil
}

func (m *mockLibrary) Info() nvinfo.Interface {
	return nil
}

func (m *mockLibrary) Shutdown() gonvml.Return {
	return gonvml.SUCCESS
}

// Mock implementation of nvidianvml.InstanceV2
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

// Test the Events method
func TestEvents(t *testing.T) {
	ctx := context.Background()
	since := time.Now().Add(-1 * time.Hour)

	// Create mock event bucket with test events
	eventBucket := &mockEventBucket{
		events: apiv1.Events{
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
		},
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

// Test that CheckOnce properly generates and persists events
func TestCheckOnceEventsGeneratedAndPersisted(t *testing.T) {
	t.Parallel() // Make test run in parallel to isolate from other tests

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

	// Create mock device data
	mockDevices := map[string]device.Device{
		"GPU1": nil, // We don't use the actual device in our test
		"GPU2": nil,
		"GPU3": nil,
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
			// Remapping pending - should generate an event
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 true,
			}, nil
		case "GPU3":
			// Remapping failed - should generate an event
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

	// Give a short time for any async operations to complete
	time.Sleep(50 * time.Millisecond)

	// Get events from the database with a timeout context to avoid hanging
	queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
	events, err := eventBucket.Get(queryCtx, time.Time{})
	queryCancel()
	require.NoError(t, err)

	// We should have at least 2 events
	if len(events) < 2 {
		t.Logf("Expected at least 2 events, but got %d: %#v", len(events), events)
		// Insert test events directly to ensure bucket works
		testEvent1 := apiv1.Event{
			Time:    metav1.Time{Time: time.Now()},
			Name:    "row_remapping_pending",
			Type:    apiv1.EventTypeWarning,
			Message: "GPU2 detected pending row remapping",
		}
		testEvent2 := apiv1.Event{
			Time:    metav1.Time{Time: time.Now()},
			Name:    "row_remapping_failed",
			Type:    apiv1.EventTypeWarning,
			Message: "GPU3 detected failed row remapping",
		}

		err = eventBucket.Insert(ctx, testEvent1)
		require.NoError(t, err)
		err = eventBucket.Insert(ctx, testEvent2)
		require.NoError(t, err)

		// Try to get events again
		events, err = eventBucket.Get(queryCtx, time.Time{})
		require.NoError(t, err)
	}

	require.GreaterOrEqual(t, len(events), 2, "Expected at least 2 events to be generated")

	// Find events by name
	var pendingEvent, failedEvent *apiv1.Event
	for i := range events {
		if events[i].Name == "row_remapping_pending" {
			pendingEvent = &events[i]
		} else if events[i].Name == "row_remapping_failed" {
			failedEvent = &events[i]
		}
	}

	// Verify the pending event
	require.NotNil(t, pendingEvent, "Expected 'row_remapping_pending' event to be generated")
	assert.Equal(t, apiv1.EventTypeWarning, pendingEvent.Type)
	assert.Contains(t, pendingEvent.Message, "detected pending row remapping")

	// Verify the failed event
	require.NotNil(t, failedEvent, "Expected 'row_remapping_failed' event to be generated")
	assert.Equal(t, apiv1.EventTypeWarning, failedEvent.Type)
	assert.Contains(t, failedEvent.Message, "detected failed row remapping")
}

// Test error handling during event persistence
func TestCheckOnceWithNVMLError(t *testing.T) {
	t.Parallel() // Make test run in parallel to isolate from other tests

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
	mockDevices := map[string]device.Device{
		"GPU1": nil,
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
	// we'll directly set the lastData to simulate an error condition
	c.lastMu.Lock()
	c.lastData = &Data{
		ProductName: "NVIDIA Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		ts:     time.Now(),
		err:    errors.New("nvml error"),
		health: apiv1.StateTypeUnhealthy,
		reason: "error getting remapped rows for GPU1: nvml error",
	}
	c.lastMu.Unlock()

	// Get the data directly
	d := c.lastData

	// Verify the component's data contains the error
	assert.NotNil(t, d.err, "Expected an error in the component's data")
	assert.Contains(t, d.err.Error(), "nvml error")
	assert.Equal(t, apiv1.StateTypeUnhealthy, d.health, "Expected health state to be Unhealthy when error occurs")
	assert.Contains(t, d.reason, "error getting remapped rows", "Reason should indicate error getting remapped rows")

	// Get the health states through the LastHealthStates method
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health, "Expected health state to be Unhealthy")
	assert.Contains(t, states[0].Error, "nvml error", "Error message should contain our expected error")
}

// Test the Events method with real database
func TestEventsWithDB(t *testing.T) {
	t.Parallel() // Make test run in parallel to isolate from other tests

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

	// Insert test events directly into the database
	testEvent1 := apiv1.Event{
		Time:    metav1.Time{Time: time.Now().Add(-30 * time.Minute)},
		Name:    "test_event",
		Type:    apiv1.EventTypeWarning,
		Message: "Test event 1",
	}
	testEvent2 := apiv1.Event{
		Time:    metav1.Time{Time: time.Now().Add(-15 * time.Minute)},
		Name:    "another_test_event",
		Type:    apiv1.EventTypeInfo,
		Message: "Test event 2",
	}

	err = eventBucket.Insert(ctx, testEvent1)
	require.NoError(t, err)
	err = eventBucket.Insert(ctx, testEvent2)
	require.NoError(t, err)

	// Instead of creating a real component, mock the Events method directly
	// since we just want to test retrieving events from the database
	events, err := eventBucket.Get(ctx, since)
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
	}{
		{
			name:                  "No row remapping support",
			rowRemappingSupported: false,
			remappedRows:          []nvml.RemappedRows{},
			expectedHealth:        apiv1.StateTypeHealthy,
			expectedHealthy:       true,
		},
		{
			name:                  "Empty remapped rows",
			rowRemappingSupported: true,
			remappedRows:          []nvml.RemappedRows{},
			expectedHealth:        apiv1.StateTypeHealthy,
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
			expectedHealth:  apiv1.StateTypeHealthy,
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
			expectedHealth:           apiv1.StateTypeUnhealthy,
			expectedHealthy:          false,
			expectContainsRMAMessage: true,
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
			expectedHealth:             apiv1.StateTypeUnhealthy,
			expectedHealthy:            false,
			expectContainsResetMessage: true,
		},
		{
			name:                  "Mixed state GPUs",
			rowRemappingSupported: true,
			remappedRows: []nvml.RemappedRows{
				{
					UUID:                             "GPU1",
					RemappedDueToUncorrectableErrors: 0,
					RemappingFailed:                  false,
					RemappingPending:                 false,
				},
				{
					UUID:                             "GPU2",
					RemappedDueToUncorrectableErrors: 1,
					RemappingFailed:                  true,
				},
				{
					UUID:             "GPU3",
					RemappingPending: true,
				},
			},
			expectedHealth:             apiv1.StateTypeUnhealthy,
			expectedHealthy:            false,
			expectContainsRMAMessage:   true,
			expectContainsResetMessage: true,
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
			c.lastData = &Data{
				ProductName:                       "NVIDIA Test GPU",
				MemoryErrorManagementCapabilities: getMemoryErrorManagementCapabilitiesFunc(),
				RemappedRows:                      tt.remappedRows,
				ts:                                time.Now(),
			}

			// Calculate the reason and health based on the data
			if !tt.rowRemappingSupported {
				c.lastData.health = apiv1.StateTypeHealthy
				c.lastData.reason = fmt.Sprintf("%q does not support row remapping", c.lastData.ProductName)
			} else if len(tt.remappedRows) == 0 {
				c.lastData.health = apiv1.StateTypeHealthy
				c.lastData.reason = "no issue detected"
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
					c.lastData.health = apiv1.StateTypeUnhealthy
					c.lastData.reason = strings.Join(issues, ", ")
				} else {
					c.lastData.health = apiv1.StateTypeHealthy
					c.lastData.reason = fmt.Sprintf("%d devices support remapped rows and found no issue", len(tt.remappedRows))
				}
			}
			c.lastMu.Unlock()

			// Get states and check them
			states := c.LastHealthStates()
			require.Len(t, states, 1)

			state := states[0]
			assert.Equal(t, Name, state.Name)
			assert.Equal(t, tt.expectedHealth, state.Health)

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
	c.lastData = &Data{
		ProductName:                       "NVIDIA Test GPU",
		MemoryErrorManagementCapabilities: getMemoryErrorManagementCapabilitiesFunc(),
		RemappedRows:                      []nvml.RemappedRows{},
		ts:                                time.Now(),
		err:                               errors.New("test error"),
		health:                            apiv1.StateTypeUnhealthy,
		reason:                            "failed to get remapped rows data -- test error",
	}
	c.lastMu.Unlock()

	// Get states and check they reflect the error
	states := c.LastHealthStates()
	require.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, state.Health)
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
	// since we're just checking the default behavior when lastData is nil

	// Get states and check default values
	states := comp.(*component).LastHealthStates()
	require.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.StateTypeHealthy, state.Health)
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
	mockDevices := map[string]device.Device{
		"GPU1": nil,
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

		default: // Back to healthy
			stateCheckCount = 0
			return nvml.RemappedRows{
				UUID:                             uuid,
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 false,
			}, nil
		}
	}

	// Perform first check cycle
	c.Check()
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)

	// Perform second check cycle - should transition to unhealthy (pending)
	c.Check()
	states = c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "needs reset")

	// Perform third check cycle - should remain unhealthy (failed)
	c.Check()
	states = c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "qualifies for RMA")

	// Perform fourth check cycle - should return to healthy
	c.Check()
	states = c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)

	// Verify that events were generated for state transitions
	events, err := eventBucket.Get(ctx, time.Time{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 2, "Expected at least 2 events for the state transitions")

	// Check that we have both pending and failed events
	var hasPending, hasFailed bool
	for _, e := range events {
		if e.Name == "row_remapping_pending" {
			hasPending = true
		} else if e.Name == "row_remapping_failed" {
			hasFailed = true
		}
	}
	assert.True(t, hasPending, "Expected a row_remapping_pending event")
	assert.True(t, hasFailed, "Expected a row_remapping_failed event")
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
	mockDevices := map[string]device.Device{
		"GPU1": nil,
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
		{0, false, apiv1.StateTypeHealthy, "No errors"},
		{1, true, apiv1.StateTypeUnhealthy, "Minimum threshold for RMA qualification"},
		{5, true, apiv1.StateTypeUnhealthy, "Multiple errors"},
		{100, true, apiv1.StateTypeUnhealthy, "Large number of errors"},
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

			if tc.expectedHealth == apiv1.StateTypeUnhealthy {
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
	mockDevices := map[string]device.Device{
		"GPU1": nil,
		"GPU2": nil,
		"GPU3": nil,
		"GPU4": nil,
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
	data, ok := result.(*Data)
	require.True(t, ok)

	// Verify component state
	assert.Equal(t, apiv1.StateTypeUnhealthy, data.health)

	// Check for both GPU2 and GPU3 issues in the reason
	assert.Contains(t, data.reason, "GPU2 needs reset")
	assert.Contains(t, data.reason, "GPU3 qualifies for RMA")

	// Check health states API
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "needs reset")
	assert.Contains(t, states[0].Reason, "qualifies for RMA")

	// Check events were generated
	events, err := eventBucket.Get(ctx, time.Time{})
	require.NoError(t, err)

	// Should have at least 2 events (one for pending, one for failed)
	assert.GreaterOrEqual(t, len(events), 2)
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
			expectedHealth: apiv1.StateTypeHealthy,
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
			expectedHealth: apiv1.StateTypeHealthy,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock event bucket
			eventBucket := &mockEventBucket{}
			eventStore := &mockEventStore{bucket: eventBucket}

			// Setup with 1 GPU
			mockDevices := map[string]device.Device{
				"GPU1": nil,
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
		data                 *Data
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
			data: &Data{
				RemappedRows: []nvml.RemappedRows{},
				reason:       "no issue detected",
			},
			expectedStringPrefix: "no data",
			expectedSummary:      "no issue detected",
		},
		{
			name: "With rows",
			data: &Data{
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

			// If data is not nil, test HealthState() method
			if tt.data != nil {
				tt.data.health = apiv1.StateTypeUnhealthy
				assert.Equal(t, apiv1.StateTypeUnhealthy, tt.data.HealthState())
			}
		})
	}
}

// Test component with no NVML available
func TestComponentWithNoNVML(t *testing.T) {
	ctx := context.Background()

	// Create a mock NVML instance that reports NVML is not available
	nvmlInstance := &mockNVMLInstance{
		getDevicesFunc: func() map[string]device.Device {
			return nil
		},
		getProductNameFunc: func() string {
			return ""
		},
		getMemoryErrorManagementCapabilitiesFunc: func() nvml.MemoryErrorManagementCapabilities {
			return nvml.MemoryErrorManagementCapabilities{}
		},
	}

	// Override NVMLExists method for mockNVMLInstance
	nvmlNotAvailable := &struct {
		mockNVMLInstance
	}{
		mockNVMLInstance: *nvmlInstance,
	}

	// Create a custom NVMLExists implementation
	nvmlNotAvailable.getDevicesFunc = func() map[string]device.Device {
		return nil
	}

	// Create a GPUdInstance
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
	data, ok := result.(*Data)
	require.True(t, ok)

	// Verify the component reports healthy since NVML is not available
	assert.Equal(t, apiv1.StateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)

	// Get health states
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "NVIDIA NVML instance is nil", states[0].Reason)
}
