package session

import (
	"context"
	"errors"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/log"
)

func (s *Session) getEvents(ctx context.Context, payload Request) (apiv1.GPUdComponentEvents, error) {
	if payload.Method != "events" {
		return nil, errors.New("mismatch method")
	}
	allComponents := s.components
	if len(payload.Components) > 0 {
		allComponents = payload.Components
	}
	startTime := time.Now()
	endTime := time.Now()
	if !payload.StartTime.IsZero() {
		startTime = payload.StartTime
	}
	if !payload.EndTime.IsZero() {
		endTime = payload.EndTime
	}
	var eventsBuf = make(chan apiv1.ComponentEvents, len(allComponents))
	localCtx, done := context.WithTimeout(ctx, time.Minute)
	defer done()
	for _, componentName := range allComponents {
		go func() {
			eventsBuf <- s.getEventsFromComponent(localCtx, componentName, startTime, endTime)
		}()
	}
	var events apiv1.GPUdComponentEvents
	for currEvent := range eventsBuf {
		events = append(events, currEvent)
		if len(events) == len(allComponents) {
			close(eventsBuf)
			break
		}
	}
	return events, nil
}

func (s *Session) getEventsFromComponent(ctx context.Context, componentName string, startTime, endTime time.Time) apiv1.ComponentEvents {
	component := s.componentsRegistry.Get(componentName)
	if component == nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetEvents",
			"component", componentName,
			"error", errdefs.ErrNotFound,
		)
		return apiv1.ComponentEvents{
			Component: componentName,
			StartTime: startTime,
			EndTime:   endTime,
		}
	}
	currEvent := apiv1.ComponentEvents{
		Component: componentName,
		StartTime: startTime,
		EndTime:   endTime,
	}
	log.Logger.Debugw("getting events", "component", componentName)
	event, err := component.Events(ctx, startTime)
	if err != nil {
		log.Logger.Errorw("failed to invoke component events",
			"operation", "GetEvents",
			"component", componentName,
			"error", err,
		)
	} else if len(event) > 0 {
		log.Logger.Debugw("successfully got events", "component", componentName)
		currEvent.Events = event
	}
	return currEvent
}
