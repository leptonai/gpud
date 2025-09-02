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
	"github.com/leptonai/gpud/pkg/config"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/errdefs"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	pkdsystemd "github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/pkg/update"
)

const (
	DefaultQuerySince     = 30 * time.Minute
	initializeGracePeriod = 5 * time.Minute
)

// Request is the request from the control plane to GPUd.
type Request struct {
	Method        string            `json:"method,omitempty"`
	Components    []string          `json:"components,omitempty"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	Since         time.Duration     `json:"since"`
	UpdateVersion string            `json:"update_version,omitempty"`
	UpdateConfig  map[string]string `json:"update_config,omitempty"`

	Bootstrap          *BootstrapRequest         `json:"bootstrap,omitempty"`
	InjectFaultRequest *pkgfaultinjector.Request `json:"inject_fault_request,omitempty"`

	// ComponentName is the name of the component to query or deregister.
	ComponentName string `json:"component_name,omitempty"`

	// TagName is the tag of the component to trigger check.
	// Optional. If set, it triggers all the component checks
	// that match this tag value.
	TagName string `json:"tag_name,omitempty"`

	// CustomPluginSpecs is the specs for the custom plugins to register or overwrite.
	CustomPluginSpecs pkgcustomplugins.Specs `json:"custom_plugin_specs,omitempty"`
}

// Response is the response from GPUd to the control plane.
type Response struct {
	// Error is the error message from session processor.
	// don't use "error" type as it doesn't marshal/unmarshal well
	Error string `json:"error,omitempty"`
	// ErrorCode is the error code from session processor.
	// It uses the same semantics as the HTTP status code.
	// See: https://www.iana.org/assignments/http-status-codes/http-status-codes.xhtml
	ErrorCode int32 `json:"error_code,omitempty"`

	GossipRequest *apiv1.GossipRequest `json:"gossip_request,omitempty"`

	States  apiv1.GPUdComponentHealthStates `json:"states,omitempty"`
	Events  apiv1.GPUdComponentEvents       `json:"events,omitempty"`
	Metrics apiv1.GPUdComponentMetrics      `json:"metrics,omitempty"`

	Bootstrap *BootstrapResponse `json:"bootstrap,omitempty"`

	PackageStatus []apiv1.PackageStatus `json:"package_status,omitempty"`

	// CustomPluginSpecs lists the specs for the custom plugins.
	CustomPluginSpecs pkgcustomplugins.Specs `json:"custom_plugin_specs,omitempty"`
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
			log.Logger.Errorf("failed to unmarshal request: %v", err, "requestID", body.ReqID)
			continue
		}

		s.auditLogger.Log(
			log.WithKind("Session"),
			log.WithAuditID(body.ReqID),
			log.WithMachineID(s.machineID),
			log.WithStage("RequestDecoded"),
			log.WithRequestURI(s.epControlPlane+"/api/v1/session"),
			log.WithVerb(payload.Method),
			log.WithData(payload),
		)

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

		restartExitCode := -1
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
			states, err := s.getHealthStates(payload)
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

		case "logout":
			stateFile, err := config.DefaultStateFile()
			if err != nil {
				log.Logger.Errorw("failed to get state file", "error", err)
				response.Error = err.Error()
				break
			}
			dbRW, err := sqlite.Open(stateFile)
			if err != nil {
				log.Logger.Errorw("failed to open state file", "error", err)
				response.Error = err.Error()
				dbRW.Close()
				break
			}
			if err = pkgmetadata.DeleteAllMetadata(ctx, dbRW); err != nil {
				log.Logger.Errorw("failed to purge metadata", "error", err)
				response.Error = err.Error()
				dbRW.Close()
				break
			}
			dbRW.Close()
			err = pkghost.Stop(s.ctx, pkghost.WithDelaySeconds(10))
			if err != nil {
				log.Logger.Errorf("failed to trigger stop gpud: %v", err)
				response.Error = err.Error()
			}

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

		case "gossip":
			s.processGossip(response)

		case "packageStatus":
			packageStatus, err := gpudmanager.GlobalController.Status(ctx)
			if err != nil {
				response.Error = err.Error()
			}
			var result []apiv1.PackageStatus
			for _, currPackage := range packageStatus {
				packagePhase := apiv1.UnknownPhase
				if currPackage.IsInstalled {
					packagePhase = apiv1.InstalledPhase
				} else if currPackage.Installing {
					packagePhase = apiv1.InstallingPhase
				}
				status := "Unhealthy"
				if currPackage.Status {
					status = "Healthy"
				}
				result = append(result, apiv1.PackageStatus{
					Name:           currPackage.Name,
					Phase:          packagePhase,
					Status:         status,
					CurrentVersion: currPackage.CurrentVersion,
				})
			}
			response.PackageStatus = result

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
					if uerr := pkdsystemd.CreateDefaultEnvFile(""); uerr != nil {
						response.Error = uerr.Error()
						break
					}
				}

				// even if it's systemd managed, it's using "Restart=always"
				// thus we simply exit the process to trigger the restart
				// do not use "systemctl restart gpud.service"
				// as it immediately restarts the service,
				// failing to respond to the control plane
				uerr := update.UpdateExecutable(nextVersion, update.DefaultUpdateURL, systemdManaged)
				if uerr != nil {
					response.Error = uerr.Error()
				} else {
					restartExitCode = s.autoUpdateExitCode
					log.Logger.Infow("scheduled process exit for auto update", "code", restartExitCode)
				}
			}

		case "updateConfig":
			s.processUpdateConfig(payload.UpdateConfig, response)

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

		case "injectFault":
			if payload.InjectFaultRequest != nil {
				if s.faultInjector == nil {
					response.Error = "fault injector is not initialized"
					break
				}

				if err := payload.InjectFaultRequest.Validate(); err != nil {
					response.Error = err.Error()
					log.Logger.Errorw("invalid fault inject request", "error", err)
					break
				}

				switch {
				case payload.InjectFaultRequest.KernelMessage != nil:
					if err := s.faultInjector.KmsgWriter().Write(payload.InjectFaultRequest.KernelMessage); err != nil {
						response.Error = err.Error()
						log.Logger.Errorw("failed to inject kernel message", "message", payload.InjectFaultRequest.KernelMessage.Message, "error", err)
					} else {
						log.Logger.Infow("successfully injected kernel message", "message", payload.InjectFaultRequest.KernelMessage.Message)
					}

				default:
					log.Logger.Warnw("fault inject request is nil or kernel message is nil")
				}
			} else {
				log.Logger.Warnw("fault inject request is nil")
			}

			// TODO: deprecate "triggerComponentCheck" after control plane supports "triggerComponent"
		case "triggerComponent",
			"triggerComponentCheck":
			checkResults := make([]components.CheckResult, 0)
			if payload.ComponentName != "" {
				// requesting a specific component, tag is ignored
				comp := s.componentsRegistry.Get(payload.ComponentName)
				if comp == nil {
					log.Logger.Warnw("component not found", "name", payload.ComponentName)
					response.ErrorCode = http.StatusNotFound
					break
				}

				checkResults = append(checkResults, comp.Check())
			} else if payload.TagName != "" {
				components := s.componentsRegistry.All()
				for _, comp := range components {
					matched := false
					for _, tag := range comp.Tags() {
						if tag == payload.TagName {
							matched = true
							break
						}
					}
					if !matched {
						continue
					}

					checkResults = append(checkResults, comp.Check())
				}
			}

			response.States = apiv1.GPUdComponentHealthStates{}
			for _, checkResult := range checkResults {
				response.States = append(response.States, apiv1.ComponentHealthStates{
					Component: checkResult.ComponentName(),
					States:    checkResult.HealthStates(),
				})
			}

		case "deregisterComponent":
			if payload.ComponentName != "" {
				comp := s.componentsRegistry.Get(payload.ComponentName)
				if comp == nil {
					log.Logger.Warnw("component not found", "name", payload.ComponentName)
					response.ErrorCode = http.StatusNotFound
					break
				}

				deregisterable, ok := comp.(components.Deregisterable)
				if !ok {
					log.Logger.Warnw("component is not deregisterable, not implementing Deregisterable interface", "name", comp.Name())
					response.ErrorCode = http.StatusBadRequest
					response.Error = "component is not deregisterable"
					break
				}

				if !deregisterable.CanDeregister() {
					log.Logger.Warnw("component is not deregisterable", "name", comp.Name())
					response.ErrorCode = http.StatusBadRequest
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

		case "setPluginSpecs":
			exitCode := s.processSetPluginSpecs(ctx, response, payload.CustomPluginSpecs)
			if exitCode != nil {
				restartExitCode = *exitCode
				log.Logger.Infow("scheduled process exit for plugin specs update", "code", restartExitCode)
			}

		case "getPluginSpecs":
			s.processGetPluginSpecs(response)
		}

		cancel()

		responseRaw, _ := json.Marshal(response)
		s.writer <- Body{
			Data:  responseRaw,
			ReqID: body.ReqID,
		}

		s.auditLogger.Log(
			log.WithKind("Session"),
			log.WithAuditID(body.ReqID),
			log.WithMachineID(s.machineID),
			log.WithStage("RequestCompleted"),
			log.WithRequestURI(s.epControlPlane+"/api/v1/session"),
			log.WithVerb(payload.Method),
			log.WithData(response),
		)

		if restartExitCode != -1 {
			go func() {
				log.Logger.Infow("process exiting as scheduled", "code", restartExitCode)
				select {
				case <-s.ctx.Done():
				case <-time.After(10 * time.Second):
					// enough time to send response back to control plane
				}

				s.auditLogger.Log(
					log.WithKind("AutoUpdateRestart"),
					log.WithAuditID(body.ReqID),
					log.WithMachineID(s.machineID),
				)
				os.Exit(s.autoUpdateExitCode)
			}()
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

func (s *Session) getHealthStates(payload Request) (apiv1.GPUdComponentHealthStates, error) {
	if payload.Method != "states" {
		return nil, errors.New("mismatch method")
	}
	allComponents := s.components
	if len(payload.Components) > 0 {
		allComponents = payload.Components
	}
	var statesBuf = make(chan apiv1.ComponentHealthStates, len(allComponents))
	// Use BootTimeUnixSeconds which reads directly from system sources
	// avoiding timezone parsing issues with "uptime -s"
	bootTimeUnix := pkghost.BootTimeUnixSeconds()
	rebootTime := time.Unix(int64(bootTimeUnix), 0)

	// Sanity check: if boot time is 0 or in the future, use zero time
	// This prevents accidentally setting components to initializing
	now := time.Now().UTC()
	if bootTimeUnix == 0 || rebootTime.After(now) {
		log.Logger.Warnw("invalid boot time detected, using zero time",
			"bootTimeUnix", bootTimeUnix,
			"rebootTime", rebootTime,
			"now", now,
		)
		rebootTime = time.Time{} // Zero time will prevent initializing state
	}
	for _, componentName := range allComponents {
		go func(name string) {
			statesBuf <- s.getStatesFromComponent(name, rebootTime)
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
			UnixSeconds: data.UnixMilliseconds,
			Name:        data.Name,
			Labels:      data.Labels,
			Value:       data.Value,
		})
	}
	return currMetrics
}

// getComponentFunc is a function type for getting a component by name
type getComponentFunc func(string) components.Component

func (s *Session) getStatesFromComponent(componentName string, lastRebootTime time.Time) apiv1.ComponentHealthStates {
	return getStatesFromComponentWithDeps(
		componentName,
		lastRebootTime,
		s.componentsRegistry.Get,
	)
}

func getStatesFromComponentWithDeps(
	componentName string,
	lastRebootTime time.Time,
	getComponent getComponentFunc,
) apiv1.ComponentHealthStates {
	component := getComponent(componentName)
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

	elapsedSinceReboot := time.Since(lastRebootTime)
	// Only reset to initializing if:
	// 1. We're within the grace period (5 minutes)
	// 2. The elapsed time is positive (not in the future)
	// 3. The reboot time is reasonable (not Unix epoch)
	resetToInitializing := elapsedSinceReboot < initializeGracePeriod &&
		elapsedSinceReboot >= 0 &&
		!lastRebootTime.IsZero() &&
		lastRebootTime.Unix() > 0

	for i, componentState := range currState.States {
		if componentState.Health == apiv1.HealthStateTypeHealthy {
			continue
		}
		if lastRebootTime.IsZero() {
			continue
		}

		if resetToInitializing {
			log.Logger.Warnw(
				"setting unhealthy state to initializing due to recent reboot",
				"component", componentName,
				"lastRebootTime", lastRebootTime,
				"elapsedSinceReboot", elapsedSinceReboot,
				"initializeGracePeriod", initializeGracePeriod,
				"originalHealth", componentState.Health,
			)
			currState.States[i].Health = apiv1.HealthStateTypeInitializing
		}

		if componentState.Error != "" &&
			(strings.Contains(componentState.Error, context.DeadlineExceeded.Error()) ||
				strings.Contains(componentState.Error, context.Canceled.Error())) {
			log.Logger.Errorw("state error due to deadline exceeded or canceled error", "component", componentName, "error", componentState.Error)
		}
	}
	return currState
}
