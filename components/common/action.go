package common

// RequiredAction represents a set of required actions to mitigate an issue.
// Each field is independent, and it's up to the caller to determine the best course of action.
// e.g., if both ResetGPU and Reboot are true, the caller may just reboot the system skipping the reset.
type RequiredActions struct {
	// Set to true if the error requires a GPU reset.
	ResetGPU bool `json:"reset_gpu"`
	// Set to true if the error requires a system reboot.
	RebootSystem bool `json:"reboot_system"`

	// A list of descriptions for the required actions.
	Descriptions []string `json:"descriptions"`
}
