package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	componentsnvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/errdefs"
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

	Bootstrap *BootstrapRequest `json:"bootstrap,omitempty"`

	// ComponentName is the name of the component to deregister.
	ComponentName string `json:"component_name,omitempty"`

	// CustomPluginSpec is the spec for the custom plugin to register or update.
	CustomPluginSpec *pkgcustomplugins.Spec `json:"custom_plugin_spec,omitempty"`
}

type Response struct {
	// Error is the error message from session processor.
	// don't use "error" type as it doesn't marshal/unmarshal well
	Error string `json:"error,omitempty"`
	// ErrorCode is the error code from session processor.
	// It uses the same semantics as the HTTP status code.
	// See: https://www.iana.org/assignments/http-status-codes/http-status-codes.xhtml
	ErrorCode int32 `json:"error_code,omitempty"`

	States  apiv1.GPUdComponentHealthStates `json:"states,omitempty"`
	Events  apiv1.GPUdComponentEvents       `json:"events,omitempty"`
	Metrics apiv1.GPUdComponentMetrics      `json:"metrics,omitempty"`

	Bootstrap *BootstrapResponse `json:"bootstrap,omitempty"`

	Plugins map[string]pkgcustomplugins.Spec `json:"plugins,omitempty"`
}

type BootstrapRequest struct {
	// TimeoutInSeconds is the timeout for the bootstrap script.
	// If not set, the default timeout is 10 seconds.
	TimeoutInSeconds int `json:"timeout_in_seconds,omitempty"`

	// ScriptBase64 is the base64 encoded script to run.
	ScriptBase64 string `json:"script_base64,omitempty"`
}

type BootstrapResponse struct {
	Output   string `json:"output,omitempty"`
	ExitCode int32  `json:"exit_code,omitempty"`
}

func (s *Session) serve() {
	for body := range s.reader {
		var payload Request
		if err := json.Unmarshal(body.Data, &payload); err != nil {
			log.Logger.Errorf("failed to unmarshal request: %v", err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

		needExit := -1
		response := &Response{}

		switch payload.Method {
		case "reboot":
			// To inform the control plane that the reboot request has been processed, reboot after 10 seconds.
			err := pkghost.Reboot(s.ctx, pkghost.WithDelaySeconds(10))
			if err != nil {
				log.Logger.Errorf("failed to trigger reboot machine: %v", err)
				response.Error = err.Error()
			}

		case "metrics":
			metrics, err := s.getMetrics(ctx, payload)
			if err != nil {
				response.Error = err.Error()
			}
			response.Metrics = metrics

		case "states":
			states, err := s.getHealthStates(ctx, payload)
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

		case "setHealthy":
			log.Logger.Infow("setHealthy received", "components", payload.Components)
			for _, componentName := range payload.Components {
				comp := s.componentsRegistry.Get(componentName)
				if comp == nil {
					log.Logger.Errorw("failed to get component", "error", errdefs.ErrNotFound)
					continue
				}
				if healthSettable, ok := comp.(components.HealthSettable); ok {
					if err := healthSettable.SetHealthy(); err != nil {
						log.Logger.Errorw("failed to set healthy", "component", componentName, "error", err)
					}
				} else {
					log.Logger.Warnw("component does not implement HealthSettable, dropping setHealthy request", "component", componentName)
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
					case componentsnvidiainfiniband.Name:
						var updateCfg infiniband.ExpectedPortStates
						if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
							log.Logger.Warnw("failed to unmarshal update config", "error", err)
						} else {
							componentsnvidiainfiniband.SetDefaultExpectedPortStates(updateCfg)
						}
					default:
						log.Logger.Warnw("unsupported component for updateConfig", "component", componentName)
					}
				}
			}

		case "bootstrap":
			if payload.Bootstrap != nil {
				script, err := base64.StdEncoding.DecodeString(payload.Bootstrap.ScriptBase64)
				if err != nil {
					response.Error = err.Error()
					break
				}

				timeout := time.Duration(payload.Bootstrap.TimeoutInSeconds) * time.Second
				if timeout == 0 {
					timeout = 10 * time.Second
				}

				cctx, cancel := context.WithTimeout(ctx, timeout)
				output, exitCode, err := s.processRunner.RunUntilCompletion(cctx, string(script))
				cancel()
				response.Bootstrap = &BootstrapResponse{
					Output:   string(output),
					ExitCode: exitCode,
				}
				if err != nil {
					response.Error = err.Error()
				}
			}

		case "deregisterComponent":
			if payload.ComponentName != "" {
				comp := s.componentsRegistry.Get(payload.ComponentName)
				if comp == nil {
					log.Logger.Warnw("component not found", "name", payload.ComponentName)
					break
				}

				deregisterable, ok := comp.(components.Deregisterable)
				if !ok {
					log.Logger.Warnw("component is not deregisterable, not implementing Deregisterable interface", "name", comp.Name())
					response.Error = "component is not deregisterable"
					break
				}

				if !deregisterable.CanDeregister() {
					log.Logger.Warnw("component is not deregisterable", "name", comp.Name())
					response.Error = "component is not deregisterable"
					break
				}

				cerr := comp.Close()
				if cerr != nil {
					log.Logger.Errorw("failed to close component", "error", cerr)
					response.Error = cerr.Error()
					break
				}

				// only deregister if the component is successfully closed
				_ = s.componentsRegistry.Deregister(payload.ComponentName)
			}

		case "getPlugins":
			cs := make(map[string]pkgcustomplugins.Spec, 0)
			for _, c := range s.componentsRegistry.All() {
				if customPluginRegisteree, ok := c.(pkgcustomplugins.CustomPluginRegisteree); ok {
					if customPluginRegisteree.IsCustomPlugin() {
						cs[c.Name()] = customPluginRegisteree.Spec()
					}
				}
			}
			response.Plugins = cs

		case "registerPlugin":
			if payload.CustomPluginSpec != nil {
				if err := payload.CustomPluginSpec.Validate(); err != nil {
					response.Error = err.Error()
					break
				}

				initFunc := payload.CustomPluginSpec.NewInitFunc()
				if initFunc == nil {
					response.Error = fmt.Sprintf("failed to create init function for plugin %s", payload.CustomPluginSpec.ComponentName())
					break
				}

				comp, err := s.componentsRegistry.Register(initFunc)
				if err != nil {
					if errors.Is(err, components.ErrAlreadyRegistered) {
						response.ErrorCode = http.StatusConflict
					}
					response.Error = err.Error()
					break
				}

				if err := comp.Start(); err != nil {
					response.Error = err.Error()
					break
				}
				log.Logger.Infow("registered and started custom plugin", "name", comp.Name())
			}

		case "updatePlugin":
			if payload.CustomPluginSpec != nil {
				if err := payload.CustomPluginSpec.Validate(); err != nil {
					response.Error = err.Error()
					break
				}

				prevComp := s.componentsRegistry.Get(payload.CustomPluginSpec.ComponentName())
				if prevComp == nil {
					response.ErrorCode = http.StatusNotFound
					response.Error = fmt.Sprintf("plugin %s not found", payload.CustomPluginSpec.ComponentName())
					break
				}

				initFunc := payload.CustomPluginSpec.NewInitFunc()
				if initFunc == nil {
					response.Error = fmt.Sprintf("failed to create init function for plugin %s", payload.CustomPluginSpec.ComponentName())
					break
				}

				// now that we know the component is registered, we can deregister and register it
				prevComp = s.componentsRegistry.Deregister(prevComp.Name())
				_ = prevComp.Close()

				comp, err := s.componentsRegistry.Register(initFunc)
				if err != nil {
					response.Error = err.Error()
					break
				}

				if err := comp.Start(); err != nil {
					response.Error = err.Error()
					break
				}

				log.Logger.Infow("registered and started custom plugin", "name", comp.Name())
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

func (s *Session) getHealthStates(ctx context.Context, payload Request) (apiv1.GPUdComponentHealthStates, error) {
	if payload.Method != "states" {
		return nil, errors.New("mismatch method")
	}
	allComponents := s.components
	if len(payload.Components) > 0 {
		allComponents = payload.Components
	}
	var statesBuf = make(chan apiv1.ComponentHealthStates, len(allComponents))
	var lastRebootTime *time.Time
	localCtx, done := context.WithTimeout(ctx, time.Minute)
	defer done()
	for _, componentName := range allComponents {
		go func(name string) {
			statesBuf <- s.getStatesFromComponent(localCtx, name, lastRebootTime)
		}(componentName)
	}
	var states apiv1.GPUdComponentHealthStates
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

func (s *Session) getMetricsFromComponent(ctx context.Context, componentName string, since time.Time) apiv1.ComponentMetrics {
	component := s.componentsRegistry.Get(componentName)
	if component == nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetEvents",
			"component", componentName,
			"error", errdefs.ErrNotFound,
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

func (s *Session) getStatesFromComponent(ctx context.Context, componentName string, lastRebootTime *time.Time) apiv1.ComponentHealthStates {
	component := s.componentsRegistry.Get(componentName)
	if component == nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetStates",
			"component", componentName,
			"error", errdefs.ErrNotFound,
		)
		return apiv1.ComponentHealthStates{
			Component: componentName,
		}
	}
	currState := apiv1.ComponentHealthStates{
		Component: componentName,
	}
	log.Logger.Debugw("getting states", "component", componentName)
	state := component.LastHealthStates()
	log.Logger.Debugw("successfully got states", "component", componentName)
	currState.States = state

	for i, componentState := range currState.States {
		if componentState.Health != apiv1.HealthStateTypeHealthy {
			if lastRebootTime == nil {
				rebootTime, err := pkghost.LastReboot(context.Background())
				lastRebootTime = &rebootTime
				if err != nil {
					log.Logger.Errorw("failed to get last reboot time", "error", err)
				}
			}
			if time.Since(*lastRebootTime) < initializeGracePeriod {
				log.Logger.Warnw("set unhealthy state initializing due to recent reboot", "component", componentName)
				currState.States[i].Health = apiv1.HealthStateTypeInitializing
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
