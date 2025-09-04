package gpucounts

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
)

// TestComponent_Events_WithSetHealthy tests that the Events method properly filters and returns SetHealthy events
func TestComponent_Events_WithSetHealthy(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	testCases := []struct {
		name           string
		bucketEvents   []eventstore.Event
		since          time.Time
		expectedCount  int
		expectedEvents []string
	}{
		{
			name: "only SetHealthy events",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Component marked as healthy",
				},
				{
					Component: Name,
					Time:      now.Add(-1 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Component marked as healthy again",
				},
			},
			since:          now.Add(-3 * time.Hour),
			expectedCount:  2,
			expectedEvents: []string{"SetHealthy", "SetHealthy"},
		},
		{
			name: "mixed events - only SetHealthy returned",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-3 * time.Hour),
					Name:      EventNameMisMatch,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "GPU count mismatch",
				},
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Component marked as healthy",
				},
				{
					Component: Name,
					Time:      now.Add(-1 * time.Hour),
					Name:      "OtherEvent",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Some other event",
				},
			},
			since:          now.Add(-4 * time.Hour),
			expectedCount:  1,
			expectedEvents: []string{"SetHealthy"},
		},
		{
			name: "no SetHealthy events",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      EventNameMisMatch,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "GPU count mismatch",
				},
				{
					Component: Name,
					Time:      now.Add(-1 * time.Hour),
					Name:      "OtherEvent",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Some other event",
				},
			},
			since:         now.Add(-3 * time.Hour),
			expectedCount: 0,
		},
		{
			name: "SetHealthy events filtered by time",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-5 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Old SetHealthy event",
				},
				{
					Component: Name,
					Time:      now.Add(-30 * time.Minute),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Recent SetHealthy event",
				},
			},
			since:          now.Add(-1 * time.Hour),
			expectedCount:  1,
			expectedEvents: []string{"SetHealthy"},
		},
		{
			name:          "empty event bucket",
			bucketEvents:  []eventstore.Event{},
			since:         now.Add(-1 * time.Hour),
			expectedCount: 0,
		},
		{
			name: "multiple SetHealthy events with other events",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-6 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "First SetHealthy",
				},
				{
					Component: Name,
					Time:      now.Add(-5 * time.Hour),
					Name:      EventNameMisMatch,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "GPU mismatch",
				},
				{
					Component: Name,
					Time:      now.Add(-4 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Second SetHealthy",
				},
				{
					Component: Name,
					Time:      now.Add(-3 * time.Hour),
					Name:      "RandomEvent",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Random",
				},
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Third SetHealthy",
				},
				{
					Component: Name,
					Time:      now.Add(-1 * time.Hour),
					Name:      EventNameMisMatch,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "Another GPU mismatch",
				},
			},
			since:          now.Add(-7 * time.Hour),
			expectedCount:  3,
			expectedEvents: []string{"SetHealthy", "SetHealthy", "SetHealthy"},
		},
		{
			name: "SetHealthy event exactly at since time",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "SetHealthy at boundary",
				},
			},
			since:          now.Add(-2 * time.Hour),
			expectedCount:  1,
			expectedEvents: []string{"SetHealthy"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockBucket := &mockEventBucket{
				name:   Name,
				events: tc.bucketEvents,
			}

			c := &component{
				ctx:         ctx,
				eventBucket: mockBucket,
			}

			events, err := c.Events(ctx, tc.since)
			require.NoError(t, err)

			if tc.expectedCount == 0 {
				assert.Nil(t, events)
			} else {
				require.NotNil(t, events)
				assert.Len(t, events, tc.expectedCount)

				// Verify all returned events are SetHealthy events
				for i, ev := range events {
					assert.Equal(t, tc.expectedEvents[i], ev.Name)
					assert.Equal(t, Name, ev.Component)
				}
			}
		})
	}
}

// TestComponent_Events_WithError tests error handling in the Events method
func TestComponent_Events_WithError(t *testing.T) {
	ctx := context.Background()

	t.Run("bucket Get error", func(t *testing.T) {
		mockBucket := &mockEventBucket{
			name:   Name,
			getErr: assert.AnError,
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
		}

		events, err := c.Events(ctx, time.Now().Add(-1*time.Hour))
		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("nil event bucket", func(t *testing.T) {
		c := &component{
			ctx:         ctx,
			eventBucket: nil,
		}

		events, err := c.Events(ctx, time.Now().Add(-1*time.Hour))
		assert.NoError(t, err)
		assert.Nil(t, events)
	})

	t.Run("context cancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		mockBucket := &mockEventBucket{
			name: Name,
			events: []eventstore.Event{
				{
					Component: Name,
					Time:      time.Now().Add(-1 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Test event",
				},
			},
		}

		c := &component{
			ctx:         cancelCtx,
			eventBucket: mockBucket,
		}

		// Even with cancelled context, the Events method should still work
		// as it uses the passed context, not the component's context
		events, err := c.Events(context.Background(), time.Now().Add(-2*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})
}

// TestComponent_Events_Integration tests the Events method in an integration scenario
func TestComponent_Events_Integration(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	// Simulate a realistic sequence of events
	bucketEvents := []eventstore.Event{
		// Initial GPU mismatch
		{
			Component: Name,
			Time:      now.Add(-24 * time.Hour),
			Name:      EventNameMisMatch,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "nvidia gpu count mismatch (found 2, expected 4)",
		},
		// Admin sets healthy after inspection
		{
			Component: Name,
			Time:      now.Add(-20 * time.Hour),
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "Component marked as healthy by admin",
		},
		// Another mismatch occurs
		{
			Component: Name,
			Time:      now.Add(-10 * time.Hour),
			Name:      EventNameMisMatch,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "nvidia gpu count mismatch (found 3, expected 4)",
		},
		// Admin sets healthy again
		{
			Component: Name,
			Time:      now.Add(-5 * time.Hour),
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "Component marked as healthy after hardware replacement",
		},
		// System stable, periodic check event
		{
			Component: Name,
			Time:      now.Add(-1 * time.Hour),
			Name:      "PeriodicCheck",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "Periodic health check passed",
		},
	}

	mockBucket := &mockEventBucket{
		name:   Name,
		events: bucketEvents,
	}

	c := &component{
		ctx:         ctx,
		eventBucket: mockBucket,
	}

	// Test 1: Get events from the last 25 hours (should get only SetHealthy events)
	events, err := c.Events(ctx, now.Add(-25*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 2, "Should have 2 SetHealthy events")

	for _, ev := range events {
		assert.Equal(t, "SetHealthy", ev.Name)
		assert.Equal(t, Name, ev.Component)
		assert.Equal(t, apiv1.EventTypeInfo, ev.Type)
	}

	// Test 2: Get events from the last 6 hours (should get 1 SetHealthy event)
	events, err = c.Events(ctx, now.Add(-6*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 1, "Should have 1 SetHealthy event")
	assert.Equal(t, "SetHealthy", events[0].Name)

	// Test 3: Get events from the last 30 minutes (no SetHealthy events)
	events, err = c.Events(ctx, now.Add(-30*time.Minute))
	require.NoError(t, err)
	assert.Nil(t, events, "Should have no SetHealthy events in the last 30 minutes")
}

// TestComponent_Events_Ordering tests that SetHealthy events maintain their order
func TestComponent_Events_Ordering(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	bucketEvents := []eventstore.Event{
		{
			Component: Name,
			Time:      now.Add(-5 * time.Hour),
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "First SetHealthy",
		},
		{
			Component: Name,
			Time:      now.Add(-3 * time.Hour),
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "Second SetHealthy",
		},
		{
			Component: Name,
			Time:      now.Add(-1 * time.Hour),
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "Third SetHealthy",
		},
	}

	mockBucket := &mockEventBucket{
		name:   Name,
		events: bucketEvents,
	}

	c := &component{
		ctx:         ctx,
		eventBucket: mockBucket,
	}

	events, err := c.Events(ctx, now.Add(-6*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 3)

	// Verify events are in chronological order
	assert.Equal(t, "First SetHealthy", events[0].Message)
	assert.Equal(t, "Second SetHealthy", events[1].Message)
	assert.Equal(t, "Third SetHealthy", events[2].Message)

	// Verify timestamps are in order
	for i := 1; i < len(events); i++ {
		assert.True(t, events[i].Time.Time.After(events[i-1].Time.Time),
			"Events should be in chronological order")
	}
}
