package xid

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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

	// EventKeyNVSentinelCheckName records which NVSentinel check produced the
	// event (for example "SysLogsXIDError").
	EventKeyNVSentinelCheckName = "nvsentinel_check_name"
	// EventKeyNVSentinelEventID records the NVSentinel event ID. Operators can
	// cross-reference it with the NVSentinel datastore.
	EventKeyNVSentinelEventID = "nvsentinel_event_id"
)

// watchNVSentinel subscribes the component to the NVSentinel event source
// and starts a goroutine that forwards matching Xid events into the
// component's event bucket. The bucket is the same one kmsg-detected
// events use, so health evolution and the events API work without changes.
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
	// GPUd heals its xid state through the lookback window and reboot
	// tracking, so the event is logged and skipped.
	if ev.IsHealthy {
		log.Logger.Debugw("skipping healthy nvsentinel event", "checkName", ev.CheckName)
		return
	}

	xidNum, deviceUUID, ok := c.matchNVSentinelXid(ev)
	if !ok {
		return
	}

	// The NVSentinel verdict drives the GPUd event severity.
	// isFatal → Fatal. Unhealthy and not fatal → Critical.
	// The GPUd catalog enriches the description and repair actions at
	// read time regardless of severity.
	eventType := string(apiv1.EventTypeCritical)
	if ev.IsFatal {
		eventType = string(apiv1.EventTypeFatal)
	}

	xidValue, ok := uint64FromInt(xidNum)
	if !ok {
		return
	}

	payload := xidErrorEventDetail{
		Time:       metav1.NewTime(ev.GeneratedTimestamp),
		DataSource: dataSourceNVSentinel,
		DeviceUUID: deviceUUID,
		Xid:        xidValue,
	}
	// Start with the GPUd catalog entry. Then let the NVSentinel
	// recommended action win when it maps to a GPUd repair action.
	if detail, found := GetDetail(xidNum); found {
		payload.Description = detail.Description
		payload.SuggestedActionsByGPUd = detail.SuggestedActionsByGPUd
	}
	if actions := ev.SuggestedRepairActions(); len(actions) > 0 {
		payload.SuggestedActionsByGPUd = &apiv1.SuggestedActions{RepairActions: actions}
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		log.Logger.Errorw("failed to marshal nvsentinel xid payload", "error", err)
		return
	}

	event := &eventstore.Event{
		Time:    ev.GeneratedTimestamp,
		Name:    EventNameErrorXid,
		Type:    eventType,
		Message: ev.Message,
		ExtraInfo: map[string]string{
			EventKeyErrorXidData:        string(rawPayload),
			EventKeyDeviceUUID:          deviceUUID,
			EventKeyNVSentinelCheckName: ev.CheckName,
			EventKeyNVSentinelEventID:   ev.ID,
		},
	}

	// When GPUd's kmsg detection won the delivery race, its copy is
	// already in the bucket. Skip this NVSentinel copy so the incident
	// stays at one event.
	if c.hasStoredXidTwin(xidNum, deviceUUID, ev.GeneratedTimestamp) {
		log.Logger.Infow("gpud already stored this xid data point, skipping nvsentinel copy",
			"xid", xidNum, "deviceUUID", deviceUUID, "checkName", ev.CheckName)
		return
	}

	// Count NVSentinel-delivered Xid errors the same as kmsg-detected ones,
	// so the metric keeps meaning "Xid occurred on this node".
	metricXIDErrs.With(prometheus.Labels{
		"uuid": convertBusIDToUUID(deviceUUID, c.devices),
		"xid":  strconv.Itoa(xidNum),
	}).Inc()

	select {
	case c.extraEventCh <- event:
	case <-c.ctx.Done():
	}
}

// checkNameSyslogsSXIDError is the NVSentinel syslog monitor SXid check.
// NVSentinel labels both its Xid and SXid syslog checks with component
// class GPU. This component explicitly excludes the SXid check because
// the sxid component owns those events.
const checkNameSyslogsSXIDError = "SysLogsSXIDError"

// matchNVSentinelXid reports whether the NVSentinel event carries an Xid
// data point for this component. It returns the Xid number and the GPU
// UUID. NVSentinel stores the UUID in the GPU_UUID entity.
//
// The matcher is data-driven rather than check-name-driven. An event
// qualifies when it targets the GPU component class and carries a numeric
// error code. The GPUd catalog enriches known codes. An unknown code is
// still stored — NVSentinel's detection must not be dropped just because
// the catalog is older than the hardware.
func (c *component) matchNVSentinelXid(ev nvsentinel.HealthEvent) (int, string, bool) {
	if ev.ComponentClass != "GPU" || ev.CheckName == checkNameSyslogsSXIDError {
		return 0, "", false
	}

	xidNum := -1
	for _, code := range ev.ErrorCodes {
		if n, err := strconv.Atoi(code); err == nil {
			xidNum = n
			break
		}
	}
	if xidNum < 0 {
		return 0, "", false
	}

	// Row remapping pending/failure (Xid 63/64) belongs to the remapped-rows
	// component. Apply the same discard as the kmsg path.
	if c.nvmlInstance != nil && c.nvmlInstance.GetMemoryErrorManagementCapabilities().RowRemapping && (xidNum == 63 || xidNum == 64) {
		return 0, "", false
	}

	// NVSentinel monitors report the GPU UUID under the GPU_UUID entity
	// type. Fall back to GPU for forward compatibility.
	deviceUUID, ok := ev.EntityValue("GPU_UUID")
	if !ok {
		deviceUUID, _ = ev.EntityValue("GPU")
	}
	return xidNum, deviceUUID, true
}

// nvsentinelCoversXid reports whether NVSentinel reported this Xid data
// point within the dedup window. The kmsg path calls it to decide whether to
// suppress its own copy.
func (c *component) nvsentinelCoversXid(xidErr *Error) bool {
	if c.nvsSource == nil || xidErr == nil {
		return false
	}

	deviceUUID := xidErr.DeviceUUID
	if uuid := convertBusIDToUUID(deviceUUID, c.devices); uuid != "" {
		deviceUUID = uuid
	}

	return c.nvsSource.Covers(c.nvsDedupWindow, func(ev nvsentinel.HealthEvent) bool {
		evXid, evUUID, ok := c.matchNVSentinelXid(ev)
		if !ok || evXid != xidErr.Xid {
			return false
		}
		// Both detectors read the same kernel report. Suppress the kmsg
		// copy unless the two sides name different devices — then they
		// describe distinct incidents on different GPUs.
		if deviceUUID != "" && evUUID != "" {
			return deviceUUID == evUUID
		}
		return true
	})
}

// hasStoredXidTwin reports whether the bucket already holds an event for
// the same Xid data point within the dedup window around ts. It handles
// both the JSON payload format and the legacy plain-number format.
func (c *component) hasStoredXidTwin(xidNum int, deviceUUID string, ts time.Time) bool {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	events, err := c.eventBucket.Get(ctx, ts.Add(-c.nvsDedupWindow))
	cancel()
	if err != nil {
		log.Logger.Warnw("failed to check stored xid events", "error", err)
		return false
	}

	for _, stored := range events {
		if stored.Name != EventNameErrorXid {
			continue
		}
		if stored.Time.After(ts.Add(c.nvsDedupWindow)) {
			continue
		}

		raw := stored.ExtraInfo[EventKeyErrorXidData]
		// Try the JSON payload (the current format) first.
		var detail xidErrorEventDetail
		if err := json.Unmarshal([]byte(raw), &detail); err == nil {
			storedXid, ok := intFromUint64(detail.Xid)
			if ok && storedXid == xidNum {
				if deviceUUID == "" || detail.DeviceUUID == "" || detail.DeviceUUID == deviceUUID {
					return true
				}
			}
			continue
		}
		// Fall back to the legacy plain-number format ("79").
		if legacyXid, err := strconv.Atoi(raw); err == nil && legacyXid == xidNum {
			// Legacy events carry the device UUID in the device_uuid key.
			storedUUID := stored.ExtraInfo[EventKeyDeviceUUID]
			if deviceUUID == "" || storedUUID == "" || storedUUID == deviceUUID {
				return true
			}
		}
	}
	return false
}
