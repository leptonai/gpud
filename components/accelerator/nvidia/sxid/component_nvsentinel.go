package sxid

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvsentinel"
)

const (
	// dataSourceNVSentinel is the data_source value for events that came from
	// NVSentinel rather than GPUd's own kmsg scanning.
	dataSourceNVSentinel = "nvsentinel"

	// checkNameSyslogsSXIDError is the NVSentinel check that reports NVSwitch
	// SXid errors from the syslog monitor.
	checkNameSyslogsSXIDError = "SysLogsSXIDError"

	// EventKeyNVSentinelCheckName records which NVSentinel check produced the
	// event.
	EventKeyNVSentinelCheckName = "nvsentinel_check_name"
	// EventKeyNVSentinelEventID records the NVSentinel event ID. Operators can
	// cross-reference it with the NVSentinel datastore.
	EventKeyNVSentinelEventID = "nvsentinel_event_id"
)

// pendingEvent is one event queued for insertion into the event bucket.
// nvsEvent, when non-nil, is the NVSentinel health event the entry was
// translated from. Coverage is reported to the NVSentinel source only after
// the bucket insert succeeds, so a failed insert never suppresses GPUd's
// native detection of the same incident.
type pendingEvent struct {
	event    *eventstore.Event
	nvsEvent *nvsentinel.HealthEvent
}

// watchNVSentinel subscribes the component to the NVSentinel event source
// and starts a goroutine that forwards matching SXid events into the
// component's event bucket. The bucket is the same one kmsg-detected
// events use, so health evolution and the events API work unchanged.
func (c *component) watchNVSentinel() {
	if c.nvsSource == nil {
		return
	}

	ch, unsubscribe := c.nvsSource.Subscribe()
	c.nvsUnsubscribe = unsubscribe

	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				c.onNVSentinelEvent(ev)
			}
		}
	}()
}

// onNVSentinelEvent translates one NVSentinel event into the component's own
// event format and queues it for insertion.
func (c *component) onNVSentinelEvent(ev nvsentinel.HealthEvent) {
	if c.eventBucket == nil {
		return
	}

	// A healthy event does not carry a new data point for this component.
	// GPUd heals its sxid state through the lookback window and reboot
	// tracking, so the event is logged and skipped.
	if ev.IsHealthy {
		log.Logger.Debugw("skipping healthy nvsentinel event", "checkName", ev.CheckName)
		return
	}

	sxidNum, deviceUUID, ok := c.matchNVSentinelSXid(ev)
	if !ok {
		return
	}

	// The NVSentinel verdict drives the GPUd event severity.
	// isFatal → Fatal. Unhealthy and not fatal → Critical.
	eventType := string(apiv1.EventTypeCritical)
	if ev.IsFatal {
		eventType = string(apiv1.EventTypeFatal)
	}

	sxidValue, ok := uint64FromInt(sxidNum)
	if !ok {
		return
	}

	payload := sxidErrorEventDetail{
		Time:       metav1.NewTime(ev.GeneratedTimestamp),
		DataSource: dataSourceNVSentinel,
		DeviceUUID: deviceUUID,
		SXid:       sxidValue,
	}
	// Start with the GPUd catalog entry. Then let the NVSentinel
	// recommended action win when it maps to a GPUd repair action.
	if detail, found := GetDetail(sxidNum); found {
		payload.SuggestedActionsByGPUd = detail.SuggestedActionsByGPUd
	}
	if actions := ev.SuggestedRepairActions(); len(actions) > 0 {
		payload.SuggestedActionsByGPUd = &apiv1.SuggestedActions{RepairActions: actions}
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		log.Logger.Errorw("failed to marshal nvsentinel sxid payload", "error", err)
		return
	}

	event := &eventstore.Event{
		Time:    ev.GeneratedTimestamp,
		Name:    EventNameErrorSXid,
		Type:    eventType,
		Message: ev.Message,
		ExtraInfo: map[string]string{
			EventKeyErrorSXidData:       string(rawPayload),
			EventKeyDeviceUUID:          deviceUUID,
			EventKeyNVSentinelCheckName: ev.CheckName,
			EventKeyNVSentinelEventID:   ev.ID,
		},
	}

	// When GPUd's kmsg detection won the delivery race, its copy is
	// already in the bucket. Skip this NVSentinel copy so the incident
	// stays at one event. The data point is already durable, so coverage
	// is safe to claim.
	if c.hasStoredSXidTwin(sxidNum, deviceUUID, ev.GeneratedTimestamp) {
		log.Logger.Infow("gpud already stored this sxid data point, skipping nvsentinel copy",
			"sxid", sxidNum, "deviceUUID", deviceUUID, "checkName", ev.CheckName)
		if c.nvsSource != nil {
			c.nvsSource.RecordCoverage(ev)
		}
		return
	}

	// Coverage is reported by the start loop only after the bucket insert
	// succeeds. Until then the kmsg path stays free to store its own copy.
	select {
	case c.extraEventCh <- pendingEvent{event: event, nvsEvent: &ev}:
	case <-c.ctx.Done():
	}
}

// matchNVSentinelSXid reports whether the NVSentinel event is an SXid data
// point for this component, and extracts the SXid number and GPU UUID.
//
// The matcher accepts events with the SysLogsSXIDError check name or the
// NVSWITCH component class. Any numeric error code qualifies. The GPUd
// catalog enriches known codes. An unknown code is still stored —
// NVSentinel's detection must not be dropped just because the catalog is
// older than the hardware. The GPU UUID comes from the PCI entity when
// the NVML device list can map it.
func (c *component) matchNVSentinelSXid(ev nvsentinel.HealthEvent) (int, string, bool) {
	if ev.CheckName != checkNameSyslogsSXIDError && ev.ComponentClass != "NVSWITCH" {
		return 0, "", false
	}

	sxidNum := -1
	for _, code := range ev.ErrorCodes {
		if n, err := strconv.Atoi(code); err == nil {
			sxidNum = n
			break
		}
	}
	if sxidNum < 0 {
		return 0, "", false
	}

	// NVSentinel reports the GPU UUID directly in the GPU_UUID entity.
	// No PCI-to-UUID resolution is needed.
	deviceUUID, _ := ev.EntityValue("GPU_UUID")
	return sxidNum, deviceUUID, true
}

func (c *component) nvsentinelCoversSXid(sxidErr *Error) bool {
	if c.nvsSource == nil || sxidErr == nil {
		return false
	}

	// The native kmsg report names the switch by PCI address
	// ("PCI:0000:05:00.0"); NVSentinel carries the same address in its PCI
	// entity ("0000:05:00.0").
	nativePCI := normalizePCIAddress(sxidErr.DeviceUUID)

	return c.nvsSource.Covers(c.nvsDedupWindow, func(ev nvsentinel.HealthEvent) bool {
		// A healthy event carries no new data point: it must never
		// suppress a native incident. The source already excludes healthy
		// events from coverage; this guard keeps the intent explicit here.
		if ev.IsHealthy {
			return false
		}
		evSXid, _, ok := c.matchNVSentinelSXid(ev)
		if !ok || evSXid != sxidErr.SXid {
			return false
		}
		// The SXid number alone does not identify the incident on a
		// multi-switch node: two NVSwitches can emit the same code within
		// the dedup window. When both sides name the switch PCI address,
		// only the same switch covers. When either side lacks the address,
		// deliberately fall back to the number-only match — both detectors
		// read the same kernel report.
		evPCI, _ := ev.EntityValue("PCI")
		evPCI = normalizePCIAddress(evPCI)
		if nativePCI != "" && evPCI != "" {
			return nativePCI == evPCI
		}
		return true
	})
}

// normalizePCIAddress canonicalizes a PCI BDF address for comparison. The
// kmsg report carries "PCI:0000:05:00.0" while the NVSentinel PCI entity
// carries "0000:05:00.0".
func normalizePCIAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if len(addr) >= 4 && strings.EqualFold(addr[:4], "PCI:") {
		addr = addr[4:]
	}
	return strings.ToLower(addr)
}

// hasStoredSXidTwin reports whether the bucket already holds an event for the
// same SXid data point within the dedup window around ts.
func (c *component) hasStoredSXidTwin(sxidNum int, deviceUUID string, ts time.Time) bool {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	events, err := c.eventBucket.Get(ctx, ts.Add(-c.nvsDedupWindow))
	cancel()
	if err != nil {
		log.Logger.Warnw("failed to check stored sxid events", "error", err)
		return false
	}

	for _, stored := range events {
		if stored.Name != EventNameErrorSXid {
			continue
		}
		if stored.Time.After(ts.Add(c.nvsDedupWindow)) {
			continue
		}

		// The stored payload is either the legacy plain-number format or the
		// JSON detail format. Try the legacy parse first; it is cheaper.
		raw := stored.ExtraInfo[EventKeyErrorSXidData]
		if legacy, err := strconv.Atoi(raw); err == nil {
			if legacy == sxidNum {
				return true
			}
			continue
		}
		var detail sxidErrorEventDetail
		if err := json.Unmarshal([]byte(raw), &detail); err != nil {
			continue
		}
		storedSXid, ok := intFromUint64(detail.SXid)
		if !ok || storedSXid != sxidNum {
			continue
		}
		if deviceUUID != "" && detail.DeviceUUID != "" && detail.DeviceUUID != deviceUUID {
			continue
		}
		return true
	}
	return false
}
