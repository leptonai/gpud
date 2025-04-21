// Package sxid tracks the NVIDIA GPU SXid errors scanning the kmsg.
// See fabric manager documentation https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf.
package sxid

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// Name is the name of the SXID component.
const Name = "accelerator-nvidia-error-sxid"

const (
	StateNameErrorSXid = "error_sxid"

	EventNameErrorSXid    = "error_sxid"
	EventKeyErrorSXidData = "data"
	EventKeyDeviceUUID    = "device_uuid"

	DefaultRetentionPeriod   = eventstore.DefaultRetention
	DefaultStateUpdatePeriod = 30 * time.Second
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance

	rebootEventStore pkghost.RebootEventStore
	eventBucket      eventstore.Bucket
	kmsgWatcher      kmsg.Watcher

	readAllKmsg  func(context.Context) ([]kmsg.Message, error)
	extraEventCh chan *apiv1.Event

	lastMu          sync.RWMutex
	lastCheckResult *checkResult

	mu        sync.RWMutex
	currState apiv1.HealthState
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:              cctx,
		cancel:           ccancel,
		nvmlInstance:     gpudInstance.NVMLInstance,
		rebootEventStore: gpudInstance.RebootEventStore,

		extraEventCh: make(chan *apiv1.Event, 256),
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
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
		ret = append(ret, resolveSXIDEvent(event))
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

// Check checks the current SXID errors (e.g., "gpud scan")
// by reading all kmsg logs.
func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu sxid")

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
		cr.reason = fmt.Sprintf("failed to read kmsg: %v", err)
		cr.health = apiv1.HealthStateTypeUnhealthy
		return cr
	}

	for _, kmsg := range kmsgs {
		sxidErr := Match(kmsg.Message)
		if sxidErr == nil {
			continue
		}
		cr.FoundErrors = append(cr.FoundErrors, FoundError{
			Kmsg:      kmsg,
			SXidError: *sxidErr,
		})
	}

	cr.reason = fmt.Sprintf("matched %d sxid errors from %d kmsg(s)", len(cr.FoundErrors), len(kmsgs))
	cr.health = apiv1.HealthStateTypeHealthy

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

// FoundError represents a found SXID error and its corresponding kmsg.
type FoundError struct {
	Kmsg kmsg.Message
	SXidError
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	if len(cr.FoundErrors) == 0 {
		return "no sxid error found"
	}

	now := time.Now().UTC()
	header := []string{"Time", "SXID", "DeviceUUID", "Name", "Critical", "Action(s)"}
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

		critical := false
		if foundErr.Detail != nil {
			critical = foundErr.Detail.CriticalErrorMarkedByGPUd
		}

		buf := bytes.NewBuffer(nil)
		table := tablewriter.NewWriter(buf)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader(header)
		table.Append([]string{
			foundErr.Kmsg.DescribeTimestamp(now),
			fmt.Sprintf("%d", foundErr.SXid),
			foundErr.DeviceUUID,
			foundErr.Detail.Name,
			strconv.FormatBool(critical),
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

func (cr *checkResult) HealthState() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
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
			sxidErr := Match(message.Message)
			if sxidErr == nil {
				log.Logger.Debugw("not sxid event, skip", "kmsg", message)
				continue
			}

			id := uuid.New()
			var sxidName string
			if sxidErr.Detail != nil {
				sxidName = sxidErr.Detail.Name
			}
			logger := log.Logger.With("id", id, "sxid", sxidErr.SXid, "sxidName", sxidName, "deviceUUID", sxidErr.DeviceUUID)
			logger.Infow("got sxid event", "kmsg", message, "kmsgTimestamp", message.Timestamp.Unix())

			event := apiv1.Event{
				Time: message.Timestamp,
				Name: EventNameErrorSXid,
				DeprecatedExtraInfo: map[string]string{
					EventKeyErrorSXidData: strconv.FormatInt(int64(sxidErr.SXid), 10),
					EventKeyDeviceUUID:    sxidErr.DeviceUUID,
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
			if err = c.updateCurrentState(); err != nil {
				logger.Errorw("failed to update current state", "error", err)
				continue
			}
		}
	}
}

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Debugw("set healthy event received")
	newEvent := &apiv1.Event{Time: metav1.Time{Time: time.Now().UTC()}, Name: "SetHealthy"}
	select {
	case c.extraEventCh <- newEvent:
	default:
		log.Logger.Debugw("channel full, set healthy event skipped")
	}
	return nil
}

func (c *component) updateCurrentState() error {
	if c.rebootEventStore == nil || c.eventBucket == nil {
		return nil
	}

	var rebootErr string
	rebootEvents, err := c.rebootEventStore.GetRebootEvents(c.ctx, time.Now().Add(-DefaultRetentionPeriod))
	if err != nil {
		rebootErr = fmt.Sprintf("failed to get reboot events: %v", err)
		log.Logger.Errorw("failed to get reboot events", "error", err)
	}

	localEvents, err := c.eventBucket.Get(c.ctx, time.Now().Add(-DefaultRetentionPeriod))
	if err != nil {
		return fmt.Errorf("failed to get all events: %w", err)
	}

	events := mergeEvents(rebootEvents, localEvents)

	c.mu.Lock()
	c.currState = evolveHealthyState(events)
	if rebootErr != "" {
		c.currState.Error = fmt.Sprintf("%s\n%s", rebootErr, c.currState.Error)
	}
	c.mu.Unlock()

	return nil
}

// mergeEvents merges two event slices and returns a time descending sorted new slice
func mergeEvents(a, b apiv1.Events) apiv1.Events {
	totalLen := len(a) + len(b)
	if totalLen == 0 {
		return nil
	}
	result := make(apiv1.Events, 0, totalLen)
	result = append(result, a...)
	result = append(result, b...)

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Time.After(result[j].Time.Time)
	})

	return result
}
