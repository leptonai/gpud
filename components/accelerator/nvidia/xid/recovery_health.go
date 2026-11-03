package xid

import (
	"slices"
	"strings"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
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
}

// Keep XID 154 ownership separate from originating faults. A recognized transition
// replaces only that GPU's recovery guidance, never another diagnosis.
type recoveryAccumulator struct {
	active map[string]recoveryEntry
}

func (r *recoveryAccumulator) reboot() {
	// Reboot resets the driver's recovery state; it need not emit a None report.
	clear(r.active)
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

func (r *recoveryAccumulator) update(detail xidErrorEventDetail, eventType string, timestamp time.Time, devices map[string]device.Device) {
	key := xidDeviceKey(detail.DeviceUUID, devices)
	current, exists := r.active[key]
	// Unreadable or future ABI values cannot retire a known recovery request.
	// Their raw evidence remains available through the Events API.
	if !knownRecoveryAction(detail.RecoveryAction) && knownRecoveryAction(current.detail.RecoveryAction) {
		return
	}
	health := apiv1.HealthStateTypeHealthy
	switch eventType {
	case string(apiv1.EventTypeFatal):
		health = apiv1.HealthStateTypeUnhealthy
	case string(apiv1.EventTypeCritical):
		health = apiv1.HealthStateTypeDegraded
	}
	clearRequest := knownRecoveryAction(detail.RecoveryAction) && detail.RecoveryAction.To == 0
	// SQLite keeps whole seconds, and producer arrival order is not driver
	// order. Tied or missing timestamps cannot establish a weaker recovery
	// state, including None. A strictly later transition can still retire it.
	if exists && (timestamp.IsZero() || current.timestamp.IsZero() || !timestamp.After(current.timestamp)) &&
		(clearRequest || healthPriority(health) < healthPriority(current.health) ||
			(health == current.health && repairPriority(detail.SuggestedActionsByGPUd) < repairPriority(current.detail.SuggestedActionsByGPUd))) {
		return
	}
	if clearRequest {
		delete(r.active, key)
		return
	}
	if r.active == nil {
		r.active = make(map[string]recoveryEntry)
	}
	// XID 154 reports an action, not a diagnosis. Repeated requests cannot
	// establish hardware failure; originating XIDs keep their own thresholds.
	r.active[key] = recoveryEntry{detail: detail, health: health, timestamp: timestamp}
}

func (r *recoveryAccumulator) health(devices map[string]device.Device) apiv1.HealthState {
	state := apiv1.HealthState{Health: apiv1.HealthStateTypeHealthy}
	// Stable order also makes combined originating/recovery evidence deterministic.
	keys := make([]string, 0, len(r.active))
	for key := range r.active {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		entry := r.active[key]
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
