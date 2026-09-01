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
	// EventKeyNVSentinelSwitchPCI records the normalized NVSwitch PCI address
	// from the NVSentinel PCI entity (for example "0000:05:00.0"). An SXid
	// error belongs to a switch, not to a GPU: the stored-twin check compares
	// this switch identity rather than the GPU UUID, because two switches
	// reporting the same SXid can map to the same GPU (or to no GPU at all).
	EventKeyNVSentinelSwitchPCI = "nvsentinel_switch_pci"
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

	sxidNum, deviceUUID, switchPCI, ok := c.matchNVSentinelSXid(ev)
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
			EventKeyNVSentinelSwitchPCI: switchPCI,
		},
	}

	// When GPUd's kmsg detection won the delivery race, its copy is
	// already in the bucket. Skip this NVSentinel copy so the incident
	// stays at one event. The data point is already durable, so coverage
	// is safe to claim. The twin check compares the switch PCI identity:
	// an SXid error belongs to a switch, and two switches reporting the
	// same SXid are distinct incidents even when they map to the same GPU.
	if c.hasStoredSXidTwin(sxidNum, switchPCI, ev.GeneratedTimestamp) {
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
// point for this component, and extracts the SXid number, the GPU UUID, and
// the normalized NVSwitch PCI address.
//
// The matcher accepts events with the SysLogsSXIDError check name or the
// NVSWITCH component class. Any numeric error code qualifies. The GPUd
// catalog enriches known codes. An unknown code is still stored —
// NVSentinel's detection must not be dropped just because the catalog is
// older than the hardware.
//
// The GPU UUID comes from the GPU_UUID entity and identifies the impacted
// GPU for operators. The switch PCI address comes from the PCI entity and
// identifies the reporting switch; it is the identity used for incident
// correlation, because an SXid error belongs to a switch, not to a GPU.
func (c *component) matchNVSentinelSXid(ev nvsentinel.HealthEvent) (sxidNum int, deviceUUID string, switchPCI string, ok bool) {
	if ev.CheckName != checkNameSyslogsSXIDError && ev.ComponentClass != "NVSWITCH" {
		return 0, "", "", false
	}

	num := -1
	for _, code := range ev.ErrorCodes {
		if n, err := strconv.Atoi(code); err == nil {
			num = n
			break
		}
	}
	if num < 0 {
		return 0, "", "", false
	}

	// NVSentinel reports the GPU UUID directly in the GPU_UUID entity.
	// No PCI-to-UUID resolution is needed.
	deviceUUID, _ = ev.EntityValue("GPU_UUID")

	// NVSentinel carries the switch PCI address in its PCI entity
	// ("0000:05:00.0"); the native kmsg report carries the same address
	// with a "PCI:" prefix. Normalize for comparison.
	pci, _ := ev.EntityValue("PCI")
	switchPCI = normalizePCIAddress(pci)
	return num, deviceUUID, switchPCI, true
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
		evSXid, _, evPCI, ok := c.matchNVSentinelSXid(ev)
		if !ok || evSXid != sxidErr.SXid {
			return false
		}
		// The SXid number alone does not identify the incident on a
		// multi-switch node: two NVSwitches can emit the same code within
		// the dedup window. When both sides name the switch PCI address,
		// only the same switch covers. When either side lacks the address,
		// deliberately fall back to the number-only match — both detectors
		// read the same kernel report.
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
// same SXid data point within the dedup window around ts. Incidents are
// correlated by the switch PCI address: an SXid error belongs to an NVSwitch,
// and two switches reporting the same SXid are distinct incidents even when
// they map to the same GPU UUID (or when the GPU UUID is empty). Claiming a
// twin also records NVSentinel coverage for the incoming event, so a false
// twin would suppress the second switch's native report without ever storing
// it.
func (c *component) hasStoredSXidTwin(sxidNum int, switchPCI string, ts time.Time) bool {
	switchPCI = normalizePCIAddress(switchPCI)

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
			if legacy != sxidNum {
				continue
			}
			// The native kmsg copy stores the switch PCI address
			// ("PCI:0000:05:00.0") in the device_uuid key.
			storedPCI := normalizePCIAddress(stored.ExtraInfo[EventKeyDeviceUUID])
			if !sameSwitchPCI(storedPCI, switchPCI) {
				continue
			}
			return true
		}
		var detail sxidErrorEventDetail
		if err := json.Unmarshal([]byte(raw), &detail); err != nil {
			continue
		}
		storedSXid, ok := intFromUint64(detail.SXid)
		if !ok || storedSXid != sxidNum {
			continue
		}
		// NVSentinel-stored copies persist the switch PCI address under the
		// nvsentinel_switch_pci key. detail.DeviceUUID is the GPU UUID and
		// must not be compared here: two switches can map to the same GPU.
		storedPCI := normalizePCIAddress(stored.ExtraInfo[EventKeyNVSentinelSwitchPCI])
		if !sameSwitchPCI(storedPCI, switchPCI) {
			continue
		}
		return true
	}
	return false
}

// sameSwitchPCI reports whether two normalized switch PCI addresses identify
// the same NVSwitch. When either side lacks the address, the comparison is
// inconclusive and deliberately falls back to the SXid-number-only match —
// both detectors read the same kernel report.
func sameSwitchPCI(a, b string) bool {
	if a != "" && b != "" {
		return a == b
	}
	return true
}
