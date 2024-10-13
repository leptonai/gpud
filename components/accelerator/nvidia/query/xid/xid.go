// Package xid provides the NVIDIA XID error details.
package xid

import "github.com/leptonai/gpud/components/common"

// Defines the XID error type.
//
// ref. https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf
// ref. https://github.com/NVIDIA/open-gpu-kernel-modules/blob/main/src/common/sdk/nvidia/inc/nverror.h
//
// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-error-listing
// ref. https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages
// ref. https://docs.nvidia.com/deploy/xid-errors/index.html
// ref. https://github.com/NVIDIA/open-gpu-kernel-modules/blob/main/src/common/sdk/nvidia/inc/nverror.h
// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
type Detail struct {
	DocumentVersion string `json:"documentation_version"`

	XID                    int    `json:"xid"`
	Name                   string `json:"name"`
	Description            string `json:"description"`
	HWError                bool   `json:"hw_error"`
	DriverError            bool   `json:"driver_error"`
	UserAppError           bool   `json:"user_app_error"`
	SystemMemoryCorruption bool   `json:"system_memory_corruption"`
	BusError               bool   `json:"bus_error"`
	ThermalIssue           bool   `json:"thermal_issue"`
	FBCorruption           bool   `json:"fb_corruption"`

	SuggestedActions *common.SuggestedActions `json:"suggested_actions,omitempty"`
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
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    1,
		Name:                   "Invalid or corrupted push buffer stream",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	2: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    2,
		Name:                   "Invalid or corrupted push buffer stream",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	3: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    3,
		Name:                   "Invalid or corrupted push buffer stream",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	4: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    4,
		Name:                   "Invalid or corrupted push buffer stream",
		Description:            "or GPU semaphore timeout (then user app error is true)",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	5: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    5,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	6: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    6,
		Name:                   "Invalid or corrupted push buffer stream",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	7: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    7,
		Name:                   "Invalid or corrupted push buffer address",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	8: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    8,
		Name:                   "GPU stopped processing",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           true,
		SystemMemoryCorruption: false,
		BusError:               true,
		ThermalIssue:           true,
		FBCorruption:           false,
	},
	9: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    9,
		Name:                   "Driver error programming GPU",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	10: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    10,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	11: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    11,
		Name:                   "Invalid or corrupted push buffer stream",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	12: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    12,
		Name:                   "Driver error handling GPU exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	13: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             13,
		Name:            "Graphics Engine Exception",
		Description:     `Run DCGM and Field diagnostics to confirm if the issue is related to hardware. If not, debug the user application using guidance from https://docs.nvidia.com/deploy/xid-errors/index.html. If the latter, see Report a GPU Issue at https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.`,
		HWError:         true,
		DriverError:     true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		UserAppError: true,

		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           true,
		FBCorruption:           true,

		// "may indicate anomalies in GPU memory affecting code and data segments"
		// even though the official doc says it's a user app error
		// it's possible that this indicates a deeper issue in the GPU
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		//
		// "the screen blacks out and I am given NVRM Nvidia XID 45"
		// "GPU was dead"
		// ref. https://www.reddit.com/r/pop_os/comments/joq8zn/nvrm_nvidia_xid_45_error_intermittent/
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
			},
			Descriptions: []string{
				"Software-related issue affecting code and data segments, possibly GPU memory issue (Xid 13) -- check user applications and GPUs.",
			},
		},
	},
	14: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    14,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	15: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    15,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	16: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    16,
		Name:                   "Display engine hung",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	17: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    17,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	18: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    18,
		Name:                   "Bus mastering disabled in PCI Config Space",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	19: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    19,
		Name:                   "Display Engine error",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	20: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    20,
		Name:                   "Invalid or corrupted Mpeg push buffer",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	21: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    21,
		Name:                   "Invalid or corrupted Motion Estimation push buffer",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	22: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    22,
		Name:                   "Invalid or corrupted Video Processor push buffer",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	23: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    23,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	24: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    24,
		Name:                   "GPU semaphore timeout",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           true,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           true,
		FBCorruption:           true,
	},
	25: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    25,
		Name:                   "Invalid or illegal push buffer stream",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           true,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	26: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    26,
		Name:                   "Framebuffer timeout",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	27: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    27,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	28: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    28,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	29: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    29,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	30: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    30,
		Name:                   "GPU semaphore access error",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	31: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             31,
		Name:            "GPU memory page fault",
		Description:     `Debug the user application unless the issue is new and there have been no changes to the application but there has been changes to GPU driver or other GPU system software. If the latter, see Report a GPU Issue via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.`,
		HWError:         true,
		DriverError:     true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		UserAppError: true,

		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "may indicate anomalies in GPU memory affecting code and data segments"
		// even though the official doc says it's a user app error
		// it's possible that this indicates a deeper issue in the GPU
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		//
		// "the screen blacks out and I am given NVRM Nvidia XID 45"
		// "GPU was dead"
		// ref. https://www.reddit.com/r/pop_os/comments/joq8zn/nvrm_nvidia_xid_45_error_intermittent/
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
			},
			Descriptions: []string{
				"Software-related issue affecting code and data segments, possibly GPU memory issue (Xid 31) -- check user applications and GPUs.",
			},
		},
	},
	32: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    32,
		Name:                   "Invalid or corrupted push buffer stream",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           true,
		FBCorruption:           true,
	},
	33: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    33,
		Name:                   "Internal micro-controller error",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	34: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    34,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	35: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    35,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	36: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    36,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	37: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    37,
		Name:                   "Driver firmware error",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	38: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    38,
		Name:                   "Driver firmware error",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	39: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    39,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	40: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    40,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	41: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    41,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	42: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    42,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	43: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             43,
		Name:            "GPU stopped processing",
		Description:     "",
		HWError:         false,
		DriverError:     true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		UserAppError: true,

		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "may indicate anomalies in GPU memory affecting code and data segments"
		// even though the official doc says it's a user app error
		// it's possible that this indicates a deeper issue in the GPU
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		//
		// "the screen blacks out and I am given NVRM Nvidia XID 45"
		// "GPU was dead"
		// ref. https://www.reddit.com/r/pop_os/comments/joq8zn/nvrm_nvidia_xid_45_error_intermittent/
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
			},
			Descriptions: []string{
				"Software-related issue affecting code and data segments, possibly GPU memory issue (Xid 43) -- check user applications and GPUs.",
			},
		},
	},
	44: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    44,
		Name:                   "Graphics Engine fault during context switch",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "Thease failures mean an uncorrectable error occurs on the GPU, which is also reported back to the user application."
		// "A GPU reset or node reboot is needed to clear this error."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
			Descriptions: []string{
				"Uncorrectable GPU error occurred (Xid 44) -- GPU reset or node reboot is needed.",
			},
		},
	},
	45: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             45,
		Name:            "Preemptive cleanup, due to previous errors – Most likely to see when running multiple cuda applications and hitting a DBE.",
		Description:     "Robust Channel Preemptive Removal. No action, informative only. Indicates channels affected by another failure. On A100, this error could be seen by itself due to unexpected Fabric Manager shutdown when FM is running in the same OS environment as the GPU. Otherwise, this error is safe to ignore as an informational message.",
		HWError:         false,
		DriverError:     true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		UserAppError: true,

		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "may indicate anomalies in GPU memory affecting code and data segments"
		// even though the official doc says it's a user app error
		// it's possible that this indicates a deeper issue in the GPU
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		//
		// "the screen blacks out and I am given NVRM Nvidia XID 45"
		// "GPU was dead"
		// ref. https://www.reddit.com/r/pop_os/comments/joq8zn/nvrm_nvidia_xid_45_error_intermittent/
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
				common.RepairActionTypeCheckUserAppAndGPU,
			},
			Descriptions: []string{
				"System reboot is recommended as Xid 45 often blocks nvidia-smi, sometimes indicating a deeper GPU issue.",
				"Software-related issue affecting code and data segments, possibly GPU memory issue (Xid 45) -- check user applications and GPUs.",
			},
		},
	},
	46: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    46,
		Name:                   "GPU stopped processing",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	47: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    47,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	48: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             48,
		Name:            "Double Bit ECC Error",
		Description: `This event is logged when the GPU detects that an uncorrectable error occurs on the GPU. This is also reported back to the user application. A GPU reset or node reboot is needed to clear this error.

If Xid 48 is followed by Xid 63 or 64: Drain/cordon the node, wait for all work to complete, and reset GPU(s) reporting the XID (refer to GPU reset capabilities/limitations section below).

If Xid 48 is not followed by Xid 63 or 64: see Running Field Diagnostics to collect additional debug information, via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#running-field-diag.

See below for guidelines on when to RMA GPUs based on excessive errors.
`,
		HWError:                true,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "A GPU reset or node reboot is needed to clear this error."
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-48-dbe-double-bit-error-ecc-error
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
			Descriptions: []string{
				"Uncorrectable GPU error occurred -- GPU reset required due to Xid 48 (double bit ECC error).",
			},
		},
	},
	49: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    49,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	50: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    50,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	51: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    51,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	52: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    52,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	53: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    53,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	54: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    54,
		Name:                   "Auxiliary power is not connected to the GPU board",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	55: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    55,
		Name:                   "Unused",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	56: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    56,
		Name:                   "Display Engine error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	57: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    57,
		Name:                   "Error programming video memory interface",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	58: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    58,
		Name:                   "Unstable video memory interface detected",
		Description:            "or EDC error - clarified in printout (driver error=false)",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	59: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    59,
		Name:                   "Internal micro-controller error (older drivers)",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	60: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    60,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	61: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    61,
		Name:                   "Internal micro-controller breakpoint/warning (newer drivers)",
		Description:            "PMU Breakpoint. Report a GPU Issue and Reset GPU(s) reporting the XID (refer GPU reset capabilities/limitations section below).",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "Thease failures mean an uncorrectable error occurs on the GPU, which is also reported back to the user application."
		// "A GPU reset or node reboot is needed to clear this error."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
			Descriptions: []string{
				"Uncorrectable GPU error occurred (Xid 61) -- GPU reset or node reboot is needed.",
			},
		},
	},
	62: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    62,
		Name:                   "Internal micro-controller halt (newer drivers)",
		Description:            "PMU Halt Error. Report a GPU Issue and Reset GPU(s) reporting the XID (refer GPU reset capabilities/limitations section below).",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           true,
		FBCorruption:           false,

		// "Thease failures mean an uncorrectable error occurs on the GPU, which is also reported back to the user application."
		// "A GPU reset or node reboot is needed to clear this error."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
			Descriptions: []string{
				"Uncorrectable GPU error occurred (Xid 61) -- GPU reset or node reboot is needed.",
			},
		},
	},
	63: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             63,
		Name:            "ECC page retirement or row remapping recording event",
		Description: `
These events are logged when the GPU handles ECC memory errors on the GPU.

A100: Row-remapping recording event.

This XID indicates successful recording of a row-remapping entry to the InfoROM.

If associated with XID 94, the application that encountered the error needs to be restarted. All other applications on the system can keep running as is until there is a convenient time to reset the GPU (refer GPU reset capabilities/limitations section below) or reboot for row remapping to activate.

Legacy GPU: ECC page retirement recording event.

If associated with XID 48, drain/cordon the node, wait for all work to complete, and reset GPU(s) reporting the XID (refer GPU reset capabilities/limitations section below).

If not, it is from a single bit error and the system can keep running as is until there is a convenient time to reboot it.

`,
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           true,

		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
			},
			Descriptions: []string{
				"Row-remapping happened (Xid 63, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but for optimal performance, reset the GPU or reboot the system.",
			},
		},
	},
	64: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             64,
		Name:            "ECC page retirement or row remapper recording failure",
		Description: `
These events are logged when the GPU handles ECC memory errors on the GPU.

A100: Row-remapping recording failure.

This XID indicates a failure in recording a row-remapping entry to the InfoROM.

The node should be rebooted immediately since there is a recording failure. If the errors continue, drain, triage, and see Report a GPU Issue, via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.

Legacy GPU: ECC page retirement recording failure.

See above, however the node should be monitored closely. If there is no associated XID 48 error, then these are related to single bit-errors. The GPU(s) reporting the error must be reset (refer to GPU reset capabilities/limitations section below) immediately since there is a recording failure. If the errors continue, drain, triage, and see Report a GPU Issue.

See below for guidelines on when to RMA GPUs based on excessive errors, via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.

`,
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
				common.RepairActionTypeRebootSystem,
			},
			Descriptions: []string{
				"Row-remapping happened (Xid 64, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but to achieve optimal performance, reset the GPU or reboot the system when convenient.",
				"System reboot is recommended when convenient, but not required immediately.",
			},
		},
	},
	65: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    65,
		Name:                   "Video processor exception",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
				common.RepairActionTypeRebootSystem,
			},
			Descriptions: []string{
				"Row-remapping happened (Xid 65, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but to achieve optimal performance, reset the GPU or reboot the system when convenient.",
				"System reboot is recommended when convenient, but not required immediately.",
			},
		},
	},
	66: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    66,
		Name:                   "Illegal access by driver",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           true,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	67: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    67,
		Name:                   "Illegal access by driver",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           true,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	68: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             68,
		Name:            "NVDEC0 Exception",
		Description:     "Video processor exception",
		HWError:         true,
		DriverError:     true,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		// TODO: verify whether this is still true https://github.com/NVIDIA/k8s-device-plugin/issues/945
		UserAppError: true,

		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	69: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    69,
		Name:                   "Graphics Engine class error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "Thease failures mean an uncorrectable error occurs on the GPU, which is also reported back to the user application."
		// "A GPU reset or node reboot is needed to clear this error."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
			Descriptions: []string{
				"Uncorrectable GPU error occurred (Xid 61) -- GPU reset or node reboot is needed.",
			},
		},
	},
	70: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    70,
		Name:                   "CE3: Unknown Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	71: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    71,
		Name:                   "CE4: Unknown Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	72: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    72,
		Name:                   "CE5: Unknown Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	73: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    73,
		Name:                   "NVENC2 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	74: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             74,
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

Xid74 indicates errors in NVLink.
For PCIe A100, it's mainly occurred on the NVLink Bridge between two GPUs.
Its occurrence rate is several orders of magnitude higher than other hardware faults.
Apart from stress testing to exclude those that are constantly repeating errors, there isn't a good way to avoid the occurrence of Xid74 issues.

`,
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           false,

		// "A GPU reset or node reboot is needed to clear this error."
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-74-nvlink-error
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
				common.RepairActionTypeRepairHardware,
			},
			Descriptions: []string{
				"GPU reset or node reboot is needed to clear this error (Xid 74).",
				"If this error is seen repeatedly (Xid 74), contact hardware vendor to check the physical link.",
			},
		},
	},
	75: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    75,
		Name:                   "CE6: Unknown Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	76: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    76,
		Name:                   "CE7: Unknown Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	77: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    77,
		Name:                   "CE8: Unknown Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	78: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    78,
		Name:                   "vGPU Start Error",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	79: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             79,
		Name:            "GPU has fallen off the bus",
		Description: `
This event is logged when the GPU driver attempts to access the GPU over its PCI Express connection and finds that the GPU is not accessible.

This event is often caused by hardware failures on the PCI Express link causing the GPU to be inaccessible due to the link being brought down.

Reviewing system event logs and kernel PCI event logs may provide additional indications of the source of the link failures.

This event may also be cause by failing GPU hardware or other driver issues.
`,
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           true,
		FBCorruption:           false,

		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-79-gpu-has-fallen-off-the-bus
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRepairHardware,
			},
			Descriptions: []string{
				"GPU not accessible due to failing hardware (Xid 79, 'GPU has fallen off the bus') -- check with the data center.",
			},
		},
	},
	80: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    80,
		Name:                   "Corrupted data sent to GPU",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	81: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    81,
		Name:                   "VGA Subsystem Error",
		Description:            "",
		HWError:                true,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	82: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    82,
		Name:                   "NVJPG0 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	83: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    83,
		Name:                   "NVDEC1 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	84: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    84,
		Name:                   "NVDEC2 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	85: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    85,
		Name:                   "CE9: Unknown Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	86: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    86,
		Name:                   "OFA Exception",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	87: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    87,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	88: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    88,
		Name:                   "NVDEC3 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	89: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    89,
		Name:                   "NVDEC4 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	90: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    90,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	91: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    91,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	92: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             92,
		Name:            "High single-bit ECC error rate",
		Description: `
See Running Field Diagnostics to collect additional debug information, via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#running-field-diag.

See below for guidelines on when to RMA GPUs based on excessive errors.
`,
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	93: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    93,
		Name:                   "Non-fatal violation of provisioned InfoROM wear limit",
		Description:            "",
		HWError:                false,
		DriverError:            true,
		UserAppError:           true,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	94: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             94,
		Name:            "Contained ECC error",
		Description: `
This XID indicates a contained ECC error has occurred.

These events are logged when GPU drivers handle errors in GPUs that support error containment, starting with NVIDIA® A100 GPUs.

For Xid 94, these errors are contained to one application, and the application that encountered this error must be restarted.

All other applications running at the time of the Xid are unaffected.

It is recommended to reset the GPU when convenient. Applications can continue to be run until the reset can be performed.

(A100 only)

The application that encountered the error needs to be restarted. All other applications on the system can keep running as is until there is a convenient time to reset the GPU (refer to GPU reset capabilities/limitations section below) or reboot for row remapping to activate.

See below for guidelines on when to RMA GPUs based on row remapping failures
`,
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           true,

		// "recommended to reset the GPU when convenient"
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-94-95-contained-uncontained
		//
		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeCheckUserAppAndGPU,
				common.RepairActionTypeRebootSystem,
			},
			Descriptions: []string{
				"Row-remapping happened (Xid 94, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but to achieve optimal performance, reset the GPU or reboot the system when convenient.",
				"System reboot is recommended when convenient, but not required immediately.",
			},
		},
	},
	95: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             95,
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
`,
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           true,

		// "the affected GPU must be reset before applications can restart."
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-94-95-contained-uncontained
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
				common.RepairActionTypeRepairHardware,
			},
			Descriptions: []string{
				"GPU reset or system reboot is needed to clear this uncontained ECC error (Xid 95). If MIG is disabled, the node should be rebooted immediately since there is an uncorrectable uncontained ECC error.",
				"If the errors continue (Xid 95), drain the node and contact the hardware vendor for assistance.",
			},
		},
	},
	96: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    96,
		Name:                   "NVDEC5 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	97: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    97,
		Name:                   "NVDEC6 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	98: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    98,
		Name:                   "NVDEC7 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	99: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    99,
		Name:                   "NVJPG1 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	100: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    100,
		Name:                   "NVJPG2 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	101: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    101,
		Name:                   "NVJPG3 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	102: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    102,
		Name:                   "NVJPG4 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	103: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    103,
		Name:                   "NVJPG5 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	104: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    104,
		Name:                   "NVJPG6 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	105: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    105,
		Name:                   "NVJPG7 Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	106: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    106,
		Name:                   "SMBPBI Test Message",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           true,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	107: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    107,
		Name:                   "SMBPBI Test Message Silent",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           true,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	108: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    108,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	109: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    109,
		Name:                   "Context Switch Timeout Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           true,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           true,
		FBCorruption:           true,
	},
	110: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    110,
		Name:                   "Security Fault Error",
		Description:            `This event should be uncommon unless there is a hardware failure. To recover, revert any recent system hardware modifications and cold reset the system. If this fails to correct the issue, contact your hardware vendor for assistance.`,
		HWError:                true,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,

		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-110-security-fault-error
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
			},
			Descriptions: []string{
				"This event should be uncommon unless there is a hardware failure (Xid 110).",
			},
		},
	},
	111: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    111,
		Name:                   "Display Bundle Error Event",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	112: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    112,
		Name:                   "Display Supervisor Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	113: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    113,
		Name:                   "DP Link Training Erro",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	114: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    114,
		Name:                   "Display Pipeline Underflow Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
	115: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    115,
		Name:                   "Display Core Channel Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	116: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    116,
		Name:                   "Display Window Channel Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	117: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    117,
		Name:                   "Display Cursor Channel Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	118: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    118,
		Name:                   "Display Pixel Pipeline Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	119: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    119,
		Name:                   "GSP RPC Timeout",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           true,
		FBCorruption:           true,

		// "Xid119 means GPU GSP module failed."
		// "These failures need to do fieldiag test, and most need to RMA."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRepairHardware,
			},
			Descriptions: []string{
				"GPU GSP module failed (Xid 119) -- check if GPU qualifies for RMA.",
			},
		},
	},
	120: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    120,
		Name:                   "GSP Error",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: true,
		BusError:               true,
		ThermalIssue:           true,
		FBCorruption:           true,
	},
	121: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    121,
		Name:                   "C2C Link Error",
		Description:            "",
		HWError:                true,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               true,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	122: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    122,
		Name:                   "SPI PMU RPC Read Failure",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	123: {
		DocumentVersion: "r555 (Sep 24, 2024)",
		XID:             123,
		Name:            "SPI PMU RPC Write Failure",
		Description: `
Report a GPU issue and reset GPU(s) reporting the XID (refer to GPU reset capabilities/limitations section provided in Section D.9 of the FM User Guide: https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf).
`,
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	124: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    124,
		Name:                   "SPI PMU RPC Erase Failure",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	125: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    125,
		Name:                   "Inforom FS Failure",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	126: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    126,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	127: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    127,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	128: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    128,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	129: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    129,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	130: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    130,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	131: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    131,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	132: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    132,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	134: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    134,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	135: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    135,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	136: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    136,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	137: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    137,
		Name:                   "NVLink FLA privilege error",
		Description:            `This event is logged when a fault is reported by the remote MMU, such as when an illegal NVLink peer-to-peer access is made by an applicable unit on the chip. Typically these are application-level bugs, but can also be driver bugs or hardware bugs.`,
		HWError:                false,
		DriverError:            false,
		UserAppError:           true,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	138: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    138,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	139: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    139,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	140: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    140,
		Name:                   "Unrecovered ECC Error",
		Description:            `This event may occur when the GPU driver has observed uncorrectable errors in GPU memory, in such a way as to interrupt the GPU driver’s ability to mark the pages for dynamic page offlining or row remapping. Reset the GPU, and if the problem persists, contact your hardware vendor for support.`,
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           true,

		// "Reset the GPU, and if the problem persists, contact your hardware vendor for support"
		// ref. https://docs.nvidia.com/deploy/xid-errors/index.html#xid-140-ecc-unrecovered-error
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeRebootSystem,
				common.RepairActionTypeRepairHardware,
			},
			Descriptions: []string{
				"Reset the GPU in case the row remapping is pending (Xid 140).",
				"Requires hardware vendor support if the problem persists after reboot (Xid 140).",
			},
		},
	},
	141: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    141,
		Name:                   "Reserved",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	142: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    142,
		Name:                   "Unrecovered ECC Error",
		Description:            "",
		HWError:                false,
		DriverError:            false,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           false,
	},
	143: {
		DocumentVersion:        "r555 (Sep 24, 2024)",
		XID:                    143,
		Name:                   "GPU Initialization Failure",
		Description:            "",
		HWError:                true,
		DriverError:            true,
		UserAppError:           false,
		SystemMemoryCorruption: false,
		BusError:               false,
		ThermalIssue:           false,
		FBCorruption:           true,
	},
}
