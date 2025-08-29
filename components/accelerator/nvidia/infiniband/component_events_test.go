package infiniband

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
)

// testEventBucket is a test-specific mock for the Events tests
type testEventBucket struct {
	events eventstore.Events
	getErr error
}

func (t *testEventBucket) Name() string {
	return "test-bucket"
}

func (t *testEventBucket) Insert(ctx context.Context, ev eventstore.Event) error {
	t.events = append(t.events, ev)
	return nil
}

func (t *testEventBucket) Find(ctx context.Context, ev eventstore.Event) (*eventstore.Event, error) {
	return nil, nil
}

func (t *testEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if t.getErr != nil {
		return nil, t.getErr
	}
	var result eventstore.Events
	for _, ev := range t.events {
		if ev.Time.After(since) || ev.Time.Equal(since) {
			result = append(result, ev)
		}
	}
	return result, nil
}

func (t *testEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	if len(t.events) == 0 {
		return nil, nil
	}
	return &t.events[len(t.events)-1], nil
}

func (t *testEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return 0, nil
}

func (t *testEventBucket) Close() {}

// TestComponent_Events_FilteredEvents tests that the Events method properly filters events
func TestComponent_Events_FilteredEvents(t *testing.T) {
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
			name: "only filtered events returned",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-3 * time.Hour),
					Name:      eventPCIPowerInsufficient,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "PCI power insufficient",
				},
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      eventPortModuleHighTemperature,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "Port module temperature high",
				},
				{
					Component: Name,
					Time:      now.Add(-1 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Component marked as healthy",
				},
			},
			since:          now.Add(-4 * time.Hour),
			expectedCount:  3,
			expectedEvents: []string{eventPCIPowerInsufficient, eventPortModuleHighTemperature, "SetHealthy"},
		},
		{
			name: "mixed events - only specific ones returned",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-5 * time.Hour),
					Name:      "UnfilteredEvent1",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Some unfiltered event",
				},
				{
					Component: Name,
					Time:      now.Add(-4 * time.Hour),
					Name:      eventPCIPowerInsufficient,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "PCI power issue",
				},
				{
					Component: Name,
					Time:      now.Add(-3 * time.Hour),
					Name:      "UnfilteredEvent2",
					Type:      string(apiv1.EventTypeWarning),
					Message:   "Another unfiltered event",
				},
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Marked healthy",
				},
				{
					Component: Name,
					Time:      now.Add(-1 * time.Hour),
					Name:      "RandomEvent",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Random event",
				},
			},
			since:          now.Add(-6 * time.Hour),
			expectedCount:  2,
			expectedEvents: []string{eventPCIPowerInsufficient, "SetHealthy"},
		},
		{
			name: "no filtered events present",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      "UnfilteredEvent1",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Event 1",
				},
				{
					Component: Name,
					Time:      now.Add(-1 * time.Hour),
					Name:      "UnfilteredEvent2",
					Type:      string(apiv1.EventTypeWarning),
					Message:   "Event 2",
				},
			},
			since:         now.Add(-3 * time.Hour),
			expectedCount: 0,
		},
		{
			name: "all three types of filtered events",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-6 * time.Hour),
					Name:      eventPCIPowerInsufficient,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "PCI power issue 1",
				},
				{
					Component: Name,
					Time:      now.Add(-5 * time.Hour),
					Name:      eventPortModuleHighTemperature,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "Temperature issue 1",
				},
				{
					Component: Name,
					Time:      now.Add(-4 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "First SetHealthy",
				},
				{
					Component: Name,
					Time:      now.Add(-3 * time.Hour),
					Name:      eventPCIPowerInsufficient,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "PCI power issue 2",
				},
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      eventPortModuleHighTemperature,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "Temperature issue 2",
				},
				{
					Component: Name,
					Time:      now.Add(-1 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Second SetHealthy",
				},
			},
			since:         now.Add(-7 * time.Hour),
			expectedCount: 6,
			expectedEvents: []string{
				eventPCIPowerInsufficient,
				eventPortModuleHighTemperature,
				"SetHealthy",
				eventPCIPowerInsufficient,
				eventPortModuleHighTemperature,
				"SetHealthy",
			},
		},
		{
			name:          "empty event bucket",
			bucketEvents:  []eventstore.Event{},
			since:         now.Add(-1 * time.Hour),
			expectedCount: 0,
		},
		{
			name: "events filtered by time",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-5 * time.Hour),
					Name:      eventPCIPowerInsufficient,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "Old PCI power issue",
				},
				{
					Component: Name,
					Time:      now.Add(-30 * time.Minute),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Recent SetHealthy",
				},
			},
			since:          now.Add(-1 * time.Hour),
			expectedCount:  1,
			expectedEvents: []string{"SetHealthy"},
		},
		{
			name: "multiple SetHealthy events",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-4 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "SetHealthy 1",
				},
				{
					Component: Name,
					Time:      now.Add(-3 * time.Hour),
					Name:      "UnfilteredEvent",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "Unfiltered",
				},
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "SetHealthy 2",
				},
				{
					Component: Name,
					Time:      now.Add(-1 * time.Hour),
					Name:      "SetHealthy",
					Type:      string(apiv1.EventTypeInfo),
					Message:   "SetHealthy 3",
				},
			},
			since:          now.Add(-5 * time.Hour),
			expectedCount:  3,
			expectedEvents: []string{"SetHealthy", "SetHealthy", "SetHealthy"},
		},
		{
			name: "event exactly at since time",
			bucketEvents: []eventstore.Event{
				{
					Component: Name,
					Time:      now.Add(-2 * time.Hour),
					Name:      eventPortModuleHighTemperature,
					Type:      string(apiv1.EventTypeWarning),
					Message:   "Temperature at boundary",
				},
			},
			since:          now.Add(-2 * time.Hour),
			expectedCount:  1,
			expectedEvents: []string{eventPortModuleHighTemperature},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockBucket := &testEventBucket{
				events: tc.bucketEvents,
			}

			c := &component{
				ctx:         ctx,
				eventBucket: mockBucket,
			}

			events, err := c.Events(ctx, tc.since)
			require.NoError(t, err)

			if tc.expectedCount == 0 {
				assert.Empty(t, events)
			} else {
				require.NotNil(t, events)
				assert.Len(t, events, tc.expectedCount)

				// Verify the returned events match expected
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
		mockBucket := &testEventBucket{
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

		mockBucket := &testEventBucket{
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

		// The Events method uses the passed context, not the component's context
		events, err := c.Events(context.Background(), time.Now().Add(-2*time.Hour))
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})
}

// TestComponent_Events_Integration tests the Events method in an integration scenario
func TestComponent_Events_Integration(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	// Simulate a realistic sequence of InfiniBand events
	bucketEvents := []eventstore.Event{
		// Initial PCI power issue
		{
			Component: Name,
			Time:      now.Add(-24 * time.Hour),
			Name:      eventPCIPowerInsufficient,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "mlx5_core 0000:07:00.0: PCI power is not sufficient",
		},
		// IB port flap event (internal, not returned)
		{
			Component: Name,
			Time:      now.Add(-20 * time.Hour),
			Name:      "IbPortFlap",
			Type:      string(apiv1.EventTypeWarning),
			Message:   "InfiniBand port flapping detected",
		},
		// Admin sets healthy after inspection
		{
			Component: Name,
			Time:      now.Add(-18 * time.Hour),
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "Component marked as healthy after power adjustment",
		},
		// Temperature issue
		{
			Component: Name,
			Time:      now.Add(-10 * time.Hour),
			Name:      eventPortModuleHighTemperature,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "mlx5_core 0000:07:00.0: Port module temperature too high",
		},
		// IB port drop event (internal, not returned)
		{
			Component: Name,
			Time:      now.Add(-8 * time.Hour),
			Name:      "IbPortDrop",
			Type:      string(apiv1.EventTypeWarning),
			Message:   "InfiniBand port dropped",
		},
		// Admin sets healthy again
		{
			Component: Name,
			Time:      now.Add(-5 * time.Hour),
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "Component marked as healthy after cooling adjustment",
		},
		// Another PCI power issue
		{
			Component: Name,
			Time:      now.Add(-2 * time.Hour),
			Name:      eventPCIPowerInsufficient,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "mlx5_core 0000:08:00.0: PCI power is not sufficient",
		},
		// Periodic check (internal, not returned)
		{
			Component: Name,
			Time:      now.Add(-1 * time.Hour),
			Name:      "PeriodicCheck",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "Periodic health check",
		},
	}

	mockBucket := &testEventBucket{
		events: bucketEvents,
	}

	c := &component{
		ctx:         ctx,
		eventBucket: mockBucket,
	}

	// Test 1: Get events from the last 25 hours (should get filtered events only)
	events, err := c.Events(ctx, now.Add(-25*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 5, "Should have 5 filtered events")

	expectedNames := []string{
		eventPCIPowerInsufficient,
		"SetHealthy",
		eventPortModuleHighTemperature,
		"SetHealthy",
		eventPCIPowerInsufficient,
	}
	for i, ev := range events {
		assert.Equal(t, expectedNames[i], ev.Name)
		assert.Equal(t, Name, ev.Component)
	}

	// Test 2: Get events from the last 6 hours (should get 2 events)
	events, err = c.Events(ctx, now.Add(-6*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 2, "Should have 2 filtered events")
	assert.Equal(t, "SetHealthy", events[0].Name)
	assert.Equal(t, eventPCIPowerInsufficient, events[1].Name)

	// Test 3: Get events from the last 30 minutes (no filtered events)
	events, err = c.Events(ctx, now.Add(-30*time.Minute))
	require.NoError(t, err)
	assert.Empty(t, events, "Should have no filtered events in the last 30 minutes")
}

// TestComponent_Events_Ordering tests that filtered events maintain their order
func TestComponent_Events_Ordering(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	bucketEvents := []eventstore.Event{
		{
			Component: Name,
			Time:      now.Add(-5 * time.Hour),
			Name:      eventPCIPowerInsufficient,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "First PCI power issue",
		},
		{
			Component: Name,
			Time:      now.Add(-4 * time.Hour),
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "First SetHealthy",
		},
		{
			Component: Name,
			Time:      now.Add(-3 * time.Hour),
			Name:      eventPortModuleHighTemperature,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "Temperature issue",
		},
		{
			Component: Name,
			Time:      now.Add(-2 * time.Hour),
			Name:      "SetHealthy",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "Second SetHealthy",
		},
		{
			Component: Name,
			Time:      now.Add(-1 * time.Hour),
			Name:      eventPCIPowerInsufficient,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "Second PCI power issue",
		},
	}

	mockBucket := &testEventBucket{
		events: bucketEvents,
	}

	c := &component{
		ctx:         ctx,
		eventBucket: mockBucket,
	}

	events, err := c.Events(ctx, now.Add(-6*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 5)

	// Verify events are in chronological order
	expectedMessages := []string{
		"First PCI power issue",
		"First SetHealthy",
		"Temperature issue",
		"Second SetHealthy",
		"Second PCI power issue",
	}
	for i, ev := range events {
		assert.Equal(t, expectedMessages[i], ev.Message)
	}

	// Verify timestamps are in order
	for i := 1; i < len(events); i++ {
		assert.True(t, events[i].Time.Time.After(events[i-1].Time.Time),
			"Events should be in chronological order")
	}
}

// TestComponent_Events_OnlyFilteredTypes tests that only the specific event types are returned
func TestComponent_Events_OnlyFilteredTypes(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	// Create events with various names to ensure only specific ones are filtered
	bucketEvents := []eventstore.Event{
		// These should be included
		{Component: Name, Time: now.Add(-10 * time.Hour), Name: eventPCIPowerInsufficient, Type: string(apiv1.EventTypeWarning), Message: "Include 1"},
		{Component: Name, Time: now.Add(-9 * time.Hour), Name: eventPortModuleHighTemperature, Type: string(apiv1.EventTypeWarning), Message: "Include 2"},
		{Component: Name, Time: now.Add(-8 * time.Hour), Name: "SetHealthy", Type: string(apiv1.EventTypeInfo), Message: "Include 3"},
		// These should NOT be included
		{Component: Name, Time: now.Add(-7 * time.Hour), Name: "IbPortFlap", Type: string(apiv1.EventTypeWarning), Message: "Exclude 1"},
		{Component: Name, Time: now.Add(-6 * time.Hour), Name: "IbPortDrop", Type: string(apiv1.EventTypeWarning), Message: "Exclude 2"},
		{Component: Name, Time: now.Add(-5 * time.Hour), Name: "SomeOtherEvent", Type: string(apiv1.EventTypeInfo), Message: "Exclude 3"},
		{Component: Name, Time: now.Add(-4 * time.Hour), Name: "PeriodicCheck", Type: string(apiv1.EventTypeInfo), Message: "Exclude 4"},
		// More events to include
		{Component: Name, Time: now.Add(-3 * time.Hour), Name: eventPCIPowerInsufficient, Type: string(apiv1.EventTypeWarning), Message: "Include 4"},
		{Component: Name, Time: now.Add(-2 * time.Hour), Name: "SetHealthy", Type: string(apiv1.EventTypeInfo), Message: "Include 5"},
		// Case sensitivity check (should not match)
		{Component: Name, Time: now.Add(-1 * time.Hour), Name: "sethealthy", Type: string(apiv1.EventTypeInfo), Message: "Exclude lowercase"},
	}

	mockBucket := &testEventBucket{
		events: bucketEvents,
	}

	c := &component{
		ctx:         ctx,
		eventBucket: mockBucket,
	}

	events, err := c.Events(ctx, now.Add(-11*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 5, "Should have exactly 5 filtered events")

	// Verify only the correct event types are included
	expectedMessages := []string{"Include 1", "Include 2", "Include 3", "Include 4", "Include 5"}
	for i, ev := range events {
		assert.Equal(t, expectedMessages[i], ev.Message)
		// Verify the event name is one of the allowed types
		assert.True(t,
			ev.Name == eventPCIPowerInsufficient ||
				ev.Name == eventPortModuleHighTemperature ||
				ev.Name == "SetHealthy",
			"Event name should be one of the filtered types")
	}
}
