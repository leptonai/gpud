// Package xid provides the NVIDIA XID error details.
package xid

import (
	"github.com/leptonai/gpud/components/common"
)

// Defines the Xid error information that is static.
type Detail struct {
	DocumentVersion string `json:"documentation_version"`

	Xid         int    `json:"xid"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// SuggestedActionsByGPUd is the suggested actions by GPUd.
	SuggestedActionsByGPUd *common.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`
	// CriticalErrorMarkedByGPUd is true if the GPUd marks this Xid as a critical error.
	// You may use this field to decide whether to alert or not.
	CriticalErrorMarkedByGPUd bool `json:"critical_error_marked_by_gpud"`

	// PotentialHWError is true if the Xid indicates a potential hardware error.
	// Source: https://docs.nvidia.com/deploy/xid-errors/index.html#xid-error-listing
	PotentialHWError bool `json:"potential_hw_error"`

	// PotentialDriverError is true if the Xid indicates a potential driver error.
	// Source: https://docs.nvidia.com/deploy/xid-errors/index.html#xid-error-listing
	PotentialDriverError bool `json:"potential_driver_error"`

	// PotentialUserAppError is true if the Xid indicates a potential user application error.
	// Source: https://docs.nvidia.com/deploy/xid-errors/index.html#xid-error-listing
	PotentialUserAppError bool `json:"potential_user_app_error"`

	// PotentialSystemMemoryCorruption is true if the Xid indicates a potential system memory corruption.
	// Source: https://docs.nvidia.com/deploy/xid-errors/index.html#xid-error-listing
	PotentialSystemMemoryCorruption bool `json:"potential_system_memory_corruption"`

	// PotentialBusError is true if the Xid indicates a potential bus error.
	// Source: https://docs.nvidia.com/deploy/xid-errors/index.html#xid-error-listing
	PotentialBusError bool `json:"potential_bus_error"`

	// PotentialThermalIssue is true if the Xid indicates a potential thermal issue.
	// Source: https://docs.nvidia.com/deploy/xid-errors/index.html#xid-error-listing
	PotentialThermalIssue bool `json:"potential_thermal_issue"`

	// PotentialFBCorruption is true if the Xid indicates a potential framebuffer corruption.
	// Source: https://docs.nvidia.com/deploy/xid-errors/index.html#xid-error-listing
	PotentialFBCorruption bool `json:"potential_fb_corruption"`
}

// IsMarkedAsCriticalByGPUd returns true if the GPUd marks this Xid as a critical error.
func (d Detail) IsMarkedAsCriticalByGPUd() bool {
	return d.CriticalErrorMarkedByGPUd
}

// Returns the error if found.
// Otherwise, returns false.
func GetDetail(id int) (*Detail, bool) {
	e, ok := details[id]
	return &e, ok
}

// Copied from https://docs.nvidia.com/deploy/xid-details/index.html#xid-error-listing.
// See https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages for more details.
var details = map[int]Detail{
	1: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             1,
		Name:            "Invalid or corrupted push buffer stream",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	2: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             2,
		Name:            "Invalid or corrupted push buffer stream",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	3: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             3,
		Name:            "Invalid or corrupted push buffer stream",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	4: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             4,
		Name:            "Invalid or corrupted push buffer stream",
		Description:     "or GPU semaphore timeout (then user app error is true)",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	5: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             5,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	6: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             6,
		Name:            "Invalid or corrupted push buffer stream",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	7: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             7,
		Name:            "Invalid or corrupted push buffer address",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	8: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             8,
		Name:            "GPU stopped processing",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               true,
		PotentialThermalIssue:           true,
		PotentialFBCorruption:           false,
	},
	9: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             9,
		Name:            "Driver error programming GPU",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	10: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             10,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	11: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             11,
		Name:            "Invalid or corrupted push buffer stream",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	12: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             12,
		Name:            "Driver error handling GPU exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	13: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             13,
		Name:            "Graphics Engine Exception",
		Description:     `Run DCGM and Field diagnostics to confirm if the issue is related to hardware. If not, debug the user application using guidance from https://docs.nvidia.com/deploy/xid-errors/index.html. If the latter, see Report a GPU Issue at https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.`,

		// "may indicate anomalies in GPU memory affecting code and data segments"
		// even though the official doc says it's a user app error
		// it's possible that this indicates a deeper issue in the GPU
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		//
		// "the screen blacks out and I am given NVRM Nvidia XID 45"
		// "GPU was dead"
		// ref. https://www.reddit.com/r/pop_os/comments/joq8zn/nvrm_nvidia_xid_45_error_intermittent/
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Software-related issue affecting code and data segments, possibly GPU memory issue (Xid 13) -- check user applications and GPUs.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:     true,
		PotentialDriverError: true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		PotentialUserAppError: true,

		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           true,
		PotentialFBCorruption:           true,
	},
	14: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             14,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	15: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             15,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	16: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             16,
		Name:            "Display engine hung",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	17: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             17,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	18: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             18,
		Name:            "Bus mastering disabled in PCI Config Space",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	19: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             19,
		Name:            "Display Engine error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	20: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             20,
		Name:            "Invalid or corrupted Mpeg push buffer",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	21: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             21,
		Name:            "Invalid or corrupted Motion Estimation push buffer",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	22: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             22,
		Name:            "Invalid or corrupted Video Processor push buffer",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	23: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             23,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	24: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             24,
		Name:            "GPU semaphore timeout",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           true,
		PotentialFBCorruption:           true,
	},
	25: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             25,
		Name:            "Invalid or illegal push buffer stream",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	26: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             26,
		Name:            "Framebuffer timeout",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	27: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             27,
		Name:            "Video processor exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	28: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             28,
		Name:            "Video processor exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	29: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             29,
		Name:            "Video processor exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	30: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             30,
		Name:            "GPU semaphore access error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	31: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             31,
		Name:            "GPU memory page fault",
		Description:     `Debug the user application unless the issue is new and there have been no changes to the application but there has been changes to GPU driver or other GPU system software. If the latter, see Report a GPU Issue via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.`,

		// "may indicate anomalies in GPU memory affecting code and data segments"
		// even though the official doc says it's a user app error
		// it's possible that this indicates a deeper issue in the GPU
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		//
		// "the screen blacks out and I am given NVRM Nvidia XID 45"
		// "GPU was dead"
		// ref. https://www.reddit.com/r/pop_os/comments/joq8zn/nvrm_nvidia_xid_45_error_intermittent/
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Software-related issue affecting code and data segments, possibly GPU memory issue (Xid 31) -- check user applications and GPUs.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:     true,
		PotentialDriverError: true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		PotentialUserAppError: true,

		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	32: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             32,
		Name:            "Invalid or corrupted push buffer stream",
		Description:     "The event is reported by the DMA controller of the PCIE bus that manages communication between the NVIDIA driver and GPU. In most cases, a PCI quality issue occurs.",

		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Please submit a technical support ticket to check the physical link.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRepairHardware,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           true,
		PotentialFBCorruption:           true,
	},
	33: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             33,
		Name:            "Internal micro-controller error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	34: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             34,
		Name:            "Video processor exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	35: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             35,
		Name:            "Video processor exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	36: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             36,
		Name:            "Video processor exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	37: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             37,
		Name:            "Driver firmware error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	38: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             38,
		Name:            "Driver firmware error",
		Description:     "",

		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Please submit a technical support ticket to check the driver firmware issues.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRepairHardware,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	39: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             39,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	40: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             40,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	41: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             41,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	42: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             42,
		Name:            "Video processor exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	43: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             43,
		Name:            "GPU stopped processing",
		Description:     "",

		// "may indicate anomalies in GPU memory affecting code and data segments"
		// even though the official doc says it's a user app error
		// it's possible that this indicates a deeper issue in the GPU
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		//
		// "the screen blacks out and I am given NVRM Nvidia XID 45"
		// "GPU was dead"
		// ref. https://www.reddit.com/r/pop_os/comments/joq8zn/nvrm_nvidia_xid_45_error_intermittent/
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Software-related issue affecting code and data segments, possibly GPU memory issue (Xid 43) -- check user applications and GPUs.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:     false,
		PotentialDriverError: true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		PotentialUserAppError: true,

		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	44: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             44,
		Name:            "Graphics Engine fault during context switch",
		Description:     "",

		// "Thease failures mean an uncorrectable error occurs on the GPU, which is also reported back to the user application."
		// "A GPU reset or node reboot is needed to clear this error."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Uncorrectable GPU error occurred (Xid 44) -- GPU reset or node reboot is needed.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	45: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             45,
		Name:            "Preemptive cleanup, due to previous errors – Most likely to see when running multiple cuda applications and hitting a DBE.",
		Description:     "Robust Channel Preemptive Removal. No action, informative only. Indicates channels affected by another failure. On A100, this error could be seen by itself due to unexpected Fabric Manager shutdown when FM is running in the same OS environment as the GPU. Otherwise, this error is safe to ignore as an informational message.",

		// "may indicate anomalies in GPU memory affecting code and data segments"
		// even though the official doc says it's a user app error
		// it's possible that this indicates a deeper issue in the GPU
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		//
		// "the screen blacks out and I am given NVRM Nvidia XID 45"
		// "GPU was dead"
		// ref. https://www.reddit.com/r/pop_os/comments/joq8zn/nvrm_nvidia_xid_45_error_intermittent/
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"System reboot is recommended as Xid 45 often blocks nvidia-smi, sometimes indicating a deeper GPU issue.",
				"Software-related issue affecting code and data segments, possibly GPU memory issue (Xid 45) -- check user applications and GPUs.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
				common.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:     false,
		PotentialDriverError: true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		PotentialUserAppError: true,

		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	46: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             46,
		Name:            "GPU stopped processing",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	47: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             47,
		Name:            "Video processor exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	48: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             48,
		Name:            "Double Bit ECC Error",
		Description: `This event is logged when the GPU detects that an uncorrectable error occurs on the GPU. This is also reported back to the user application. A GPU reset or node reboot is needed to clear this error.

If Xid 48 is followed by Xid 63 or 64: Drain/cordon the node, wait for all work to complete, and reset GPU(s) reporting the XID (refer to GPU reset capabilities/limitations section below).

If Xid 48 is not followed by Xid 63 or 64: see Running Field Diagnostics to collect additional debug information, via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#running-field-diag.

See below for guidelines on when to RMA GPUs based on excessive errors.

The error is also reported to your application. In most cases, you need to reset the GPU or node to fix this error.
`,

		// "A GPU reset or node reboot is needed to clear this error."
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-48-dbe-double-bit-error-ecc-error
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Uncorrectable GPU error occurred -- GPU reset required due to Xid 48 (double bit ECC error).",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	49: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             49,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	50: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             50,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	51: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             51,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	52: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             52,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	53: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             53,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	54: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             54,
		Name:            "Auxiliary power is not connected to the GPU board",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	55: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             55,
		Name:            "Unused",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	56: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             56,
		Name:            "Display Engine error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	57: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             57,
		Name:            "Error programming video memory interface",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	58: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             58,
		Name:            "Unstable video memory interface detected",
		Description:     "or EDC error - clarified in printout (driver error=false)",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	59: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             59,
		Name:            "Internal micro-controller error (older drivers)",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	60: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             60,
		Name:            "Video processor exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	61: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             61,
		Name:            "Internal micro-controller breakpoint/warning (newer drivers)",
		Description: `PMU Breakpoint. Report a GPU Issue and Reset GPU(s) reporting the XID (refer GPU reset capabilities/limitations section below).

Internal micro-controller breakpoint/warning. The GPU internal engine stops working. Consequently, your businesses are affected.
`,

		// "Thease failures mean an uncorrectable error occurs on the GPU, which is also reported back to the user application."
		// "A GPU reset or node reboot is needed to clear this error."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Uncorrectable GPU error occurred (Xid 61) -- GPU reset or node reboot is needed.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	62: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             62,
		Name:            "Internal micro-controller halt (newer drivers)",
		Description:     "This event is similar to Xid 61. PMU Halt Error. Report a GPU Issue and Reset GPU(s) reporting the XID (refer GPU reset capabilities/limitations section below).",

		// "Thease failures mean an uncorrectable error occurs on the GPU, which is also reported back to the user application."
		// "A GPU reset or node reboot is needed to clear this error."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Uncorrectable GPU error occurred (Xid 62) -- GPU reset or node reboot is needed.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           true,
		PotentialFBCorruption:           false,
	},
	63: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             63,
		Name:            "ECC page retirement or row remapping recording event",
		Description: `
These events are logged when the GPU handles ECC memory errors on the GPU.

A100: Row-remapping recording event.

This XID indicates successful recording of a row-remapping entry to the InfoROM.

If associated with XID 94, the application that encountered the error needs to be restarted. All other applications on the system can keep running as is until there is a convenient time to reset the GPU (refer GPU reset capabilities/limitations section below) or reboot for row remapping to activate.

Legacy GPU: ECC page retirement recording event.

If associated with XID 48, drain/cordon the node, wait for all work to complete, and reset GPU(s) reporting the XID (refer GPU reset capabilities/limitations section below).

If not, it is from a single bit error and the system can keep running as is until there is a convenient time to reboot it.

Xid 63 indicates that the retirement or remapping information is successfully recorded in infoROM.
`,

		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Row-remapping happened (Xid 63, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but for optimal performance, reset the GPU or reboot the system.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	64: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             64,
		Name:            "ECC page retirement or row remapper recording failure",
		Description: `
These events are logged when the GPU handles ECC memory errors on the GPU.

A100: Row-remapping recording failure.

This XID indicates a failure in recording a row-remapping entry to the InfoROM.

The node should be rebooted immediately since there is a recording failure. If the errors continue, drain, triage, and see Report a GPU Issue, via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.

Legacy GPU: ECC page retirement recording failure.

See above, however the node should be monitored closely. If there is no associated XID 48 error, then these are related to single bit-errors. The GPU(s) reporting the error must be reset (refer to GPU reset capabilities/limitations section below) immediately since there is a recording failure. If the errors continue, drain, triage, and see Report a GPU Issue.

See below for guidelines on when to RMA GPUs based on excessive errors, via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.

ECC page retirement or row remapper recording failure. This event is similar to XID 63. However, Xid 63 indicates that the retirement or remapping information is successfully recorded in infoROM.

Xid 64 indicates that the retirement or remapping information fails to be recorded.
`,

		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Row-remapping happened (Xid 64, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but to achieve optimal performance, reset the GPU or reboot the system when convenient.",
				"System reboot is recommended when convenient, but not required immediately.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
				common.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	65: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             65,
		Name:            "Video processor exception",
		Description:     "",

		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
				common.RepairActionTypeRebootSystem,
			},

			Descriptions: []string{
				"Row-remapping happened (Xid 65, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but to achieve optimal performance, reset the GPU or reboot the system when convenient.",
				"System reboot is recommended when convenient, but not required immediately.",
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	66: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             66,
		Name:            "Illegal access by driver",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	67: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             67,
		Name:            "Illegal access by driver",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	68: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             68,
		Name:            "NVDEC0 Exception",
		Description:     "Video processor exception",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:     true,
		PotentialDriverError: true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		// TODO: verify whether this is still true https://github.com/NVIDIA/k8s-device-plugin/issues/945
		PotentialUserAppError: true,

		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	69: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             69,
		Name:            "Graphics Engine class error",
		Description:     "",

		// "Thease failures mean an uncorrectable error occurs on the GPU, which is also reported back to the user application."
		// "A GPU reset or node reboot is needed to clear this error."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Uncorrectable GPU error occurred (Xid 69) -- GPU reset or node reboot is needed.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	70: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             70,
		Name:            "CE3: Unknown Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	71: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             71,
		Name:            "CE4: Unknown Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	72: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             72,
		Name:            "CE5: Unknown Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	73: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             73,
		Name:            "NVENC2 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	74: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             74,
		Name:            "NVLINK Error",
		Description: `
This event is logged when the GPU detects that a problem with a connection from the GPU to another GPU or NVSwitch over NVLink.

A GPU reset or node reboot is needed to clear this error.

This event may indicate a hardware failure with the link itself, or may indicate a problem with the device at the remote end of the link. For example, if a GPU fails, another GPU connected to it over NVLink may report an Xid 74 simply because the link went down as a result.

The nvidia-smi nvlink command can provide additional details on NVLink errors, and connection information on the links.

If this error is seen repeatedly and GPU reset or node reboot fails to clear the condition, contact your hardware vendor for support.

Extract the hex strings from the XID error message. eg: (0x12345678, 0x12345678, 0x12345678, 0x12345678, 0x12345678, 0x12345678, 0x12345678) Look at the bolded DWORD (the first) and take the following paths if the particular bits (counting from LSB side) are set.

Bits 4 or 5: Likely HW issue with ECC/Parity --> If seen more than 2 times on the same link, report a bug.

Bits 21 or 22: Marginal channel SI issue. Check link mechanical connecetions. If other errors accompany, follow the resolution for those.

Bits 8, 9, 12, 16, 17, 24, 28: Could possibly be a HW issue: Check link mechanical connecetions and re-seat if a field resolution is required. Run diags if issue persists.



"Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158 explains:

Xid 74 indicates errors in NVLink.

For PCIe A100, it's mainly occurred on the NVLink Bridge between two GPUs.
Its occurrence rate is several orders of magnitude higher than other hardware faults.
Apart from stress testing to exclude those that are constantly repeating errors, there isn't a good way to avoid the occurrence of Xid74 issues.

The XID indicates an NVLink hardware error. The GPU encounters a critical hardware error and must be repaired.
`,

		// "A GPU reset or node reboot is needed to clear this error."
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-74-nvlink-error
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"GPU reset or node reboot is needed to clear this error (Xid 74).",
				"If this error is seen repeatedly (Xid 74), contact hardware vendor to check the physical link.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
				common.RepairActionTypeRepairHardware,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	75: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             75,
		Name:            "CE6: Unknown Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	76: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             76,
		Name:            "CE7: Unknown Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	77: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             77,
		Name:            "CE8: Unknown Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	78: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             78,
		Name:            "vGPU Start Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	79: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             79,
		Name:            "GPU has fallen off the bus",
		Description: `
This event is logged when the GPU driver attempts to access the GPU over its PCI Express connection and finds that the GPU is not accessible.

This event is often caused by hardware failures on the PCI Express link causing the GPU to be inaccessible due to the link being brought down.

Reviewing system event logs and kernel PCI event logs may provide additional indications of the source of the link failures.

This event may also be cause by failing GPU hardware or other driver issues.
`,

		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-79-gpu-has-fallen-off-the-bus
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"GPU not accessible due to failing hardware (Xid 79, 'GPU has fallen off the bus') -- check with the data center.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRepairHardware,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           true,
		PotentialFBCorruption:           false,
	},
	80: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             80,
		Name:            "Corrupted data sent to GPU",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	81: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             81,
		Name:            "VGA Subsystem Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	82: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             82,
		Name:            "NVJPG0 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	83: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             83,
		Name:            "NVDEC1 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	84: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             84,
		Name:            "NVDEC2 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	85: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             85,
		Name:            "CE9: Unknown Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	86: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             86,
		Name:            "OFA Exception",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	87: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             87,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	88: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             88,
		Name:            "NVDEC3 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	89: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             89,
		Name:            "NVDEC4 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	90: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             90,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	91: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             91,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	92: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             92,
		Name:            "High single-bit ECC error rate",
		Description: `A hardware or driver error occurs.

See Running Field Diagnostics to collect additional debug information, via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#running-field-diag.

See below for guidelines on when to RMA GPUs based on excessive errors.
`,

		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRepairHardware,
			},

			Descriptions: []string{
				"Please submit a technical support ticket to check hardware or driver errors.",
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	93: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             93,
		Name:            "Non-fatal violation of provisioned InfoROM wear limit",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            true,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	94: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             94,
		Name:            "Contained ECC error",
		Description: `
This XID indicates a contained ECC error has occurred.

These events are logged when GPU drivers handle errors in GPUs that support error containment, starting with NVIDIA® A100 GPUs.

For Xid 94, these errors are contained to one application, and the application that encountered this error must be restarted.

All other applications running at the time of the Xid are unaffected.

It is recommended to reset the GPU when convenient. Applications can continue to be run until the reset can be performed.

(A100 only)

The application that encountered the error needs to be restarted. All other applications on the system can keep running as is until there is a convenient time to reset the GPU (refer to GPU reset capabilities/limitations section below) or reboot for row remapping to activate.

See below for guidelines on when to RMA GPUs based on row remapping failures.

When the application encounters an uncorrectable GPU memory ECC error, the ECC mechanism of NVIDIA attempts to suppress the error in the faulty application in case the error affects other applications on the GPU-accelerated node.

This event is generated if the error suppression mechanism successfully suppresses the error. In this case, only the faulty application is affected by the uncorrectable ECC error.
`,

		// "recommended to reset the GPU when convenient"
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-94-95-contained-uncontained
		//
		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Row-remapping happened (Xid 94, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but to achieve optimal performance, reset the GPU or reboot the system when convenient.",
				"System reboot is recommended when convenient, but not required immediately.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
				common.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	95: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             95,
		Name:            "Uncontained ECC error",
		Description: `
This XID indicates an uncontained ECC error has occurred.

These events are logged when GPU drivers handle errors in GPUs that support error containment, starting with NVIDIA® A100 GPUs.

For Xid 95, these errors affect multiple applications, and the affected GPU must be reset before applications can restart. Refer https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html for GPU Reset capabilities & limitations

(A100 only)

If MIG is enabled, drain any work on the other GPU instances, wait for all work to complete, and reset GPU(s) reporting the XID (refer to the GPU reset capabilities/limitations section below).

If MIG is disabled, the node should be rebooted immediately since there is an uncorrectable uncontained ECC error. If the errors continue, drain, triage, and see Report a GPU Issue, via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.

See below for guidelines on when to RMA GPUs based on row remapping failures.

References:
https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#user-visible-statistics

This event is similar to Xid 94. However, Xid 94 indicates that the error is suppressed. Xid 95 indicates that the error fails to be suppressed. Other applications on the GPU-accelerated node are also affected.
`,

		// "the affected GPU must be reset before applications can restart."
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-94-95-contained-uncontained
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"GPU reset or system reboot is needed to clear this uncontained ECC error (Xid 95). If MIG is disabled, the node should be rebooted immediately since there is an uncorrectable uncontained ECC error.",
				"If the errors continue (Xid 95), drain the node and contact the hardware vendor for assistance.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
				common.RepairActionTypeRepairHardware,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	96: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             96,
		Name:            "NVDEC5 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	97: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             97,
		Name:            "NVDEC6 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	98: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             98,
		Name:            "NVDEC7 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	99: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             99,
		Name:            "NVJPG1 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	100: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             100,
		Name:            "NVJPG2 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	101: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             101,
		Name:            "NVJPG3 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	102: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             102,
		Name:            "NVJPG4 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	103: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             103,
		Name:            "NVJPG5 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	104: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             104,
		Name:            "NVJPG6 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	105: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             105,
		Name:            "NVJPG7 Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	106: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             106,
		Name:            "SMBPBI Test Message",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	107: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             107,
		Name:            "SMBPBI Test Message Silent",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	108: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             108,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	109: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             109,
		Name:            "Context Switch Timeout Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           true,
		PotentialFBCorruption:           true,
	},
	110: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             110,
		Name:            "Security Fault Error",
		Description:     `This event should be uncommon unless there is a hardware failure. To recover, revert any recent system hardware modifications and cold reset the system. If this fails to correct the issue, contact your hardware vendor for assistance.`,

		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-110-security-fault-error
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"This event should be uncommon unless there is a hardware failure (Xid 110).",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	111: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             111,
		Name:            "Display Bundle Error Event",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	112: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             112,
		Name:            "Display Supervisor Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	113: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             113,
		Name:            "DP Link Training Erro",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	114: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             114,
		Name:            "Display Pipeline Underflow Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	115: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             115,
		Name:            "Display Core Channel Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	116: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             116,
		Name:            "Display Window Channel Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	117: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             117,
		Name:            "Display Cursor Channel Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	118: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             118,
		Name:            "Display Pixel Pipeline Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	119: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             119,
		Name:            "GSP RPC Timeout",
		Description:     "",

		// "Xid119 means GPU GSP module failed."
		// "These failures need to do fieldiag test, and most need to RMA."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"GPU GSP module failed (Xid 119) -- check if GPU qualifies for RMA.",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRepairHardware,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           true,
		PotentialFBCorruption:           true,
	},
	120: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             120,
		Name:            "GSP Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: true,
		PotentialBusError:               true,
		PotentialThermalIssue:           true,
		PotentialFBCorruption:           true,
	},
	121: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             121,
		Name:            "C2C Link Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               true,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	122: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             122,
		Name:            "SPI PMU RPC Read Failure",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	123: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             123,
		Name:            "SPI PMU RPC Write Failure",
		Description: `
Report a GPU issue and reset GPU(s) reporting the XID (refer to GPU reset capabilities/limitations section provided in Section D.9 of the FM User Guide: https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf).
`,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	124: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             124,
		Name:            "SPI PMU RPC Erase Failure",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	125: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             125,
		Name:            "Inforom FS Failure",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	126: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             126,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	127: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             127,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	128: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             128,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	129: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             129,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	130: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             130,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	131: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             131,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	132: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             132,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	134: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             134,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	135: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             135,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	136: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             136,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	137: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             137,
		Name:            "NVLink FLA privilege error",
		Description:     `This event is logged when a fault is reported by the remote MMU, such as when an illegal NVLink peer-to-peer access is made by an applicable unit on the chip. Typically these are application-level bugs, but can also be driver bugs or hardware bugs.`,

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           true,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	138: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             138,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	139: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             139,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	140: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             140,
		Name:            "Unrecovered ECC Error",
		Description:     `This event may occur when the GPU driver has observed uncorrectable errors in GPU memory, in such a way as to interrupt the GPU driver’s ability to mark the pages for dynamic page offlining or row remapping. Reset the GPU, and if the problem persists, contact your hardware vendor for support.`,

		// "Reset the GPU, and if the problem persists, contact your hardware vendor for support"
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-140-ecc-unrecovered-error
		SuggestedActionsByGPUd: &common.SuggestedActions{
			References: []string{
				//
			},

			Descriptions: []string{
				"Reset the GPU in case the row remapping is pending (Xid 140).",
				"Requires hardware vendor support if the problem persists after reboot (Xid 140).",
			},

			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
				common.RepairActionTypeRepairHardware,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
	141: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             141,
		Name:            "Reserved",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	142: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             142,
		Name:            "Unrecovered ECC Error",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                false,
		PotentialDriverError:            false,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           false,
	},
	143: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		Xid:             143,
		Name:            "GPU Initialization Failure",
		Description:     "",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// below are defined in https://docs.nvidia.com/deploy/xid-errors/index.html
		// only indicates potential causes thus we do not solely rely on them
		PotentialHWError:                true,
		PotentialDriverError:            true,
		PotentialUserAppError:           false,
		PotentialSystemMemoryCorruption: false,
		PotentialBusError:               false,
		PotentialThermalIssue:           false,
		PotentialFBCorruption:           true,
	},
}
