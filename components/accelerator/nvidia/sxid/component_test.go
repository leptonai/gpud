package sxid

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/sqlite"
)

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
	// initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	component := New(ctx, rebootEventStore, store)
	assert.NotNil(t, component)
	err = component.SetHealthy()
	assert.NoError(t, err)

	select {
	case event := <-component.extraEventCh:
		assert.Equal(t, "SetHealthy", event.Name)
	default:
		t.Error("expected event in channel but got none")
	}
}

func TestSXIDComponent_Events(t *testing.T) {
	// initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	component := New(ctx, rebootEventStore, store)
	assert.NotNil(t, component)
	go component.start(make(<-chan kmsg.Message, 1), 1*time.Second)
	defer func() {
		if err := component.Close(); err != nil {
			t.Error("failed to close component")
		}
	}()

	testEvents := apiv1.Events{
		createTestEvent(time.Now()),
	}

	// insert test events
	for _, event := range testEvents {
		select {
		case component.extraEventCh <- &event:
		default:
			t.Error("failed to insert event into channel")
		}
	}

	// wait for events to be processed
	time.Sleep(5 * time.Second)

	events, err := component.Events(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Len(t, events, len(testEvents))
	for i, event := range events {
		assert.Equal(t, testEvents[i].Time.Time.Unix(), event.Time.Time.Unix())
		assert.Equal(t, testEvents[i].Name, event.Name)
		assert.Equal(t, testEvents[i].Type, event.Type)
		assert.Equal(t, testEvents[i].Message, event.Message)
		assert.Equal(t, testEvents[i].DeprecatedExtraInfo, event.DeprecatedExtraInfo)
		assert.Equal(t, testEvents[i].DeprecatedSuggestedActions, event.DeprecatedSuggestedActions)
	}
}

func TestSXIDComponent_States(t *testing.T) {
	// initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)

	rebootEventStore := pkghost.NewRebootEventStore(store)

	component := New(ctx, rebootEventStore, store)

	assert.NotNil(t, component)
	go component.start(make(<-chan kmsg.Message, 1), 100*time.Millisecond)
	defer func() {
		if err := component.Close(); err != nil {
			t.Error("failed to close component")
		}
	}()

	s := apiv1.HealthState{
		Name:              StateNameErrorSXid,
		DeprecatedHealthy: true,
		Health:            apiv1.StateTypeHealthy,
		Reason:            "SXIDComponent is healthy",
	}
	component.currState = s
	states, err := component.HealthStates(ctx)
	assert.NoError(t, err)
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
				{DeprecatedHealthy: true, Health: apiv1.StateTypeHealthy, SuggestedActions: nil},
				{DeprecatedHealthy: false, Health: apiv1.StateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{DeprecatedHealthy: false, Health: apiv1.StateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{DeprecatedHealthy: true, Health: apiv1.StateTypeHealthy, SuggestedActions: nil},
				{DeprecatedHealthy: false, Health: apiv1.StateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}}},
				{DeprecatedHealthy: true, Health: apiv1.StateTypeHealthy, SuggestedActions: nil},
				{DeprecatedHealthy: false, Health: apiv1.StateTypeUnhealthy, SuggestedActions: &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// insert test events
			for i, event := range tt.events {
				select {
				case component.extraEventCh <- &event:
				default:
					t.Error("failed to insert event into channel")
				}
				// wait for events to be processed
				time.Sleep(1 * time.Second)
				states, err = component.HealthStates(ctx)
				t.Log(states[0])
				assert.NoError(t, err, "index %d", i)
				assert.Len(t, states, 1, "index %d", i)
				assert.Equal(t, tt.wantState[i].DeprecatedHealthy, states[0].DeprecatedHealthy, "index %d", i)
				assert.Equal(t, tt.wantState[i].Health, states[0].Health, "index %d", i)
				if tt.wantState[i].SuggestedActions == nil {
					assert.Equal(t, tt.wantState[i].SuggestedActions, states[0].SuggestedActions, "index %d", i)
				}
				if tt.wantState[i].SuggestedActions != nil && states[0].SuggestedActions != nil {
					assert.Equal(t, tt.wantState[i].SuggestedActions.RepairActions, states[0].SuggestedActions.RepairActions, "index %d", i)
				}
			}
			err = component.SetHealthy()
			assert.NoError(t, err)
			// wait for events to be processed
			time.Sleep(1 * time.Second)
		})
	}
}
