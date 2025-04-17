// Package infiniband monitors the infiniband status of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package infiniband

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/olekukonko/tablewriter"
)

const Name = "accelerator-nvidia-infiniband"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance   nvidianvml.InstanceV2
	toolOverwrites nvidia_common.ToolOverwrites

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	getIbstatOutputFunc func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error)

	lastMu   sync.RWMutex
	lastData *Data

	lastEventMu        sync.Mutex
	lastEvent          *apiv1.Event
	lastEventThreshold infiniband.ExpectedPortStates
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		nvmlInstance:        gpudInstance.NVMLInstance,
		toolOverwrites:      gpudInstance.NVIDIAToolOverwrites,
		getIbstatOutputFunc: infiniband.GetIbstatOutput,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		c.kmsgSyncer, err = kmsg.NewSyncer(cctx, Match, c.eventBucket)
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) HealthStates(ctx context.Context) (apiv1.HealthStates, error) {
	return c.getHealthStates(ctx, time.Now().UTC(), GetDefaultExpectedPortStates())
}

var noDataEvents = apiv1.HealthStates{
	{
		Name:   "ibstat",
		Health: apiv1.StateTypeHealthy,
		Reason: msgThresholdNotSetSkipped,
	},
}

func (c *component) getHealthStates(ctx context.Context, now time.Time, thresholds infiniband.ExpectedPortStates) (apiv1.HealthStates, error) {
	// in rare cases, some machines have "ibstat" installed that returns empty output
	// not failing the ibstat check, thus we need manual check on the thresholds here
	// before we call the ibstat command
	if thresholds.AtLeastPorts <= 0 && thresholds.AtLeastRate <= 0 {
		return noDataEvents, nil
	}

	lastEvent, err := c.checkOnceIbstat(now.UTC(), thresholds)
	if err != nil {
		return nil, err
	}
	if lastEvent == nil {
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

	return apiv1.HealthStates{convertToState(lastEvent)}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	thresholds := GetDefaultExpectedPortStates()
	if _, err := c.checkOnceIbstat(time.Now().UTC(), thresholds); err != nil {
		return nil, err
	}
	if c.eventBucket == nil {
		return nil, nil
	}
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu nccl")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil || !c.nvmlInstance.NVMLExists() {
		d.reason = "NVIDIA NVML is not loaded"
		d.health = apiv1.StateTypeHealthy
		return d
	}

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (d *Data) String() string {
	if d == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.Render()

	return buf.String()
}

func (d *Data) Summary() string {
	if d == nil {
		return ""
	}
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	return d.health
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getLastHealthStates() apiv1.HealthStates {
	if d == nil {
		return apiv1.HealthStates{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),
		Health: d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}

func convertToState(ev *apiv1.Event) apiv1.HealthState {
	state := apiv1.HealthState{
		Name:             ev.Name,
		Health:           apiv1.StateTypeHealthy,
		Reason:           ev.Message,
		SuggestedActions: ev.DeprecatedSuggestedActions,
	}
	if len(ev.DeprecatedExtraInfo) > 0 {
		state.Health = apiv1.HealthStateType(ev.DeprecatedExtraInfo["state_health"])
	}
	return state
}

// check "ibstat" once, and return the last event
// if the last event happened within the last 10 seconds, skip the check and return the cached last event
// if unhealthy ibstat status is found, it persists the unhealthy event in the database
// if a unexpected error is found, it returns the error (regardless of the ibstat status)
func (c *component) checkOnceIbstat(ts time.Time, thresholds infiniband.ExpectedPortStates) (*apiv1.Event, error) {
	if thresholds.AtLeastPorts <= 0 && thresholds.AtLeastRate <= 0 {
		return nil, nil
	}

	ev := apiv1.Event{
		Time:    metav1.Time{Time: ts},
		Name:    "ibstat",
		Type:    apiv1.EventTypeInfo,
		Message: "",
		DeprecatedExtraInfo: map[string]string{
			"state_healthy": "true",
			"state_health":  string(apiv1.StateTypeHealthy),
		},
		DeprecatedSuggestedActions: nil,
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

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	o, err := c.getIbstatOutputFunc(cctx, []string{c.toolOverwrites.IbstatCommand})
	ccancel()

	if err != nil {
		ev.Type = apiv1.EventTypeWarning
		ev.DeprecatedExtraInfo["state_healthy"] = "false"
		ev.DeprecatedExtraInfo["state_health"] = string(apiv1.StateTypeUnhealthy)

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
			ev.Type = apiv1.EventTypeInfo
			ev.DeprecatedExtraInfo["state_healthy"] = "true"
			ev.DeprecatedExtraInfo["state_health"] = string(apiv1.StateTypeHealthy)
		} else {
			ev.Type = apiv1.EventTypeWarning
			ev.DeprecatedExtraInfo["state_healthy"] = "false"
			ev.DeprecatedExtraInfo["state_health"] = string(apiv1.StateTypeUnhealthy)

			ev.DeprecatedSuggestedActions = &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeHardwareInspection,
				},
				DeprecatedDescriptions: []string{
					"potential infiniband switch/hardware issue needs immediate attention",
				},
			}

			log.Logger.Warnw("ibstat issue found", "reason", reason, "output", o.Raw)
		}
	}

	c.lastEvent = &ev
	c.lastEventThreshold = thresholds

	// we only care about unhealthy events
	if ev.Type == apiv1.EventTypeInfo {
		return c.lastEvent, nil
	}

	// lookup to prevent duplicate event insertions
	cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
	found, err := c.eventBucket.Find(cctx, ev)
	ccancel()
	if err != nil {
		return nil, err
	}
	if found != nil {
		return c.lastEvent, nil
	}

	// insert event
	cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
	err = c.eventBucket.Insert(cctx, ev)
	ccancel()
	if err != nil {
		return nil, err
	}

	return c.lastEvent, nil
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
