package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	v1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/reboot"
	"github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/update"
)

const DefaultQuerySince = 30 * time.Minute

type Request struct {
	Method        string        `json:"method,omitempty"`
	Components    []string      `json:"components,omitempty"`
	StartTime     time.Time     `json:"start_time"`
	EndTime       time.Time     `json:"end_time"`
	Since         time.Duration `json:"since"`
	UpdateVersion string        `json:"update_version,omitempty"`
}

type Response struct {
	Error   error            `json:"error,omitempty"`
	States  v1.LeptonStates  `json:"states,omitempty"`
	Events  v1.LeptonEvents  `json:"events,omitempty"`
	Metrics v1.LeptonMetrics `json:"metrics,omitempty"`
}

func (s *Session) serve() {
	for body := range s.reader {
		var payload Request
		if err := json.Unmarshal(body.Data, &payload); err != nil {
			log.Logger.Errorf("failed to unmarshal request: %v", err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		if payload.Method == "reboot" {
			rerr := reboot.Reboot(ctx, reboot.WithDelaySeconds(0))

			if rerr != nil {
				log.Logger.Errorf("failed to trigger reboot machine: %v", rerr)
			}

			cancel()
			continue
		}

		needExit := -1
		response := &Response{}

		switch payload.Method {
		case "metrics":
			metrics, err := s.getMetrics(ctx, payload)
			response.Error = err
			response.Metrics = metrics

		case "states":
			states, err := s.getStates(ctx, payload)
			response.Error = err
			response.States = states

		case "events":
			events, err := s.getEvents(ctx, payload)
			response.Error = err
			response.Events = events

		case "update":
			if !s.enableAutoUpdate {
				log.Logger.Warnw("auto update is disabled -- skipping update")
				response.Error = errors.New("auto update is disabled")
				break
			}

			systemdManaged, _ := systemd.IsActive("gpud.service")
			if s.autoUpdateExitCode == -1 && !systemdManaged {
				log.Logger.Warnw("gpud is not managed with systemd and auto update by exit code is not set -- skipping update")
				response.Error = errors.New("gpud is not managed with systemd")
				break
			}

			nextVersion := payload.UpdateVersion
			if nextVersion == "" {
				log.Logger.Warnw("target update_version is empty -- skipping update")
				response.Error = errors.New("update_version is empty")
				break
			}

			if systemdManaged {
				response.Error = update.Update(nextVersion, update.DefaultUpdateURL)
				break
			}

			if s.autoUpdateExitCode != -1 {
				response.Error = update.UpdateOnlyBinary(nextVersion, update.DefaultUpdateURL)
				if response.Error == nil {
					needExit = s.autoUpdateExitCode
				}
			}
		}

		cancel()

		responseRaw, _ := json.Marshal(response)
		s.writer <- Body{
			Data:  responseRaw,
			ReqID: body.ReqID,
		}

		if needExit != -1 {
			log.Logger.Infow("exiting with code for auto update", "code", needExit)
			os.Exit(needExit)
		}
	}
}

func (s *Session) getEvents(ctx context.Context, payload Request) (v1.LeptonEvents, error) {
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
		startTime = payload.EndTime
	}
	var events v1.LeptonEvents
	for _, componentName := range allComponents {
		currEvent := v1.LeptonComponentEvents{
			Component: componentName,
			StartTime: startTime,
			EndTime:   endTime,
		}
		component, err := components.GetComponent(componentName)
		if err != nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetEvents",
				"component", componentName,
				"error", err,
			)
			events = append(events, currEvent)
			continue
		}
		event, err := component.Events(ctx, startTime)
		if err != nil {
			log.Logger.Errorw("failed to invoke component events",
				"operation", "GetEvents",
				"component", componentName,
				"error", err,
			)
		} else {
			currEvent.Events = event
		}
		events = append(events, currEvent)
	}
	return events, nil
}

func (s *Session) getMetrics(ctx context.Context, payload Request) (v1.LeptonMetrics, error) {
	if payload.Method != "metrics" {
		return nil, errors.New("mismatch method")
	}
	allComponents := s.components
	if len(payload.Components) > 0 {
		allComponents = payload.Components
	}

	now := time.Now().UTC()
	metricsSince := now.Add(-DefaultQuerySince)
	if payload.Since > 0 {
		metricsSince = now.Add(-payload.Since)
	}

	var metrics v1.LeptonMetrics
	for _, componentName := range allComponents {
		currMetrics := v1.LeptonComponentMetrics{
			Component: componentName,
		}
		component, err := components.GetComponent(componentName)
		if err != nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetEvents",
				"component", componentName,
				"error", err,
			)
			metrics = append(metrics, currMetrics)
			continue
		}
		currMetric, err := component.Metrics(ctx, metricsSince)
		if err != nil {
			log.Logger.Errorw("failed to invoke component metrics",
				"operation", "GetEvents",
				"component", componentName,
				"error", err,
			)
		} else {
			currMetrics.Metrics = currMetric
		}
		metrics = append(metrics, currMetrics)
	}
	return metrics, nil
}

func (s *Session) getStates(ctx context.Context, payload Request) (v1.LeptonStates, error) {
	if payload.Method != "states" {
		return nil, errors.New("mismatch method")
	}
	allComponents := s.components
	if len(payload.Components) > 0 {
		allComponents = payload.Components
	}
	var states v1.LeptonStates
	for _, componentName := range allComponents {
		currState := v1.LeptonComponentStates{
			Component: componentName,
		}
		component, err := components.GetComponent(componentName)
		if err != nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetStates",
				"component", componentName,
				"error", err,
			)
			states = append(states, currState)
			continue
		}

		log.Logger.Debugw("getting states", "component", componentName)
		state, err := component.States(ctx)
		if err != nil {
			log.Logger.Errorw("failed to invoke component state",
				"operation", "GetStates",
				"component", componentName,
				"error", err,
			)
		} else {
			log.Logger.Debugw("successfully got states", "component", componentName)
			currState.States = state
		}
		states = append(states, currState)
	}
	return states, nil
}
