package infiniband

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvsentinel"
)

const (
	// checkNameSyslogsNICDriverError is the NVSentinel check that reports NIC
	// driver errors from the syslog monitor. The error code holds the pattern
	// name (for example "pci_power_insufficient").
	checkNameSyslogsNICDriverError = "SysLogsNICDriverError"

	// EventKeyNVSentinelCheckName records which NVSentinel check produced the
	// event.
	EventKeyNVSentinelCheckName = "nvsentinel_check_name"
	// EventKeyNVSentinelEventID records the NVSentinel event ID. Operators can
	// cross-reference it with the NVSentinel datastore.
	EventKeyNVSentinelEventID = "nvsentinel_event_id"
	// EventKeyNVSentinelNICDevice records the NIC device name (for example
	// "mlx5_0") from the NVSentinel NIC entity.
	EventKeyNVSentinelNICDevice = "nvsentinel_nic_device"
)

// nvsPatternToEventName maps NVSentinel NIC driver pattern names to this
// component's event names. Patterns without a GPUd counterpart are skipped.
// The component's event vocabulary is fixed, and its health evaluation
// reads live port state, not the event bucket.
var nvsPatternToEventName = map[string]string{
	"pci_power_insufficient": eventPCIPowerInsufficient,
	"port_module_high_temp":  eventPortModuleHighTemperature,
	"access_reg_failed":      eventAccessRegFailed,
}

// eventNameToNVSPattern reverses nvsPatternToEventName. The kmsg matcher
// uses it to find which NVSentinel pattern covers a GPUd event.
var eventNameToNVSPattern = func() map[string]string {
	m := make(map[string]string, len(nvsPatternToEventName))
	for pattern, name := range nvsPatternToEventName {
		m[name] = pattern
	}
	return m
}()

// watchNVSentinel subscribes the component to the NVSentinel event source
// and starts a goroutine that forwards matching NIC driver events into
// the component's event bucket.
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

// matchNVSentinelNIC maps an NVSentinel NIC driver event to this
// component's event name. ok is false when the event does not report an
// InfiniBand data point this component covers.
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

// onNVSentinelEvent translates a qualifying NVSentinel event into the
// component's event format and inserts it into the event bucket.
func (c *component) onNVSentinelEvent(ev nvsentinel.HealthEvent) {
	if c.eventBucket == nil {
		return
	}

	// A healthy event does not carry a new data point. Log and skip.
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

	// When GPUd's kmsg detection won the delivery race, its copy is
	// already in the bucket. Skip this copy so the incident stays at
	// one event. The data point is already durable, so coverage is safe
	// to claim.
	if c.hasStoredEventTwin(eventName, device, ev.GeneratedTimestamp) {
		log.Logger.Infow("gpud already stored this infiniband data point, skipping nvsentinel copy",
			"eventName", eventName, "checkName", ev.CheckName, "device", device)
		if c.nvsSource != nil {
			c.nvsSource.RecordCoverage(ev)
		}
		return
	}

	if err := c.eventBucket.Insert(c.ctx, event); err != nil {
		// The insert failed, so no coverage is reported: the native kmsg
		// path stays free to store its own copy of this incident.
		log.Logger.Errorw("failed to insert nvsentinel infiniband event", "error", err)
		return
	}

	// Coverage is reported only after the insert succeeds, so a failed
	// insert never suppresses native detection of this incident.
	if c.nvsSource != nil {
		c.nvsSource.RecordCoverage(ev)
	}
}

// matchPreferNVSentinel wraps the kmsg matcher. When NVSentinel already
// reported the same NIC driver pattern within the dedup window, the kmsg
// line produces no event — the NVSentinel copy stays the single record.
//
// ACCESS_REG failures are correlated per device: the native normalized
// message carries the adapter's PCI BDF, while the NVSentinel event names
// the NIC (for example "mlx5_0"), so the two identities are compared
// through the sysfs device symlink. When the identities cannot be compared
// the native event is retained — suppressing it could hide a distinct
// device incident.
func (c *component) matchPreferNVSentinel(line string) (string, string) {
	eventName, message := Match(line)
	if eventName == "" || c.nvsSource == nil {
		return eventName, message
	}

	pattern, ok := eventNameToNVSPattern[eventName]
	if !ok {
		return eventName, message
	}

	// The ACCESS_REG dedup policy is per-device (see kmsgEventDedupWindow).
	// Without the native PCI address the devices cannot be compared, so
	// keep the native event rather than suppressing it on a pattern-only
	// match from a potentially different adapter.
	nativePCI := ""
	if eventName == eventAccessRegFailed {
		nativePCI = pciDeviceFromMessage(message)
		if nativePCI == "" {
			return eventName, message
		}
	}

	covered := c.nvsSource.Covers(c.nvsDedupWindow, func(ev nvsentinel.HealthEvent) bool {
		// A healthy event carries no new data point: it must never
		// suppress a native incident. The source already excludes healthy
		// events from coverage; this guard keeps the intent explicit here.
		if ev.IsHealthy {
			return false
		}
		if ev.CheckName != checkNameSyslogsNICDriverError || len(ev.ErrorCodes) == 0 {
			return false
		}
		if ev.ErrorCodes[0] != pattern {
			return false
		}
		if eventName == eventAccessRegFailed {
			nic, _ := ev.EntityValue("NIC")
			evPCI := c.resolveNICPCIAddress(nic)
			if evPCI == "" {
				// The NVSentinel event names no comparable device; it must
				// not suppress this native report.
				return false
			}
			return evPCI == nativePCI
		}
		return true
	})
	if covered {
		log.Logger.Infow("nvsentinel covers this infiniband data point, skipping gpud-native insert",
			"eventName", eventName)
		return "", ""
	}

	return eventName, message
}

// hasStoredEventTwin reports whether the bucket already holds an event
// with the same name within the dedup window around ts. ACCESS_REG events
// are additionally correlated by NIC device: two adapters can fail within
// the window, and the component's dedup policy for ACCESS_REG is
// per-device (see kmsgEventDedupWindow). When the incoming NVSentinel
// event names no device, the match deliberately falls back to the
// name-only policy.
func (c *component) hasStoredEventTwin(eventName string, device string, ts time.Time) bool {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	events, err := c.eventBucket.Get(ctx, ts.Add(-c.nvsDedupWindow))
	cancel()
	if err != nil {
		log.Logger.Warnw("failed to check stored infiniband events", "error", err)
		return false
	}

	for _, stored := range events {
		if stored.Name != eventName || stored.Time.After(ts.Add(c.nvsDedupWindow)) {
			continue
		}
		if eventName == eventAccessRegFailed && device != "" {
			if !c.sameAccessRegDevice(stored, device) {
				continue
			}
		}
		return true
	}
	return false
}

// sameAccessRegDevice reports whether a stored ACCESS_REG event belongs to
// the same NIC as the incoming NVSentinel event. A stored NVSentinel copy
// names the NIC directly. A native kmsg copy does not populate the
// NVSentinel-specific key, but its normalized message embeds the adapter's
// PCI BDF, which is mapped back to the NIC name through the sysfs device
// symlink. When the stored event's device identity cannot be determined,
// no twin is claimed: skipping the incoming event would also mark it
// covered and lose a potentially distinct device incident.
func (c *component) sameAccessRegDevice(stored eventstore.Event, device string) bool {
	if storedDevice := stored.ExtraInfo[EventKeyNVSentinelNICDevice]; storedDevice != "" {
		return storedDevice == device
	}
	storedPCI := pciDeviceFromMessage(stored.Message)
	if storedPCI == "" {
		return false
	}
	devicePCI := c.resolveNICPCIAddress(device)
	if devicePCI == "" {
		return false
	}
	return storedPCI == devicePCI
}

// pciDeviceFromMessage extracts the PCI BDF the kmsg matcher embedded in a
// normalized ACCESS_REG message ("(PCI device 0000:5c:00.0)", see
// accessRegFailedMessage). It returns "" when the message carries no PCI
// address.
func pciDeviceFromMessage(message string) string {
	idx := strings.Index(message, pciDeviceMessagePrefix)
	if idx < 0 {
		return ""
	}
	rest := message[idx+len(pciDeviceMessagePrefix):]
	if end := strings.IndexByte(rest, ')'); end >= 0 {
		rest = rest[:end]
	}
	return strings.ToLower(rest)
}

// resolveNICPCIAddress maps an InfiniBand device name (for example
// "mlx5_0") to its PCI BDF address (for example "0000:5c:00.0") by reading
// the device symlink under the InfiniBand class root directory
// (/sys/class/infiniband/<name>/device). It returns "" when the mapping
// cannot be resolved. Successful resolutions are cached: the name-to-BDF
// mapping is static for the lifetime of a boot, and ACCESS_REG floods can
// otherwise turn into repeated reads of the same symlink.
func (c *component) resolveNICPCIAddress(nicName string) string {
	if nicName == "" {
		return ""
	}

	c.nicPCICacheMu.RLock()
	bdf, ok := c.nicPCICache[nicName]
	c.nicPCICacheMu.RUnlock()
	if ok {
		return bdf
	}

	rootDir := c.toolOverwrites.InfinibandClassRootDir
	if rootDir == "" {
		rootDir = infinibandclass.DefaultRootDir
	}
	link, err := os.Readlink(filepath.Join(rootDir, nicName, "device"))
	if err != nil {
		return ""
	}
	bdf = strings.ToLower(filepath.Base(link))
	if !compiledPCIDevice.MatchString(bdf) {
		return ""
	}

	c.nicPCICacheMu.Lock()
	if c.nicPCICache == nil {
		c.nicPCICache = make(map[string]string)
	}
	c.nicPCICache[nicName] = bdf
	c.nicPCICacheMu.Unlock()
	return bdf
}
