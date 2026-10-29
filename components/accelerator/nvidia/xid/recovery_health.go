package xid

import (
	"slices"
	"strconv"
	"strings"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

func repairPriority(actions *apiv1.SuggestedActions) int {
	if actions == nil || len(actions.RepairActions) == 0 {
		return 0
	}
	switch actions.RepairActions[0] {
	case apiv1.RepairActionTypeHardwareInspection:
		return 3
	case apiv1.RepairActionTypeRebootSystem:
		return 2
	default:
		return 1
	}
}

func healthPriority(health apiv1.HealthStateType) int {
	switch health {
	case apiv1.HealthStateTypeUnhealthy:
		return 2
	case apiv1.HealthStateTypeDegraded:
		return 1
	default:
		return 0
	}
}

type recoveryEntry struct {
	detail    xidErrorEventDetail
	health    apiv1.HealthStateType
	timestamp time.Time
	sequence  string
	bootID    string
}

// Keep XID 154 ownership separate from originating faults. A recognized transition
// replaces only that GPU's recovery guidance, never another diagnosis.
type recoveryAccumulator struct {
	reports map[string][]recoveryEntry
}

func (r *recoveryAccumulator) reboot() {
	// Reboot resets the driver's recovery state; it need not emit a None report.
	clear(r.reports)
}

func xidDeviceKey(id string, devices map[string]device.Device) string {
	if uuid := convertBusIDToUUID(id, devices); uuid != "" {
		return uuid
	}
	if strings.HasPrefix(id, "GPU-") {
		return id
	}
	return strings.ToLower(normalizePCIBDF(strings.TrimSuffix(id, ".0")))
}

func (r *recoveryAccumulator) update(detail xidErrorEventDetail, event eventstore.Event, devices map[string]device.Device) {
	health := apiv1.HealthStateTypeHealthy
	switch event.Type {
	case string(apiv1.EventTypeFatal):
		health = apiv1.HealthStateTypeUnhealthy
	case string(apiv1.EventTypeCritical):
		health = apiv1.HealthStateTypeDegraded
	}
	if r.reports == nil {
		r.reports = make(map[string][]recoveryEntry)
	}
	key := xidDeviceKey(detail.DeviceUUID, devices)
	r.reports[key] = append(r.reports[key], recoveryEntry{
		detail: detail, health: health, timestamp: event.Time,
		sequence: event.ExtraInfo[eventKeyKmsgSequence], bootID: event.ExtraInfo[eventKeyKmsgBootID],
	})
}

// currentRecovery uses kernel position only when every readable report has a
// distinct position in the same boot. A matching From/To chain is not ordering
// evidence: a request can recur before or after a None with that same From.
func currentRecovery(reports []recoveryEntry) recoveryEntry {
	known := make([]recoveryEntry, 0, len(reports))
	for _, report := range reports {
		if knownRecoveryAction(report.detail.RecoveryAction) {
			known = append(known, report)
		}
	}
	// Unreadable or future ABI values cannot retire a known recovery request.
	// Their raw evidence remains available through the Events API.
	if len(known) > 0 {
		reports = known
	}
	seen := make(map[int64]bool, len(reports))
	hasSequence, ordered := false, true
	bootID := ""
	latest, highest := reports[0], int64(-1)
	for _, report := range reports {
		hasSequence = hasSequence || report.sequence != ""
		sequence, err := strconv.ParseInt(report.sequence, 10, 64)
		if err != nil || sequence < 0 || seen[sequence] {
			ordered = false
		}
		// Legacy events have no boot ID; the accumulator's reboot boundary
		// still scopes their sequence. Explicitly different boots never order.
		if report.bootID != "" {
			if bootID != "" && report.bootID != bootID {
				ordered = false
			}
			bootID = report.bootID
		}
		seen[sequence] = true
		if sequence > highest {
			latest, highest = report, sequence
		}
	}
	if ordered {
		return latest
	}
	// NVSentinel and older stored events have no kernel position. Their source
	// timestamps can establish order, but ties must retain the stronger request.
	if !hasSequence {
		latestTime := reports[0].timestamp
		timed := true
		for _, report := range reports {
			timed = timed && !report.timestamp.IsZero()
			if report.timestamp.After(latestTime) {
				latestTime = report.timestamp
			}
		}
		if timed {
			newest := make([]recoveryEntry, 0, len(reports))
			for _, report := range reports {
				if report.timestamp.Equal(latestTime) {
					newest = append(newest, report)
				}
			}
			reports = newest
		}
	}
	strongest := reports[0]
	for _, report := range reports[1:] {
		if healthPriority(report.health) > healthPriority(strongest.health) ||
			(report.health == strongest.health && repairPriority(report.detail.SuggestedActionsByGPUd) > repairPriority(strongest.detail.SuggestedActionsByGPUd)) ||
			(recoveryIsNone(strongest) && !recoveryIsNone(report)) {
			strongest = report
		}
	}
	return strongest
}

func recoveryIsNone(entry recoveryEntry) bool {
	return knownRecoveryAction(entry.detail.RecoveryAction) && entry.detail.RecoveryAction.To == 0
}

func (r *recoveryAccumulator) health(devices map[string]device.Device) apiv1.HealthState {
	state := apiv1.HealthState{Health: apiv1.HealthStateTypeHealthy}
	// Stable order also makes combined originating/recovery evidence deterministic.
	keys := make([]string, 0, len(r.reports))
	for key := range r.reports {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		entry := currentRecovery(r.reports[key])
		if recoveryIsNone(entry) {
			continue
		}
		detail, health := entry.detail, entry.health
		if healthPriority(health) > healthPriority(state.Health) || (health == state.Health && repairPriority(detail.SuggestedActionsByGPUd) >= repairPriority(state.SuggestedActions)) {
			state.Health = health
			state.SuggestedActions = detail.SuggestedActionsByGPUd
		}
		if state.Reason != "" {
			state.Reason += "; "
		}
		state.Reason += detail.buildMessage(devices)
	}
	return state
}

func mergeRecoveryHealth(fault, recovery apiv1.HealthState) apiv1.HealthState {
	if recovery.Reason == "" {
		return fault
	}
	// Choose health and actions together. Repair priority only breaks a tie
	// in health, so an informational IGNORE cannot mask degraded guidance.
	if healthPriority(recovery.Health) > healthPriority(fault.Health) ||
		(recovery.Health == fault.Health && repairPriority(recovery.SuggestedActions) > repairPriority(fault.SuggestedActions)) {
		fault.Health = recovery.Health
		fault.SuggestedActions = recovery.SuggestedActions
		if fault.Reason != "" && fault.Reason != "XIDComponent is healthy" {
			recovery.Reason += "; " + fault.Reason
		}
		fault.Reason = recovery.Reason
	} else if fault.Reason == "XIDComponent is healthy" || fault.Reason == "" {
		fault.Reason = recovery.Reason
	} else {
		fault.Reason += "; " + recovery.Reason
	}
	return fault
}
