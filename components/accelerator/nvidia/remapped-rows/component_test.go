package remappedrows

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// Mock implementation of nvml.InstanceV2
type mockNVMLInstance struct {
	productName                       string
	memoryErrorManagementCapabilities nvml.MemoryErrorManagementCapabilities
	devices                           map[string]device.Device
}

func (m *mockNVMLInstance) Library() lib.Library {
	return nil
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return true
}

func (m *mockNVMLInstance) ProductName() string {
	return m.productName
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvml.MemoryErrorManagementCapabilities {
	return m.memoryErrorManagementCapabilities
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

// Mock implementation of eventstore.Bucket
type mockEventBucket struct {
	events []components.Event
	err    error
}

func (m *mockEventBucket) Name() string {
	return "mock-event-bucket"
}

func (m *mockEventBucket) Insert(ctx context.Context, event components.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBucket) Find(ctx context.Context, event components.Event) (*components.Event, error) {
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

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) ([]components.Event, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*components.Event, error) {
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

// Helper function to create a mock NVML instance with test data
func createMockNVMLInstance() *mockNVMLInstance {
	// For testing, we won't add actual devices since we're not testing CheckOnce
	return &mockNVMLInstance{
		productName: "NVIDIA Test GPU",
		memoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		devices: make(map[string]device.Device),
	}
}

// Test the New constructor
func TestNew(t *testing.T) {
	ctx := context.Background()
	nvmlInstance := createMockNVMLInstance()
	eventBucket := &mockEventBucket{}

	comp, err := New(ctx, nvmlInstance, eventBucket)
	require.NoError(t, err)
	require.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

// Test only the Data.getReason method
func TestDataGetReason(t *testing.T) {
	// Test with nil Data
	var nilData *Data
	reason := nilData.getReason()
	assert.Equal(t, "no remapped rows data", reason)

	// Test with error
	errorData := &Data{
		err: errors.New("test error"),
	}
	reason = errorData.getReason()
	assert.Contains(t, reason, "failed to get remapped rows data")
	assert.Contains(t, reason, "test error")

	// Test with data that has no row remapping support
	noSupportData := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: false,
		},
		RemappedRows: make(map[string]nvml.RemappedRows), // Initialize to empty map
	}
	reason = noSupportData.getReason()
	assert.Contains(t, reason, "does not support row remapping")

	// Test with data that has RMA qualifying GPU
	rmaData := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		RemappedRows: map[string]nvml.RemappedRows{
			"GPU1": {
				RemappedDueToUncorrectableErrors: 1,
				RemappingFailed:                  true,
			},
		},
	}
	reason = rmaData.getReason()
	assert.Contains(t, reason, "qualifies for RMA")

	// Test with data that has reset required GPU
	resetData := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		RemappedRows: map[string]nvml.RemappedRows{
			"GPU1": {
				RemappingPending: true,
			},
		},
	}
	reason = resetData.getReason()
	assert.Contains(t, reason, "needs reset")
}

// Test the Events method
func TestEvents(t *testing.T) {
	ctx := context.Background()
	since := time.Now().Add(-1 * time.Hour)

	// Create mock event bucket with test events
	eventBucket := &mockEventBucket{
		events: []components.Event{
			{
				Time:    metav1.Time{Time: time.Now().Add(-30 * time.Minute)},
				Name:    "test_event",
				Type:    common.EventTypeWarning,
				Message: "Test event 1",
			},
			{
				Time:    metav1.Time{Time: time.Now().Add(-15 * time.Minute)},
				Name:    "test_event",
				Type:    common.EventTypeInfo,
				Message: "Test event 2",
			},
		},
	}

	nvmlInstance := createMockNVMLInstance()
	comp, err := New(ctx, nvmlInstance, eventBucket)
	require.NoError(t, err)

	// Get events
	events, err := comp.Events(ctx, since)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

// Test the RegisterCollectors method
func TestRegisterCollectors(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx := context.Background()
	nvmlInstance := createMockNVMLInstance()
	eventBucket := &mockEventBucket{}

	comp, err := New(ctx, nvmlInstance, eventBucket)
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	err = comp.(*component).RegisterCollectors(reg, dbRW, dbRO, "test_metrics")
	require.NoError(t, err)

	// Verify gatherer is set
	assert.Equal(t, reg, comp.(*component).gatherer)
}

// Test the Data.getHealth method
func TestDataGetHealth(t *testing.T) {
	// Test with error
	errorData := &Data{
		err: errors.New("test error"),
	}
	health, healthy := errorData.getHealth()
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)

	// Test with no row remapping support
	noSupportData := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: false,
		},
		RemappedRows: make(map[string]nvml.RemappedRows),
	}
	health, healthy = noSupportData.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	// Test with healthy GPUs
	healthyData := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		RemappedRows: map[string]nvml.RemappedRows{
			"GPU1": {
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 false,
			},
		},
	}
	health, healthy = healthyData.getHealth()
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	// Test with RMA qualifying GPU
	rmaData := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		RemappedRows: map[string]nvml.RemappedRows{
			"GPU1": {
				RemappedDueToUncorrectableErrors: 1,
				RemappingFailed:                  true,
			},
		},
	}
	health, healthy = rmaData.getHealth()
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)

	// Test with reset required GPU
	resetData := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		RemappedRows: map[string]nvml.RemappedRows{
			"GPU1": {
				RemappingPending: true,
			},
		},
	}
	health, healthy = resetData.getHealth()
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)
}

// Test the Data.getStates method
func TestDataGetStates(t *testing.T) {
	// Test with RMA qualifying GPU
	rmaData := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		RemappedRows: map[string]nvml.RemappedRows{
			"GPU1": {
				RemappedDueToUncorrectableErrors: 1,
				RemappingFailed:                  true,
			},
		},
	}
	states, err := rmaData.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "row_remapping", states[0].Name)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "qualifies for RMA")
	assert.Contains(t, states[0].ExtraInfo["data"], "Test GPU")
	assert.Empty(t, states[0].Error, "Error should be empty when Data.err is nil")

	// Test with mixed state GPUs
	mixedData := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		RemappedRows: map[string]nvml.RemappedRows{
			"GPU1": {
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 false,
			},
			"GPU2": {
				RemappedDueToUncorrectableErrors: 1,
				RemappingFailed:                  true,
			},
			"GPU3": {
				RemappingPending: true,
			},
		},
	}
	states, err = mixedData.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "row_remapping", states[0].Name)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "GPU2 qualifies for RMA")
	assert.Contains(t, states[0].Reason, "GPU3 needs reset")
	assert.NotContains(t, states[0].Reason, "GPU1")
	assert.Empty(t, states[0].Error, "Error should be empty when Data.err is nil")
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

	// Create mock NVML instance with test devices
	nvmlInstance := &mockNVMLInstance{
		productName: "NVIDIA Test GPU",
		memoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		devices: map[string]device.Device{
			"GPU1": nil, // We don't use the actual device in our test
			"GPU2": nil,
			"GPU3": nil,
		},
	}

	// Create the component properly through the New function
	comp, err := New(ctx, nvmlInstance, eventBucket)
	require.NoError(t, err)

	// Defer shutdown to ensure proper cleanup
	defer func() {
		err := comp.Close()
		require.NoError(t, err)
	}()

	// Get the underlying component to modify getRemappedRowsFunc
	c := comp.(*component)
	c.getRemappedRowsFunc = func(uuid string, dev device.Device) (nvml.RemappedRows, error) {
		switch uuid {
		case "GPU1":
			// Healthy GPU - no events expected
			return nvml.RemappedRows{
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 false,
			}, nil
		case "GPU2":
			// Remapping pending - should generate an event
			return nvml.RemappedRows{
				RemappedDueToUncorrectableErrors: 0,
				RemappingFailed:                  false,
				RemappingPending:                 true,
			}, nil
		case "GPU3":
			// Remapping failed - should generate an event
			return nvml.RemappedRows{
				RemappedDueToUncorrectableErrors: 2,
				RemappingFailed:                  true,
				RemappingPending:                 false,
			}, nil
		default:
			return nvml.RemappedRows{}, errors.New("unknown GPU")
		}
	}

	// Run CheckOnce
	c.CheckOnce()

	// Get events from the database with a timeout context to avoid hanging
	queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
	events, err := eventBucket.Get(queryCtx, time.Time{})
	queryCancel()
	require.NoError(t, err)
	require.Len(t, events, 2, "Expected 2 events to be generated")

	// Find events by name
	var pendingEvent, failedEvent *components.Event
	for i := range events {
		if events[i].Name == "row_remapping_pending" {
			pendingEvent = &events[i]
		} else if events[i].Name == "row_remapping_failed" {
			failedEvent = &events[i]
		}
	}

	// Verify the pending event
	require.NotNil(t, pendingEvent, "Expected 'row_remapping_pending' event to be generated")
	assert.Equal(t, common.EventTypeWarning, pendingEvent.Type)
	assert.Contains(t, pendingEvent.Message, "GPU2")
	assert.Contains(t, pendingEvent.Message, "pending row remapping")
	assert.Equal(t, "GPU2", pendingEvent.ExtraInfo["gpu_id"])
	assert.Contains(t, pendingEvent.ExtraInfo["data"], "NVIDIA Test GPU")

	// Verify the failed event
	require.NotNil(t, failedEvent, "Expected 'row_remapping_failed' event to be generated")
	assert.Equal(t, common.EventTypeWarning, failedEvent.Type)
	assert.Contains(t, failedEvent.Message, "GPU3")
	assert.Contains(t, failedEvent.Message, "failed row remapping")
	assert.Equal(t, "GPU3", failedEvent.ExtraInfo["gpu_id"])
	assert.Contains(t, failedEvent.ExtraInfo["data"], "NVIDIA Test GPU")

	// Verify no events for healthy GPU
	for _, event := range events {
		assert.NotContains(t, event.Message, "GPU1", "No events should be generated for healthy GPU")
	}
}

// Test error handling during event persistence
func TestCheckOnceWithNVMLError(t *testing.T) {
	t.Parallel() // Make test run in parallel to isolate from other tests

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Create mock NVML instance with test device
	nvmlInstance := &mockNVMLInstance{
		productName: "NVIDIA Test GPU",
		memoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		devices: map[string]device.Device{
			"GPU1": nil,
		},
	}

	// Create the component
	comp, err := New(ctx, nvmlInstance, eventBucket)
	require.NoError(t, err)

	// Defer shutdown to ensure proper cleanup
	defer func() {
		err := comp.Close()
		require.NoError(t, err)
	}()

	// Override getRemappedRowsFunc to return an error
	c := comp.(*component)
	expectedErr := errors.New("nvml error")
	c.getRemappedRowsFunc = func(uuid string, dev device.Device) (nvml.RemappedRows, error) {
		return nvml.RemappedRows{}, expectedErr
	}

	// Run CheckOnce
	c.CheckOnce()

	// Verify the component's data contains the error
	c.lastMu.RLock()
	defer c.lastMu.RUnlock()

	require.NotNil(t, c.lastData)
	assert.NotNil(t, c.lastData.err, "Expected an error in the component's data")
	assert.Contains(t, c.lastData.err.Error(), "failed to get remapped rows")
	assert.Contains(t, c.lastData.err.Error(), expectedErr.Error())

	// Get events from the database - should be empty since we had an error before creating events
	queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
	events, err := eventBucket.Get(queryCtx, time.Time{})
	queryCancel()
	require.NoError(t, err)
	assert.Len(t, events, 0, "No events should be generated when there's an NVML error")
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
	testEvent1 := components.Event{
		Time:    metav1.Time{Time: time.Now().Add(-30 * time.Minute)},
		Name:    "test_event",
		Type:    common.EventTypeWarning,
		Message: "Test event 1",
	}
	testEvent2 := components.Event{
		Time:    metav1.Time{Time: time.Now().Add(-15 * time.Minute)},
		Name:    "another_test_event",
		Type:    common.EventTypeInfo,
		Message: "Test event 2",
	}

	err = eventBucket.Insert(ctx, testEvent1)
	require.NoError(t, err)
	err = eventBucket.Insert(ctx, testEvent2)
	require.NoError(t, err)

	// Create the component with the real event bucket
	nvmlInstance := createMockNVMLInstance()
	comp, err := New(ctx, nvmlInstance, eventBucket)
	require.NoError(t, err)

	// Defer shutdown to ensure proper cleanup
	defer func() {
		err := comp.Close()
		require.NoError(t, err)
	}()

	// Get events
	queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
	events, err := comp.Events(queryCtx, since)
	queryCancel()
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

// TestDataGetReasonEdgeCases tests edge cases for the getReason method
func TestDataGetReasonEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		expected string
	}{
		{
			name: "empty data with no remapped rows",
			data: &Data{
				ProductName: "Test GPU",
				MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRows: map[string]nvml.RemappedRows{}, // Empty map
			},
			expected: "no issue detected",
		},
		{
			name: "nil remapped rows map",
			data: &Data{
				ProductName: "Test GPU",
				MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRows: nil, // Nil map
			},
			expected: "no issue detected",
		},
		{
			name: "error with specific message",
			data: &Data{
				err: fmt.Errorf("NVML library initialization failed"),
			},
			expected: "failed to get remapped rows data -- NVML library initialization failed",
		},
		{
			name: "multiple GPUs with mixed issues",
			data: &Data{
				ProductName: "Test GPU",
				MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRows: map[string]nvml.RemappedRows{
					"GPU0": { // Healthy
						RemappedDueToUncorrectableErrors: 0,
						RemappingFailed:                  false,
						RemappingPending:                 false,
					},
					"GPU1": { // RMA qualifying
						RemappedDueToUncorrectableErrors: 5,
						RemappingFailed:                  true,
					},
					"GPU2": { // Reset required
						RemappingPending: true,
					},
					"GPU3": { // Both issues
						RemappedDueToUncorrectableErrors: 3,
						RemappingFailed:                  true,
						RemappingPending:                 true,
					},
				},
			},
			expected: "GPU1 qualifies for RMA (row remapping failed, remapped due to 5 uncorrectable error(s)); GPU2 needs reset (detected pending row remapping); GPU3 qualifies for RMA (row remapping failed, remapped due to 3 uncorrectable error(s)); GPU3 needs reset (detected pending row remapping)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := tt.data.getReason()
			// For the mixed issues test, just check that all expected GPUs are mentioned
			if tt.name == "multiple GPUs with mixed issues" {
				assert.Contains(t, reason, "GPU1 qualifies for RMA")
				assert.Contains(t, reason, "GPU2 needs reset")
				assert.Contains(t, reason, "GPU3 qualifies for RMA")
				assert.Contains(t, reason, "GPU3 needs reset")
				assert.NotContains(t, reason, "GPU0") // Should not mention healthy GPU
			} else {
				assert.Equal(t, tt.expected, reason)
			}
		})
	}
}

// TestDataGetHealthEdgeCases tests edge cases for the getHealth method
func TestDataGetHealthEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		data          *Data
		expectedState string
		expectHealthy bool
	}{
		{
			name:          "nil data",
			data:          nil,
			expectedState: components.StateHealthy,
			expectHealthy: true,
		},
		{
			name: "error in data",
			data: &Data{
				err: fmt.Errorf("test error"),
			},
			expectedState: components.StateUnhealthy,
			expectHealthy: false,
		},
		{
			name: "no row remapping support",
			data: &Data{
				ProductName: "Test GPU",
				MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
					RowRemapping: false,
				},
			},
			expectedState: components.StateHealthy,
			expectHealthy: true,
		},
		{
			name: "empty remapped rows",
			data: &Data{
				ProductName: "Test GPU",
				MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRows: map[string]nvml.RemappedRows{},
			},
			expectedState: components.StateHealthy,
			expectHealthy: true,
		},
		{
			name: "nil remapped rows",
			data: &Data{
				ProductName: "Test GPU",
				MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRows: nil,
			},
			expectedState: components.StateHealthy,
			expectHealthy: true,
		},
		{
			name: "one unhealthy GPU among healthy GPUs",
			data: &Data{
				ProductName: "Test GPU",
				MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
					RowRemapping: true,
				},
				RemappedRows: map[string]nvml.RemappedRows{
					"GPU0": { // Healthy
						RemappedDueToUncorrectableErrors: 0,
						RemappingFailed:                  false,
						RemappingPending:                 false,
					},
					"GPU1": { // Healthy
						RemappedDueToUncorrectableErrors: 0,
						RemappingFailed:                  false,
						RemappingPending:                 false,
					},
					"GPU2": { // Unhealthy - needs reset
						RemappingPending: true,
					},
				},
			},
			expectedState: components.StateUnhealthy,
			expectHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health, healthy := tt.data.getHealth()
			assert.Equal(t, tt.expectedState, health)
			assert.Equal(t, tt.expectHealthy, healthy)
		})
	}
}

// TestDataGetStatesErrorHandling tests error handling in the getStates method
func TestDataGetStatesErrorHandling(t *testing.T) {
	// Test with an error condition
	errorData := &Data{
		err: errors.New("failed to get remapped rows data"),
	}
	states, err := errorData.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "row_remapping", states[0].Name)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "failed to get remapped rows data")
	assert.Equal(t, errorData.err.Error(), states[0].Error, "Error should match Data.err")

	// Test with nil Data
	var nilData *Data
	states, err = nilData.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "no data yet")
	assert.Empty(t, states[0].Error, "Error should be empty for nil data")
}

// TestDataGetStatesMalformedData tests getStates with potentially problematic data structures
func TestDataGetStatesMalformedData(t *testing.T) {
	// Test with GPU that doesn't support row remapping
	noSupportData := &Data{
		ProductName: "Test GPU without support",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: false,
		},
	}
	states, err := noSupportData.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "row_remapping", states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "does not support row remapping")
	assert.Empty(t, states[0].Error, "Error should be empty when there's no error")

	// Test with empty RemappedRows map
	emptyRemappedRows := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
		RemappedRows: map[string]nvml.RemappedRows{},
	}
	states, err = emptyRemappedRows.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "row_remapping", states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no issue detected", states[0].Reason)
	assert.Empty(t, states[0].Error, "Error should be empty when there's no error")

	// Test with nil RemappedRows map
	nilRemappedRows := &Data{
		ProductName: "Test GPU",
		MemoryErrorManagementCapabilities: nvml.MemoryErrorManagementCapabilities{
			RowRemapping: true,
		},
	}
	states, err = nilRemappedRows.getStates()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "row_remapping", states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no issue detected", states[0].Reason)
	assert.Empty(t, states[0].Error, "Error should be empty when there's no error")
}
