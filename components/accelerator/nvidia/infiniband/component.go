// Package infiniband monitors the infiniband status of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package infiniband

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

	nvmlInstance   nvidianvml.Instance
	toolOverwrites nvidia_common.ToolOverwrites

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	getIbstatOutputFunc func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error)
	getThresholdsFunc   func() infiniband.ExpectedPortStates

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		nvmlInstance:        gpudInstance.NVMLInstance,
		toolOverwrites:      gpudInstance.NVIDIAToolOverwrites,
		getIbstatOutputFunc: infiniband.GetIbstatOutput,
		getThresholdsFunc:   GetDefaultExpectedPortStates,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		if os.Geteuid() == 0 {
			c.kmsgSyncer, err = kmsg.NewSyncer(cctx, Match, c.eventBucket)
			if err != nil {
				ccancel()
				return nil, err
			}
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

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.HealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.eventBucket == nil {
		return nil, nil
	}
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.kmsgSyncer != nil {
		c.kmsgSyncer.Close()
	}
	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu infiniband")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// nothing specified for this machine, gpud MUST skip the ib check
	thresholds := c.getThresholdsFunc()
	if thresholds.IsZero() {
		cr.reason = reasonThresholdNotSetSkipped
		cr.health = apiv1.HealthStateTypeHealthy
		return cr
	}

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	if c.getIbstatOutputFunc == nil {
		cr.reason = "ibstat checker not found"
		cr.health = apiv1.HealthStateTypeHealthy
		return cr
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	cr.IbstatOutput, cr.err = c.getIbstatOutputFunc(cctx, []string{c.toolOverwrites.IbstatCommand})
	ccancel()
	if cr.err != nil {
		if errors.Is(cr.err, infiniband.ErrNoIbstatCommand) {
			cr.reason = "ibstat command not found"
			cr.health = apiv1.HealthStateTypeHealthy
		} else {
			cr.reason = fmt.Sprintf("ibstat command failed: %v", cr.err)
			cr.health = apiv1.HealthStateTypeUnhealthy
		}
		return cr
	}

	if cr.IbstatOutput == nil {
		cr.reason = reasonMissingIbstatOutput
		cr.health = apiv1.HealthStateTypeHealthy
		return cr
	}

	// no event bucket, no need for timeseries data checks
	// (e.g., "gpud scan" one-off checks)
	if c.eventBucket == nil {
		cr.reason = reasonMissingEventBucket
		cr.health = apiv1.HealthStateTypeHealthy
		return cr
	}

	cr.reason, cr.health = evaluateIbstatOutputAgainstThresholds(cr.IbstatOutput, thresholds)

	// we only care about unhealthy events, no need to persist healthy events
	if cr.health == apiv1.HealthStateTypeHealthy {
		return cr
	}

	// now that event store/bucket is set
	// now that ibstat output has some issues with its thresholds (unhealthy state)
	// we persist such unhealthy state event
	ev := apiv1.Event{
		Time:    metav1.Time{Time: cr.ts},
		Name:    "ibstat",
		Type:    apiv1.EventTypeWarning,
		Message: cr.reason,

		DeprecatedSuggestedActions: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeHardwareInspection,
			},
			DeprecatedDescriptions: []string{
				"potential infiniband switch/hardware issue needs immediate attention",
			},
		},
	}

	// lookup to prevent duplicate event insertions
	cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
	found, err := c.eventBucket.Find(cctx, ev)
	ccancel()
	if err != nil {
		cr.reason = fmt.Sprintf("failed to find ibstat event: %v", err)
		cr.health = apiv1.HealthStateTypeUnhealthy
		return cr
	}

	// already exists, no need to insert
	if found != nil {
		return cr
	}

	// insert event
	cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
	err = c.eventBucket.Insert(cctx, ev)
	ccancel()
	if err != nil {
		cr.reason = fmt.Sprintf("failed to insert ibstat event: %v", err)
		cr.health = apiv1.HealthStateTypeUnhealthy
		return cr
	}

	return cr
}

var (
	// nothing specified for this machine, gpud MUST skip the ib check
	reasonThresholdNotSetSkipped = "ports or rate threshold not set, skipping"

	reasonMissingIbstatOutput = "missing ibstat output (skipped evaluation)"
	reasonMissingEventBucket  = "missing event storage (skipped evaluation)"
	reasonNoIbIssueFound      = "no infiniband issue found (in ibstat)"
)

// Returns the output evaluation reason and its health state.
// We DO NOT auto-detect infiniband devices/PCI buses, strictly rely on the user-specified config.
func evaluateIbstatOutputAgainstThresholds(o *infiniband.IbstatOutput, thresholds infiniband.ExpectedPortStates) (string, apiv1.HealthStateType) {
	if thresholds.IsZero() {
		return reasonThresholdNotSetSkipped, apiv1.HealthStateTypeHealthy
	}

	atLeastPorts := thresholds.AtLeastPorts
	atLeastRate := thresholds.AtLeastRate
	if err := o.Parsed.CheckPortsAndRate(atLeastPorts, atLeastRate); err != nil {
		return err.Error(), apiv1.HealthStateTypeUnhealthy
	}

	return reasonNoIbIssueFound, apiv1.HealthStateTypeHealthy
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	IbstatOutput *infiniband.IbstatOutput `json:"ibstat_output"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if cr.IbstatOutput == nil {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Port Name", "Port1 State", "Port1 Physical State", "Port1 Rate"})
	for _, card := range cr.IbstatOutput.Parsed {
		table.Append([]string{
			card.Name,
			card.Port1.State,
			card.Port1.PhysicalState,
			fmt.Sprintf("%d", card.Port1.Rate),
		})
	}
	table.Render()

	return buf.String()
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
