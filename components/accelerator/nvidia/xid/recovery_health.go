package xid

import (
	"fmt"
	"slices"
	"strings"

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

func recoveryHealthType(transition *RecoveryActionTransition) apiv1.HealthStateType {
	if transition != nil && transition.Valid {
		switch transition.To {
		case 0:
			return apiv1.HealthStateTypeHealthy
		case 1, 2, 4:
			return apiv1.HealthStateTypeUnhealthy
		}
	}
	return apiv1.HealthStateTypeDegraded
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

// Keep XID 154 ownership separate from originating faults. The latest transition
// on a GPU replaces only that GPU's recovery guidance, never another diagnosis.
type recoveryAccumulator struct {
	active        map[string]xidErrorEventDetail
	failedReboots map[string]int
}

func (r *recoveryAccumulator) reboot() {
	for key, detail := range r.active {
		if repairPriority(detail.SuggestedActionsByGPUd) == 2 {
			r.failedReboots[key]++
			delete(r.active, key)
		}
	}
}

func (r *recoveryAccumulator) update(detail xidErrorEventDetail, devices map[string]device.Device, rebootThreshold int, thresholds Thresholds) {
	key := detail.DeviceUUID
	if uuid := convertBusIDToUUID(key, devices); uuid != "" {
		key = uuid
	} else if !strings.HasPrefix(key, "GPU-") {
		key = strings.ToLower(normalizePCIBDF(strings.TrimSuffix(key, ".0")))
	}
	if detail.RecoveryAction != nil && detail.RecoveryAction.Valid && detail.RecoveryAction.To == 0 {
		delete(r.active, key)
		delete(r.failedReboots, key)
		return
	}
	if r.active == nil {
		r.active = make(map[string]xidErrorEventDetail)
		r.failedReboots = make(map[string]int)
	}
	if repairPriority(detail.SuggestedActionsByGPUd) == 2 && r.failedReboots[key] >= rebootThresholdForXID(154, rebootThreshold, thresholds) {
		detail.SuggestedActionsByGPUd.RepairActions[0] = apiv1.RepairActionTypeHardwareInspection
		detail.SuggestedActionsByGPUd.Description = fmt.Sprintf("GPU recovery remains requested after %d failed reboot cycles; hardware inspection is required. Driver guidance: %s", r.failedReboots[key], detail.Description)
	}
	r.active[key] = detail
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
		detail := r.active[key]
		health := recoveryHealthType(detail.RecoveryAction)
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
	if fault.Reason == "XIDComponent is healthy" || fault.Reason == "" {
		fault.Reason = recovery.Reason
	} else {
		fault.Reason += "; " + recovery.Reason
	}
	if healthPriority(recovery.Health) > healthPriority(fault.Health) {
		fault.Health = recovery.Health
	}
	// Hardware inspection must not be downgraded by equally fatal recovery
	// guidance. Conversely, recovery can supplement a non-paging app fault.
	if repairPriority(recovery.SuggestedActions) > repairPriority(fault.SuggestedActions) {
		fault.SuggestedActions = recovery.SuggestedActions
	}
	return fault
}
