// Package xid tracks the NVIDIA GPU Xid errors scanning the kmsg
// See Xid messages https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages.
package xid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	componentkmsg "github.com/leptonai/gpud/components/kmsg"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvsentinel"
)

// Name is the name of the XID component.
const Name = "accelerator-nvidia-error-xid"

const (
	// StateNameErrorXid is the health state name for XID errors.
	StateNameErrorXid = "error_xid"

	// EventNameErrorXid is emitted when GPUd detects an XID error.
	EventNameErrorXid = "error_xid"
	// EventKeyErrorXidData stores the serialized XID payload.
	EventKeyErrorXidData = "data"
	// EventKeyDeviceUUID stores the device identifier associated with an XID event.
	EventKeyDeviceUUID = "device_uuid"

	// DefaultStateUpdatePeriod is the background XID state refresh period.
	DefaultStateUpdatePeriod = 30 * time.Second
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance
	devices      map[string]device.Device

	getTimeNowFunc   func() time.Time
	getThresholdFunc func() Thresholds

	rebootEventStore pkghost.RebootEventStore
	eventBucket      eventstore.Bucket
	kernelMessages   <-chan kernelRecord
	stopKernel       func()
	kmsgWatcher      kmsg.Watcher
	started          atomic.Bool
	monitoring       atomic.Bool

	readAllKmsg  func(context.Context) ([]kernelRecord, error)
	extraEventCh chan pendingEvent

	lastMu          sync.RWMutex
	lastCheckResult *checkResult

	mu        sync.RWMutex
	currState apiv1.HealthState

	// nvsSource is the optional NVSentinel event source. When set, the
	// component prefers NVSentinel Xid data points and suppresses its own
	// kmsg detection of the same data point, except for XID 154, which uses
	// the native stream while it is running to preserve kernel ordering.
	nvsSource      nvsentinel.Source
	nvsDedupWindow time.Duration
	nvsUnsubscribe func()
	// ^ stop function returned by nvsSource.Subscribe
}

// New returns the NVIDIA XID component.
func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: gpudInstance.NVMLInstance,

		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdFunc: GetDefaultThresholds,

		rebootEventStore: gpudInstance.RebootEventStore,
		extraEventCh:     make(chan pendingEvent, 256),
	}

	if gpudInstance.NVMLInstance != nil {
		c.devices = gpudInstance.NVMLInstance.Devices()
	}

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		if os.Geteuid() == 0 {
			if gpudInstance.ExperimentalHealthValidation {
				c.kernelMessages, c.stopKernel, err = c.observeKernel(gpudInstance.KernelStream().Subscribe)
			} else {
				c.kmsgWatcher, err = kmsg.NewWatcher(kmsg.WithExactDedupMatch(isRecoveryMessage))
			}
			if err != nil {
				ccancel()
				return nil, err
			}
		}
	}

	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		c.readAllKmsg = readLegacyKernelMessages
		if gpudInstance.ExperimentalHealthValidation {
			c.readAllKmsg = readKernelMessages
		}
	}

	if gpudInstance.NVSentinel != nil && c.eventBucket != nil {
		c.nvsSource = gpudInstance.NVSentinel
		c.nvsDedupWindow = gpudInstance.NVSentinelEventDedupWindow
		if c.nvsDedupWindow <= 0 {
			c.nvsDedupWindow = nvsentinel.DefaultEventDedupWindow
		}
		c.watchNVSentinel()
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
	if !c.started.CompareAndSwap(false, true) {
		return components.ErrAlreadyStarted
	}
	for {
		err := c.updateCurrentState()
		if err == nil {
			break
		}

		if errors.Is(err, context.Canceled) {
			log.Logger.Infow("context canceled, exiting")
			return nil
		}

		log.Logger.Errorw("failed to fetch current events", "error", err)
		select {
		case <-c.ctx.Done():
			return nil
		case <-time.After(1 * time.Second):
		}
	}

	// Run the event loop whenever any event source exists: the kmsg watcher
	// (root with /dev/kmsg) or the NVSentinel receiver. NVSentinel's syslog
	// health monitor may be the only kmsg reader on a node; without this,
	// NVSentinel events would queue in extraEventCh and never be stored.
	// A nil kmsgCh is never ready in start's select, which is fine.
	if c.kmsgWatcher != nil {
		ch, err := componentkmsg.WatchLegacy(c.ctx, c.kmsgWatcher)
		if err != nil {
			return err
		}
		c.kernelMessages = c.convertLegacyKernel(ch)
	}
	if c.kernelMessages != nil || c.nvsSource != nil {
		go c.start(c.kernelMessages, DefaultStateUpdatePeriod)
	}

	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.mu.RLock()
	defer c.mu.RUnlock()
	state := c.currState
	if state.Evaluated == nil || (*state.Evaluated && !c.monitoring.Load()) {
		state.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedNoData)
	}
	return apiv1.HealthStates{state}
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.eventBucket == nil {
		return nil, nil
	}

	events, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}

	var ret apiv1.Events
	for _, event := range events {
		ev := resolveXIDEvent(event, c.devices)
		ret = append(ret, ev.ToEvent())
	}
	return ret, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.nvsUnsubscribe != nil {
		c.nvsUnsubscribe()
	}

	if c.kmsgWatcher != nil {
		if err := c.kmsgWatcher.Close(); err != nil {
			log.Logger.Errorw("failed to close kmsg watcher", "error", err)
		}
	}
	if c.stopKernel != nil {
		c.stopKernel()
	}

	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

// Check checks the current XID errors (e.g., "gpud scan")
// by reading all kmsg logs.
func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu xid")

	cr := &checkResult{
		Evaluation: apiv1.Evaluated(),
		ts:         c.getTimeNowFunc(),
	}
	defer func() {
		if cr.err != nil && cr.Evaluated != nil && *cr.Evaluated {
			cr.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedCollection)
		}
		if cr.Evaluated != nil && *cr.Evaluated && c.nvmlInstance != nil && len(c.nvmlInstance.Devices()) == 0 {
			cr.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedNoData)
		}
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		cr.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedUnsupported)
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		cr.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedUnsupported)
		return cr
	}
	// Check for NVML initialization errors first.
	// This handles cases like "error getting device handle for index 'N': Unknown Error"
	// which corresponds to nvidia-smi showing "Unable to determine the device handle for GPU".
	if err := c.nvmlInstance.InitError(); err != nil {
		cr.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedCollection)
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
		cr.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedNoData)
		return cr
	}

	if c.readAllKmsg == nil {
		cr.reason = "kmsg reader is not set"
		cr.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedNoData)
		cr.health = apiv1.HealthStateTypeHealthy
		return cr
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	kmsgs, err := c.readAllKmsg(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedCollection)
		cr.reason = "failed to read kmsg"
		cr.health = apiv1.HealthStateTypeUnhealthy
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}

	for _, kmsg := range kmsgs {
		xidErr := kmsg.xid
		if xidErr == nil {
			continue
		}

		// row remapping pending/failure (Xid 63/64)
		// can also be detected by NVML API (vs. kmsg scanning)
		// thus we discard Xid 63/64 in favor of row remapping checks
		// especially, NVIDIA row remapping pending can happen >3 times
		// which warrants system reboots, before reaching its row remapping
		// failures threshold which requires hardware inspection
		// in other words, we do not want to blindly suggest hw inspection
		// from Xid 63/64, while row remmaping pending may self-resolve
		// after >3 times of system reboots
		// this is why we here discard Xid 63/64 in favor of row remapping checks
		if c.nvmlInstance.GetMemoryErrorManagementCapabilities().RowRemapping && (xidErr.Xid == 63 || xidErr.Xid == 64) {
			log.Logger.Warnw("discarding Xid 63/64 in favor of remapped-rows component", "xid", xidErr.Xid, "deviceUUID", xidErr.DeviceUUID)
			continue
		}

		cr.FoundErrors = append(cr.FoundErrors, FoundError{
			Kmsg:  kmsg.KernelMessage,
			Error: *xidErr,
		})
	}

	cr.reason = fmt.Sprintf("matched %d xid errors from %d kmsg(s)", len(cr.FoundErrors), len(kmsgs))

	// Scan shares XID 154 recovery ownership with the durable consumer, while
	// retaining its existing non-154 Critical/Fatal => Unhealthy policy.
	var scanEvents eventstore.Events
	nonRecoveryUnhealthy := false
	for _, foundErr := range cr.FoundErrors {
		xidValue, ok := uint64FromInt(foundErr.Xid)
		if !ok {
			continue
		}
		payload := xidErrorEventDetail{
			DeviceUUID:     foundErr.DeviceUUID,
			Xid:            xidValue,
			RecoveryAction: foundErr.RecoveryAction,
		}
		if foundErr.Detail != nil {
			payload.Description = foundErr.Detail.Description
			payload.SubCode = foundErr.Detail.SubCode
			payload.ErrorStatus = foundErr.Detail.ErrorStatus
			payload.SuggestedActionsByGPUd = foundErr.Detail.SuggestedActionsByGPUd
		}
		raw, _ := json.Marshal(payload)
		event := eventstore.Event{Time: foundErr.Kmsg.Timestamp.Time, Name: EventNameErrorXid, ExtraInfo: map[string]string{EventKeyErrorXidData: string(raw)}}
		if foundErr.Detail != nil {
			if foundErr.Xid != 154 && (foundErr.Detail.EventType == apiv1.EventTypeCritical || foundErr.Detail.EventType == apiv1.EventTypeFatal) {
				nonRecoveryUnhealthy = true
			}
			event.Type = string(foundErr.Detail.EventType)
		}
		scanEvents = append(scanEvents, event)
	}
	state := evolveHealthyState(mergeEvents(nil, scanEvents), c.devices, GetDefaultRebootThreshold())
	cr.health = state.Health
	if nonRecoveryUnhealthy {
		cr.health = apiv1.HealthStateTypeUnhealthy
	}
	cr.suggestedActions = state.SuggestedActions
	if len(scanEvents) > 0 {
		cr.reason += "; " + state.Reason
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	apiv1.Evaluation

	FoundErrors []FoundError `json:"found_errors"`

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

// FoundError represents a found XID error and its corresponding kmsg.
type FoundError struct {
	Kmsg apiv1.KernelMessage
	Error
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	if len(cr.FoundErrors) == 0 {
		return "no xid error found"
	}

	now := time.Now().UTC()
	header := []string{"Time", "XID", "DeviceUUID", "Name", "Criticality", "Action(s)"}
	outputs := make([]string, 0, len(cr.FoundErrors))
	for _, foundErr := range cr.FoundErrors {
		action := "unknown"
		if foundErr.Detail != nil && foundErr.Detail.SuggestedActionsByGPUd != nil && len(foundErr.Detail.SuggestedActionsByGPUd.RepairActions) > 0 {
			actions := make([]string, 0, len(foundErr.Detail.SuggestedActionsByGPUd.RepairActions))
			for _, action := range foundErr.Detail.SuggestedActionsByGPUd.RepairActions {
				actions = append(actions, string(action))
			}
			action = strings.Join(actions, ", ")
		}

		criticality := "unknown"
		if foundErr.Detail != nil {
			criticality = string(foundErr.Detail.EventType)
		}

		name := "unknown"
		if foundErr.Detail != nil {
			name = foundErr.Detail.Description
		}

		buf := bytes.NewBuffer(nil)
		table := tablewriter.NewWriter(buf)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader(header)
		table.Append([]string{
			foundErr.Kmsg.DescribeTimestamp(now),
			fmt.Sprintf("%d", foundErr.Xid),
			foundErr.DeviceUUID,
			name,
			criticality,
			action,
		})
		table.Render()
		outputs = append(outputs, buf.String())
	}

	return strings.Join(outputs, "\n\n")
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
				Time:       metav1.NewTime(time.Now().UTC()),
				Component:  Name,
				Name:       Name,
				Health:     apiv1.HealthStateTypeHealthy,
				Reason:     "no data yet",
				Evaluation: apiv1.NotEvaluated(apiv1.NotEvaluatedNoData),
			},
		}
	}

	state := apiv1.HealthState{
		Evaluation:       cr.Evaluation,
		Time:             metav1.NewTime(cr.ts),
		Component:        Name,
		Name:             Name,
		Reason:           cr.reason,
		Error:            cr.getError(),
		Health:           cr.health,
		SuggestedActions: cr.suggestedActions,
	}
	return apiv1.HealthStates{state}
}

func (c *component) start(kmsgCh <-chan kernelRecord, updatePeriod time.Duration) {
	c.monitoring.Store(kmsgCh != nil || c.nvsSource != nil)
	defer c.monitoring.Store(false)

	ticker := time.NewTicker(updatePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return

		case <-ticker.C:
			if err := c.updateCurrentState(); err != nil {
				log.Logger.Debugw("failed to fetch current events", "error", err)
				continue
			}

		case pending := <-c.extraEventCh:
			if pending.event == nil {
				continue
			}
			recoveryFromNVSentinel := pending.nvsEvent != nil && pending.xid == 154
			// NVSentinel replays journal batches using processing timestamps.
			// Prefer the native stream, including over copies queued before Start.
			if recoveryFromNVSentinel && kmsgCh != nil {
				continue
			}
			if err := c.eventBucket.Insert(c.ctx, *pending.event); err != nil {
				log.Logger.Errorw("failed to create event", "error", err)
				continue
			}
			if recoveryFromNVSentinel {
				metricXIDErrs.With(prometheus.Labels{
					"uuid": convertBusIDToUUID(pending.event.ExtraInfo[EventKeyDeviceUUID], c.devices),
					"xid":  "154",
				}).Inc()
			}
			// Coverage is reported only after the insert succeeds: a failed
			// insert must never suppress native detection of this incident.
			if pending.nvsEvent != nil && c.nvsSource != nil {
				c.nvsSource.RecordCoverage(*pending.nvsEvent)
			}
			if err := c.updateCurrentState(); err != nil {
				log.Logger.Errorw("failed to update current state", "error", err)
				continue
			}

		case message, ok := <-kmsgCh:
			if !ok {
				// Resume NVSentinel delivery if the native reader exits.
				kmsgCh = nil
				if c.nvsSource == nil {
					c.monitoring.Store(false)
				}
				continue
			}
			xidErr := message.xid
			if xidErr == nil {
				log.Logger.Debugw("not xid event, skip", "kmsg", message)
				continue
			}

			// row remapping pending/failure (Xid 63/64)
			// can also be detected by NVML API (vs. kmsg scanning)
			// thus we discard Xid 63/64 in favor of row remapping checks
			// especially, NVIDIA row remapping pending can happen >3 times
			// which warrants system reboots, before reaching its row remapping
			// failures threshold which requires hardware inspection
			// in other words, we do not want to blindly suggest hw inspection
			// from Xid 63/64, while row remmaping pending may self-resolve
			// after >3 times of system reboots
			// this is why we here discard Xid 63/64 in favor of row remapping checks
			if c.nvmlInstance != nil && c.nvmlInstance.GetMemoryErrorManagementCapabilities().RowRemapping && (xidErr.Xid == 63 || xidErr.Xid == 64) {
				log.Logger.Warnw("discarding Xid 63/64 in favor of remapped-rows component", "xid", xidErr.Xid, "deviceUUID", xidErr.DeviceUUID)
				continue
			}

			// When NVSentinel already reported this incident, skip the
			// GPUd-native copy. Both detectors read the same kernel report.
			// Storing both copies would double-count the same incident in
			// thresholds.
			if c.nvsentinelCoversXid(xidErr) {
				log.Logger.Infow("nvsentinel covers this xid data point, skipping gpud-native insert",
					"xid", xidErr.Xid, "deviceUUID", xidErr.DeviceUUID)
				continue
			}

			id := uuid.New()
			var xidName string
			if xidErr.Detail != nil {
				xidName = xidErr.Detail.Description
			}
			logger := log.Logger.With("id", id, "xid", xidErr.Xid, "xidName", xidName, "deviceUUID", xidErr.DeviceUUID)
			logger.Infow("got xid event", "kmsg", message, "kmsgTimestamp", message.Timestamp.Unix())

			xidValue, ok := uint64FromInt(xidErr.Xid)
			if !ok {
				logger.Errorw("invalid negative xid code", "xid", xidErr.Xid)
				continue
			}

			xidPayload := xidErrorEventDetail{
				Time:           metav1.NewTime(message.Timestamp.Time),
				DataSource:     "kmsg",
				DeviceUUID:     xidErr.DeviceUUID,
				Xid:            xidValue,
				RecoveryAction: xidErr.RecoveryAction,
			}
			if xidErr.Detail != nil {
				xidPayload.SubCode = xidErr.Detail.SubCode
				xidPayload.SubCodeDescription = xidErr.Detail.SubCodeDescription
				xidPayload.InvestigatoryHint = xidErr.Detail.InvestigatoryHint
				xidPayload.Description = xidErr.Detail.Description
				xidPayload.ErrorStatus = xidErr.Detail.ErrorStatus
				xidPayload.SuggestedActionsByGPUd = xidErr.Detail.SuggestedActionsByGPUd
			}

			rawPayload, err := json.Marshal(xidPayload)
			if err != nil {
				logger.Errorw("failed to marshal xid payload", "error", err)
			}

			event := eventstore.Event{
				Time: message.Timestamp.Time,
				Name: EventNameErrorXid,
				ExtraInfo: map[string]string{
					EventKeyDeviceUUID: xidErr.DeviceUUID,
				},
			}
			if xidErr.Xid == 154 {
				// Keep producer precision and identity through SQLite's seconds-only
				// timestamp column, including repeated transitions within a second.
				event.ExtraInfo[eventKeyRecoveryTimestamp] = message.Timestamp.Format(time.RFC3339Nano)
				event.ExtraInfo[eventKeyKmsgSequence] = strconv.Itoa(message.SequenceNumber)
			}
			// Preserve the shim's unit-specific severity. The historical
			// subcode table merges different units with the same subcode and
			// status, so recomputing it there can incorrectly escalate an event.
			if xidErr.Detail != nil && xidErr.Detail.EventType != "" {
				event.Type = string(xidErr.Detail.EventType)
			}
			if len(rawPayload) > 0 {
				event.ExtraInfo[EventKeyErrorXidData] = string(rawPayload)
			} else {
				event.ExtraInfo[EventKeyErrorXidData] = strconv.FormatInt(int64(xidErr.Xid), 10)
			}
			sameEvent, err := c.eventBucket.Find(c.ctx, event)
			if err != nil {
				logger.Errorw("failed to check event existence", "error", err)
				continue
			}
			if sameEvent != nil {
				logger.Infow("find the same event, skip inserting it")
				continue
			}
			if err = c.eventBucket.Insert(c.ctx, event); err != nil {
				logger.Errorw("failed to create event", "error", err)
				continue
			}
			logger.Infow("inserted the event successfully")
			metricXIDErrs.With(prometheus.Labels{
				"uuid": convertBusIDToUUID(xidErr.DeviceUUID, c.devices),
				"xid":  strconv.Itoa(xidErr.Xid),
			}).Inc()
			if err = c.updateCurrentState(); err != nil {
				logger.Errorw("failed to update current state", "error", err)
				continue
			}
		}
	}
}

func (c *component) updateCurrentState() (retErr error) {
	defer func() {
		if retErr != nil {
			c.mu.Lock()
			c.currState.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedCollection)
			c.mu.Unlock()
		}
	}()
	if c.rebootEventStore == nil || c.eventBucket == nil {
		return nil
	}

	now := c.getTimeNowFunc()
	rebootThreshold := GetDefaultRebootThreshold()
	thresholds := GetDefaultThresholds()
	if c.getThresholdFunc != nil {
		thresholds = c.getThresholdFunc()
	}

	var rebootErr string
	rebootEvents, err := c.rebootEventStore.GetRebootEvents(c.ctx, now.Add(-GetLookbackPeriod()))
	if err != nil {
		rebootErr = fmt.Sprintf("failed to get reboot events: %v", err)
		log.Logger.Errorw("failed to get reboot events", "error", err)
	}

	localEvents, err := c.eventBucket.Get(c.ctx, now.Add(-GetLookbackPeriod()))
	if err != nil {
		return fmt.Errorf("failed to get all events: %w", err)
	}
	localEvents = trimEventsAfterSetHealthy(localEvents)
	events := mergeEvents(rebootEvents, localEvents)

	c.mu.Lock()
	c.currState = evolveHealthyStateWithThresholds(events, c.devices, rebootThreshold, thresholds)
	c.currState.Evaluation = apiv1.Evaluated()
	if !c.monitoring.Load() {
		c.currState.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedNoData)
	}
	if rebootErr != "" {
		c.currState.Evaluation = apiv1.NotEvaluated(apiv1.NotEvaluatedPartial)
		c.currState.Error = fmt.Sprintf("%s\n%s", rebootErr, c.currState.Error)
	}
	c.mu.Unlock()

	return nil
}

// mergeEvents merges two event slices and returns a time descending sorted new slice
func mergeEvents(a, b eventstore.Events) eventstore.Events {
	totalLen := len(a) + len(b)
	if totalLen == 0 {
		return nil
	}
	result := make(eventstore.Events, 0, totalLen)
	result = append(result, a...)
	result = append(result, b...)
	for i := range result {
		if raw := result[i].ExtraInfo[eventKeyRecoveryTimestamp]; raw != "" {
			if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil && ts.Unix() == result[i].Time.Unix() {
				result[i].Time = ts
			}
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Time.After(result[j].Time)
	})

	return result
}

func trimEventsAfterSetHealthy(events eventstore.Events) eventstore.Events {
	for idx, event := range events {
		if event.Name == "SetHealthy" {
			if idx == 0 {
				return nil
			}
			trimmed := make(eventstore.Events, idx)
			copy(trimmed, events[:idx])
			return trimmed
		}
	}
	return events
}
