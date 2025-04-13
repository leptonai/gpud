package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidia_infiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	"github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/pkg/update"
)

const (
	DefaultQuerySince     = 30 * time.Minute
	initializeGracePeriod = 3 * time.Minute
)

type Request struct {
	Method        string            `json:"method,omitempty"`
	Components    []string          `json:"components,omitempty"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	Since         time.Duration     `json:"since"`
	UpdateVersion string            `json:"update_version,omitempty"`
	UpdateConfig  map[string]string `json:"update_config,omitempty"`
}

type Response struct {
	// don't use "error" type as it doesn't marshal/unmarshal well
	Error string `json:"error,omitempty"`

	States  apiv1.GPUdComponentStates  `json:"states,omitempty"`
	Events  apiv1.GPUdComponentEvents  `json:"events,omitempty"`
	Metrics apiv1.GPUdComponentMetrics `json:"metrics,omitempty"`
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
			rerr := pkghost.Reboot(ctx, pkghost.WithDelaySeconds(0))

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
			if err != nil {
				response.Error = err.Error()
			}
			response.Metrics = metrics

		case "states":
			states, err := s.getStates(ctx, payload)
			if err != nil {
				response.Error = err.Error()
			}
			response.States = states

		case "events":
			events, err := s.getEvents(ctx, payload)
			if err != nil {
				response.Error = err.Error()
			}
			response.Events = events

		case "delete":
			go s.delete()

		case "sethealthy":
			log.Logger.Infow("sethealthy received", "components", payload.Components)
			for _, componentName := range payload.Components {
				comp, err := components.GetComponent(componentName)
				if err != nil {
					log.Logger.Errorw("failed to get component", "error", err)
					continue
				}
				if healthSettable, ok := comp.(components.HealthSettable); ok {
					if err := healthSettable.SetHealthy(); err != nil {
						log.Logger.Errorw("failed to set healthy", "component", componentName, "error", err)
					}
				} else {
					log.Logger.Warnw("component does not implement HealthSettable, dropping sethealthy request", "component", componentName)
				}
			}

		case "update":
			if targetVersion := strings.Split(payload.UpdateVersion, ":"); len(targetVersion) == 2 {
				err := update.PackageUpdate(targetVersion[0], targetVersion[1], update.DefaultUpdateURL)
				log.Logger.Infow("Update received for machine", "version", targetVersion[1], "package", targetVersion[0], "error", err)
			} else {
				if !s.enableAutoUpdate {
					log.Logger.Warnw("auto update is disabled -- skipping update")
					response.Error = "auto update is disabled"
					break
				}

				systemdManaged, _ := systemd.IsActive("gpud.service")
				if s.autoUpdateExitCode == -1 && !systemdManaged {
					log.Logger.Warnw("gpud is not managed with systemd and auto update by exit code is not set -- skipping update")
					response.Error = "gpud is not managed with systemd"
					break
				}

				nextVersion := payload.UpdateVersion
				if nextVersion == "" {
					log.Logger.Warnw("target update_version is empty -- skipping update")
					response.Error = "update_version is empty"
					break
				}

				if systemdManaged {
					uerr := update.Update(nextVersion, update.DefaultUpdateURL)
					if uerr != nil {
						response.Error = uerr.Error()
					}
					break
				}

				if s.autoUpdateExitCode != -1 {
					uerr := update.UpdateOnlyBinary(nextVersion, update.DefaultUpdateURL)
					if uerr != nil {
						response.Error = uerr.Error()
					}
					if response.Error == "" {
						needExit = s.autoUpdateExitCode
					}
				}
			}

		case "updateConfig":
			if payload.UpdateConfig != nil {
				for componentName, value := range payload.UpdateConfig {
					log.Logger.Infow("Update config received for component", "component", componentName, "config", value)

					switch componentName {
					case nvidia_infiniband.Name:
						var updateCfg infiniband.ExpectedPortStates
						if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
							log.Logger.Warnw("failed to unmarshal update config", "error", err)
						} else {
							nvidia_infiniband.SetDefaultExpectedPortStates(updateCfg)
						}
					default:
						log.Logger.Warnw("unsupported component for updateConfig", "component", componentName)
					}
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
			os.Exit(s.autoUpdateExitCode)
		}
	}
}

func (s *Session) delete() {
	// cleanup packages
	if err := createNeedDeleteFiles("/var/lib/gpud/packages"); err != nil {
		log.Logger.Errorw("failed to delete packages",
			"error", err,
		)
	}
}

func createNeedDeleteFiles(rootPath string) error {
	return filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && path != rootPath {
			needDeleteFilePath := filepath.Join(path, "needDelete")
			file, err := os.Create(needDeleteFilePath)
			if err != nil {
				return fmt.Errorf("failed to create needDelete file in %s: %w", path, err)
			}
			defer file.Close()
		}
		return nil
	})
}

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

func (s *Session) getMetrics(ctx context.Context, payload Request) (apiv1.GPUdComponentMetrics, error) {
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

	var metricBuf = make(chan apiv1.ComponentMetrics, len(allComponents))
	localCtx, done := context.WithTimeout(ctx, time.Minute)
	defer done()
	for _, componentName := range allComponents {
		go func(name string) {
			metricBuf <- s.getMetricsFromComponent(localCtx, name, metricsSince)
		}(componentName)
	}
	var retMetrics apiv1.GPUdComponentMetrics
	for currMetric := range metricBuf {
		retMetrics = append(retMetrics, currMetric)
		if len(retMetrics) == len(allComponents) {
			close(metricBuf)
			break
		}
	}
	return retMetrics, nil
}

func (s *Session) getStates(ctx context.Context, payload Request) (apiv1.GPUdComponentStates, error) {
	if payload.Method != "states" {
		return nil, errors.New("mismatch method")
	}
	allComponents := s.components
	if len(payload.Components) > 0 {
		allComponents = payload.Components
	}
	var statesBuf = make(chan apiv1.ComponentStates, len(allComponents))
	var lastRebootTime *time.Time
	localCtx, done := context.WithTimeout(ctx, time.Minute)
	defer done()
	for _, componentName := range allComponents {
		go func(name string) {
			statesBuf <- s.getStatesFromComponent(localCtx, name, lastRebootTime)
		}(componentName)
	}
	var states apiv1.GPUdComponentStates
	for currState := range statesBuf {
		states = append(states, currState)
		if len(states) == len(allComponents) {
			close(statesBuf)
			break
		}
	}
	return states, nil
}

func (s *Session) getEventsFromComponent(ctx context.Context, componentName string, startTime, endTime time.Time) apiv1.ComponentEvents {
	component, err := components.GetComponent(componentName)
	if err != nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetEvents",
			"component", componentName,
			"error", err,
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

func (s *Session) getMetricsFromComponent(ctx context.Context, componentName string, since time.Time) apiv1.ComponentMetrics {
	if _, err := components.GetComponent(componentName); err != nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetEvents",
			"component", componentName,
			"error", err,
		)
		return apiv1.ComponentMetrics{
			Component: componentName,
		}
	}
	currMetrics := apiv1.ComponentMetrics{
		Component: componentName,
	}
	metricsData, err := s.metricsStore.Read(ctx, pkgmetrics.WithSince(since), pkgmetrics.WithComponents(componentName))
	if err != nil {
		log.Logger.Errorw("failed to invoke component metrics",
			"operation", "GetEvents",
			"component", componentName,
			"error", err,
		)
		return currMetrics
	}

	for _, data := range metricsData {
		currMetrics.Metrics = append(currMetrics.Metrics, apiv1.Metric{
			UnixSeconds:                   data.UnixMilliseconds,
			DeprecatedMetricName:          data.Name,
			DeprecatedMetricSecondaryName: data.Label,
			Value:                         data.Value,
		})
	}
	return currMetrics
}

func (s *Session) getStatesFromComponent(ctx context.Context, componentName string, lastRebootTime *time.Time) apiv1.ComponentStates {
	component, err := components.GetComponent(componentName)
	if err != nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetStates",
			"component", componentName,
			"error", err,
		)
		return apiv1.ComponentStates{
			Component: componentName,
		}
	}
	currState := apiv1.ComponentStates{
		Component: componentName,
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
	for i, componentState := range currState.States {
		if !componentState.DeprecatedHealthy {
			if lastRebootTime == nil {
				rebootTime, err := pkghost.LastReboot(context.Background())
				lastRebootTime = &rebootTime
				if err != nil {
					log.Logger.Errorw("failed to get last reboot time", "error", err)
				}
			}
			if time.Since(*lastRebootTime) < initializeGracePeriod {
				log.Logger.Warnw("set unhealthy state initializing due to recent reboot", "component", componentName)
				currState.States[i].Health = apiv1.StateTypeInitializing
				currState.States[i].DeprecatedHealthy = true
			}

			if componentState.Error != "" &&
				(strings.Contains(componentState.Error, context.DeadlineExceeded.Error()) ||
					strings.Contains(componentState.Error, context.Canceled.Error())) {
				log.Logger.Errorw("state error due to deadline exceeded or canceled error", "component", componentName, "error", componentState.Error)
			}
		}
	}
	return currState
}
