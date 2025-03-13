package common

import "strings"

type RepairActionType string

const (
	// RepairActionTypeIgnoreNoActionRequired represents a suggested action to ignore the issue,
	// meaning no action is needed until further notice.
	RepairActionTypeIgnoreNoActionRequired RepairActionType = "IGNORE_NO_ACTION_REQUIRED"

	// RepairActionTypeRebootSystem represents a suggested action to reboot the system.
	// Specific to NVIDIA GPUs, this implies GPU reset by rebooting the system.
	RepairActionTypeRebootSystem RepairActionType = "REBOOT_SYSTEM"

	// RepairActionTypeHardwareInspection represents a suggested action for hardware inspection
	// and repair if any issue is found. This often involves data center (or cloud provider) support
	// to physically check/repair the machine.
	RepairActionTypeHardwareInspection RepairActionType = "HARDWARE_INSPECTION"

	// RepairActionTypeCheckUserApp represents a suggested action to check the user application.
	// For instance, NVIDIA may report XID 45 as user app error, but the underlying GPU might have other issues
	// thus requires further diagnosis of the application and the GPU.
	RepairActionTypeCheckUserAppAndGPU RepairActionType = "CHECK_USER_APP_AND_GPU"
)

// SuggestedActions represents a set of suggested actions to mitigate an issue.
type SuggestedActions struct {
	// References to the descriptions.
	References []string `json:"references,omitempty"`

	// A list of reasons and descriptions for the suggested actions.
	Descriptions []string `json:"descriptions,omitempty"`

	// A list of repair actions to mitigate the issue.
	RepairActions []RepairActionType `json:"repair_actions"`
}

func (sa *SuggestedActions) DescribeActions() string {
	acts := make([]string, 0)
	for _, act := range sa.RepairActions {
		acts = append(acts, string(act))
	}
	return strings.Join(acts, ", ")
}

// Simplify simplifies the suggested actions by removing duplicate actions and
// keeping only the most severe actions.
// And it also removes the descriptions and references.
// HW inspection takes priority over reboot.
func (sa *SuggestedActions) Simplify() *SuggestedActions {
	simplified := &SuggestedActions{
		References:   nil,
		Descriptions: nil,
	}

	actionsToKeep := make(map[RepairActionType]bool)
	for _, act := range sa.RepairActions {
		actionsToKeep[act] = true
	}
	if _, ok := actionsToKeep[RepairActionTypeHardwareInspection]; ok {
		delete(actionsToKeep, RepairActionTypeRebootSystem)
	}

	simplified.RepairActions = make([]RepairActionType, 0)
	for _, act := range sa.RepairActions {
		if _, ok := actionsToKeep[act]; !ok {
			continue
		}
		simplified.RepairActions = append(simplified.RepairActions, act)
		delete(actionsToKeep, act)
	}

	return simplified
}
