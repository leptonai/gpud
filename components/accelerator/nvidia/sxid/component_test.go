package sxid

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// createTestEvent creates a test event with the specified timestamp
func createTestEvent(timestamp time.Time) eventstore.Event {
	return eventstore.Event{
		Time:    timestamp,
		Name:    "test_event",
		Type:    "test_type",
		Message: "test message",
		ExtraInfo: map[string]string{
			"key": "value",
		},
	}
}

// createGPUdInstance creates a mock GPUdInstance for testing
func createGPUdInstance(ctx context.Context, rebootEventStore pkghost.RebootEventStore, eventStore eventstore.Store) *components.GPUdInstance {
	return &components.GPUdInstance{
		RootCtx:          ctx,
		EventStore:       eventStore,
		RebootEventStore: rebootEventStore,
	}
}

// initComponentForTest initializes a component and sets up necessary test mocks
func initComponentForTest(ctx context.Context, t *testing.T) (*component, func()) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	gpudInstance := createGPUdInstance(ctx, rebootEventStore, store)
	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Type assertion to access component methods
	component, ok := comp.(*component)
	assert.True(t, ok, "Failed to cast to *component type")

	// Ensure the component has an eventBucket
	if component.eventBucket == nil {
		bucket, err := store.Bucket(Name)
		assert.NoError(t, err)
		component.eventBucket = bucket
	}

	return component, cleanup
}

func TestMergeEvents(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		a        eventstore.Events
		b        eventstore.Events
		expected int
	}{
		{
			name:     "both empty",
			a:        nil,
			b:        nil,
			expected: 0,
		},
		{
			name: "a empty",
			a:    nil,
			b: eventstore.Events{
				createTestEvent(now),
			},
			expected: 1,
		},
		{
			name: "b empty",
			a: eventstore.Events{
				createTestEvent(now),
			},
			b:        nil,
			expected: 1,
		},
		{
			name: "both non-empty",
			a: eventstore.Events{
				createTestEvent(now.Add(-1 * time.Hour)),
				createTestEvent(now),
			},
			b: eventstore.Events{
				createTestEvent(now.Add(-2 * time.Hour)),
				createTestEvent(now.Add(-30 * time.Minute)),
			},
			expected: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeEvents(tt.a, tt.b)
			assert.Equal(t, tt.expected, len(result))
			if len(result) > 1 {
				for i := 1; i < len(result); i++ {
					assert.True(t, result[i-1].Time.After(result[i].Time) ||
						result[i-1].Time.Equal(result[i].Time),
						"events should be sorted by timestamp")
				}
			}
		})
	}

	t.Run("verify sorting", func(t *testing.T) {
		a := eventstore.Events{
			createTestEvent(now.Add(2 * time.Hour)),
			createTestEvent(now.Add(-1 * time.Hour)),
		}
		b := eventstore.Events{
			createTestEvent(now),
			createTestEvent(now.Add(-2 * time.Hour)),
		}
		result := mergeEvents(a, b)
		assert.Len(t, result, 4)
		expectedTimes := []time.Time{
			now.Add(2 * time.Hour),
			now,
			now.Add(-1 * time.Hour),
			now.Add(-2 * time.Hour),
		}
		for i, expectedTime := range expectedTimes {
			assert.Equal(t, expectedTime.Unix(), result[i].Time.Unix(),
				"event at index %d should have correct timestamp", i)
		}
	})
}

func TestSXIDComponent_SetHealthy(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	err := component.SetHealthy()
	assert.NoError(t, err)

	select {
	case event := <-component.extraEventCh:
		assert.Equal(t, "SetHealthy", event.Name)
	default:
		t.Error("expected event in channel but got none")
	}
}

func TestSXIDComponent_SetHealthy_ChannelFull(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Create a channel with a small buffer capacity
	component.extraEventCh = make(chan *eventstore.Event, 1)

	// Fill the channel
	component.extraEventCh <- &eventstore.Event{
		Time: time.Now(),
		Name: "dummy",
	}

	// SetHealthy should not block when the channel is full
	err := component.SetHealthy()
	assert.NoError(t, err)
}

func TestSXIDComponent_Events(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Create a channel with a buffer to avoid blocking
	msgCh := make(chan kmsg.Message, 1)
	go component.start(msgCh, 500*time.Millisecond)
	defer component.Close()

	testEvents := eventstore.Events{
		createTestEvent(time.Now()),
	}

	// Insert test events
	for _, event := range testEvents {
		event := event // To avoid capturing the loop variable
		err := component.eventBucket.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Wait for events to be processed
	time.Sleep(1 * time.Second)

	events, err := component.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), len(testEvents))

	if len(events) > 0 {
		// Check that at least one of the events matches what we expect
		found := false
		for _, event := range events {
			if event.Name == testEvents[0].Name {
				found = true
				assert.Equal(t, testEvents[0].Type, string(event.Type))
				assert.Equal(t, testEvents[0].Message, event.Message)
				break
			}
		}
		assert.True(t, found, "Couldn't find the test event in the retrieved events")
	}
}

func TestSXIDComponent_States(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Create a channel with a buffer to avoid blocking
	msgCh := make(chan kmsg.Message, 1)
	go component.start(msgCh, 100*time.Millisecond)
	defer component.Close()

	s := apiv1.HealthState{
		Name:   StateNameErrorSXid,
		Health: apiv1.HealthStateTypeHealthy,
		Reason: "SXIDComponent is healthy",
	}
	component.currState = s
	states := component.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, s, states[0])

	startTime := time.Now().Add(-1 * time.Hour)

	tests := []struct {
		name      string
		events    eventstore.Events
		wantState []apiv1.HealthState
	}{
		{
			name: "critical sxid happened and reboot recovered",
			events: eventstore.Events{
				createSXidEvent(time.Now().Add(-5*24*time.Hour), 31, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
				createSXidEvent(startTime, 31, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
				createSXidEvent(startTime.Add(5*time.Minute), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
				{Name: "reboot", Time: startTime.Add(10 * time.Minute)},
				createSXidEvent(startTime.Add(15*time.Minute), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
				{Name: "reboot", Time: startTime.Add(20 * time.Minute)},
				createSXidEvent(startTime.Add(25*time.Minute), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			},
			wantState: []apiv1.HealthState{
				{Health: apiv1.HealthStateTypeHealthy, SuggestedActions: nil},
				{Health: apiv1.HealthStateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{Health: apiv1.HealthStateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{Health: apiv1.HealthStateTypeHealthy, SuggestedActions: nil},
				{Health: apiv1.HealthStateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{Health: apiv1.HealthStateTypeHealthy, SuggestedActions: nil},
				{Health: apiv1.HealthStateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Insert test events directly into eventBucket rather than using extraEventCh
			for i, event := range tt.events {
				err := component.eventBucket.Insert(ctx, event)
				assert.NoError(t, err)

				// Manually trigger state update rather than waiting for channel
				err = component.updateCurrentState()
				assert.NoError(t, err)

				states := component.LastHealthStates()
				t.Log(states[0])
				assert.Len(t, states, 1, "index %d", i)
				assert.Equal(t, tt.wantState[i].Health, states[0].Health, "index %d", i)
				if tt.wantState[i].SuggestedActions == nil {
					assert.Equal(t, tt.wantState[i].SuggestedActions, states[0].SuggestedActions, "index %d", i)
				}
				if tt.wantState[i].SuggestedActions != nil && states[0].SuggestedActions != nil {
					assert.Equal(t, tt.wantState[i].SuggestedActions.RepairActions, states[0].SuggestedActions.RepairActions, "index %d", i)
				}
			}
			err := component.SetHealthy()
			assert.NoError(t, err)
			// Wait for events to be processed
			time.Sleep(500 * time.Millisecond)
		})
	}
}

func TestSXIDComponent_Check(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Create a mock NVML instance
	mockNVML := &MockNVMLInstance{
		exists: true,
	}
	component.nvmlInstance = mockNVML

	// Mock the readAllKmsg function to return test data
	mockMessages := []kmsg.Message{
		{
			Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
			Timestamp: metav1.Time{Time: time.Now()},
		},
		{
			Message:   "some other message that doesn't match",
			Timestamp: metav1.Time{Time: time.Now()},
		},
	}
	component.readAllKmsg = func(ctx context.Context) ([]kmsg.Message, error) {
		return mockMessages, nil
	}

	// Run the check
	result := component.Check()

	// Verify the result
	data, ok := result.(*checkResult)
	assert.True(t, ok, "Result should be of type *checkResult")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Contains(t, data.reason, "matched")
}

func TestSXIDComponent_Check_Error(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Create a mock NVML instance
	mockNVML := &MockNVMLInstance{
		exists: true,
	}
	component.nvmlInstance = mockNVML

	// Mock the readAllKmsg function to return an error
	component.readAllKmsg = func(ctx context.Context) ([]kmsg.Message, error) {
		return nil, errors.New("test error")
	}

	// Run the check
	result := component.Check()

	// Verify the result
	data, ok := result.(*checkResult)
	assert.True(t, ok, "Result should be of type *checkResult")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "failed to read kmsg")
}

func TestSXIDComponent_Check_NoNVML(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Set nvmlInstance to nil to simulate no NVML
	component.nvmlInstance = nil

	// Run the check
	result := component.Check()

	// Verify the result
	data, ok := result.(*checkResult)
	assert.True(t, ok, "Result should be of type *checkResult")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Contains(t, data.reason, "NVIDIA NVML instance is nil")
}

func TestSXIDComponent_Close(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Call Close directly and verify it doesn't error
	err := component.Close()
	assert.NoError(t, err)
}

func TestSXIDComponent_Name(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Verify the component name
	assert.Equal(t, Name, component.Name())
}

func TestTags(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	tags := component.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 4, "Component should return exactly 4 tags")
}

// MockNVMLInstanceNoProduct is a mock implementation that has exists=true but empty product name
type MockNVMLInstanceNoProduct struct {
	MockNVMLInstance
}

func (m *MockNVMLInstanceNoProduct) ProductName() string {
	return ""
}

// MockNVMLInstanceWithProduct is a mock implementation that has exists=true and a valid product name
type MockNVMLInstanceWithProduct struct {
	MockNVMLInstance
}

func (m *MockNVMLInstanceWithProduct) ProductName() string {
	return "Tesla V100"
}

func TestIsSupported(t *testing.T) {
	// Test when nvmlInstance is nil
	comp := &component{}
	assert.False(t, comp.IsSupported())

	// Test when NVMLExists returns false
	comp = &component{
		nvmlInstance: &MockNVMLInstance{exists: false},
	}
	assert.False(t, comp.IsSupported())

	// Test when ProductName returns empty string
	comp = &component{
		nvmlInstance: &MockNVMLInstanceNoProduct{
			MockNVMLInstance: MockNVMLInstance{exists: true},
		},
	}
	assert.False(t, comp.IsSupported())

	// Test when all conditions are met
	comp = &component{
		nvmlInstance: &MockNVMLInstanceWithProduct{
			MockNVMLInstance: MockNVMLInstance{exists: true},
		},
	}
	assert.True(t, comp.IsSupported())
}

func TestDataString(t *testing.T) {
	tests := []struct {
		name        string
		data        *checkResult
		shouldMatch []string
	}{
		{
			name: "data with errors",
			data: &checkResult{
				FoundErrors: []FoundError{
					{
						Kmsg: kmsg.Message{
							Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal error",
							Timestamp: metav1.Time{Time: time.Now()},
						},
						SXidError: SXidError{
							SXid:       12028,
							DeviceUUID: "PCI:0000:05:00.0",
							Detail: &Detail{
								Name: "Test SXid Error",
								SuggestedActionsByGPUd: &apiv1.SuggestedActions{
									RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
								},
								EventType: apiv1.EventTypeFatal,
							},
						},
					},
				},
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "found some errors",
			},
			shouldMatch: []string{"Test SXid Error", "12028", "PCI:0000:05:00.0", "true"},
		},
		{
			name: "empty data",
			data: &checkResult{
				FoundErrors: []FoundError{},
				health:      apiv1.HealthStateTypeHealthy,
				reason:      "no errors found",
			},
			shouldMatch: []string{"no sxid error found"},
		},
		{
			name:        "nil data",
			data:        nil,
			shouldMatch: []string{""},
		},
		{
			name: "data with nil Detail",
			data: &checkResult{
				FoundErrors: []FoundError{
					{
						Kmsg: kmsg.Message{
							Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 20123, Unknown error",
							Timestamp: metav1.Time{Time: time.Now()},
						},
						SXidError: SXidError{
							SXid:       20123,
							DeviceUUID: "PCI:0000:05:00.0",
							Detail:     nil, // This should not cause a panic
						},
					},
				},
				health: apiv1.HealthStateTypeHealthy,
				reason: "found 1 error with nil detail",
			},
			shouldMatch: []string{"unknown", "20123", "PCI:0000:05:00.0", "false"},
		},
		{
			name: "data with nil SuggestedActionsByGPUd",
			data: &checkResult{
				FoundErrors: []FoundError{
					{
						Kmsg: kmsg.Message{
							Message:   "nvidia-nvswitch1: SXid (PCI:0000:0a:00.0): 22013, Non-fatal, Link 57 Minion Link DLREQ interrupt",
							Timestamp: metav1.Time{Time: time.Now()},
						},
						SXidError: SXidError{
							SXid:       22013,
							DeviceUUID: "PCI:0000:0a:00.0",
							Detail: &Detail{
								Name:                   "Minion Link DLREQ interrupt",
								SuggestedActionsByGPUd: nil, // This should not cause a panic (issue #1129)
								EventType:              apiv1.EventTypeCritical,
							},
						},
					},
				},
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "found 1 error with nil suggested actions",
			},
			shouldMatch: []string{"Minion Link DLREQ interrupt", "22013", "PCI:0000:0a:00.0", "unknown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.String()
			for _, substr := range tt.shouldMatch {
				assert.Contains(t, result, substr)
			}
		})
	}
}

func TestDataSummary(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		expected string
	}{
		{
			name: "with reason",
			data: &checkResult{
				reason: "test reason",
			},
			expected: "test reason",
		},
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.Summary()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataHealthState(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		expected apiv1.HealthStateType
	}{
		{
			name: "healthy state",
			data: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
			},
			expected: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy state",
			data: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expected: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.HealthStateType()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatcher(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectMatch  bool
		expectedSXid int
		expectedUUID string
	}{
		{
			name:         "valid SXid message",
			input:        "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
			expectMatch:  true,
			expectedSXid: 12028,
			expectedUUID: "PCI:0000:05:00.0",
		},
		{
			name:        "non-matching message",
			input:       "some random log line",
			expectMatch: false,
		},
		{
			name:        "empty string",
			input:       "",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Match(tt.input)
			if tt.expectMatch {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedSXid, result.SXid)
				assert.Equal(t, tt.expectedUUID, result.DeviceUUID)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestSXIDComponent_UpdateCurrentState_NilStores(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Set stores to nil
	component.rebootEventStore = nil
	component.eventBucket = nil

	// This should not cause an error
	err := component.updateCurrentState()
	assert.NoError(t, err)
}

func TestSXIDComponent_UpdateCurrentState_ErrorOnGet(t *testing.T) {
	// Create a mock event bucket that returns an error on Get
	mockBucket := &mockEventBucket{
		getError: errors.New("test error"),
	}

	// Initialize component with the mock bucket
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	component.eventBucket = mockBucket

	// Should return an error
	err := component.updateCurrentState()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get all events")
}

// mockEventBucket implements eventstore.Bucket for testing
type mockEventBucket struct {
	getError error
	events   eventstore.Events
}

func (m *mockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	return nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	return m.events, nil
}

func (m *mockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	return nil, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	if len(m.events) == 0 {
		return nil, nil
	}
	return &m.events[0], nil
}

func (m *mockEventBucket) Close() {}

func (m *mockEventBucket) Name() string {
	return "mock-bucket"
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeUnixTime int64) (int, error) {
	return 0, nil
}

// MockNVMLInstance implements nvidianvml.Instance for testing
type MockNVMLInstance struct {
	exists bool
}

func (m *MockNVMLInstance) NVMLExists() bool {
	return m.exists
}

func (m *MockNVMLInstance) DeviceGetCount() (int, error) {
	return 0, nil
}

func (m *MockNVMLInstance) DeviceGetHandleByIndex(idx int) (interface{}, error) {
	return nil, nil
}

func (m *MockNVMLInstance) Devices() map[string]device.Device {
	return make(map[string]device.Device)
}

func (m *MockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *MockNVMLInstance) ProductName() string {
	return "Test GPU"
}

func (m *MockNVMLInstance) Architecture() string {
	return ""
}

func (m *MockNVMLInstance) Brand() string {
	return ""
}

func (m *MockNVMLInstance) DriverVersion() string {
	return "test-driver-version"
}

func (m *MockNVMLInstance) DriverMajor() int {
	return 0
}

func (m *MockNVMLInstance) CUDAVersion() string {
	return "test-cuda-version"
}

func (m *MockNVMLInstance) FabricManagerSupported() bool {
	return true
}

func (m *MockNVMLInstance) FabricStateSupported() bool {
	return false
}

func (m *MockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *MockNVMLInstance) Shutdown() error {
	return nil
}

func TestSXIDComponent_Start(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Call Start
	err := component.Start()
	assert.NoError(t, err)

	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		// Start again to ensure it doesn't cause issues
		err = component.Start()
		assert.Equal(t, kmsg.ErrWatcherAlreadyStarted, err)
	}
}

func TestSXIDComponent_Start_ContextCanceled(t *testing.T) {
	// Initialize component with a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Start should exit cleanly when context is canceled
	err := component.Start()
	assert.NoError(t, err)
}

func TestSXIDComponent_Events_NoEventBucket(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Set eventBucket to nil
	component.eventBucket = nil

	// Events should return nil, nil
	events, err := component.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestSXIDComponent_UpdateCurrentState_RebootError(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Create a mock reboot event store that returns an error on GetRebootEvents
	mockRebootStore := &mockRebootEventStore{
		getRebootEventsError: errors.New("test error"),
	}
	component.rebootEventStore = mockRebootStore

	// Create a mock event bucket with events
	mockBucket := &mockEventBucket{
		events: eventstore.Events{
			createTestEvent(time.Now()),
		},
	}
	component.eventBucket = mockBucket

	// Call updateCurrentState - should still work but the error should be in the state
	err := component.updateCurrentState()
	assert.NoError(t, err)

	// Check that the error is in the state
	state := component.LastHealthStates()[0]
	assert.Contains(t, state.Error, "failed to get reboot events")
}

// mockRebootEventStore implements pkghost.RebootEventStore for testing
type mockRebootEventStore struct {
	rebootEvents         eventstore.Events
	getRebootEventsError error
}

func (m *mockRebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if m.getRebootEventsError != nil {
		return nil, m.getRebootEventsError
	}
	return m.rebootEvents, nil
}

func (m *mockRebootEventStore) RecordReboot(ctx context.Context) error {
	return nil
}

func TestSXIDComponent_Start_KmsgWatcherError(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Replace kmsgWatcher with a mock that returns an error when Watch is called
	component.kmsgWatcher = &MockKmsgWatcher{
		watchError: errors.New("watch error"),
	}

	// Start should return the error from kmsgWatcher.Watch
	err := component.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "watch error")
}

func TestSXIDComponent_Start_ChannelHandling(t *testing.T) {
	// Initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	component, cleanup := initComponentForTest(ctx, t)
	defer cleanup()

	// Create a mock kmsg watcher that returns a test channel
	mockCh := make(chan kmsg.Message, 2)
	component.kmsgWatcher = &MockKmsgWatcher{
		watchCh: mockCh,
	}

	// Start the component in a goroutine
	startDone := make(chan struct{})
	go func() {
		err := component.Start()
		assert.NoError(t, err)
		close(startDone)
	}()

	// Send a message to extraEventCh to verify processing
	event := &eventstore.Event{
		Time:    time.Now().UTC(),
		Name:    "test_event",
		Message: "test message",
	}
	component.extraEventCh <- event

	// Also send a kmsg message to mock channel
	mockCh <- kmsg.Message{
		Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
		Timestamp: metav1.Time{Time: time.Now()},
	}

	// Send another kmsg message that doesn't match SXid pattern
	mockCh <- kmsg.Message{
		Message:   "some other non-matching message",
		Timestamp: metav1.Time{Time: time.Now()},
	}

	// Cancel to clean up
	time.Sleep(1 * time.Second) // Allow time for processing
	cancel()

	// Wait for start to finish
	<-startDone
}

// MockKmsgWatcher implements kmsg.Watcher for testing
type MockKmsgWatcher struct {
	watchCh    chan kmsg.Message
	watchError error
}

func (m *MockKmsgWatcher) Watch() (<-chan kmsg.Message, error) {
	if m.watchError != nil {
		return nil, m.watchError
	}

	if m.watchCh == nil {
		m.watchCh = make(chan kmsg.Message)
	}

	return m.watchCh, nil
}

func (m *MockKmsgWatcher) Close() error {
	if m.watchCh != nil {
		close(m.watchCh)
	}
	return nil
}

func TestCheckResult_getError(t *testing.T) {
	tests := []struct {
		name     string
		cr       *checkResult
		expected string
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name:     "checkResult with nil error",
			cr:       &checkResult{err: nil},
			expected: "",
		},
		{
			name:     "checkResult with error",
			cr:       &checkResult{err: errors.New("test error")},
			expected: "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cr.getError()
			if result != tt.expected {
				t.Errorf("getError() = %v, want %v", result, tt.expected)
			}
		})
	}
}
