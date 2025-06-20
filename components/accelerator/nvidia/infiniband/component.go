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
	"sort"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dustin/go-humanize"
	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
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
	toolOverwrites pkgconfigcommon.ToolOverwrites

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	getIbstatOutputFunc   func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error)
	getIbstatusOutputFunc func(ctx context.Context, ibstatusCommands []string) (*infiniband.IbstatusOutput, error)
	getThresholdsFunc     func() infiniband.ExpectedPortStates

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:                   cctx,
		cancel:                ccancel,
		nvmlInstance:          gpudInstance.NVMLInstance,
		toolOverwrites:        gpudInstance.NVIDIAToolOverwrites,
		getIbstatOutputFunc:   infiniband.GetIbstatOutput,
		getIbstatusOutputFunc: infiniband.GetIbstatusOutput,
		getThresholdsFunc:     GetDefaultExpectedPortStates,
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

	filtered := make(apiv1.Events, 0)
	for _, ev := range evs.Events() {
		// skip healthy ibport events
		// that are only used for ib flapping evaluation
		if ev.Name == "ibstat" && ev.Type == apiv1.EventTypeInfo {
			continue
		}
		filtered = append(filtered, ev)
	}
	return filtered, nil
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
	log.Logger.Infow("checking nvidia infiniband")

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

	if c.getIbstatOutputFunc == nil || c.getIbstatusOutputFunc == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "ibstat checker not found"
		return cr
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	cr.IbstatusOutput, cr.errIbstatus = c.getIbstatusOutputFunc(cctx, []string{c.toolOverwrites.IbstatusCommand})
	ccancel()
	if cr.errIbstatus != nil {
		// this fallback is only used when the "ibstat" command fails
		// then we don't care if this fallback "ibstatus" command fails
		// as long as the following "ibstat" command succeeds
		log.Logger.Warnw("ibstatus command failed", "error", cr.errIbstatus)
	}

	// "ibstat" may fail if there's a port device that is wrongly mapped (e.g., exit 255)
	// but can still return the partial output with the correct data
	// if there's any partial data, we should use it
	// and only fallback to "ibstatus" if there's no data from "ibstat"
	cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
	cr.IbstatOutput, cr.err = c.getIbstatOutputFunc(cctx, []string{c.toolOverwrites.IbstatCommand})
	ccancel()

	if cr.err != nil {
		if errors.Is(cr.err, infiniband.ErrNoIbstatCommand) {
			cr.health = apiv1.HealthStateTypeHealthy
			cr.reason = "ibstat command not found"
		} else {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "ibstat command failed"
			log.Logger.Errorw(cr.reason, "error", cr.err)
		}
	}

	// no event bucket, no need for timeseries data checks
	// (e.g., "gpud scan" one-off checks)
	if c.eventBucket == nil {
		cr.reason = reasonMissingEventBucket
		cr.health = apiv1.HealthStateTypeHealthy
		return cr
	}

	if cr.IbstatOutput != nil {
		cr.allIBPorts = cr.IbstatOutput.Parsed.IBPorts()
	} else if cr.IbstatusOutput != nil {
		cr.allIBPorts = cr.IbstatusOutput.Parsed.IBPorts()
	}
	evaluateThresholds(cr, thresholds)

	// record events whether ib ports are healthy or not
	// as we use historical data including healthy ports
	// to evaluate ib port drop and flap
	if err := c.recordIbEvent(cr); err != nil {
		return cr
	}

	c.evaluateIbSwitchFault(cr)
	c.evaluateIbPortDrop(cr)
	c.evaluateIbPortFlap(cr)

	return cr
}

var (
	// nothing specified for this machine, gpud MUST skip the ib check
	reasonThresholdNotSetSkipped = "ports or rate threshold not set, skipping"

	reasonMissingIbstatIbstatusOutput = "missing ibstat/ibstatus output (skipped evaluation)"
	reasonMissingEventBucket          = "missing event storage (skipped evaluation)"
	reasonNoIbIssueFoundFromIbstat    = "no infiniband issue found (in ibstat/ibstatus)"
)

func evaluateThresholds(cr *checkResult, thresholds infiniband.ExpectedPortStates) {
	// DO NOT auto-detect infiniband devices/PCI buses, strictly rely on the user-specified config.
	if thresholds.IsZero() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.suggestedActions = nil
		cr.unhealthyIBPorts = nil
		cr.reason = reasonThresholdNotSetSkipped

		cr.err = nil
		cr.errIbstatus = nil
		return
	}

	// neither "ibstat" nor "ibstatus" command returned any data
	// then we just skip the evaluation
	if len(cr.allIBPorts) == 0 {
		cr.reason = reasonMissingIbstatIbstatusOutput
		cr.health = apiv1.HealthStateTypeHealthy
		log.Logger.Errorw(cr.reason)
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
	unhealthy, err := infiniband.EvaluatePortsAndRate(cr.allIBPorts, atLeastPorts, atLeastRate)
	if err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
		}
		cr.unhealthyIBPorts = unhealthy
		cr.reason = err.Error()
		return
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.suggestedActions = nil
	cr.unhealthyIBPorts = nil
	cr.reason = reasonNoIbIssueFoundFromIbstat

	// partial output from "ibstat" command worked
	if cr.err != nil && cr.health == apiv1.HealthStateTypeHealthy {
		log.Logger.Debugw("ibstat command returned partial output -- discarding error", "error", cr.err, "reason", cr.reason)
		cr.err = nil
		cr.errIbstatus = nil
	}
}

func (cr *checkResult) convertToIbstatEvent() *eventstore.Event {
	if cr == nil {
		return nil
	}

	ibportsEncoded := []byte("[]")
	if len(cr.allIBPorts) > 0 {
		all := make([]infiniband.IBPort, 0)
		for _, port := range cr.allIBPorts {
			all = append(all, infiniband.IBPort{
				Device: port.Device,
				State:  port.State,

				// we do not need the physical state and rate
				// as we only use the device name to evaluate ib port drop and flap
				// this saves the space in db file
				PhysicalState: "",
				Rate:          0,
			})
		}
		ibportsEncoded, _ = json.Marshal(all)
	}

	eventType := string(apiv1.EventTypeInfo)
	if len(cr.unhealthyIBPorts) > 0 {
		eventType = string(apiv1.EventTypeWarning)
	}

	return &eventstore.Event{
		Time:    cr.ts,
		Name:    "ibstat",
		Type:    eventType,
		Message: cr.reason,
		ExtraInfo: map[string]string{
			"all_ibports": string(ibportsEncoded),
		},
	}
}

func parseIBPortsFromEvent(ev eventstore.Event) []infiniband.IBPort {
	if ev.Name != "ibstat" {
		return nil
	}

	raw := ev.ExtraInfo["all_ibports"]
	if raw == "" {
		return nil
	}

	var ports []infiniband.IBPort
	if err := json.Unmarshal([]byte(raw), &ports); err != nil {
		log.Logger.Errorw("error unmarshalling ib ports", "error", err)
		return nil
	}

	return ports
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
		log.Logger.Errorw(cr.reason, "error", cr.err)
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
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr.err
	}

	return nil
}

// readAllIbstatEvents reads all ibstat events from the event store
// and returns them in ascending order by time
// including the healthy ib port events
func (c *component) readAllIbstatEvents(since time.Time) ([]eventstore.Event, error) {
	if c.eventBucket == nil {
		return nil, nil
	}

	// (events are sorted by time ascending, latest event is the last one)
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	events, err := c.eventBucket.Get(cctx, since)
	ccancel()
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}

	ibstatEvents := make([]eventstore.Event, 0)
	for _, ev := range events {
		if ev.Name != "ibstat" {
			continue
		}
		ibstatEvents = append(ibstatEvents, ev)
	}

	if len(ibstatEvents) == 0 {
		return nil, nil
	}

	return ibstatEvents, nil
}

// evaluateIbSwitchFault evaluates whether the check result is caused by
// the ib switch fault, where all ports are down
// if that's the case, it sets the field [checkResult.reasonIbSwitchFault]
func (c *component) evaluateIbSwitchFault(cr *checkResult) {
	if cr == nil {
		return
	}

	if cr.health == apiv1.HealthStateTypeHealthy {
		// currently no unhealthy port, thus assume no ib switch fault
		return
	}

	if len(cr.unhealthyIBPorts) == 0 {
		// currently no unhealthy port, thus assume no ib switch fault
		return
	}

	// need to check total number of ports from the output
	var totalPorts int
	if cr.IbstatOutput != nil {
		totalPorts = len(cr.IbstatOutput.Parsed)
	} else if cr.IbstatusOutput != nil {
		totalPorts = len(cr.IbstatusOutput.Parsed)
	}

	if totalPorts == 0 || len(cr.unhealthyIBPorts) != totalPorts {
		// maybe some ports are down, but not all ports are down
		// thus assume no ib switch fault
		return
	}

	cr.reasonIbSwitchFault = "ib switch fault, all ports down"
}

// evaluateIbPortDrop evaluates whether the check result is caused by
// the ib ports being down for more than 4 minutes
// it uses the historical data in the event store to evaluate the ib port drop
// if that's the case, it sets the field [checkResult.reasonIbPortDrop]
func (c *component) evaluateIbPortDrop(cr *checkResult) {
	if cr == nil {
		return
	}

	if cr.health == apiv1.HealthStateTypeHealthy {
		// currently no unhealthy port, thus assume no ib port drop
		// impossible to have ports down more than 4 minutes since now all ports are healthy
		return
	}

	if cr.ts.IsZero() {
		// current check result timestamp is unknown, can't evaluate
		return
	}

	if c.eventBucket == nil {
		// no event bucket, can't evaluate
		return
	}

	// query the last 4 minutes with some buffer
	// since we only check once per minute
	// (events are sorted by time ascending, latest event is the last one)
	since := cr.ts.Add(-10 * time.Minute)
	ibstatEvents, err := c.readAllIbstatEvents(since)
	if err != nil {
		log.Logger.Errorw("error reading ibstat events", "error", err)
		return
	}
	if len(ibstatEvents) == 0 {
		// no unhealthy port event in the last 4 minutes
		// thus safe to assume no ib port drop
		return
	}
	if len(ibstatEvents) == 1 && cr.ts == ibstatEvents[0].Time {
		// read the one that we just inserted
		return
	}

	// maps from port device name to the time when the port first dropped
	droppedSince := make(map[string]time.Time)
	for _, ev := range ibstatEvents {
		allPorts := parseIBPortsFromEvent(ev)
		for _, port := range allPorts {
			// delete in for-loop, because the later one in the entry
			// is the latest one, thus, if the latest event says this port is up
			// we should delete the entry from the map since it's not down anymore
			if port.State != "Down" {
				delete(droppedSince, port.Device)
				continue
			}

			// only track the first time the port dropped
			if _, ok := droppedSince[port.Device]; !ok {
				droppedSince[port.Device] = ev.Time
			}
		}
	}

	// now all entries in "dropSince" are the ports that are STILL down
	// now we have the ib port drop that lasted >= 4 minutes
	// collect more detailed information
	msgs := make([]string, 0)
	for dev, ts := range droppedSince {
		elapsed := cr.ts.Sub(ts)
		if elapsed < 0 {
			// something wrong with the event store
			log.Logger.Warnw("unexpected event timestamp", "checkResultTimestamp", cr.ts, "eventTimestamp", ibstatEvents[0].Time)
			continue
		}

		if elapsed < 4*time.Minute {
			// some ports are down, but only down for less than 4 minutes (too recent!)
			// thus safe to assume no ib port drop
			// even if we have more events, all only elapsed less than 4 minutes
			// thus safe to assume no ib port drop
			// may come back later!
			log.Logger.Warnw("ib port drop too recent", "device", dev, "elapsed", elapsed)
			continue
		}

		dropHumanized := humanize.RelTime(ts, cr.ts, "ago", "from now")
		msgs = append(msgs, fmt.Sprintf("%s dropped %s", dev, dropHumanized))
	}
	if len(msgs) == 0 {
		// no ib port drop
		return
	}
	sort.Strings(msgs)

	cr.reasonIbPortDrop = "ib port drop -- " + strings.Join(msgs, ", ")
}

// evaluateIbPortFlap evaluates whether the check result is caused by
// the ib port flap, where the port is down and back to active
// for the last 4 minutes
// it uses the historical data in the event store to evaluate the ib port flap
// if that's the case, it sets the field [checkResult.reasonIbPortFlap]
func (c *component) evaluateIbPortFlap(cr *checkResult) {
	if cr == nil {
		return
	}

	// even when the current check result is healthy
	// if the old results were unhealthy
	// we still need to evaluate the ib port flap

	if cr.ts.IsZero() {
		// current check result timestamp is unknown, can't evaluate
		return
	}

	if c.eventBucket == nil {
		// no event bucket, can't evaluate
		return
	}

	// query the last 4 minutes with some buffer
	// since we only check once per minute
	// (events are sorted by time ascending, latest event is the last one)
	since := cr.ts.Add(-10 * time.Minute)
	ibstatEvents, err := c.readAllIbstatEvents(since)
	if err != nil {
		log.Logger.Errorw("error reading ibstat events", "error", err)
		return
	}
	if len(ibstatEvents) <= 1 {
		// no unhealthy port event in the last 4 minutes
		// thus safe to assume no ib port flap
		//
		// or
		//
		// not enough number of events to evalute ib port flaps
		return
	}

	// check if there was any ibstat event and lasted >= 4 minutes
	elapsedSinceOldest := cr.ts.Sub(ibstatEvents[0].Time)
	if elapsedSinceOldest < 0 {
		// something wrong with the event store
		log.Logger.Warnw("unexpected event timestamp", "checkResultTimestamp", cr.ts, "eventTimestamp", ibstatEvents[0].Time)
		return
	}

	// maps from port device name to the state transitions
	stateTransitions := make(map[string][]string)
	for _, ev := range ibstatEvents {
		elapsed := cr.ts.Sub(ev.Time)

		// ib port flag is only evaluated for the last 4 minutes
		// old events should be ignored
		if elapsed > 4*time.Minute {
			continue
		}

		allPorts := parseIBPortsFromEvent(ev)
		for _, port := range allPorts {
			prev, ok := stateTransitions[port.Device]
			if !ok || len(prev) == 0 {
				stateTransitions[port.Device] = []string{port.State}
			} else if prev[len(prev)-1] != port.State {
				// ip port state flapped!
				stateTransitions[port.Device] = append(stateTransitions[port.Device], port.State)
			}
		}
	}

	// no state transitions in the last 4 minutes
	if len(stateTransitions) == 0 {
		return
	}

	msgs := make([]string, 0)
	for dev, transitions := range stateTransitions {
		if len(transitions) < 2 {
			continue
		}

		// keep up to 4 entries
		if len(transitions) > 4 {
			// keep the last 4 entries
			transitions = transitions[len(transitions)-4:]
		}

		// Down -> Active == ib port flap
		// Active -> Down == ib port flap
		// Active -> Down -> Active == ib port flap
		msgs = append(msgs, fmt.Sprintf("%s %s", dev, strings.Join(transitions, " -> ")))
	}
	if len(msgs) == 0 {
		// no ib port state flapped
		return
	}
	sort.Strings(msgs)

	cr.reasonIbPortFlap = "ib port flap -- " + strings.Join(msgs, ", ")
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	IbstatOutput   *infiniband.IbstatOutput   `json:"ibstat_output"`
	IbstatusOutput *infiniband.IbstatusOutput `json:"ibstatus_output"`

	allIBPorts []infiniband.IBPort

	// current unhealthy ib ports that are problematic
	// (down/polling/disabled, below expected ib port thresholds)
	unhealthyIBPorts []infiniband.IBPort

	// timestamp of the last check
	ts time.Time
	// error from the last check with "ibstat" command and other operations
	err error
	// error from the last check with "ibstatus" command
	errIbstatus error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the suggested actions for the last check
	suggestedActions *apiv1.SuggestedActions
	// tracks the reason of the last check
	reason string

	reasonIbSwitchFault string
	reasonIbPortDrop    string
	reasonIbPortFlap    string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if cr.IbstatOutput == nil && cr.IbstatusOutput == nil {
		return "no data"
	}

	out := ""

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

	if cr.IbstatusOutput != nil {
		buf := bytes.NewBuffer(nil)
		table := tablewriter.NewWriter(buf)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader([]string{"Device", "State", "Physical State", "Rate", "Link Layer"})
		for _, dev := range cr.IbstatusOutput.Parsed {
			table.Append([]string{
				dev.Device,
				dev.State,
				dev.PhysicalState,
				dev.Rate,
				dev.LinkLayer,
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

	reason := cr.reason

	if cr.reasonIbSwitchFault != "" {
		reason += "; " + cr.reasonIbSwitchFault
	}

	if cr.reasonIbPortDrop != "" {
		reason += "; " + cr.reasonIbPortDrop
	}

	if cr.reasonIbPortFlap != "" {
		reason += "; " + cr.reasonIbPortFlap
	}

	return reason
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

	// "cr.errIbstatus" is only used for fallback
	// thus do not return it as the error

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
		Reason:           cr.Summary(),
		Error:            cr.getError(),
		SuggestedActions: cr.getSuggestedActions(),
	}

	if cr.IbstatOutput != nil {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
