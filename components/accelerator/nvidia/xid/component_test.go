package xid

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func createTestEvent(timestamp time.Time) components.Event {
	return components.Event{
		Time:    metav1.Time{Time: timestamp},
		Name:    "test_event",
		Type:    "test_type",
		Message: "test message",
		ExtraInfo: map[string]string{
			"key": "value",
		},
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{common.RepairActionTypeRebootSystem},
		},
	}
}

func TestMergeEvents(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		a        []components.Event
		b        []components.Event
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
			b: []components.Event{
				createTestEvent(now),
			},
			expected: 1,
		},
		{
			name: "b empty",
			a: []components.Event{
				createTestEvent(now),
			},
			b:        nil,
			expected: 1,
		},
		{
			name: "both non-empty",
			a: []components.Event{
				createTestEvent(now.Add(-1 * time.Hour)),
				createTestEvent(now),
			},
			b: []components.Event{
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
		a := []components.Event{
			createTestEvent(now.Add(2 * time.Hour)),
			createTestEvent(now.Add(-1 * time.Hour)),
		}
		b := []components.Event{
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
	component := New(ctx, store)
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

func TestXIDComponent_Events(t *testing.T) {
	t.Parallel()

	// initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	require.NoError(t, err)

	component := New(ctx, store)
	require.NotNil(t, component)

	watcher, err := pkg_dmesg.NewWatcher()
	require.NoError(t, err)

	// Start component in background
	go component.start(watcher, 100*time.Millisecond) // Use shorter interval for testing
	defer func() {
		err := component.Close()
		assert.NoError(t, err, "failed to close component")
	}()

	// Create timestamp for events
	now := time.Now().UTC()

	// Create test events
	testEvents := []components.Event{
		createTestEvent(now),
		createTestEvent(now.Add(1 * time.Minute)),
	}

	// Get bucket directly for testing
	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	// Insert test events directly into the bucket
	for _, event := range testEvents {
		err := bucket.Insert(ctx, event)
		require.NoError(t, err, "failed to insert test event")
	}

	// Test part 1: Verify events are retrievable directly from bucket
	// Note: We don't use component.Events() because it only returns OS reboot events
	bucketEvents, err := bucket.Get(ctx, now.Add(-1*time.Hour))
	require.NoError(t, err)
	require.Len(t, bucketEvents, len(testEvents), "should have same number of events in bucket")

	// Sort and compare events
	sortEvents := func(evts []components.Event) {
		sort.Slice(evts, func(i, j int) bool {
			return evts[i].Time.Time.Before(evts[j].Time.Time)
		})
	}

	sortEvents(bucketEvents)
	sortEvents(testEvents)

	for i, event := range bucketEvents {
		assert.Equal(t, testEvents[i].Time.Time.Unix(), event.Time.Time.Unix())
		assert.Equal(t, testEvents[i].Name, event.Name)
		assert.Equal(t, testEvents[i].Type, event.Type)
		assert.Equal(t, testEvents[i].Message, event.Message)
		assert.Equal(t, testEvents[i].ExtraInfo, event.ExtraInfo)
		assert.Equal(t, testEvents[i].SuggestedActions, event.SuggestedActions)
	}

	// Test part 2: Test extraEventCh functionality
	// Use a test event with a future timestamp
	testChannelEvent := createTestEvent(now.Add(2 * time.Minute))

	// Use a WaitGroup to ensure the event is processed
	var wg sync.WaitGroup
	wg.Add(1)

	// Create a goroutine to check when the event appears in the bucket
	go func() {
		defer wg.Done()

		checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
		defer checkCancel()

		// Poll for the event until it appears or timeout
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			events, err := bucket.Get(checkCtx, testChannelEvent.Time.Time.Add(-1*time.Second))
			if err != nil {
				continue
			}

			for _, e := range events {
				if e.Name == testChannelEvent.Name &&
					e.Message == testChannelEvent.Message &&
					e.Time.Time.Unix() == testChannelEvent.Time.Time.Unix() {
					return // Found the event, exit goroutine
				}
			}

			select {
			case <-ticker.C:
				continue
			case <-checkCtx.Done():
				t.Errorf("timeout waiting for event to be processed")
				return
			}
		}
	}()

	// Send event through the channel
	component.extraEventCh <- &testChannelEvent

	// Wait for event processing to complete
	wg.Wait()
}

func TestXIDComponent_States(t *testing.T) {
	// initialize component
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	store, err := eventstore.New(dbRW, dbRO, DefaultRetentionPeriod)
	assert.NoError(t, err)
	component := New(ctx, store)
	assert.NotNil(t, component)
	watcher, err := pkg_dmesg.NewWatcher()
	assert.NoError(t, err)
	go component.start(watcher, 100*time.Millisecond)
	defer func() {
		if err := component.Close(); err != nil {
			t.Error("failed to close component")
		}
	}()

	s := components.State{
		Name:    StateNameErrorXid,
		Healthy: true,
		Health:  components.StateHealthy,
		Reason:  "XIDComponent is healthy",
	}
	component.currState = s
	states, err := component.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, s, states[0])

	startTime := time.Now().Add(-1 * time.Hour)

	tests := []struct {
		name      string
		events    []components.Event
		wantState []components.State
	}{
		{
			name: "critical xid happened and reboot recovered",
			events: []components.Event{
				createXidEvent(time.Now().Add(-5*24*time.Hour), 31, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
				createXidEvent(startTime, 31, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
				createXidEvent(startTime.Add(5*time.Minute), 94, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
				{Name: "reboot", Time: metav1.Time{Time: startTime.Add(10 * time.Minute)}},
				createXidEvent(startTime.Add(15*time.Minute), 94, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
				{Name: "reboot", Time: metav1.Time{Time: startTime.Add(20 * time.Minute)}},
				createXidEvent(startTime.Add(25*time.Minute), 94, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
			},
			wantState: []components.State{
				{Healthy: true, Health: components.StateHealthy, SuggestedActions: nil},
				{Healthy: false, Health: components.StateDegraded, SuggestedActions: &common.SuggestedActions{RepairActions: []common.RepairActionType{common.RepairActionTypeRebootSystem}}},
				{Healthy: false, Health: components.StateDegraded, SuggestedActions: &common.SuggestedActions{RepairActions: []common.RepairActionType{common.RepairActionTypeRebootSystem}}},
				{Healthy: true, Health: components.StateHealthy, SuggestedActions: nil},
				{Healthy: false, Health: components.StateDegraded, SuggestedActions: &common.SuggestedActions{RepairActions: []common.RepairActionType{common.RepairActionTypeRebootSystem}}},
				{Healthy: true, Health: components.StateHealthy, SuggestedActions: nil},
				{Healthy: false, Health: components.StateDegraded, SuggestedActions: &common.SuggestedActions{RepairActions: []common.RepairActionType{common.RepairActionTypeHardwareInspection}}},
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
				states, err = component.States(ctx)
				t.Log(states[0])
				assert.NoError(t, err, "index %d", i)
				assert.Len(t, states, 1, "index %d", i)
				assert.Equal(t, tt.wantState[i].Healthy, states[0].Healthy, "index %d", i)
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
