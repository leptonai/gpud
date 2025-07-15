package xid

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/xid"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponentNameSimple(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.Equal(t, Name, comp.Name())
}

func TestTags(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,
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

func TestIsSupported(t *testing.T) {
	// Test with nil NVML instance
	comp := &component{}
	assert.False(t, comp.IsSupported())
}

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

func TestXIDComponent_SetHealthy(t *testing.T) {
	// initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	gpudInstance := &components.GPUdInstance{
		RootCtx:          ctx,
		EventStore:       store,
		RebootEventStore: rebootEventStore,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Cast to HealthSettable interface
	healthSettable, ok := comp.(components.HealthSettable)
	assert.True(t, ok, "component should implement HealthSettable interface")
	err = healthSettable.SetHealthy()
	assert.NoError(t, err)

	c := comp.(*component)
	select {
	case event := <-c.extraEventCh:
		assert.Equal(t, "SetHealthy", event.Name)
	default:
		t.Error("expected event in channel but got none")
	}
}

func TestXIDComponent_Events(t *testing.T) {
	// initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	gpudInstance := &components.GPUdInstance{
		RootCtx:          ctx,
		EventStore:       store,
		RebootEventStore: rebootEventStore,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	c := comp.(*component)

	// If eventBucket is nil, create it manually for testing
	if c.eventBucket == nil {
		c.eventBucket, err = store.Bucket(Name)
		assert.NoError(t, err)
	}

	// Setup a test channel for events to avoid using kmsg
	eventCh := make(chan kmsg.Message, 1)
	go c.start(eventCh, 1*time.Second)

	defer func() {
		if err := comp.Close(); err != nil {
			t.Error("failed to close component")
		}
	}()

	testEvents := eventstore.Events{
		createTestEvent(time.Now()),
	}

	// insert test events
	for _, event := range testEvents {
		select {
		case c.extraEventCh <- &event:
		default:
			t.Error("failed to insert event into channel")
		}
	}

	// wait for events to be processed
	time.Sleep(5 * time.Second)

	events, err := comp.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Len(t, events, len(testEvents))
	for i, event := range events {
		assert.Equal(t, testEvents[i].Time.Unix(), event.Time.Unix())
		assert.Equal(t, testEvents[i].Name, event.Name)
		assert.Equal(t, testEvents[i].Type, string(event.Type))
		assert.Equal(t, testEvents[i].Message, event.Message)
	}
}

func TestXIDComponent_States(t *testing.T) {
	// initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	gpudInstance := &components.GPUdInstance{
		RootCtx:          ctx,
		EventStore:       store,
		RebootEventStore: rebootEventStore,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	c := comp.(*component)

	// If eventBucket is nil, create it manually for testing
	if c.eventBucket == nil {
		c.eventBucket, err = store.Bucket(Name)
		assert.NoError(t, err)
	}

	// Setup a test channel for events to avoid using kmsg
	eventCh := make(chan kmsg.Message, 1)
	go c.start(eventCh, 100*time.Millisecond)

	defer func() {
		if err := comp.Close(); err != nil {
			t.Error("failed to close component")
		}
	}()

	s := apiv1.HealthState{
		Name:   StateNameErrorXid,
		Health: apiv1.HealthStateTypeHealthy,
		Reason: "XIDComponent is healthy",
	}
	c.currState = s
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, s, states[0])

	startTime := time.Now().Add(-1 * time.Hour)

	tests := []struct {
		name      string
		events    eventstore.Events
		wantState []apiv1.HealthState
	}{
		{
			name: "critical xid happened and reboot recovered",
			events: eventstore.Events{
				createXidEvent(time.Now().Add(-5*24*time.Hour), 31, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
				createXidEvent(startTime, 31, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
				createXidEvent(startTime.Add(5*time.Minute), 94, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
				{Name: "reboot", Time: startTime.Add(10 * time.Minute)},
				createXidEvent(startTime.Add(15*time.Minute), 94, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
				{Name: "reboot", Time: startTime.Add(20 * time.Minute)},
				createXidEvent(startTime.Add(25*time.Minute), 94, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
			},
			wantState: []apiv1.HealthState{
				{Health: apiv1.HealthStateTypeHealthy, SuggestedActions: nil},
				{Health: apiv1.HealthStateTypeDegraded, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{Health: apiv1.HealthStateTypeDegraded, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{Health: apiv1.HealthStateTypeHealthy, SuggestedActions: nil},
				{Health: apiv1.HealthStateTypeDegraded, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{Health: apiv1.HealthStateTypeHealthy, SuggestedActions: nil},
				{Health: apiv1.HealthStateTypeDegraded, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// insert test events
			for i, event := range tt.events {
				select {
				case c.extraEventCh <- &event:
				default:
					t.Error("failed to insert event into channel")
				}
				// wait for events to be processed
				time.Sleep(1 * time.Second)
				states = comp.LastHealthStates()
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

			// Cast to HealthSettable interface
			healthSettable, ok := comp.(components.HealthSettable)
			assert.True(t, ok, "component should implement HealthSettable interface")
			err = healthSettable.SetHealthy()
			assert.NoError(t, err)

			// wait for events to be processed
			time.Sleep(1 * time.Second)
		})
	}
}

func TestNewWithDifferentConfigurations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("with nil event store", func(t *testing.T) {
		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			EventStore:   nil,
			NVMLInstance: nil,
		}

		comp, err := New(gpudInstance)
		assert.NoError(t, err)
		assert.NotNil(t, comp)

		// Check that the component initialized correctly with nil event store
		c := comp.(*component)
		assert.Nil(t, c.eventBucket)
		assert.Nil(t, c.kmsgWatcher)
	})

	t.Run("with event store", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
		assert.NoError(t, err)

		gpudInstance := &components.GPUdInstance{
			RootCtx:    ctx,
			EventStore: store,
		}

		// We expect this to complete successfully even on non-Linux platforms
		comp, err := New(gpudInstance)
		assert.NoError(t, err)
		assert.NotNil(t, comp)
	})
}

func TestCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("with no NVML instance", func(t *testing.T) {
		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: nil,
		}

		comp, err := New(gpudInstance)
		assert.NoError(t, err)

		result := comp.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
		assert.Contains(t, result.Summary(), "NVIDIA NVML instance is nil")
	})

	t.Run("with no kmsg reader", func(t *testing.T) {
		// Using a properly implemented mock
		mockedNVML := createMockNVMLInstance()
		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockedNVML,
		}

		comp, err := New(gpudInstance)
		assert.NoError(t, err)

		c := comp.(*component)
		c.readAllKmsg = nil

		result := comp.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
		assert.Contains(t, result.Summary(), "kmsg reader is not set")
	})

	t.Run("with kmsg reader error", func(t *testing.T) {
		// Using a properly implemented mock
		mockedNVML := createMockNVMLInstance()
		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockedNVML,
		}

		comp, err := New(gpudInstance)
		assert.NoError(t, err)

		c := comp.(*component)
		c.readAllKmsg = func(ctx context.Context) ([]kmsg.Message, error) {
			return nil, assert.AnError
		}

		result := comp.Check()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
		assert.Contains(t, result.Summary(), "failed to read kmsg")
	})

	t.Run("with XID errors", func(t *testing.T) {
		// Using a properly implemented mock
		mockedNVML := createMockNVMLInstance()
		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockedNVML,
		}

		comp, err := New(gpudInstance)
		assert.NoError(t, err)

		c := comp.(*component)
		c.readAllKmsg = func(ctx context.Context) ([]kmsg.Message, error) {
			return []kmsg.Message{
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "NVRM: Xid (PCI:0000:01:00): 31, pid=XXX",
				},
			}, nil
		}

		result := comp.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
		assert.Contains(t, result.Summary(), "matched")
		data := result.(*checkResult)
		assert.Len(t, data.FoundErrors, 1)
		assert.Equal(t, 31, data.FoundErrors[0].Xid)
	})

	t.Run("with XID 63 and 64 errors that should be skipped", func(t *testing.T) {
		// Using a properly implemented mock
		mockedNVML := createMockNVMLInstance()
		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockedNVML,
		}

		comp, err := New(gpudInstance)
		assert.NoError(t, err)

		c := comp.(*component)
		c.readAllKmsg = func(ctx context.Context) ([]kmsg.Message, error) {
			return []kmsg.Message{
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "NVRM: Xid (PCI:0000:01:00): 63, Row remapping pending",
				},
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "NVRM: Xid (PCI:0000:02:00): 64, Row remapping failure",
				},
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "NVRM: Xid (PCI:0000:03:00): 31, GPU has fallen off the bus",
				},
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "NVRM: Xid (PCI:0000:04:00): 63, Row remapping pending again",
				},
			}, nil
		}

		result := comp.Check()
		data := result.(*checkResult)

		// Should only find XID 31, not 63 or 64
		assert.Len(t, data.FoundErrors, 1)
		assert.Equal(t, 31, data.FoundErrors[0].Xid)
		assert.Contains(t, result.Summary(), "matched 1 xid errors from 4 kmsg(s)")

		// Verify that XID 63 and 64 are not in the found errors
		for _, foundErr := range data.FoundErrors {
			assert.NotEqual(t, 63, foundErr.Xid, "XID 63 should be skipped")
			assert.NotEqual(t, 64, foundErr.Xid, "XID 64 should be skipped")
		}
	})

	t.Run("with only XID 63 and 64 errors", func(t *testing.T) {
		// Using a properly implemented mock
		mockedNVML := createMockNVMLInstance()
		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockedNVML,
		}

		comp, err := New(gpudInstance)
		assert.NoError(t, err)

		c := comp.(*component)
		c.readAllKmsg = func(ctx context.Context) ([]kmsg.Message, error) {
			return []kmsg.Message{
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "NVRM: Xid (PCI:0000:01:00): 63, Row remapping pending",
				},
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "NVRM: Xid (PCI:0000:02:00): 64, Row remapping failure",
				},
			}, nil
		}

		result := comp.Check()
		data := result.(*checkResult)

		// Should find no errors since both XID 63 and 64 are skipped
		assert.Len(t, data.FoundErrors, 0)
		assert.Contains(t, result.Summary(), "matched 0 xid errors from 2 kmsg(s)")
		assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	})
}

func TestClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	gpudInstance := &components.GPUdInstance{
		RootCtx:    ctx,
		EventStore: store,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	// Test that Close cleans up resources properly
	err = comp.Close()
	assert.NoError(t, err)

	// Verify component context is canceled
	c := comp.(*component)
	select {
	case <-c.ctx.Done():
		// Expected, context should be canceled
	default:
		t.Error("context should be canceled after Close")
	}
}

func TestUpdateCurrentState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	gpudInstance := &components.GPUdInstance{
		RootCtx:          ctx,
		EventStore:       store,
		RebootEventStore: rebootEventStore,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	c := comp.(*component)

	// If eventBucket is nil, create it manually for testing
	if c.eventBucket == nil {
		c.eventBucket, err = store.Bucket(Name)
		assert.NoError(t, err)
	}

	// Test initial state (should be healthy since no events)
	err = c.updateCurrentState()
	assert.NoError(t, err)
	initialStates := comp.LastHealthStates()
	assert.Len(t, initialStates, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, initialStates[0].Health, "Initial state should be healthy")

	warningTime := time.Now().Add(-5 * time.Minute)
	xid94Event := eventstore.Event{
		Time: warningTime,
		Name: EventNameErrorXid,
		Type: string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: "94",
			EventKeyDeviceUUID:   "GPU-12345678-1234-5678-1234-567812345678",
		},
	}

	// First, process the event through resolveXIDEvent to ensure correct format
	xid94Event = resolveXIDEvent(xid94Event)

	err = c.eventBucket.Insert(ctx, xid94Event)
	assert.NoError(t, err)

	// Update state based on the warning event
	err = c.updateCurrentState()
	assert.NoError(t, err)

	// Check that the state was updated to degraded
	warningStates := comp.LastHealthStates()
	assert.Len(t, warningStates, 1)
	assert.Equal(t, apiv1.HealthStateTypeDegraded, warningStates[0].Health, "State should be degraded after warning event")
	assert.NotNil(t, warningStates[0].SuggestedActions, "Should have suggested actions")
	assert.Contains(t, warningStates[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)

	// Now test with XID 79 (GPU has fallen off the bus) which should recommend reboot
	fatalTime := time.Now().Add(-4 * time.Minute)
	xid79Event := eventstore.Event{
		Time: fatalTime,
		Name: EventNameErrorXid,
		Type: string(apiv1.EventTypeFatal),
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: "79",
			EventKeyDeviceUUID:   "GPU-12345678-1234-5678-1234-567812345678",
		},
	}

	// Process the event through resolveXIDEvent
	xid79Event = resolveXIDEvent(xid79Event)

	err = c.eventBucket.Insert(ctx, xid79Event)
	assert.NoError(t, err)

	// Update state based on the fatal event
	err = c.updateCurrentState()
	assert.NoError(t, err)

	// Check that the state is unhealthy and recommends reboot
	fatalStates := comp.LastHealthStates()
	assert.Len(t, fatalStates, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, fatalStates[0].Health, "State should be unhealthy after fatal event")
	assert.NotNil(t, fatalStates[0].SuggestedActions, "Should have suggested actions")
	assert.Contains(t, fatalStates[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)

	// Insert a reboot event (which should reset the state to healthy)
	rebootTime := time.Now().Add(-2 * time.Minute)
	rebootEvent := eventstore.Event{
		Time: rebootTime,
		Name: "reboot",
	}
	err = c.eventBucket.Insert(ctx, rebootEvent)
	assert.NoError(t, err)

	// Update state after reboot
	err = c.updateCurrentState()
	assert.NoError(t, err)

	// Check that the state is now healthy
	rebootStates := comp.LastHealthStates()
	assert.Len(t, rebootStates, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, rebootStates[0].Health, "State should be healthy after reboot")
	assert.Nil(t, rebootStates[0].SuggestedActions, "Should not have suggested actions")

	// Insert a SetHealthy event
	healthyTime := time.Now().Add(-1 * time.Minute)
	healthyEvent := eventstore.Event{
		Time: healthyTime,
		Name: "SetHealthy",
	}
	err = c.eventBucket.Insert(ctx, healthyEvent)
	assert.NoError(t, err)

	// Update state after SetHealthy
	err = c.updateCurrentState()
	assert.NoError(t, err)

	// Check that the state is still healthy
	healthyStates := comp.LastHealthStates()
	assert.Len(t, healthyStates, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, healthyStates[0].Health, "State should remain healthy after SetHealthy")
	assert.Nil(t, healthyStates[0].SuggestedActions, "Should not have suggested actions")

	// Test with nil rebootEventStore
	c.rebootEventStore = nil
	err = c.updateCurrentState()
	assert.NoError(t, err, "Should not error with nil rebootEventStore")

	// Test with nil eventBucket
	c.rebootEventStore = rebootEventStore
	eventBucket := c.eventBucket
	c.eventBucket = nil
	err = c.updateCurrentState()
	assert.NoError(t, err, "Should not error with nil eventBucket")

	// Restore for cleanup
	c.eventBucket = eventBucket
}

// Helper function to create a mock NVML instance for testing
func createMockNVMLInstance() *mockNVMLInstance {
	return &mockNVMLInstance{
		devices: make(map[string]device.Device),
	}
}

// Mock NVML implementation for testing
type mockNVMLInstance struct {
	devices map[string]device.Device
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return true
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockNVMLInstance) ProductName() string {
	return "Test GPU"
}

func (m *mockNVMLInstance) Architecture() string {
	return "Test Architecture"
}

func (m *mockNVMLInstance) Brand() string {
	return "Test Brand"
}

func (m *mockNVMLInstance) DriverVersion() string {
	return "test-driver-version"
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 0
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return "test-cuda-version"
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return true
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvml.MemoryErrorManagementCapabilities {
	return nvml.MemoryErrorManagementCapabilities{
		ErrorContainment:     false,
		DynamicPageOfflining: false,
		RowRemapping:         false,
		Message:              "",
	}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func TestDataString(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "no errors",
			data: &checkResult{
				FoundErrors: []FoundError{},
				ts:          time.Now(),
				health:      apiv1.HealthStateTypeHealthy,
				reason:      "no errors found",
			},
			expected: "no xid error found",
		},
		{
			name: "with found errors",
			data: &checkResult{
				FoundErrors: []FoundError{
					{
						Kmsg: kmsg.Message{
							Timestamp: metav1.NewTime(time.Now()),
							Message:   "NVRM: Xid (PCI:0000:01:00): 31, pid=XXX",
						},
						XidError: XidError{
							Xid:        31,
							DeviceUUID: "GPU-12345678",
							Detail: &xid.Detail{
								Name:                      "GPU_HANG",
								CriticalErrorMarkedByGPUd: true,
								SuggestedActionsByGPUd: &apiv1.SuggestedActions{
									RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
								},
							},
						},
					},
				},
				ts:     time.Now(),
				health: apiv1.HealthStateTypeDegraded,
				reason: "found 1 error",
			},
			expected: "", // We'll just check that it's not empty since the table format is hard to predict exactly
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data.String()

			if tt.expected == "" && tt.data != nil && len(tt.data.FoundErrors) > 0 {
				// For cases with errors, just check that the output contains expected data
				assert.NotEmpty(t, result)
				assert.Contains(t, result, "XID")
				assert.Contains(t, result, tt.data.FoundErrors[0].DeviceUUID)
				assert.Contains(t, result, tt.data.FoundErrors[0].Detail.Name)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	gpudInstance := &components.GPUdInstance{
		RootCtx:    ctx,
		EventStore: store,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Test Start with no kmsgWatcher
	err = comp.Start()
	assert.NoError(t, err)
}

func TestResolveXIDEvent(t *testing.T) {
	// Create a test event
	now := time.Now()
	testEvent := eventstore.Event{
		Time: now,
		Name: EventNameErrorXid,
		Type: string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: "31", // GPU_HANG or similar error
			EventKeyDeviceUUID:   "GPU-12345678-1234-5678-1234-567812345678",
		},
	}

	// Resolve the event
	resolvedEvent := resolveXIDEvent(testEvent)

	// Check that the event was properly resolved
	assert.Equal(t, EventNameErrorXid, resolvedEvent.Name)
	assert.NotEmpty(t, resolvedEvent.Message)
	assert.Contains(t, resolvedEvent.Message, "31")
	assert.Contains(t, resolvedEvent.Message, "GPU-12345678-1234-5678-1234-567812345678")
	assert.NotNil(t, resolvedEvent.ExtraInfo)

	// Verify the event contains JSON data that can be parsed
	jsonData := resolvedEvent.ExtraInfo[EventKeyErrorXidData]
	assert.NotEmpty(t, jsonData)

	// Try with invalid XID
	invalidEvent := eventstore.Event{
		Time: now,
		Name: EventNameErrorXid,
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: "99999", // Invalid XID
			EventKeyDeviceUUID:   "GPU-12345678-1234-5678-1234-567812345678",
		},
	}

	resolvedInvalidEvent := resolveXIDEvent(invalidEvent)
	assert.Equal(t, invalidEvent, resolvedInvalidEvent)
}

func TestDataSummary(t *testing.T) {
	// Test nil data
	var nilData *checkResult
	summary := nilData.Summary()
	assert.Empty(t, summary)

	// Test data with reason
	data := &checkResult{
		reason: "test reason",
		health: apiv1.HealthStateTypeHealthy,
	}
	summary = data.Summary()
	assert.Equal(t, "test reason", summary)
}

func TestHandleEventChannel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	gpudInstance := &components.GPUdInstance{
		RootCtx:          ctx,
		EventStore:       store,
		RebootEventStore: rebootEventStore,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	c := comp.(*component)

	// If eventBucket is nil, create it manually for testing
	if c.eventBucket == nil {
		c.eventBucket, err = store.Bucket(Name)
		assert.NoError(t, err)
	}

	// Set up a fast ticker for testing
	fastTicker := time.NewTicker(100 * time.Millisecond)
	defer fastTicker.Stop()

	// Set up a test kmsg channel
	kmsgCh := make(chan kmsg.Message, 10)

	// Create a context with a short timeout for the test
	testCtx, testCancel := context.WithTimeout(ctx, 1*time.Second)
	defer testCancel()

	// Create a wait group to wait for the go routine to finish
	var wg sync.WaitGroup
	wg.Add(1)

	// Start the component in a goroutine with our test channels
	go func() {
		defer wg.Done()
		// Call start with our test channel and fast ticker period
		c.start(kmsgCh, 100*time.Millisecond)
	}()

	// Send a test XID kmsg
	kmsgCh <- kmsg.Message{
		Timestamp: metav1.NewTime(time.Now()),
		Message:   "NVRM: Xid (PCI:0000:01:00): 31, pid=XXX",
	}

	// Wait a bit to allow processing
	time.Sleep(300 * time.Millisecond)

	// Check that the event was processed by getting events
	events, err := comp.Events(testCtx, time.Now().Add(-1*time.Minute))
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 1)

	// Send a test event directly to the extraEventCh
	c.extraEventCh <- &eventstore.Event{
		Time: time.Now(),
		Name: "SetHealthy",
	}

	// Wait a bit to allow processing
	time.Sleep(300 * time.Millisecond)

	// Cancel the context to stop the goroutine
	testCancel()

	// Wait for the goroutine to finish
	wg.Wait()
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

func TestStartWithXID63And64Skipping(t *testing.T) {
	// initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	gpudInstance := &components.GPUdInstance{
		RootCtx:          ctx,
		EventStore:       store,
		RebootEventStore: rebootEventStore,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	c := comp.(*component)

	// If eventBucket is nil, create it manually for testing
	if c.eventBucket == nil {
		c.eventBucket, err = store.Bucket(Name)
		assert.NoError(t, err)
	}

	// Setup a test channel for events to avoid using kmsg
	kmsgCh := make(chan kmsg.Message, 10)

	// Start the component with a short update period
	go c.start(kmsgCh, 100*time.Millisecond)

	defer func() {
		if err := comp.Close(); err != nil {
			t.Error("failed to close component")
		}
	}()

	// Send various XID messages including 63 and 64
	testMessages := []kmsg.Message{
		{
			Timestamp: metav1.NewTime(time.Now()),
			Message:   "NVRM: Xid (PCI:0000:01:00): 63, Row remapping pending",
		},
		{
			Timestamp: metav1.NewTime(time.Now().Add(1 * time.Second)),
			Message:   "NVRM: Xid (PCI:0000:02:00): 31, GPU has fallen off the bus",
		},
		{
			Timestamp: metav1.NewTime(time.Now().Add(2 * time.Second)),
			Message:   "NVRM: Xid (PCI:0000:03:00): 64, Row remapping failure",
		},
		{
			Timestamp: metav1.NewTime(time.Now().Add(3 * time.Second)),
			Message:   "NVRM: Xid (PCI:0000:04:00): 79, GPU has fallen off the bus",
		},
		{
			Timestamp: metav1.NewTime(time.Now().Add(4 * time.Second)),
			Message:   "NVRM: Xid (PCI:0000:05:00): 63, Row remapping pending again",
		},
	}

	// Send all test messages
	for _, msg := range testMessages {
		kmsgCh <- msg
	}

	// Wait for processing
	time.Sleep(2 * time.Second)

	// Check that only non-63/64 XIDs were processed
	events, err := comp.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)

	// Count events by XID by parsing the message
	xidCounts := make(map[int]int)
	xidRegex := regexp.MustCompile(`XID (\d+)`)
	for _, event := range events {
		if event.Name == EventNameErrorXid {
			if matches := xidRegex.FindStringSubmatch(event.Message); len(matches) > 1 {
				if xid, err := strconv.Atoi(matches[1]); err == nil {
					xidCounts[xid]++
				}
			}
		}
	}

	// Verify that XID 63 and 64 were not stored
	assert.Equal(t, 0, xidCounts[63], "XID 63 should not be stored in events")
	assert.Equal(t, 0, xidCounts[64], "XID 64 should not be stored in events")

	// Verify that other XIDs were stored
	assert.Greater(t, xidCounts[31], 0, "XID 31 should be stored in events")
	assert.Greater(t, xidCounts[79], 0, "XID 79 should be stored in events")

	// Also verify through component state
	states := comp.LastHealthStates()
	assert.NotNil(t, states)

	// The component should be unhealthy due to XID 79 (fatal error)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
}
