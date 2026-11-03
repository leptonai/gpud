package xid

import (
	"fmt"
	"regexp"
	"strconv"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// RecoveryActionTransition preserves the driver-reported XID 154 transition.
// Valid distinguishes a decoded None (zero) from missing or malformed guidance.
// Names are evidence only; policy is selected from the numeric To value.
type RecoveryActionTransition struct {
	From     uint32 `json:"from"`
	FromName string `json:"from_name,omitempty"`
	To       uint32 `json:"to"`
	ToName   string `json:"to_name,omitempty"`
	Valid    bool   `json:"valid"`
	Raw      string `json:"raw,omitempty"`
}

var recoveryTransitionRE = regexp.MustCompile(`GPU recovery action changed from (0x[0-9a-fA-F]+) \(([^()]*)\) to (0x[0-9a-fA-F]+) \(([^()]*)\)`)

func parseRecoveryAction(line string) *RecoveryActionTransition {
	transition := &RecoveryActionTransition{Raw: line}
	match := recoveryTransitionRE.FindStringSubmatch(line)
	if match == nil {
		return transition
	}
	from, fromErr := strconv.ParseUint(match[1][2:], 16, 32)
	to, toErr := strconv.ParseUint(match[3][2:], 16, 32)
	transition.FromName = match[2]
	transition.ToName = match[4]
	if fromErr == nil && toErr == nil {
		transition.From = uint32(from)
		transition.To = uint32(to)
		transition.Valid = true
	}
	return transition
}

// Numeric values are the driver's recovery-action ABI. Guidance and preconditions:
// https://docs.nvidia.com/deploy/nvidia-smi/index.html#gpu-recovery-action
// XID 154 supplements originating faults; it is not a replacement diagnosis.
func recoveryDetail(transition *RecoveryActionTransition) *Detail {
	base, _ := GetDetail(154)
	detail := *base
	// Older persisted events have no transition evidence. Preserve their
	// informational catalog entry instead of inventing malformed guidance.
	if transition == nil {
		return &detail
	}
	detail.EventType = apiv1.EventTypeCritical
	description := "Missing or malformed GPU recovery action; investigate the originating XIDs and driver recovery guidance before choosing a repair."
	action := apiv1.RepairActionTypeCheckUserAppAndGPU
	if transition.Valid {
		switch transition.To {
		case 0:
			detail.EventType = apiv1.EventTypeWarning
			detail.SuggestedActionsByGPUd = nil
			detail.Description = "GPU recovery action None; no new repair requested. Preserve and investigate independently actionable originating XIDs."
			return &detail
		case 1:
			detail.EventType = apiv1.EventTypeFatal
			action = apiv1.RepairActionTypeRebootSystem
			description = "GPU reset required (R610 PF FLR). Terminate GPU processes before reset; GPUd recommends a system reboot under its existing GPU-reset convention."
		case 2:
			detail.EventType = apiv1.EventTypeFatal
			action = apiv1.RepairActionTypeRebootSystem
			description = "Node reboot required: the OS is inconsistent; do not restart the application before rebooting the node. A warm reboot is sufficient."
		case 3:
			description = "Drain P2P: disable new GPU scheduling, quiesce P2P traffic and UVM persistence, then re-query GPU Recovery Action. Reset only if Drain P2P remains; do not reboot blindly."
		case 4:
			detail.EventType = apiv1.EventTypeWarning
			detail.SuggestedActionsByGPUd = nil
			detail.Description = "Drain and Reset: prevent new GPU work and allow unaffected work to finish or checkpoint before reset. Retain this as row-remap evidence; the remapped-rows component owns pending-remap reboot and remapping-failure decisions."
			return &detail
		case 5:
			description = "Recover IMEX Domain: operator intervention is required to recover the multi-node IMEX domain. Investigate the originating XIDs and coordinate domain recovery; do not infer a local reboot or hardware failure."
		case 6, 7:
			detail.EventType = apiv1.EventTypeFatal
			action = apiv1.RepairActionTypeRebootSystem
			description = "Bus Reset or System Reboot required (R610): reboot the node before restarting GPU applications."
		default:
			description = fmt.Sprintf("Unknown GPU recovery action 0x%x; investigate the originating XIDs and driver recovery guidance before choosing a repair.", transition.To)
		}
	}
	detail.Description = description
	detail.SuggestedActionsByGPUd = &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{action}, Description: description}
	return &detail
}

func knownRecoveryAction(transition *RecoveryActionTransition) bool {
	return transition != nil && transition.Valid && transition.To <= 7
}

func sameRecoveryAction(a, b *RecoveryActionTransition) bool {
	if a == nil || b == nil || !a.Valid || !b.Valid {
		return a != nil && b != nil && *a == *b
	}
	return a.From == b.From && a.To == b.To && a.FromName == b.FromName && a.ToName == b.ToName
}
