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

	v1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidia_infiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	"github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	nvidia_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	"github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	nvidia_xid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
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
				switch componentName {
				case nvidia_xid.Name:
					rawComponent, err := components.GetComponent(nvidia_xid.Name)
					if err != nil {
						log.Logger.Errorw("failed to get component", "error", err)
						continue
					}
					if watchable, ok := rawComponent.(*metrics.WatchableComponentStruct); ok {
						if component, ok := watchable.Component.(*xid.XIDComponent); ok {
							if err = component.SetHealthy(); err != nil {
								log.Logger.Errorw("failed to set xid healthy", "error", err)
							}
						} else {
							log.Logger.Errorf("failed to cast component to xid component: %T", watchable)
						}
					} else {
						log.Logger.Errorf("failed to cast component to watchable component: %T", rawComponent)
					}
				case nvidia_sxid.Name:
					rawComponent, err := components.GetComponent(nvidia_sxid.Name)
					if err != nil {
						log.Logger.Errorw("failed to get component", "error", err)
						continue
					}
					if watchable, ok := rawComponent.(*metrics.WatchableComponentStruct); ok {
						if component, ok := watchable.Component.(*sxid.SXIDComponent); ok {
							if err = component.SetHealthy(); err != nil {
								log.Logger.Errorw("failed to set sxid healthy", "error", err)
							}
						} else {
							log.Logger.Errorf("failed to cast component to sxid component: %T", watchable)
						}
					} else {
						log.Logger.Errorf("failed to cast component to watchable component: %T", rawComponent)
					}
				default:
					log.Logger.Warnw("unsupported component for sethealthy", "component", componentName)
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
		endTime = payload.EndTime
	}
	var eventsBuf = make(chan v1.LeptonComponentEvents, len(allComponents))
	localCtx, done := context.WithTimeout(ctx, time.Minute)
	defer done()
	for _, componentName := range allComponents {
		go func() {
			eventsBuf <- s.getEventsFromComponent(localCtx, componentName, startTime, endTime)
		}()
	}
	var events v1.LeptonEvents
	for currEvent := range eventsBuf {
		events = append(events, currEvent)
		if len(events) == len(allComponents) {
			close(eventsBuf)
			break
		}
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

	var metricBuf = make(chan v1.LeptonComponentMetrics, len(allComponents))
	localCtx, done := context.WithTimeout(ctx, time.Minute)
	defer done()
	for _, componentName := range allComponents {
		go func(name string) {
			metricBuf <- s.getMetricsFromComponent(localCtx, name, metricsSince)
		}(componentName)
	}
	var retMetrics v1.LeptonMetrics
	for currMetric := range metricBuf {
		retMetrics = append(retMetrics, currMetric)
		if len(retMetrics) == len(allComponents) {
			close(metricBuf)
			break
		}
	}
	return retMetrics, nil
}

func (s *Session) getStates(ctx context.Context, payload Request) (v1.LeptonStates, error) {
	if payload.Method != "states" {
		return nil, errors.New("mismatch method")
	}
	allComponents := s.components
	if len(payload.Components) > 0 {
		allComponents = payload.Components
	}
	var statesBuf = make(chan v1.LeptonComponentStates, len(allComponents))
	var lastRebootTime *time.Time
	localCtx, done := context.WithTimeout(ctx, time.Minute)
	defer done()
	for _, componentName := range allComponents {
		go func(name string) {
			statesBuf <- s.getStatesFromComponent(localCtx, name, lastRebootTime)
		}(componentName)
	}
	var states v1.LeptonStates
	for currState := range statesBuf {
		states = append(states, currState)
		if len(states) == len(allComponents) {
			close(statesBuf)
			break
		}
	}
	return states, nil
}

func (s *Session) getEventsFromComponent(ctx context.Context, componentName string, startTime, endTime time.Time) v1.LeptonComponentEvents {
	component, err := components.GetComponent(componentName)
	if err != nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetEvents",
			"component", componentName,
			"error", err,
		)
		return v1.LeptonComponentEvents{
			Component: componentName,
			StartTime: startTime,
			EndTime:   endTime,
		}
	}
	currEvent := v1.LeptonComponentEvents{
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

func (s *Session) getMetricsFromComponent(ctx context.Context, componentName string, since time.Time) v1.LeptonComponentMetrics {
	component, err := components.GetComponent(componentName)
	if err != nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetEvents",
			"component", componentName,
			"error", err,
		)
		return v1.LeptonComponentMetrics{
			Component: componentName,
		}
	}
	currMetrics := v1.LeptonComponentMetrics{
		Component: componentName,
	}
	currMetric, err := component.Metrics(ctx, since)
	if err != nil {
		log.Logger.Errorw("failed to invoke component metrics",
			"operation", "GetEvents",
			"component", componentName,
			"error", err,
		)
	} else {
		currMetrics.Metrics = currMetric
	}
	return currMetrics
}

func (s *Session) getStatesFromComponent(ctx context.Context, componentName string, lastRebootTime *time.Time) v1.LeptonComponentStates {
	component, err := components.GetComponent(componentName)
	if err != nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetStates",
			"component", componentName,
			"error", err,
		)
		return v1.LeptonComponentStates{
			Component: componentName,
		}
	}
	currState := v1.LeptonComponentStates{
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
		if !componentState.Healthy {
			if lastRebootTime == nil {
				rebootTime, err := pkghost.LastReboot(context.Background())
				lastRebootTime = &rebootTime
				if err != nil {
					log.Logger.Errorw("failed to get last reboot time", "error", err)
				}
			}
			if time.Since(*lastRebootTime) < initializeGracePeriod {
				log.Logger.Warnw("set unhealthy state initializing due to recent reboot", "component", componentName)
				currState.States[i].Health = components.StateInitializing
				currState.States[i].Healthy = true
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
