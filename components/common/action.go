package common

type RepairActionType string

const (
	// RepairActionTypeRebootSystem represents a suggested action to reboot the system.
	// Specific to NVIDIA GPUs, this implies GPU reset by rebooting the system.
	RepairActionTypeRebootSystem RepairActionType = "REBOOT_SYSTEM"

	// RepairActionTypeRepairHardware represents a suggested action to repair the hardware, externally.
	// This often involves data center (or cloud provider) support to physically check/repair the machine.
	RepairActionTypeRepairHardware RepairActionType = "REPAIR_HARDWARE"

	// RepairActionTypeCheckUserApp represents a suggested action to check the user application.
	// For instance, NVIDIA may report XID 45 as user app error, but the underlying GPU might have other issues
	// thus requires further diagnosis of the application and the GPU.
	RepairActionTypeCheckUserAppAndGPU RepairActionType = "CHECK_USER_APP_AND_GPU"
)

// SuggestedActions represents a set of suggested actions to mitigate an issue.
type SuggestedActions struct {
	// A list of repair actions to mitigate the issue.
	RepairActions []RepairActionType `json:"repair_actions"`

	// A list of descriptions for the suggested actions.
	Descriptions []string `json:"descriptions"`
}

func (s *SuggestedActions) RequiresReboot() bool {
	if s == nil {
		return false
	}
	if len(s.RepairActions) == 0 {
		return false
	}
	for _, action := range s.RepairActions {
		if action == RepairActionTypeRebootSystem {
			return true
		}
	}
	return false
}

func (s *SuggestedActions) RequiresRepair() bool {
	if s == nil {
		return false
	}
	if len(s.RepairActions) == 0 {
		return false
	}
	for _, action := range s.RepairActions {
		if action == RepairActionTypeRepairHardware {
			return true
		}
	}
	return false
}

func (s *SuggestedActions) RequiresCheckUserAppAndGPU() bool {
	if s == nil {
		return false
	}
	if len(s.RepairActions) == 0 {
		return false
	}
	for _, action := range s.RepairActions {
		if action == RepairActionTypeCheckUserAppAndGPU {
			return true
		}
	}
	return false
}

func (s *SuggestedActions) Add(other *SuggestedActions) {
	if other == nil {
		return
	}

	existingActions := make(map[RepairActionType]struct{})
	for _, action := range s.RepairActions {
		existingActions[action] = struct{}{}
	}
	for _, action := range other.RepairActions {
		if _, ok := existingActions[action]; !ok {
			s.RepairActions = append(s.RepairActions, action)
		}
	}

	existingDescriptions := make(map[string]struct{})
	for _, description := range s.Descriptions {
		existingDescriptions[description] = struct{}{}
	}
	for _, description := range other.Descriptions {
		if _, ok := existingDescriptions[description]; !ok {
			s.Descriptions = append(s.Descriptions, description)
		}
	}
}
