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
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const (
	Name = "accelerator-nvidia-infiniband"

	EventNameInfinibandPortStates = "infiniband-port-states"
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance   nvidianvml.Instance
	toolOverwrites pkgconfigcommon.ToolOverwrites

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	getThresholdsFunc   func() infiniband.ExpectedPortStates
	getClassDevicesFunc func() (infinibandclass.Devices, error)
	getIbstatOutputFunc func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:               cctx,
		cancel:            ccancel,
		nvmlInstance:      gpudInstance.NVMLInstance,
		toolOverwrites:    gpudInstance.NVIDIAToolOverwrites,
		getThresholdsFunc: GetDefaultExpectedPortStates,
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.LoadDevices(infinibandclass.DefaultRootDir)
		},
		getIbstatOutputFunc: infiniband.GetIbstatOutput,
	}

	if gpudInstance.EventStore != nil {
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
	if c.eventBucket == nil {
		return nil, nil
	}
	evs, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	return evs.Events(), nil
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

	var err error
	cr.ClassDevices, err = c.getClassDevicesFunc()
	if err != nil {
		log.Logger.Warnw("error loading infiniband class devices", "error", err)
	}

	if len(cr.ClassDevices) > 0 {
		for _, dev := range cr.ClassDevices {
			for _, port := range dev.Ports {
				if port.Counters.LinkDowned == nil {
					continue
				}
				v := float64(*port.Counters.LinkDowned)

				devPort := dev.Name + "_" + port.Name
				metricIbLinkedDowned.With(prometheus.Labels{"device_port": devPort}).Set(v)
			}
		}
	}

	// "ibstat" may fail if there's a port device that is wrongly mapped (e.g., exit 255)
	// but can still return the partial output with the correct data
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	cr.IbstatOutput, cr.err = c.getIbstatOutputFunc(cctx, []string{c.toolOverwrites.IbstatCommand})
	ccancel()

	if cr.err != nil {
		if errors.Is(cr.err, infiniband.ErrNoIbstatCommand) {
			cr.health = apiv1.HealthStateTypeHealthy
			cr.reason = "ibstat command not found"
		} else {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "ibstat command failed"
			log.Logger.Warnw(cr.reason, "error", cr.err)
		}
	}

	// no event bucket, no need for timeseries data checks
	// (e.g., "gpud scan" one-off checks)
	if c.eventBucket == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonMissingEventBucket
		return cr
	}

	// "ibstat" command returned no data, skip the evaluation
	if cr.err == nil && cr.IbstatOutput == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonMissingIbstatOutput
		log.Logger.Warnw(cr.reason)
		return cr
	}

	// whether ibstat command failed or not, we use the entire/partial output
	if cr.IbstatOutput != nil {
		cr.allIBPortsFromIbstat = cr.IbstatOutput.Parsed.IBPorts()
	}
	evaluateThresholds(cr, thresholds)

	// record events whether ib ports are healthy or not
	// as we use historical data including healthy ports
	// to evaluate ib port drop and flap
	if err := c.recordIbEvent(cr); err != nil {
		return cr
	}

	return cr
}

var (
	// nothing specified for this machine, gpud MUST skip the ib check
	reasonThresholdNotSetSkipped = "ports or rate threshold not set, skipping"

	reasonMissingIbstatOutput      = "missing ibstat output (skipped evaluation)"
	reasonMissingEventBucket       = "missing event storage (skipped evaluation)"
	reasonNoIbIssueFoundFromIbstat = "no infiniband issue found (in ibstat)"
)

func evaluateThresholds(cr *checkResult, thresholds infiniband.ExpectedPortStates) {
	// DO NOT auto-detect infiniband devices/PCI buses, strictly rely on the user-specified config.
	if thresholds.IsZero() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.suggestedActions = nil
		cr.unhealthyIBPorts = nil
		cr.reason = reasonThresholdNotSetSkipped

		cr.err = nil
		return
	}

	// neither "ibstat" nor "ibstatus" command returned any data
	// then we just skip the evaluation
	if len(cr.allIBPortsFromIbstat) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonMissingIbstatOutput
		log.Logger.Warnw(cr.reason)
		return
	}

	// Link down/drop -> hardware inspection
	// Link port flap -> hardware inspection
	atLeastPorts := thresholds.AtLeastPorts
	atLeastRate := thresholds.AtLeastRate

	// whether ibstat command failed or not (e.g., one port device is wrongly mapped), we use the entire/partial output
	// but we got the entire/partial output from "ibstat" command
	// thus we use the data from "ibstat" command to evaluate
	// ok to error as long as it meets the thresholds
	// which means we may overwrite the error above
	// (e.g., "ibstat" command exited 255 but still meets the thresholds)
	unhealthy, err := infiniband.EvaluatePortsAndRate(cr.allIBPortsFromIbstat, atLeastPorts, atLeastRate)
	if err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
		}
		cr.reason = err.Error()

		cr.unhealthyIBPorts = unhealthy
		return
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.suggestedActions = nil
	cr.reason = reasonNoIbIssueFoundFromIbstat
	cr.unhealthyIBPorts = nil

	// partial output from "ibstat" command worked
	if cr.err != nil && cr.health == apiv1.HealthStateTypeHealthy {
		log.Logger.Debugw("ibstat command returned partial output -- discarding error", "error", cr.err, "reason", cr.reason)
		cr.err = nil
	}
}

func (cr *checkResult) convertToIbstatEvent() *eventstore.Event {
	if cr == nil {
		return nil
	}

	ibportsEncoded := []byte("[]")
	if len(cr.allIBPortsFromIbstat) > 0 {
		all := make([]infiniband.IBPort, 0)
		for _, port := range cr.allIBPortsFromIbstat {
			all = append(all, infiniband.IBPort{
				Device: port.Device,
				State:  port.State,

				// we do not need the physical state and rate
				// as we only use the device name to evaluate ib port drop and flap
				// this saves the space in db file
				PhysicalState: "",
				RateGBSec:     0,
			})
		}
		ibportsEncoded, _ = json.Marshal(all)
	}

	eventType := string(apiv1.EventTypeInfo)
	if len(cr.unhealthyIBPorts) > 0 {
		eventType = string(apiv1.EventTypeWarning)
	}

	return &eventstore.Event{
		Component: Name,
		Time:      cr.ts,
		Name:      EventNameInfinibandPortStates,
		Type:      eventType,
		Message:   cr.reason,
		ExtraInfo: map[string]string{
			"all_ibports": string(ibportsEncoded),
		},
	}
}

func (c *component) recordIbEvent(cr *checkResult) error {
	ev := cr.convertToIbstatEvent()
	if ev == nil {
		return nil
	}

	// lookup to prevent duplicate event insertions
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	found, err := c.eventBucket.Find(cctx, *ev)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error finding ibstat event"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return err
	}

	// already exists, no need to insert
	if found != nil {
		return nil
	}

	// insert event
	cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
	cr.err = c.eventBucket.Insert(cctx, *ev)
	ccancel()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error inserting ibstat event"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr.err
	}

	return nil
}

// evaluateIbstatOutputAgainstThresholds is a helper function for testing
// that wraps the evaluateThresholds function and returns the values expected by tests
func evaluateIbstatOutputAgainstThresholds(output *infiniband.IbstatOutput, config infiniband.ExpectedPortStates) (apiv1.HealthStateType, *apiv1.SuggestedActions, string) {
	cr := &checkResult{
		IbstatOutput: output,
		ts:           time.Now().UTC(),
	}

	if output != nil {
		cr.allIBPortsFromIbstat = output.Parsed.IBPorts()
	}

	evaluateThresholds(cr, config)

	return cr.health, cr.suggestedActions, cr.reason
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ClassDevices infinibandclass.Devices  `json:"class_devices"`
	IbstatOutput *infiniband.IbstatOutput `json:"ibstat_output"`

	allIBPortsFromIbstat []infiniband.IBPort

	// current unhealthy ib ports that are problematic
	// (down/polling/disabled, below expected ib port thresholds)
	unhealthyIBPorts []infiniband.IBPort

	// timestamp of the last check
	ts time.Time
	// error from the last check with "ibstat" command and other operations
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
	if cr.IbstatOutput == nil {
		return "no data"
	}

	out := ""

	if len(cr.ClassDevices) > 0 {
		buf := bytes.NewBuffer(nil)
		cr.ClassDevices.RenderTable(buf)

		out += buf.String() + "\n\n"
	}

	if cr.IbstatOutput != nil {
		buf := bytes.NewBuffer(nil)
		table := tablewriter.NewWriter(buf)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader([]string{"Port Device Name", "Port1 State", "Port1 Physical State", "Port1 Rate"})
		for _, card := range cr.IbstatOutput.Parsed {
			table.Append([]string{
				card.Device,
				card.Port1.State,
				card.Port1.PhysicalState,
				fmt.Sprintf("%d", card.Port1.Rate),
			})
		}
		table.Render()

		out += buf.String() + "\n\n"
	}

	return out
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
		Reason:           cr.reason,
		SuggestedActions: cr.getSuggestedActions(),
		Error:            cr.getError(),
		Health:           cr.health,
	}

	if cr.IbstatOutput != nil {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
