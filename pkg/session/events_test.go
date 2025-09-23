package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestSession_getEvents(t *testing.T) {
	t.Run("mismatch method returns error", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		ctx := context.Background()
		payload := Request{
			Method: "not_events",
		}

		result, err := session.getEvents(ctx, payload)

		assert.Error(t, err)
		assert.Equal(t, "mismatch method", err.Error())
		assert.Nil(t, result)
	})

	t.Run("uses default components when none specified", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		session.components = []string{"comp1", "comp2"}

		ctx := context.Background()
		comp1 := new(mockComponent)
		comp2 := new(mockComponent)

		events1 := apiv1.Events{{Name: "event1", Message: "message1"}}
		events2 := apiv1.Events{{Name: "event2", Message: "message2"}}

		registry.On("Get", "comp1").Return(comp1)
		registry.On("Get", "comp2").Return(comp2)
		comp1.On("Events", mock.Anything, mock.Anything).Return(events1, nil)
		comp2.On("Events", mock.Anything, mock.Anything).Return(events2, nil)

		payload := Request{
			Method: "events",
		}

		result, err := session.getEvents(ctx, payload)

		assert.NoError(t, err)
		assert.Len(t, result, 2)

		registry.AssertExpectations(t)
		comp1.AssertExpectations(t)
		comp2.AssertExpectations(t)
	})

	t.Run("uses specified components", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)
		session.components = []string{"default1", "default2"}

		ctx := context.Background()
		comp := new(mockComponent)
		events := apiv1.Events{{Name: "event", Message: "message"}}

		registry.On("Get", "specified").Return(comp)
		comp.On("Events", mock.Anything, mock.Anything).Return(events, nil)

		payload := Request{
			Method:     "events",
			Components: []string{"specified"},
		}

		result, err := session.getEvents(ctx, payload)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "specified", result[0].Component)

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})

	t.Run("uses custom time range", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		ctx := context.Background()
		startTime := time.Now().Add(-time.Hour)
		endTime := time.Now()

		comp := new(mockComponent)
		events := apiv1.Events{{Name: "event", Message: "message"}}

		registry.On("Get", "comp").Return(comp)
		comp.On("Events", mock.Anything, startTime).Return(events, nil)

		payload := Request{
			Method:     "events",
			Components: []string{"comp"},
			StartTime:  startTime,
			EndTime:    endTime,
		}

		result, err := session.getEvents(ctx, payload)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, startTime, result[0].StartTime)
		assert.Equal(t, endTime, result[0].EndTime)

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})

	t.Run("handles multiple components concurrently", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		ctx := context.Background()
		comp1 := new(mockComponent)
		comp2 := new(mockComponent)
		comp3 := new(mockComponent)

		events1 := apiv1.Events{{Name: "event1"}}
		events2 := apiv1.Events{{Name: "event2"}}
		events3 := apiv1.Events{{Name: "event3"}}

		registry.On("Get", "comp1").Return(comp1)
		registry.On("Get", "comp2").Return(comp2)
		registry.On("Get", "comp3").Return(comp3)

		comp1.On("Events", mock.Anything, mock.Anything).Return(events1, nil)
		comp2.On("Events", mock.Anything, mock.Anything).Return(events2, nil)
		comp3.On("Events", mock.Anything, mock.Anything).Return(events3, nil)

		payload := Request{
			Method:     "events",
			Components: []string{"comp1", "comp2", "comp3"},
		}

		result, err := session.getEvents(ctx, payload)

		assert.NoError(t, err)
		assert.Len(t, result, 3)

		// Verify all components are present (order not guaranteed)
		componentNames := make(map[string]bool)
		for _, event := range result {
			componentNames[event.Component] = true
		}
		assert.True(t, componentNames["comp1"])
		assert.True(t, componentNames["comp2"])
		assert.True(t, componentNames["comp3"])

		registry.AssertExpectations(t)
		comp1.AssertExpectations(t)
		comp2.AssertExpectations(t)
		comp3.AssertExpectations(t)
	})
}

func TestSession_getEventsFromComponent(t *testing.T) {
	t.Run("component not found", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		ctx := context.Background()
		startTime := time.Now().Add(-time.Hour)
		endTime := time.Now()

		registry.On("Get", "nonexistent").Return(nil)

		result := session.getEventsFromComponent(ctx, "nonexistent", startTime, endTime)

		assert.Equal(t, "nonexistent", result.Component)
		assert.Empty(t, result.Events)
		assert.Equal(t, startTime, result.StartTime)
		assert.Equal(t, endTime, result.EndTime)

		registry.AssertExpectations(t)
	})

	t.Run("successful event retrieval", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		ctx := context.Background()
		startTime := time.Now().Add(-time.Hour)
		endTime := time.Now()

		comp := new(mockComponent)
		events := apiv1.Events{
			{Name: "event1", Message: "test event 1"},
			{Name: "event2", Message: "test event 2"},
		}

		registry.On("Get", "test-comp").Return(comp)
		comp.On("Events", ctx, startTime).Return(events, nil)

		result := session.getEventsFromComponent(ctx, "test-comp", startTime, endTime)

		assert.Equal(t, "test-comp", result.Component)
		assert.Equal(t, events, result.Events)
		assert.Equal(t, startTime, result.StartTime)
		assert.Equal(t, endTime, result.EndTime)

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})

	t.Run("component events error", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		ctx := context.Background()
		startTime := time.Now().Add(-time.Hour)
		endTime := time.Now()

		comp := new(mockComponent)
		emptyEvents := apiv1.Events{}

		registry.On("Get", "error-comp").Return(comp)
		comp.On("Events", ctx, startTime).Return(emptyEvents, errors.New("events error"))

		result := session.getEventsFromComponent(ctx, "error-comp", startTime, endTime)

		assert.Equal(t, "error-comp", result.Component)
		assert.Empty(t, result.Events)
		assert.Equal(t, startTime, result.StartTime)
		assert.Equal(t, endTime, result.EndTime)

		registry.AssertExpectations(t)
		comp.AssertExpectations(t)
	})

	t.Run("handles context timeout", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		startTime := time.Now().Add(-time.Hour)
		endTime := time.Now()

		comp := new(mockComponent)

		registry.On("Get", "timeout-comp").Return(comp)
		comp.On("Events", mock.Anything, startTime).Return(apiv1.Events{}, context.DeadlineExceeded).Maybe()

		// Sleep to ensure context times out
		time.Sleep(2 * time.Millisecond)

		result := session.getEventsFromComponent(ctx, "timeout-comp", startTime, endTime)

		assert.Equal(t, "timeout-comp", result.Component)
		// Events should be empty on timeout
		assert.Empty(t, result.Events)

		registry.AssertExpectations(t)
	})
}
