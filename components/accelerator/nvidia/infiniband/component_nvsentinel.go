package infiniband

import (
	"context"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvsentinel"
)

const (
	// checkNameSyslogsNICDriverError is the NVSentinel syslog monitor check
	// that reports NIC driver errors. Its error code carries the pattern name.
	checkNameSyslogsNICDriverError = "SysLogsNICDriverError"

	// EventKeyNVSentinelCheckName records the NVSentinel check that produced
	// the event.
	EventKeyNVSentinelCheckName = "nvsentinel_check_name"
	// EventKeyNVSentinelEventID records the NVSentinel event ID for
	// cross-referencing with the NVSentinel datastore.
	EventKeyNVSentinelEventID = "nvsentinel_event_id"
	// EventKeyNVSentinelNICDevice records the NIC device name (for example
	// "mlx5_0") from the NVSentinel NIC entity.
	EventKeyNVSentinelNICDevice = "nvsentinel_nic_device"
)

// nvsPatternToEventName maps the NVSentinel NIC driver pattern names to this
// component's event names. NVSentinel patterns without a GPUd counterpart
// are skipped: the component's event vocabulary is fixed, and its health
// evaluation reads live port state, not the event bucket.
var nvsPatternToEventName = map[string]string{
	"pci_power_insufficient": eventPCIPowerInsufficient,
	"port_module_high_temp":  eventPortModuleHighTemperature,
	"access_reg_failed":      eventAccessRegFailed,
}

// eventNameToNVSPattern is the reverse of nvsPatternToEventName, used by the
// kmsg matcher to find the NVSentinel pattern that covers a GPUd event.
var eventNameToNVSPattern = func() map[string]string {
	m := make(map[string]string, len(nvsPatternToEventName))
	for pattern, name := range nvsPatternToEventName {
		m[name] = pattern
	}
	return m
}()

// watchNVSentinel subscribes the component to the NVSentinel event source.
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

// matchNVSentinelNIC maps an NVSentinel event to this component's event name.
// ok is false when the event is not an InfiniBand data point that this
// component reports.
func matchNVSentinelNIC(ev nvsentinel.HealthEvent) (eventName string, ok bool) {
	if ev.CheckName != checkNameSyslogsNICDriverError {
		return "", false
	}
	if len(ev.ErrorCodes) == 0 {
		return "", false
	}
	name, found := nvsPatternToEventName[ev.ErrorCodes[0]]
	return name, found
}

// onNVSentinelEvent translates one NVSentinel event into the component's own
// event format and inserts it into the event bucket.
func (c *component) onNVSentinelEvent(ev nvsentinel.HealthEvent) {
	if c.eventBucket == nil {
		return
	}

	// Recovery events carry no new data point for this component.
	if ev.IsHealthy {
		log.Logger.Debugw("skipping healthy nvsentinel event", "checkName", ev.CheckName)
		return
	}

	eventName, ok := matchNVSentinelNIC(ev)
	if !ok {
		return
	}

	eventType := string(apiv1.EventTypeCritical)
	if ev.IsFatal {
		eventType = string(apiv1.EventTypeFatal)
	}

	device, _ := ev.EntityValue("NIC")
	event := eventstore.Event{
		Time:    ev.GeneratedTimestamp,
		Name:    eventName,
		Type:    eventType,
		Message: ev.Message,
		ExtraInfo: map[string]string{
			EventKeyNVSentinelCheckName: ev.CheckName,
			EventKeyNVSentinelEventID:   ev.ID,
			EventKeyNVSentinelNICDevice: device,
		},
	}

	// When GPUd's own kmsg detection won the delivery race, its copy is
	// already stored. Skip the NVSentinel copy to keep one event per incident.
	if c.hasStoredEventTwin(eventName, ev.GeneratedTimestamp) {
		log.Logger.Infow("gpud already stored this infiniband data point, skipping nvsentinel copy",
			"eventName", eventName, "checkName", ev.CheckName)
		return
	}

	if err := c.eventBucket.Insert(c.ctx, event); err != nil {
		log.Logger.Errorw("failed to insert nvsentinel infiniband event", "error", err)
	}
}

// matchPreferNVSentinel wraps the kmsg matcher: when NVSentinel already
// reported the same NIC driver pattern within the dedup window, the kmsg line
// produces no event, so the NVSentinel copy stays the single record.
func (c *component) matchPreferNVSentinel(line string) (string, string) {
	eventName, message := Match(line)
	if eventName == "" || c.nvsSource == nil {
		return eventName, message
	}

	pattern, ok := eventNameToNVSPattern[eventName]
	if !ok {
		return eventName, message
	}

	covered := c.nvsSource.Covers(c.nvsDedupWindow, func(ev nvsentinel.HealthEvent) bool {
		if ev.CheckName != checkNameSyslogsNICDriverError || len(ev.ErrorCodes) == 0 {
			return false
		}
		return ev.ErrorCodes[0] == pattern
	})
	if covered {
		log.Logger.Infow("nvsentinel covers this infiniband data point, skipping gpud-native insert",
			"eventName", eventName)
		return "", ""
	}

	return eventName, message
}

// hasStoredEventTwin reports whether the bucket already holds an event with
// the same name within the dedup window around ts.
func (c *component) hasStoredEventTwin(eventName string, ts time.Time) bool {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	events, err := c.eventBucket.Get(ctx, ts.Add(-c.nvsDedupWindow))
	cancel()
	if err != nil {
		log.Logger.Warnw("failed to check stored infiniband events", "error", err)
		return false
	}

	for _, stored := range events {
		if stored.Name == eventName && !stored.Time.After(ts.Add(c.nvsDedupWindow)) {
			return true
		}
	}
	return false
}
