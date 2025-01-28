package hwslowdown

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/components/db"
	events_db "github.com/leptonai/gpud/components/db"
	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponentStates(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	testCases := []struct {
		name               string
		window             time.Duration
		thresholdPerMinute float64
		insertedEvent      []components.Event
		expectedStates     int
		expectHealthy      bool
	}{
		{
			name:               "single event within window",
			window:             10 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				{
					Time:    metav1.Time{Time: now.Add(-5 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  true,
		},
		{
			name:               "multiple events within window but below threshold",
			window:             10 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				{
					Time:    metav1.Time{Time: now.Add(-5 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  true,
		},
		{
			name:               "events above threshold",
			window:             5 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  false,
		},
		{
			name:               "events above threshold with multiple GPUs",
			window:             5 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				// GPU 0-3 events at -4 minutes
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-1",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-2",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-3",
					},
				},
				// GPU 0-3 events at -3 minutes
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-1",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-2",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-3",
					},
				},
				// GPU 0-3 events at -2 minutes
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-1",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-2",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-3",
					},
				},
				// GPU 0-3 events at -1 minutes
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-1",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-2",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-3",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  false,
		},
		{
			name:               "events outside window",
			window:             5 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				{
					Time:    metav1.Time{Time: now.Add(-10 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-8 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  true,
		},
		{
			name:               "no events",
			window:             10 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent:      []components.Event{},
			expectedStates:     1,
			expectHealthy:      true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			store, err := db.NewStore(dbRW, dbRO, "test_events", 0)
			assert.NoError(t, err)
			defer store.Close()

			if len(tc.insertedEvent) > 0 {
				for _, event := range tc.insertedEvent {
					err := store.Insert(ctx, event)
					assert.NoError(t, err)
				}
			}

			c := &component{
				stateHWSlowdownEvaluationWindow:                  tc.window,
				stateHWSlowdownEventsThresholdFrequencyPerMinute: tc.thresholdPerMinute,
				eventsStore: store,
			}

			states, err := c.States(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStates, len(states))

			if len(states) > 0 {
				assert.Equal(t, tc.expectHealthy, states[0].Healthy)
			}
		})
	}
}

func TestComponentRegisterCollectors(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	reg := prometheus.NewRegistry()
	c := &component{}

	err := c.RegisterCollectors(reg, dbRW, dbRO, "test_metrics")
	assert.NoError(t, err)
	assert.Equal(t, reg, c.gatherer)
}

func TestComponentStatesEdgeCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		window             time.Duration
		thresholdPerMinute float64
		setupStore         func(store events_db.Store, ctx context.Context) error
		expectError        bool
		expectedStates     int
		expectHealthy      bool
	}{
		{
			name:               "zero evaluation window",
			window:             0,
			thresholdPerMinute: 0.6,
			setupStore:         func(store events_db.Store, ctx context.Context) error { return nil },
			expectError:        false,
			expectedStates:     1,
			expectHealthy:      true,
		},
		{
			name:               "negative evaluation window",
			window:             -10 * time.Minute,
			thresholdPerMinute: 0.6,
			setupStore:         func(store events_db.Store, ctx context.Context) error { return nil },
			expectError:        false,
			expectedStates:     1,
			expectHealthy:      true,
		},
		{
			name:               "zero threshold",
			window:             10 * time.Minute,
			thresholdPerMinute: 0,
			setupStore: func(store events_db.Store, ctx context.Context) error {
				event := components.Event{
					Time:    metav1.Time{Time: time.Now().UTC().Add(-5 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				}
				return store.Insert(ctx, event)
			},
			expectError:    false,
			expectedStates: 1,
			expectHealthy:  false,
		},
		{
			name:               "negative threshold",
			window:             10 * time.Minute,
			thresholdPerMinute: -0.6,
			setupStore: func(store events_db.Store, ctx context.Context) error {
				event := components.Event{
					Time:    metav1.Time{Time: time.Now().UTC().Add(-5 * time.Minute)},
					Name:    EventNameHWSlowdown,
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						EventKeyGPUUUID: "gpu-0",
					},
				}
				return store.Insert(ctx, event)
			},
			expectError:    false,
			expectedStates: 1,
			expectHealthy:  false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			store, err := db.NewStore(dbRW, dbRO, "test_events", 0)
			assert.NoError(t, err)
			defer store.Close()

			err = tc.setupStore(store, ctx)
			assert.NoError(t, err)

			c := &component{
				stateHWSlowdownEvaluationWindow:                  tc.window,
				stateHWSlowdownEventsThresholdFrequencyPerMinute: tc.thresholdPerMinute,
				eventsStore: store,
			}

			states, err := c.States(ctx)
			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStates, len(states))
			if len(states) > 0 {
				assert.Equal(t, tc.expectHealthy, states[0].Healthy)
			}
		})
	}
}
