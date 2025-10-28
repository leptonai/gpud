// Package xid tracks the NVIDIA GPU Xid errors scanning the kmsg
// See Xid messages https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages.
package xid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
)

// Name is the name of the XID component.
const Name = "accelerator-nvidia-error-xid"

const (
	StateNameErrorXid = "error_xid"

	EventNameErrorXid    = "error_xid"
	EventKeyErrorXidData = "data"
	EventKeyDeviceUUID   = "device_uuid"

	DefaultRetentionPeriod   = eventstore.DefaultRetention
	DefaultStateUpdatePeriod = 30 * time.Second
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance
	devices      map[string]device.Device

	getTimeNowFunc   func() time.Time
	getThresholdFunc func() RebootThreshold

	rebootEventStore pkghost.RebootEventStore
	eventBucket      eventstore.Bucket
	kmsgWatcher      kmsg.Watcher

	readAllKmsg  func(context.Context) ([]kmsg.Message, error)
	extraEventCh chan *eventstore.Event

	lastMu          sync.RWMutex
	lastCheckResult *checkResult

	mu        sync.RWMutex
	currState apiv1.HealthState
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: gpudInstance.NVMLInstance,

		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdFunc: GetDefaultRebootThreshold,

		rebootEventStore: gpudInstance.RebootEventStore,
		extraEventCh:     make(chan *eventstore.Event, 256),
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
			c.kmsgWatcher, err = kmsg.NewWatcher()
			if err != nil {
				ccancel()
				return nil, err
			}
		}
	}

	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		c.readAllKmsg = kmsg.ReadAll
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

	if c.kmsgWatcher != nil {
		kmsgCh, err := c.kmsgWatcher.Watch()
		if err != nil {
			return err
		}
		go c.start(kmsgCh, DefaultStateUpdatePeriod)
	}

	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return apiv1.HealthStates{c.currState}
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

	if c.kmsgWatcher != nil {
		cerr := c.kmsgWatcher.Close()
		if cerr != nil {
			log.Logger.Errorw("failed to close kmsg watcher", "error", cerr)
		}
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
		ts: c.getTimeNowFunc(),
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

	if c.readAllKmsg == nil {
		cr.reason = "kmsg reader is not set"
		cr.health = apiv1.HealthStateTypeHealthy
		return cr
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	kmsgs, err := c.readAllKmsg(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.reason = "failed to read kmsg"
		cr.health = apiv1.HealthStateTypeUnhealthy
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}

	for _, kmsg := range kmsgs {
		xidErr := Match(kmsg.Message)
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
			Kmsg:     kmsg,
			XidError: *xidErr,
		})
	}

	cr.reason = fmt.Sprintf("matched %d xid errors from %d kmsg(s)", len(cr.FoundErrors), len(kmsgs))

	// only used for "gpud scan"
	// if there are any critical errors, the health state will be unhealthy
	cr.health = apiv1.HealthStateTypeHealthy
	for _, foundErr := range cr.FoundErrors {
		if foundErr.Detail != nil && (foundErr.Detail.EventType == apiv1.EventTypeCritical || foundErr.Detail.EventType == apiv1.EventTypeFatal) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			break
		}
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	FoundErrors []FoundError `json:"found_errors"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

// FoundError represents a found XID error and its corresponding kmsg.
type FoundError struct {
	Kmsg kmsg.Message
	XidError
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
		if foundErr.Detail != nil && len(foundErr.Detail.SuggestedActionsByGPUd.RepairActions) > 0 {
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
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}
	return apiv1.HealthStates{state}
}

func (c *component) start(kmsgCh <-chan kmsg.Message, updatePeriod time.Duration) {
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

		case newEvent := <-c.extraEventCh:
			if newEvent == nil {
				continue
			}
			if err := c.eventBucket.Insert(c.ctx, *newEvent); err != nil {
				log.Logger.Errorw("failed to create event", "error", err)
				continue
			}
			if err := c.updateCurrentState(); err != nil {
				log.Logger.Errorw("failed to update current state", "error", err)
				continue
			}

		case message := <-kmsgCh:
			xidErr := Match(message.Message)
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
			if c.nvmlInstance.GetMemoryErrorManagementCapabilities().RowRemapping && (xidErr.Xid == 63 || xidErr.Xid == 64) {
				log.Logger.Warnw("discarding Xid 63/64 in favor of remapped-rows component", "xid", xidErr.Xid, "deviceUUID", xidErr.DeviceUUID)
				continue
			}

			id := uuid.New()
			var xidName string
			if xidErr.Detail != nil {
				xidName = xidErr.Detail.Description
			}
			logger := log.Logger.With("id", id, "xid", xidErr.Xid, "xidName", xidName, "deviceUUID", xidErr.DeviceUUID)
			logger.Infow("got xid event", "kmsg", message, "kmsgTimestamp", message.Timestamp.Unix())

			event := eventstore.Event{
				Time: message.Timestamp.Time,
				Name: EventNameErrorXid,
				ExtraInfo: map[string]string{
					EventKeyErrorXidData: strconv.FormatInt(int64(xidErr.Xid), 10),
					EventKeyDeviceUUID:   xidErr.DeviceUUID,
				},
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

func (c *component) updateCurrentState() error {
	if c.rebootEventStore == nil || c.eventBucket == nil {
		return nil
	}

	now := c.getTimeNowFunc()
	rebootThreshold := c.getThresholdFunc()

	var rebootErr string
	rebootEvents, err := c.rebootEventStore.GetRebootEvents(c.ctx, now.Add(-DefaultRetentionPeriod))
	if err != nil {
		rebootErr = fmt.Sprintf("failed to get reboot events: %v", err)
		log.Logger.Errorw("failed to get reboot events", "error", err)
	}

	localEvents, err := c.eventBucket.Get(c.ctx, now.Add(-DefaultRetentionPeriod))
	if err != nil {
		return fmt.Errorf("failed to get all events: %w", err)
	}

	events := mergeEvents(rebootEvents, localEvents)

	c.mu.Lock()
	c.currState = evolveHealthyState(events, c.devices, rebootThreshold.Threshold)
	if rebootErr != "" {
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

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.After(result[j].Time)
	})

	return result
}
