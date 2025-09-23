package session

import (
	"context"
	"errors"
	"strings"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/errdefs"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
)

func (s *Session) getHealthStates(payload Request) (apiv1.GPUdComponentHealthStates, error) {
	if payload.Method != "states" {
		return nil, errors.New("mismatch method")
	}

	allComponents := s.components
	if len(payload.Components) > 0 {
		allComponents = payload.Components
	}

	// use BootTimeUnixSeconds which reads directly from system sources
	// avoiding timezone parsing issues with "uptime -s"
	bootTimeUnix := pkghost.BootTimeUnixSeconds()
	rebootTime := time.Unix(int64(bootTimeUnix), 0)

	// sanity check: if boot time is 0 or in the future, use zero time
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

	statesBuf := make(chan apiv1.ComponentHealthStates, len(allComponents))
	for _, componentName := range allComponents {
		go func(name string) {
			statesBuf <- s.getHealthStatesFromComponent(name, rebootTime)
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

// getComponentFunc is a function type for getting a component by name
type getComponentFunc func(string) components.Component

func (s *Session) getHealthStatesFromComponent(componentName string, lastRebootTime time.Time) apiv1.ComponentHealthStates {
	return getHealthStatesFromComponentWithDeps(
		componentName,
		lastRebootTime,
		s.componentsRegistry.Get,
	)
}

func getHealthStatesFromComponentWithDeps(
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
