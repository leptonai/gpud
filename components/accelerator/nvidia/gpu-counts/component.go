// Package gpucounts monitors the GPU count of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package gpucounts

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	nvidiaquery "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/olekukonko/tablewriter"
)

const (
	Name = "accelerator-nvidia-gpu-counts"

	EventNameMisMatch = "gpu-count-mismatch"
)

const defaultLookbackPeriod = 3 * 24 * time.Hour

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance

	rebootEventStore pkghost.RebootEventStore
	eventBucket      eventstore.Bucket

	getCountLspci     func(ctx context.Context) (int, error)
	getThresholdsFunc func() ExpectedGPUCounts

	// lookback period to query the past reboot + gpu count mismatch events
	lookbackPeriod time.Duration

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstance: gpudInstance.NVMLInstance,

		rebootEventStore: gpudInstance.RebootEventStore,

		getCountLspci: func(ctx context.Context) (int, error) {
			devs, err := nvidiaquery.ListPCIGPUs(ctx)
			if err != nil {
				return 0, err
			}
			return len(devs), nil
		},
		getThresholdsFunc: GetDefaultExpectedGPUCounts,

		lookbackPeriod: defaultLookbackPeriod,
	}

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name, eventstore.WithDisablePurge())
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}
}

func (c *component) IsSupported() bool {
	if c.nvmlInstance == nil {
		return false
	}
	return c.nvmlInstance.NVMLExists() && c.nvmlInstance.ProductName() != ""
}

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
	// gpu count mismatch events are ONLY used internally within this package
	// solely to evaluate the suggested actions
	// so we don't need to return any events externally
	return nil, nil
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
	log.Logger.Infow("checking nvidia gpu counts")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}

	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

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

	cr.ProductName = c.nvmlInstance.ProductName()
	cr.CountNVML = len(c.nvmlInstance.Devices())

	// nothing specified for this machine, gpud MUST skip the gpu count check
	thresholds := c.getThresholdsFunc()
	if thresholds.IsZero() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonThresholdNotSetSkipped
		return cr
	}

	// just for information/debugging purposes
	if c.getCountLspci != nil {
		var err error
		cr.CountLspci, err = c.getCountLspci(c.ctx)
		if err != nil {
			log.Logger.Warnw("error getting count of lspci", "error", err)
		} else {
			log.Logger.Infow("count of lspci", "count", cr.CountLspci)
		}
	}

	if cr.CountNVML == thresholds.Count {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("nvidia gpu count matching thresholds (%d)", thresholds.Count)
		return cr
	}

	cr.health = apiv1.HealthStateTypeUnhealthy
	cr.reason = fmt.Sprintf("nvidia gpu count mismatch (found %d, expected %d)", cr.CountNVML, thresholds.Count)
	log.Logger.Warnw(cr.reason, "count_lspci", cr.CountLspci, "count_nvml", cr.CountNVML, "expected", thresholds.Count)

	if c.eventBucket == nil {
		// no event store, skip lookups, skip setting up suggested actions
		return cr
	}

	// now that we found the GPU count mismatch event
	// we need to
	// 1. persist in event store (if it hasn't been)
	// 2. look up past events to derive the suggested actions
	// 3. evaluate the suggested actions
	if err := c.recordMismatchEvent(cr); err != nil {
		return cr
	}

	// look up past events to derive the suggested actions
	rebootEvents, err := c.rebootEventStore.GetRebootEvents(c.ctx, cr.ts.Add(-c.lookbackPeriod))
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}
	gpuMismatchEvents, err := c.eventBucket.Get(c.ctx, cr.ts.Add(-c.lookbackPeriod))
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}

	// evaluate the suggested actions (along with the reboot history)
	if len(gpuMismatchEvents) > 0 {
		cr.suggestedActions = eventstore.EvaluateSuggestedActions(rebootEvents, gpuMismatchEvents, 2)
	}

	return cr
}

func (c *component) recordMismatchEvent(cr *checkResult) error {
	ev := eventstore.Event{
		Component: Name,
		Time:      cr.ts,
		Name:      EventNameMisMatch,
		Type:      string(apiv1.EventTypeWarning),
		Message:   cr.reason,
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	found, err := c.eventBucket.Find(cctx, ev)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error finding gpu count mismatch event"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return err
	}

	if found != nil {
		log.Logger.Infow("gpu count mismatch event already found in db")
		return nil
	}

	// persist in event store (as it hasn't been)
	if err := c.eventBucket.Insert(c.ctx, ev); err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error inserting gpu count mismatch event"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return err
	}

	log.Logger.Infow("inserted gpu count mismatch event to db")
	return nil
}

// suggests up to reboot up to twice, otherwise, suggest hw inspection
//
// case 0.
// gpu count mismatch* -> reboot
// -> no gpu count mismatch;
// whether there was mismatch in the past, if there's no mismatch now,
// no action needed (reboot may have resolved the issue)
//
// case 1. gpu count mismatch (first time); suggest "reboot"
// (no previous reboot found)
//
// case 2.
// gpu count mismatch -> reboot
// -> gpu count mismatch; suggest second "reboot"
// (after first reboot, we still get gpu count mismatch)
//
// case 3.
// gpu count mismatch -> reboot
// -> gpu count mismatch -> reboot
// -> gpu count mismatch; suggest "hw inspection"
// (after >=2 reboots, we still get gpu count mismatch)

const reasonThresholdNotSetSkipped = "GPU count thresholds not set, skipping"

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ProductName string `json:"product_name"`
	CountLspci  int    `json:"count_lspci"`
	CountNVML   int    `json:"count_nvml"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the suggested actions for the last check
	suggestedActions *apiv1.SuggestedActions
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if cr.ProductName == "" {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"Product Name", cr.ProductName})
	table.Append([]string{"Count lspci", fmt.Sprintf("%d", cr.CountLspci)})
	table.Append([]string{"Count NVML", fmt.Sprintf("%d", cr.CountNVML)})
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

func (cr *checkResult) getSuggestedActions() *apiv1.SuggestedActions {
	if cr == nil {
		return nil
	}
	return cr.suggestedActions
}

func (cr *checkResult) getError() string {
	if cr == nil {
		return ""
	}
	if cr.err != nil {
		return cr.err.Error()
	}
	return ""
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:             metav1.NewTime(cr.ts),
		Component:        Name,
		Name:             Name,
		Health:           cr.health,
		Reason:           cr.reason,
		Error:            cr.getError(),
		SuggestedActions: cr.getSuggestedActions(),
	}
	return apiv1.HealthStates{state}
}
