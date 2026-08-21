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
	// dataSourceNVSentinel marks sxid events that came from NVSentinel
	// instead of GPUd's own kmsg scanning.
	dataSourceNVSentinel = "nvsentinel"

	// checkNameSyslogsSXIDError is the NVSentinel syslog monitor check that
	// reports NVSwitch SXid errors.
	checkNameSyslogsSXIDError = "SysLogsSXIDError"

	// EventKeyNVSentinelCheckName records the NVSentinel check that produced
	// the event.
	EventKeyNVSentinelCheckName = "nvsentinel_check_name"
	// EventKeyNVSentinelEventID records the NVSentinel event ID for
	// cross-referencing with the NVSentinel datastore.
	EventKeyNVSentinelEventID = "nvsentinel_event_id"
)

// watchNVSentinel subscribes the component to the NVSentinel event source.
// NVSentinel SXid data points enter the same event bucket as kmsg-detected
// ones, so thresholds, health evolution, and the events API stay unchanged.
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

	// Recovery events carry no new data point for this component. GPUd heals
	// its sxid state through the lookback window and reboot tracking, so a
	// healthy NVSentinel event is logged but not stored.
	if ev.IsHealthy {
		log.Logger.Debugw("skipping healthy nvsentinel event", "checkName", ev.CheckName)
		return
	}

	sxidNum, deviceUUID, ok := c.matchNVSentinelSXid(ev)
	if !ok {
		return
	}

	// NVSentinel reported a fatal verdict: map it to a fatal GPUd event.
	// A non-fatal unhealthy verdict maps to critical.
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
	// Enrich with the GPUd catalog entry, then let the NVSentinel recommended
	// action win when it maps to a GPUd repair action.
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

	// When GPUd's own kmsg detection won the delivery race, its copy is
	// already stored. Skip the NVSentinel copy to keep one event per incident.
	if c.hasStoredSXidTwin(sxidNum, deviceUUID, ev.GeneratedTimestamp) {
		log.Logger.Infow("gpud already stored this sxid data point, skipping nvsentinel copy",
			"sxid", sxidNum, "deviceUUID", deviceUUID, "checkName", ev.CheckName)
		return
	}

	select {
	case c.extraEventCh <- event:
	case <-c.ctx.Done():
	}
}

// matchNVSentinelSXid reports whether the NVSentinel event is an SXid data
// point for this component, and extracts the SXid number and GPU UUID.
//
// The SysLogsSXIDError check name and the NVSWITCH component class both
// identify SXid data points. Any numeric error code qualifies; the GPUd SXid
// catalog enriches known codes, and an unknown code is still stored because
// NVSentinel's detection must not be dropped only because the catalog is
// older than the hardware. The GPU UUID is derived from the PCI entity when
// possible, because NVSentinel reports the GPU as a numeric ID, not a UUID.
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

	deviceUUID := ""
	if pci, ok := ev.EntityValue("PCI"); ok {
		deviceUUID = c.deviceUUIDFromPCI(pci)
	}
	return sxidNum, deviceUUID, true
}

// deviceUUIDFromPCI resolves a PCI bus address from an NVSentinel event to a
// GPU UUID using the NVML device list. Kernel-style addresses
// ("0000:5c:00.0") and NVML-style addresses ("00000000:5C:00.0") differ in
// domain width and case; compare on the normalized tail.
func (c *component) deviceUUIDFromPCI(pci string) string {
	normalize := func(s string) string {
		s = strings.ToLower(strings.TrimPrefix(s, "PCI:"))
		parts := strings.SplitN(s, ":", 2)
		if len(parts) == 2 {
			// Drop the PCI domain; keep bus:device.function.
			s = parts[1]
		}
		return s
	}

	if c.nvmlInstance == nil {
		return ""
	}

	target := normalize(pci)
	for uuid, dev := range c.nvmlInstance.Devices() {
		if normalize(dev.PCIBusID()) == target {
			return uuid
		}
	}
	return ""
}

// nvsentinelCoversSXid reports whether NVSentinel already reported this SXid
// data point within the dedup window. The kmsg path uses it to prefer the
// NVSentinel copy of the same incident.
func (c *component) nvsentinelCoversSXid(sxidErr *Error) bool {
	if c.nvsSource == nil || sxidErr == nil {
		return false
	}

	return c.nvsSource.Covers(c.nvsDedupWindow, func(ev nvsentinel.HealthEvent) bool {
		evSXid, _, ok := c.matchNVSentinelSXid(ev)
		if !ok {
			return false
		}
		// SXid events describe an NVSwitch link, and the kmsg device UUID does
		// not appear in the NVSentinel entity list as a UUID, so match on the
		// SXid number and the window. Both detectors read the same kernel
		// report; a repeated number within the window is the same incident.
		return evSXid == sxidErr.SXid
	})
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

		// The stored payload may be the legacy plain-number format or the
		// JSON detail format.
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
