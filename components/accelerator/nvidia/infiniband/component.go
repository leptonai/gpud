// Package infiniband monitors the infiniband status of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package infiniband

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	nvidia_infiniband_id "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/id"
	"github.com/leptonai/gpud/pkg/common"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

var (
	defaultExpectedPortStatesMu sync.RWMutex
	defaultExpectedPortStates   = infiniband.ExpectedPortStates{
		AtLeastPorts: 0,
		AtLeastRate:  0,
	}
)

func GetDefaultExpectedPortStates() infiniband.ExpectedPortStates {
	defaultExpectedPortStatesMu.RLock()
	defer defaultExpectedPortStatesMu.RUnlock()
	return defaultExpectedPortStates
}

func SetDefaultExpectedPortStates(states infiniband.ExpectedPortStates) {
	log.Logger.Infow("setting default expected port states", "at_least_ports", states.AtLeastPorts, "at_least_rate", states.AtLeastRate)

	defaultExpectedPortStatesMu.Lock()
	defer defaultExpectedPortStatesMu.Unlock()
	defaultExpectedPortStates = states
}

func New(ctx context.Context, eventStore eventstore.Store, toolOverwrites nvidia_common.ToolOverwrites) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(nvidia_infiniband_id.Name)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	logLineProcessor, err := dmesg.NewLogLineProcessor(cctx, Match, eventBucket)
	if err != nil {
		ccancel()
		return nil, err
	}

	kmsgWatcher, err := kmsg.StartWatch(Match)
	if err != nil {
		ccancel()
		return nil, err
	}

	c := &component{
		rootCtx:          cctx,
		cancel:           ccancel,
		eventBucket:      eventBucket,
		logLineProcessor: logLineProcessor,
		toolOverwrites:   toolOverwrites,
		kmsgWatcher:      kmsgWatcher,
	}

	return c, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx          context.Context
	cancel           context.CancelFunc
	eventBucket      eventstore.Bucket
	logLineProcessor *dmesg.LogLineProcessor
	toolOverwrites   nvidia_common.ToolOverwrites

	lastEventMu        sync.Mutex
	lastEvent          *components.Event
	lastEventThreshold infiniband.ExpectedPortStates

	// experimental
	kmsgWatcher kmsg.Watcher
}

func convertToState(ev *components.Event) components.State {
	state := components.State{
		Name:             ev.Name,
		Healthy:          true,
		Health:           components.StateHealthy,
		Reason:           ev.Message,
		SuggestedActions: ev.SuggestedActions,
	}
	if len(ev.ExtraInfo) > 0 {
		state.Healthy = ev.ExtraInfo["state_healthy"] == "true"
		state.Health = ev.ExtraInfo["state_health"]
	}
	return state
}

// check "ibstat" once, and return the last event
// if the last event happened within the last 10 seconds, skip the check and return the cached last event
// if unhealthy ibstat status is found, it persists the unhealthy event in the database
// if a unexpected error is found, it returns the error (regardless of the ibstat status)
func (c *component) checkIbstatOnce(ts time.Time, thresholds infiniband.ExpectedPortStates) (*components.Event, error) {
	if thresholds.AtLeastPorts <= 0 && thresholds.AtLeastRate <= 0 {
		return nil, nil
	}

	ev := components.Event{
		Time:    metav1.Time{Time: ts},
		Name:    "ibstat",
		Type:    common.EventTypeInfo,
		Message: "",
		ExtraInfo: map[string]string{
			"state_healthy": "true",
			"state_health":  components.StateHealthy,
		},
		SuggestedActions: nil,
	}

	c.lastEventMu.Lock()
	defer c.lastEventMu.Unlock()

	// last event already happened within the last 10 seconds, skip the check
	// and no need further check, no need further state persistence
	if c.lastEventThreshold.AtLeastPorts == thresholds.AtLeastPorts &&
		c.lastEventThreshold.AtLeastRate == thresholds.AtLeastRate &&
		c.lastEvent != nil &&
		ts.UTC().Sub(c.lastEvent.Time.Time) < 10*time.Second {
		return c.lastEvent, nil
	}

	cctx, ccancel := context.WithTimeout(c.rootCtx, 15*time.Second)
	o, err := infiniband.GetIbstatOutput(cctx, []string{c.toolOverwrites.IbstatCommand})
	ccancel()

	if err != nil {
		ev.Type = common.EventTypeWarning
		ev.ExtraInfo["state_healthy"] = "false"
		ev.ExtraInfo["state_health"] = components.StateUnhealthy

		if errors.Is(err, infiniband.ErrNoIbstatCommand) {
			ev.Message = fmt.Sprintf("ibstat threshold set but %v", err)
		} else {
			ev.Message = fmt.Sprintf("ibstat command failed: %v", err)
		}
		log.Logger.Warnw("ibstat command failed", "reason", ev.Message)
	} else {
		reason, healthy, err := evaluate(o, thresholds)
		if err != nil {
			log.Logger.Warnw("ibstat evaluate error", "error", err)
			return nil, err
		}
		ev.Message = reason

		if healthy {
			ev.Type = common.EventTypeInfo
			ev.ExtraInfo["state_healthy"] = "true"
			ev.ExtraInfo["state_health"] = components.StateHealthy
		} else {
			ev.Type = common.EventTypeWarning
			ev.ExtraInfo["state_healthy"] = "false"
			ev.ExtraInfo["state_health"] = components.StateUnhealthy

			ev.SuggestedActions = &common.SuggestedActions{
				RepairActions: []common.RepairActionType{
					common.RepairActionTypeHardwareInspection,
				},
				Descriptions: []string{
					"potential infiniband switch/hardware issue needs immediate attention",
				},
			}

			log.Logger.Warnw("ibstat issue found", "reason", reason, "output", o.Raw)
		}
	}

	c.lastEvent = &ev
	c.lastEventThreshold = thresholds

	// we only care about unhealthy events
	if ev.Type == common.EventTypeInfo {
		return c.lastEvent, nil
	}

	// lookup to prevent duplicate event insertions
	cctx, ccancel = context.WithTimeout(c.rootCtx, 15*time.Second)
	found, err := c.eventBucket.Find(cctx, ev)
	ccancel()
	if err != nil {
		return nil, err
	}
	if found != nil {
		return c.lastEvent, nil
	}

	// insert event
	cctx, ccancel = context.WithTimeout(c.rootCtx, 15*time.Second)
	err = c.eventBucket.Insert(cctx, ev)
	ccancel()
	if err != nil {
		return nil, err
	}

	return c.lastEvent, nil
}

func (c *component) Name() string { return nvidia_infiniband_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return c.getStates(ctx, time.Now().UTC(), GetDefaultExpectedPortStates())
}

var noDataEvents = []components.State{
	{
		Name:    "ibstat",
		Health:  components.StateHealthy,
		Healthy: true, //TODO: depreciate Healthy field
		Reason:  msgThresholdNotSetSkipped,
	},
}

func (c *component) getStates(ctx context.Context, now time.Time, thresholds infiniband.ExpectedPortStates) ([]components.State, error) {
	// in rare cases, some machines have "ibstat" installed that returns empty output
	// not failing the ibstat check, thus we need manual check on the thresholds here
	// before we call the ibstat command
	if thresholds.AtLeastPorts <= 0 && thresholds.AtLeastRate <= 0 {
		return noDataEvents, nil
	}

	lastEvent, err := c.checkIbstatOnce(now.UTC(), thresholds)
	if err != nil {
		return nil, err
	}
	if lastEvent == nil {
		var err error
		cctx, ccancel := context.WithTimeout(ctx, 15*time.Second)
		lastEvent, err = c.eventBucket.Latest(cctx)
		ccancel()
		if err != nil {
			return nil, err
		}
	}
	if lastEvent == nil {
		return noDataEvents, nil
	}

	return []components.State{convertToState(lastEvent)}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	thresholds := GetDefaultExpectedPortStates()
	if _, err := c.checkIbstatOnce(time.Now().UTC(), thresholds); err != nil {
		return nil, err
	}
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()

	c.eventBucket.Close()

	return nil
}

var (
	msgThresholdNotSetSkipped = "ports or rate threshold not set, skipping"
	msgNoIbIssueFound         = "no infiniband issue found (in ibstat)"
)

// Returns the output evaluation reason and its healthy-ness.
// We DO NOT auto-detect infiniband devices/PCI buses, strictly rely on the user-specified config.
func evaluate(o *infiniband.IbstatOutput, cfg infiniband.ExpectedPortStates) (string, bool, error) {
	// nothing specified for this machine, gpud MUST skip the ib check
	if cfg.AtLeastPorts <= 0 && cfg.AtLeastRate <= 0 {
		return msgThresholdNotSetSkipped, true, nil
	}

	atLeastPorts := cfg.AtLeastPorts
	atLeastRate := cfg.AtLeastRate
	if err := o.Parsed.CheckPortsAndRate(atLeastPorts, atLeastRate); err != nil {
		return err.Error(), false, nil
	}

	return msgNoIbIssueFound, true, nil
}
