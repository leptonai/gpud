package sxid

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// createTestEvent creates a test event with the specified timestamp
func createTestEvent(timestamp time.Time) apiv1.Event {
	return apiv1.Event{
		Time:    metav1.Time{Time: timestamp},
		Name:    "test_event",
		Type:    "test_type",
		Message: "test message",
		DeprecatedExtraInfo: map[string]string{
			"key": "value",
		},
		DeprecatedSuggestedActions: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
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
		a        apiv1.Events
		b        apiv1.Events
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
			b: apiv1.Events{
				createTestEvent(now),
			},
			expected: 1,
		},
		{
			name: "b empty",
			a: apiv1.Events{
				createTestEvent(now),
			},
			b:        nil,
			expected: 1,
		},
		{
			name: "both non-empty",
			a: apiv1.Events{
				createTestEvent(now.Add(-1 * time.Hour)),
				createTestEvent(now),
			},
			b: apiv1.Events{
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
					assert.True(t, result[i-1].Time.Time.After(result[i].Time.Time) ||
						result[i-1].Time.Time.Equal(result[i].Time.Time),
						"events should be sorted by timestamp")
				}
			}
		})
	}

	t.Run("verify sorting", func(t *testing.T) {
		a := apiv1.Events{
			createTestEvent(now.Add(2 * time.Hour)),
			createTestEvent(now.Add(-1 * time.Hour)),
		}
		b := apiv1.Events{
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

	testEvents := apiv1.Events{
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
				assert.Equal(t, testEvents[0].Type, event.Type)
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
		Health: apiv1.StateTypeHealthy,
		Reason: "SXIDComponent is healthy",
	}
	component.currState = s
	states := component.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, s, states[0])

	startTime := time.Now().Add(-1 * time.Hour)

	tests := []struct {
		name      string
		events    apiv1.Events
		wantState []apiv1.HealthState
	}{
		{
			name: "critical sxid happened and reboot recovered",
			events: apiv1.Events{
				createSXidEvent(time.Now().Add(-5*24*time.Hour), 31, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
				createSXidEvent(startTime, 31, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
				createSXidEvent(startTime.Add(5*time.Minute), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
				{Name: "reboot", Time: metav1.Time{Time: startTime.Add(10 * time.Minute)}},
				createSXidEvent(startTime.Add(15*time.Minute), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
				{Name: "reboot", Time: metav1.Time{Time: startTime.Add(20 * time.Minute)}},
				createSXidEvent(startTime.Add(25*time.Minute), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			},
			wantState: []apiv1.HealthState{
				{Health: apiv1.StateTypeHealthy, SuggestedActions: nil},
				{Health: apiv1.StateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{Health: apiv1.StateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{Health: apiv1.StateTypeHealthy, SuggestedActions: nil},
				{Health: apiv1.StateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{Health: apiv1.StateTypeHealthy, SuggestedActions: nil},
				{Health: apiv1.StateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}}},
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
