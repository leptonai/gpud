// Package infiniband monitors the infiniband status of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package infiniband

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	infinibandstore "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/store"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const (
	Name = "accelerator-nvidia-infiniband"

	defaultCheckInterval  = 30 * time.Second
	defaultRequestTimeout = 15 * time.Second

	// defaultDropStickyWindow defines the stabilization period after an IB port drop
	// during which the component remains unhealthy even if thresholds recover.
	//
	// WHY THIS IS NEEDED:
	// Previously, IB port drops were "ephemeral" - as soon as thresholds recovered
	// (e.g., port came back up), the component would immediately flip from Unhealthy
	// back to Healthy. This created operator confusion because:
	// 1. At time T: Component marked Unhealthy with HARDWARE_INSPECTION suggested
	// 2. At time T+30s: Port recovers, component immediately becomes Healthy
	// 3. Operators miss the alert or find contradictory states in logs
	//
	// With this sticky window, drops remain unhealthy for a stabilization period,
	// giving operators time to observe and investigate, similar to how flaps work.
	// This creates consistency: both flaps and drops suggest inspection that persists.
	defaultDropStickyWindow = 10 * time.Minute
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkInterval  time.Duration
	requestTimeout time.Duration

	// dropStickyWindow defines how long after an IB port drop event
	// the component should remain unhealthy even if thresholds recover.
	// This provides a stabilization period for operators to observe issues.
	dropStickyWindow time.Duration

	nvmlInstance   nvidianvml.Instance
	toolOverwrites pkgconfigcommon.ToolOverwrites

	ibPortsStore infinibandstore.Store
	eventBucket  eventstore.Bucket
	kmsgSyncer   *kmsg.Syncer

	getTimeNowFunc      func() time.Time
	getThresholdsFunc   func() types.ExpectedPortStates
	getClassDevicesFunc func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult

	// Track when thresholds last recovered for sticky window logic
	thresholdRecoveryTimeMu sync.RWMutex
	thresholdRecoveryTime   *time.Time

	// ignoreFiles tracks the files that failed to read (e.g. EINVAL)
	// to avoid reading them again and causing kernel log spam
	ignoreFiles map[string]struct{}
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkInterval:    defaultCheckInterval,
		requestTimeout:   defaultRequestTimeout,
		dropStickyWindow: defaultDropStickyWindow,

		nvmlInstance:   gpudInstance.NVMLInstance,
		toolOverwrites: gpudInstance.NVIDIAToolOverwrites,

		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: GetDefaultExpectedPortStates,
		getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
			opts := []infinibandclass.OpOption{
				infinibandclass.WithIgnoreFiles(ignoreFiles),
			}
			// Exclude devices that have restricted PFs and cause ACCESS_REG errors.
			// ref. https://github.com/prometheus/node_exporter/issues/3434
			// ref. https://github.com/leptonai/gpud/issues/1164
			if len(gpudInstance.NVIDIAToolOverwrites.ExcludedInfinibandDevices) > 0 {
				opts = append(opts, infinibandclass.WithExcludedDevices(gpudInstance.NVIDIAToolOverwrites.ExcludedInfinibandDevices))
			}
			return infinibandclass.LoadDevices(gpudInstance.NVIDIAToolOverwrites.InfinibandClassRootDir, opts...)
		},
		ignoreFiles: make(map[string]struct{}),
	}

	if gpudInstance.DBRW != nil && gpudInstance.DBRO != nil {
		var err error
		c.ibPortsStore, err = infinibandstore.New(gpudInstance.RootCtx, gpudInstance.DBRW, gpudInstance.DBRO)
		if err != nil {
			return nil, err
		}
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
		ticker := time.NewTicker(c.checkInterval)
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

	// Filter to only return kmsg events for PCI power and port module temperature
	allEvents := evs.Events()
	filteredEvents := make(apiv1.Events, 0, len(allEvents))
	for _, ev := range allEvents {
		if ev.Name == eventPCIPowerInsufficient || ev.Name == eventPortModuleHighTemperature || ev.Name == eventAccessRegFailed {
			filteredEvents = append(filteredEvents, ev)
		}
	}
	return filteredEvents, nil
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

// Check evaluates the health of InfiniBand ports by:
// 1. Checking current port states against configured thresholds
// 2. Processing historical port events with different persistence behaviors:
//   - Flap events: Always processed (sticky until SetHealthy is called)
//   - Drop events: Processed when either:
//     a) Thresholds are currently not met (immediate detection)
//     b) Event occurred within the sticky window (default 10 minutes)
//     c) Thresholds recently recovered AND we're still within sticky window
//
// 3. Returning unhealthy if either:
//   - Current state violates thresholds, OR
//   - Flap events exist (always sticky until SetHealthy), OR
//   - Drop events exist AND (thresholds not met OR within sticky window OR recovery window)
//
// WHY THE STICKY WINDOW IS NEEDED FOR DROPS:
// Previously, when a port dropped and caused threshold failure:
// - 08:00: Port drops, thresholds fail, marked Unhealthy with HARDWARE_INSPECTION
// - 08:05: Port recovers, thresholds pass, immediately flips to Healthy
// - Result: Operators see contradictory states - inspection was requested then cleared
//
// Now with sticky window:
// - 08:00: Port drops, marked Unhealthy with HARDWARE_INSPECTION
// - 08:05: Port recovers but STAYS Unhealthy for 10 minutes (stabilization)
// - 08:15: After sticky window expires, becomes Healthy if no new issues
// - Result: Consistent experience - inspection request persists for observation
//
// This also handles dormant ports correctly: ports beyond required thresholds
// won't trigger alerts after the sticky window expires, avoiding false positives
// for unused ports on machines with more ports than required (e.g., 12 ports
// when only 8 are needed).
//
// Historical events require manual inspection and clearing via SetHealthy to reset state.
func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu infiniband")

	cr := &checkResult{
		ts: c.getTimeNowFunc(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// nothing specified for this machine, gpud MUST skip the ib check
	thresholds := c.getThresholdsFunc()
	if thresholds.IsZero() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoThreshold
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
	// Check for NVML initialization errors first.
	// This handles cases like "error getting device handle for index 'N': Unknown Error"
	// which corresponds to nvidia-smi showing "Unable to determine the device handle for GPU".
	if err := c.nvmlInstance.InitError(); err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("NVML initialization error: %v", err)
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	var err error
	cr.ClassDevices, err = c.getClassDevicesFunc(c.ignoreFiles)
	if err != nil {
		log.Logger.Warnw("error loading infiniband class devices", "devices", len(cr.ClassDevices), "error", err)
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error loading infiniband class devices"
		cr.err = err
		return cr
	}

	var sysClassIBPorts []types.IBPort
	for _, dev := range cr.ClassDevices {
		for _, port := range dev.Ports {
			ibport := types.IBPort{
				Port:          port.Port,
				Device:        dev.Name,
				State:         port.State,
				PhysicalState: port.PhysState,
				RateGBSec:     int(port.RateGBSec),
				LinkLayer:     port.LinkLayer,
			}

			if !ibport.IsIBPort() {
				continue
			}

			if port.Counters.LinkDowned != nil {
				ibport.TotalLinkDowned = *port.Counters.LinkDowned

				devicePort := dev.Name + "_" + port.Name
				linkDowned := float64(*port.Counters.LinkDowned)
				metricIbLinkedDowned.With(prometheus.Labels{"device_port": devicePort}).Set(linkDowned)
			}

			sysClassIBPorts = append(sysClassIBPorts, ibport)
		}
	}

	if c.ibPortsStore != nil && len(sysClassIBPorts) > 0 {
		err := c.ibPortsStore.Insert(c.getTimeNowFunc(), sysClassIBPorts)
		if err != nil {
			log.Logger.Warnw("error inserting ib ports into store", "error", err)
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error inserting ib ports into store"
			cr.err = err
			return cr
		}
		if err := c.ibPortsStore.Scan(); err != nil {
			log.Logger.Warnw("error scanning ib ports from store", "error", err)
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error scanning ib ports from store"
			cr.err = err
			return cr
		}
	}

	// no data, skip the evaluation
	if len(sysClassIBPorts) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoIbPortData
		log.Logger.Warnw(cr.reason)
		return cr
	}

	// STEP 1: Evaluate current threshold status
	// This sets the initial health state based on whether current port states meet thresholds.
	// If ports have recovered, this will mark the component as Healthy and set thresholdsFailing=false.
	// However, this doesn't mean we're done - we still need to check historical drop/flap events below.
	evaluateHealthStateWithThresholds(thresholds, sysClassIBPorts, cr)

	// RECOVERY TRACKING FOR CONDITION 3:
	// Track when thresholds transition from failing to passing (recovery).
	// This timestamp is used to implement the recovery sticky window, which keeps
	// the component unhealthy for a stabilization period after ports come back up.
	//
	// Example scenario this prevents:
	// - 08:00: Port mlx5_7 goes down, only 7/8 ports active, marked Unhealthy
	// - 08:47: Port mlx5_7 recovers, now 8/8 ports active
	// - WITHOUT recovery tracking: Immediately becomes Healthy (confusing!)
	// - WITH recovery tracking: Stays Unhealthy until 08:57 for observation
	c.lastMu.RLock()
	prevCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	c.thresholdRecoveryTimeMu.Lock()
	currentThresholdsFailing := cr.thresholdsFailing
	previousThresholdsFailing := prevCheckResult != nil && prevCheckResult.thresholdsFailing

	if !currentThresholdsFailing && previousThresholdsFailing && c.thresholdRecoveryTime == nil {
		// Thresholds just transitioned from failing to passing - track recovery time
		now := c.getTimeNowFunc()
		c.thresholdRecoveryTime = &now

		log.Logger.Infow("infiniband thresholds recovered, starting sticky window",
			"recoveryTime", now,
			"stickyWindow", c.dropStickyWindow)
	} else if currentThresholdsFailing {
		// Thresholds are failing, clear recovery time
		c.thresholdRecoveryTime = nil
	}
	recoveryTime := c.thresholdRecoveryTime
	c.thresholdRecoveryTimeMu.Unlock()

	// Check for IB port drop/flap events
	//
	// THE OLD PROBLEM (before sticky window):
	// Drop events were ONLY processed when thresholds were currently failing.
	// This meant: evaluateHealthStateWithThresholds() runs first → if port recovered,
	// it would set Healthy and clear unhealthyIBPorts → then drop event checking would
	// skip processing because len(unhealthyIBPorts) == 0 → result: immediate Unhealthy→Healthy flip.
	//
	// THE FIX (with sticky window):
	// Drop events are now processed under THREE conditions (see below), not just when
	// thresholds are failing. This prevents threshold recovery from immediately clearing
	// the unhealthy state, while still handling dormant ports correctly after the sticky
	// window expires.
	if c.ibPortsStore != nil {
		// we DO NOT discard past latestIbportEvents until the user explicitly
		// inspected and set healthy, in order to not miss critical latestIbportEvents
		// this will return empty, once the user inspected and set healthy (to be tombstoned)
		latestIbportEvents, err := c.ibPortsStore.LastEvents(zeroTime)
		if err != nil {
			log.Logger.Warnw("error getting ib flap/drop events", "error", err)
			// Set unhealthy state when we can't retrieve event history
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting ib flap/drop events"
			cr.err = err
			return cr
		}

		// above "ibPortsStore.LastEvents" only returns the last event per device and per port
		// while keeping the details about "when" the event first happened (e.g., flap happened at x)
		// thus, this infiniband event means, the ib port flap/drop happened at x
		// and may still happen (thus I am emitting the event!)
		// to signal that the hw inspection is required and set healthy is required

		// there are events that have not been purged yet
		// thus whether thresholds are breached or not,
		// we surface this
		// so that these will be enabled
		// if and only if admin manually calls
		// set healthy on the infiniband component
		if len(latestIbportEvents) > 0 {
			// Note: We do not persist IB port drop/flap events in the gpud event store
			// because the health state evaluation already surfaces them with historical
			// information. The ibPortsStore maintains the complete history of these events
			// and surfaces them through the health state until explicitly cleared via SetHealthy.
			// This avoids redundant event storage while ensuring critical events are not missed.
			ibDropDevs := []string{}
			ibFlapDevs := []string{}

			type portKey struct {
				device string
				port   uint
			}
			currentPortStates := make(map[portKey]string, len(sysClassIBPorts))
			for _, port := range sysClassIBPorts {
				currentPortStates[portKey{device: port.Device, port: port.Port}] = port.State
			}
			for _, event := range latestIbportEvents {
				switch event.EventType {
				case infinibandstore.EventTypeIbPortDrop:
					// WHY WE NEED THREE CONDITIONS FOR PROCESSING DROPS:
					//
					// CONDITION 1: Thresholds currently failing (e.g., only 7/8 ports active)
					// - Process ALL drops immediately for visibility into what's broken
					// - This includes old drops from dormant ports to show full picture
					//
					// CONDITION 2: Drop occurred recently (within sticky window, e.g., < 10 min ago)
					// - Even if thresholds pass NOW, recent drops indicate instability
					// - Prevents "blink and you miss it" scenarios where issues self-heal quickly
					//
					// CONDITION 3: Thresholds JUST recovered AND within recovery sticky window
					// - Port was down causing threshold failure, then recovered
					// - Without this: Unhealthy→Healthy flip within 30s confuses operators
					// - With this: Stays Unhealthy for stabilization period after recovery
					// - Example timeline from production:
					//   08:00: Port drops, marked Unhealthy + HARDWARE_INSPECTION
					//   08:47: Port recovers, thresholds pass
					//   08:47: WITHOUT sticky: Immediately Healthy (confusing!)
					//   08:47: WITH sticky: Remains Unhealthy until 08:57 (clear)
					//
					// This design preserves dormant port handling: ports beyond thresholds
					// (e.g., ports 9-12 when only 8 needed) won't cause false alerts after
					// the sticky window expires.

					// CONDITION 1: Check if thresholds are currently failing
					thresholdsFailing := cr.thresholdsFailing
					now := c.getTimeNowFunc()
					dropAge := now.Sub(event.Time)
					if dropAge < 0 {
						// Future-dated event (clock skew?) - treat as very recent but log warning
						log.Logger.Warnw("drop event has future timestamp, possible clock skew",
							"eventTime", event.Time,
							"currentTime", now,
							"device", event.Port.Device)
						dropAge = 0
					}

					// CONDITION 2: Check if drop is recent (within sticky window from event time)
					dropWithinStickyWindow := false
					if c.dropStickyWindow > 0 {
						dropWithinStickyWindow = dropAge < c.dropStickyWindow
					}

					// CONDITION 3: Check if we're within recovery sticky window
					// This handles the case where:
					// - Thresholds were failing (port down)
					// - Thresholds just recovered (port came back up)
					// - We want to stay unhealthy for stabilization period
					withinRecoveryStickyWindow := false
					timeSinceRecovery := time.Duration(0)
					portRecovered := false
					if state, ok := currentPortStates[portKey{device: event.Port.Device, port: event.Port.Port}]; ok {
						portRecovered = strings.EqualFold(state, "ACTIVE") || strings.EqualFold(state, "UP")
					}
					if recoveryTime != nil && c.dropStickyWindow > 0 && portRecovered {
						timeSinceRecovery = now.Sub(*recoveryTime)
						if timeSinceRecovery < 0 {
							timeSinceRecovery = 0
						}
						if timeSinceRecovery < c.dropStickyWindow {
							withinRecoveryStickyWindow = true
						}
					}

					// Process drop if ANY of the three conditions are met
					//
					// KEY FIX: Previously this would be: if thresholdsFailing { ... }
					// That meant once thresholds passed (port recovered), drop events were ignored,
					// causing the immediate state flip. Now we use OR logic with three conditions,
					// ensuring drop events persist even after threshold recovery.
					shouldProcessDrop := thresholdsFailing || dropWithinStickyWindow || withinRecoveryStickyWindow

					if shouldProcessDrop {
						log.Logger.Warnw(event.EventReason,
							"thresholdsFailing", thresholdsFailing,
							"dropAge", dropAge,
							"dropWithinStickyWindow", dropWithinStickyWindow,
							"withinRecoveryStickyWindow", withinRecoveryStickyWindow,
							"timeSinceRecovery", timeSinceRecovery,
							"stickyWindow", c.dropStickyWindow,
							"recoveryTimeTracked", recoveryTime != nil,
							"portRecovered", portRecovered)
						ibDropDevs = append(ibDropDevs, event.Port.Device)
					}

				case infinibandstore.EventTypeIbPortFlap:
					// Always process flap events
					log.Logger.Warnw(event.EventReason)
					ibFlapDevs = append(ibFlapDevs, event.Port.Device)

				default:
					log.Logger.Warnw("unknown ib event type", "event", event)
				}
			}

			if len(ibDropDevs) > 0 || len(ibFlapDevs) > 0 {
				sort.Strings(ibDropDevs)
				sort.Strings(ibFlapDevs)

				if cr.reason == reasonNoIbPortIssue {
					// Current ports are healthy but there are historical events
					// Only clear the "ok; no infiniband port issue" message if there are
					// actual event descriptions that would make it confusing
					cr.reason = ""
				}

				if cr.reason != "" {
					// e.g., ib port health state violates its expected state/rate threholds
					cr.reason += "; "
				}

				if len(ibDropDevs) > 0 {
					cr.reason += "device(s) down too long: " + strings.Join(ibDropDevs, ", ")
				}
				if len(ibFlapDevs) > 0 {
					if len(ibDropDevs) > 0 {
						cr.reason += "; "
					}
					cr.reason += "device(s) flapping between ACTIVE<>DOWN: " + strings.Join(ibFlapDevs, ", ")
				}

				cr.health = apiv1.HealthStateTypeUnhealthy
				log.Logger.Warnw(cr.reason)

				cr.suggestedActions = &apiv1.SuggestedActions{
					RepairActions: []apiv1.RepairActionType{
						apiv1.RepairActionTypeHardwareInspection,
					},
				}
			}
		}
	} else {
		log.Logger.Debugw("no events store set, skipped ib port flap/drop event processing")
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ClassDevices infinibandclass.Devices `json:"class_devices"`

	// current unhealthy ib ports that are problematic
	// (down/polling/disabled, below expected ib port thresholds)
	unhealthyIBPorts []types.IBPort `json:"-"`

	// indicates whether threshold evaluation failed during this check
	// This flag is used for sticky window logic (Condition 1):
	// - true: all drop events are processed immediately
	// - false: drop events may still be processed via Conditions 2 & 3 (sticky window)
	// This prevents threshold recovery from immediately clearing the unhealthy state.
	thresholdsFailing bool `json:"-"`

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

	out := ""

	if len(cr.ClassDevices) > 0 {
		buf := bytes.NewBuffer(nil)
		cr.ClassDevices.RenderTable(buf)

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
		Health:           cr.health,
		Reason:           cr.reason,
		SuggestedActions: cr.getSuggestedActions(),
		Error:            cr.getError(),
	}
	return apiv1.HealthStates{state}
}

var zeroTime = time.Time{}
