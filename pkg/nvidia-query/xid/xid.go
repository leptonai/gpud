// Package xid provides the NVIDIA XID error details.
package xid

import (
	"fmt"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// Defines the Xid error information that is static.
type Detail struct {
	DocumentVersion string `json:"documentation_version"`

	Xid         int    `json:"xid"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// SuggestedActionsByGPUd is the suggested actions by GPUd.
	SuggestedActionsByGPUd *apiv1.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`
	// CriticalErrorMarkedByGPUd is true if the GPUd marks this Xid as a critical error.
	// You may use this field to decide whether to alert or not.
	CriticalErrorMarkedByGPUd bool `json:"critical_error_marked_by_gpud"`
	// EventType is the type of the event.
	EventType apiv1.EventType `json:"event_type"`

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

// if nvidia says only possible reason is hw, then we do hard inspections directly
func (d Detail) IsOnlyHWError() bool {
	if !d.PotentialHWError {
		return false
	}
	return !d.PotentialDriverError &&
		!d.PotentialUserAppError &&
		!d.PotentialSystemMemoryCorruption &&
		!d.PotentialBusError &&
		!d.PotentialThermalIssue &&
		!d.PotentialFBCorruption
}

// if nvidia says this can be only because of user error, then we ignore, don’t mark it as critical
func (d Detail) IsOnlyUserAppError() bool {
	if !d.PotentialUserAppError {
		return false
	}
	return !d.PotentialHWError &&
		!d.PotentialDriverError &&
		!d.PotentialSystemMemoryCorruption &&
		!d.PotentialBusError &&
		!d.PotentialThermalIssue &&
		!d.PotentialFBCorruption
}

// if nvidia says this can be only because of driver error, then we only reboot
func (d Detail) IsOnlyDriverError() bool {
	if !d.PotentialDriverError {
		return false
	}
	return !d.PotentialHWError &&
		!d.PotentialUserAppError &&
		!d.PotentialSystemMemoryCorruption &&
		!d.PotentialBusError &&
		!d.PotentialThermalIssue &&
		!d.PotentialFBCorruption
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

// make sure we do not have unknown event type
func init() {
	for id, detail := range details {
		if detail.EventType == apiv1.EventTypeUnknown || string(detail.EventType) == "" {
			panic(fmt.Sprintf("unknown event type for Xid %d", id))
		}
	}
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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 9 indicates driver error programming GPU, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 12 indicates a driver error handling GPU exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 13 is a potential hw/driver/user app/system memory corruption/bus/thermal issue/fb corruption.
			// "NVIDIA Xid 13: GR: SW Notify Error", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-13-gr-sw-notify-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 13.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin implements Xid 13 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 13 as a non-hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)`
			//
			// NOTE: The AWS support doc does not mention Xid 13.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc explains Xid 13 is returned when applications have illegal memory access issues, and recommends cuda-memcheck and CUDA-GDB for debugging.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 13 requires self-check by the user and users to resubmit the load to see if the issue goes away
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 13.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement any health check for Xid 13.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 13 indicates GPU memory anomalies, but can be a hardware error.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 13, marked as non-critical in GPUd, indicates GPU memory anomalies affecting code and data segments, arrays being out of their declared ranges, applications having illegal memory access issues, or instruction errors. Restart applications and check whether the same Xid is returned. To debug, refer to cuda-memcheck https://developer.nvidia.com/cuda-memcheck or CUDA-GDB https://docs.nvidia.com/cuda/cuda-gdb/index.html. Since in rare cases it can be caused by the hardware degradation, please report if the issue persists.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is IGNORE_NO_ACTION_REQUIRED without REBOOT_SYSTEM/HARDWARE_INSPECTION
		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 16 indicates display engine hung, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 18 indicates bus mastering disabled in PCI Config Space, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 19 indicates display engine hung, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 26 indicates framebuffer timeout, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 27 indicates a video processor exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 28 indicates video processor exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			// Xid 29 indicates video processor exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			// Xid 34 indicates GPU semaphore access error, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 31 as a user application issue, but can also be driver bugs or hardware issues.
			// This event is logged when MMU reports a fault when an illegal address access is made by an application unit on the chip.
			// "NVIDIA Xid 31: FIFO: MMU Error", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-31-fifo-mmu-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline explains Xid 31 requires user application debugging.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 31.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin implements Xid 31 as a user application issue.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement any Xid 31 health checks.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)`
			//
			// NOTE: The AWS support doc does not mention Xid 31.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc explains Xid 31 is returned when applications have illegal memory access issues, and recommends cuda-memcheck and CUDA-GDB for debugging.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 31 requires self-check by users and users to resubmit the workload to see if the same Xid is returned.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 31.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement any Xid 31 health checks.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 31 is a user application issue, which may indicate the GPU memory anomalies, but can also be hardware issues.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 31, marked as non-critical in GPUd, indicates GPU memory page fault, with applications having illegal memory access issues. Restart applications and check whether the same Xid is returned. To debug, refer to cuda-memcheck https://developer.nvidia.com/cuda-memcheck or CUDA-GDB https://docs.nvidia.com/cuda/cuda-gdb/index.html.
			// Xid 31, marked as non-critical in GPUd, indicates GPU memory page fault. In rare cases it can be caused by the hardware degradation. If the issue persists, please report for hardware inspection and repair.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeCheckUserAppAndGPU,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,
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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 32 is a DMA controller error which manages the communication between the NVIDIA driver and GPU over the PCI-E bus.
			// Which indicates the PCI quality issues, not the user application issues.
			// "NVIDIA Xid 32: PBDMA Error", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-32-pbdma-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 32.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 32.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat this as an application issue.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement any Xid 32 health checks.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)`
			//
			// NOTE: The AWS support doc does not mention Xid 32.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 32.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 32 indicates invalid/corrupted push buffer stream in the PCIE bus between the NVIDIA driver and GPU, recommending submitting technical support tickets.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 32.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement any Xid 32 health checks.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper does not mention Xid 32.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			// Xid 32, marked as critical in GPUd, indicates PCI bus issues between the NVIDIA driver and GPU. Reboot the system to check whether the PCI quality issue persists.
			// Xid 32, marked as critical in GPUd, indicates PCI bus issues between the NVIDIA driver and GPU. If the issue persists after system reboot, please submit a technical support ticket for hardware inspection and repair.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 33 indicates internal micro-controller error, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 34 indicates video processor exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 35 indicates video processor exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 36 indicates video processor exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 38 as a potential driver firmware error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 38.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 38.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 38 as an application issue.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement any Xid 38 health checks.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)`
			//
			// NOTE: The AWS support doc does not mention Xid 38.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 38.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 38 as a driver firmware issue.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 38.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement any Xid 38 health checks.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper does not mention Xid 38.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 38, marked as critical in GPUd, indicates NVIDIA driver firmware issues. Reboot the system to check whether the firmware issue persists.",
			// Xid 38, marked as critical in GPUd, indicates NVIDIA driver firmware issues. If the firmware issue persists after system reboot, please submit a technical support ticket for hardware inspection and repair.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			// Xid 42 indicates a video processor exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 43 as a user application hitting a software induced faults.
			// "NVIDIA Xid 43: Reset Channel Verif Error", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-43-reset-channel-verif-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 43.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 43.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin implements Xid 43 as a user application error, indicating GPU stopped processing.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement any Xid 43 health checks.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)`
			//
			// NOTE: The AWS support doc does not mention Xid 43.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 43.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 43 as a user application error encountering a software induced fault, as a result, GPU stopped processing.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 43.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement any Xid 43 health checks.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 43 as a user application error, but may indicate GPU memory anomalies.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 43, marked as non-critical in GPUd, indicates GPU stopped processing, due to a user application encountering a software induced fault. Restart applications and check whether the same Xid is returned. And report if the issue persists.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 44 as a potential driver issue.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 44.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 44.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 44 as a user application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement any Xid 44 health checks.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)`
			//
			// NOTE: The AWS support doc does not mention Xid 44.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 44.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc does not mention Xid 44.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 44.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement any Xid 44 health checks.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 44 indicates uncorrectable GPU errors, recommends GPU reset or node reboot.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 44, marked as critical in GPUd, indicates uncorrectable GPU errors. Stop existing workloads and reboot the system (or reset GPUs) to clear this error.
			// Xid 44, marked as critical in GPUd, indicates uncorrectable GPU errors. If the uncorrectable GPU error persists after rebooting the system, inspect and repair the hardware.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 45 is returned when the kernel driver terminates a GPU application, as a result of a user of system action.
			// "NVIDIA Xid 45: OS: Preemptive Channel Removal", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-45-os-preemptive-channel-removal (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline explains Xid 45 indicates channel affected by another failure, or may indicate unexpected Fabric Manager shutdown.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 45.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin implements Xid 45 as a user application error, being preemptive cleanup due to previous errors.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement any Xid 45 health checks.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)`
			//
			// NOTE: The AWS support doc does not mention Xid 45.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 45.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 45 may indicate multiple cuda applications hitting a DBE, or the result of application being stopped due to another error.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc explains does not mention Xid 45.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement any Xid 45 health checks.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 45 indicates GPU memory anomalies, affecting code and data segments, but can be hardware-related.
			// //"Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// NOTE: A Reddit user reports Xid 45 was reported before the GPU was dead.
			// "screen blacks out and I am given NVRM Nvidia Xid 45, GPU was dead", https://www.reddit.com/r/pop_os/comments/joq8zn/nvrm_nvidia_xid_45_error_intermittent (accessed on Nov 3, 2024)
			//
			// Xid 45, indicates preemptive cleanup due to previous errors. Xid 45 indicates the result of GPU memory issues, such as multiple cuda applications hitting uncorrectable double bit errors (DBE), or an application being stopped by another error. This Xid is likely to overlap with other ongoing Xid events, thus ignore for now.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeIgnoreNoActionRequired,
			},
		},
		// TODO
		// unhealthy if there's no previous Xid event in the same time window
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is IGNORE_NO_ACTION_REQUIRED without REBOOT_SYSTEM/HARDWARE_INSPECTION
		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 46 indicates GPU stopped processing, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 47 indicates a video processor exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 48 indicates uncorrectable double bit errors (DBE), recommending GPU reset or system reboot.
			// "NVIDIA Xid 48: DBE (Double Bit Error) ECC Error", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-48-dbe-double-bit-error-ecc-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline explains if Xid 48 is followed by Xid 63 or 64, the node should be drained and the GPUs should be reset.
			// If Xid 48 is not followed by Xid 63 or 64, the user should run field diagnostics to collect additional debug information.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 48.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 48 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 48 as a hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc explains Xid 48 indicates a DBE has occurred, recommending system reboot after checking if any GPUs are missing.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc explains Xid 48 indicates a DBE has occurred, recommending stopping the existing workloads and rebooting the system.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 48 indicates Double Bit ECC Error (DBE), reported when the GPU encounters an uncorrectable error.
			// In most cases, Xid 48 requires GPU reset or system reboot to fix the error.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc explains Xid 48 indicates a Double Bit ECC error,
			// recommending GPU reset or system reboot to fix the error.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 48 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 48 as an uncorrectable GPU failure, recommending GPU reset or system reboot.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 48, marked as critical in GPUd, indicates uncorrectable double bit ECC errors (DBE), which also reports back to the user application. Stop existing workloads and reboot the system (or reset GPUs) to clear this error.",
			// Xid 48, marked as critical in GPUd, indicates uncorrectable double bit ECC errors (DBE). If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 59 indicates an internal micro-controller error, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 60 indicates video processor exception, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 61 indicates internal micro-controller warning.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline recommends resetting the GPU that reports the Xid 61.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 61.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 61 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement Xid 61 detection.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 61.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 61.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 61 indicates GPU internal engine stops working, thus affecting the business.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 61.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement Xid 61 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 61 indicates uncorrectable GPU errors, which reports back to the user application,
			// recommending GPU reset or node reboot to clear this error.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 61, marked as critical in GPUd, indicates internal micro-controller breakpoint/warning and GPU internal engine stops working. Stop existing workloads and reboot the system (or reset GPUs) to clear this error.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 62 indicates internal micro-controller halt.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline recommends resetting the GPU that reports the Xid 62.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 62.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 62 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 62 as a hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 62.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 62.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 62 indicates internal micro-controller halt, similar to Xid 61.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 62.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 62 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 62 indicates uncorrectable GPU errors, which reports back to the user application,
			// recommending GPU reset or node reboot to clear this error.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 62, marked as critical in GPUd, indicates internal micro-controller halt and GPU internal engine stops working. Stop existing workloads and reboot the system (or reset GPUs) to clear this error.
			// Xid 62, marked as critical in GPUd, indicates internal micro-controller halt. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 63 indicates ECC page retirement recording event for legacy GPUs or row-remapping recording event for A100.
			// "NVIDIA Xid 63, 64: ECC Page Retirement or Row Remapping", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-63-64-ecc-page-retirement-or-row-remapping (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline explains for legacy GPUs, if Xid 63 is associated with Xid 48, then drain/cordon the node and reset the GPU.
			// If not, it is from a single bit error, then the system can keep running as is until there is a convenient time to reboot it.
			// For A100 GPUs that support row-remapping, if associated with Xid 94, the application that encountered the error needs to be restarted.
			// All other applications on the system can keep running until there's a convenient time to reset the GPU or reboot the system.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc explains Xid 63 indicates successful recording of a row-remapping entry to the InfoROM.
			// The row-remapping process requires GPU reset or system reboot to take effect.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 63 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 63 as a hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc explains Xid 63 indicates a page has successfully been retired,
			// recommending checking the number of attached GPUs.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc explains Xid 63 indicates ECC page retirement or row remapping recording event,
			// recommending stopping the workloads and GPU reset to clear this error.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains explains Xid 63 as ECC page retirement or row remapping recording event,
			// recommending submitting a ticket to request for technical support.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 63.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 63 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 63 as memory ECC error, which can be resolved by simply resetting the GPU to retain the optimal performance.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 63, marked as critical in GPUd, indicates ECC page retirement recording event for legacy GPUs, row remapping recording event for >=A100/H100. User applications can keep running, but for optimal performance, stop the workloads and reset the GPU or reboot the system. The process of row-remapping requires GPU reset or system reboot to take effect, and to remain permanently effective.
			// Xid 63, marked as critical in GPUd, indicates ECC page retirement recording event or row remapping recording event. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 64 indicates ECC page retirement recording failure or row-remapping recording failure.
			// "NVIDIA Xid 63, 64: ECC Page Retirement or Row Remapping", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-63-64-ecc-page-retirement-or-row-remapping (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline explains if not associated with Xid 48, then these are related to single bit-errors.
			// If the errors persist, drain and triage the machine.
			// If the error is the row-remapping recording failure, the node should be rebooted immediately.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc explains Xid 64 indicates a failure in recording a row-remapping entry to the InfoROM.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 64 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 64 as a hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc explains Xid 64 indicates a page has failed retirement due to an error,
			// recommending checking the number of attached GPUs.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc explains Xid 64 indicates ECC page retirement or row remapper recording failure,
			// recommending stopping the workloads and rebooting the system.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains explains Xid 64 as ECC page retirement or row remapper recording failure,
			// recommending submitting a ticket to request for technical support.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 64.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 64 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 64 as memory ECC error, which can be resolved by simply resetting the GPU to retain the optimal performance.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 64, marked as critical in GPUd, indicates ECC page retirement recording failure for legacy GPUs, row remapping recording failure for >=A100/H100. The node should be rebooted immediately since there is a recording failure.
			// Xid 64, marked as critical in GPUd, indicates ECC page retirement recording failure or row remapping recording failure. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// Row-remapping happened (Xid 65, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but to achieve optimal performance, reset the GPU or reboot the system when convenient.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM but no immediate reboot is required
		EventType: apiv1.EventTypeCritical,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 68 as a potential hardware/driver issue or a user application error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 68.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 68.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin implements Xid 68 as a user application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 68 as a hardware issue.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 68.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 68.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 68 requires user-side troubleshooting, recommending resubmitting the workloads and report if the issue persists.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 68.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 68 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper does not mention Xid 68.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 68, marked as non-critical in GPUd, indicates video processor exception. In some cases, Xid 68 indicates deeper GPU driver or hardware issues. Thus, reboot the system.
			// Xid 68, marked as non-critical in GPUd, indicates video processor exception. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		// TODO
		// ignore first xid 68 occurrences
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 69 as a potential hardware/driver issue.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 69.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 69.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 69 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 69 as a hardware issue.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 69.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 69.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc does not mention Xid 69.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 69.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 69 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 69 indicates GPU uncorrectable errors, recommending GPU reset or node reboot.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 69, marked as critical in GPUd, indicates uncorrectable GPU errors. Stop the workloads and reboot the system. And report if the issue persists.
			// Xid 69, marked as critical in GPUd, indicates uncorrectable GPU errors. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

Bits 21 or 22: Marginal channel SI issue. Check link mechanical connections. If other errors accompany, follow the resolution for those.

Bits 8, 9, 12, 16, 17, 24, 28: Could possibly be a HW issue: Check link mechanical connections and re-seat if a field resolution is required. Run diags if issue persists.



"Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158 explains:

Xid 74 indicates errors in NVLink.

For PCIe A100, it's mainly occurred on the NVLink Bridge between two GPUs.
Its occurrence rate is several orders of magnitude higher than other hardware faults.
Apart from stress testing to exclude those that are constantly repeating errors, there isn't a good way to avoid the occurrence of Xid74 issues.

The XID indicates an NVLink hardware error. The GPU encounters a critical hardware error and must be repaired.
`,

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 74 indicates a connection problem between GPUs, or NVSwitch over NVLink.
			// GPU reset or system reboot is needed to clear the error.
			// "NVIDIA Xid 74: NVLink Error", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-74-nvlink-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline explains Xid 74 indicates a NVLink hardware error.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 74.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 74 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 74 as a hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 74.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc Xid 74 requires stopping the workloads and GPU reset.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 74 indicates a NVLink hardware error, recommending submitting a ticket for technical support.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 74.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 74 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 74 indicates NVLink errors.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 74, marked as critical in GPUd, indicates an NVLink hardware error. It is a critical hardware error that impacts the physical link between the GPUs, and must be repaired. Stop the workloads and reboot the system.
			// Xid 74, marked as critical in GPUd, is a critical hardware error that impacts the physical link between the GPUs, must be repaired if the issue persists after rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 78 indicates vGPU start error, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 79 indicates GPUs not being accessible, due to the PCI express connection failures.
			// "NVIDIA Xid 79: GPU has fallen off the bus", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-79-gpu-has-fallen-off-the-bus (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline explains Xid 79 requires node drain.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 79.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 79 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 79 as a hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc explains Xid 79 indicates that the instance lost communication with the GPUs, recommending system reboot.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc explains Xid 79 indicates that the driver is not able to communicate with the GPUs, and requires system reboot.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 79 indicates that the GPU has fallen off the bus, not able to find the GPUs, recommending hardware repair.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc explains Xid 79 is returned due to a GPU driver or hardware issue, recommending system reboot.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 79 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 79 as uncorrectable GPU errors, recommending GPU reset or system reboot.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 79, marked as critical in GPUd, indicates GPU has fallen off the bus, with the driver not able to communicate with underlying GPUs. Stop the workloads and reboot the system.
			// Xid 79, marked as critical in GPUd, indicates GPU driver is not able to communicate with underlying GPUs. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says only possible reason is hw, then we do hard inspections directly
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// Xid 81 indicates VGA subsystem error, labeling a hardware failure as an only possible reason, thus we recommend submitting a ticket for hardware inspection.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is HARDWARE_INSPECTION
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 92 as a potential hardware or driver issue.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline explains Xid 92 indicates a high single-bit ECC error rate,
			// which may qualify for the RMA process, if the rates are excessive.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 92.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not implement Xid 92 as an application-level error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 92 as a hardware issue.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 92.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc explains Xid 92 is returned after the GPU correcting the correctable errors, not affecting your workloads.
			// This Xid is informational only -- no action is required.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc explains Xid 92 can be returned when a hardware or driver issue occurs.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 92.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 92 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper does not mention Xid 92.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			// Xid 92, indicates high single-bit ECC error rate, meaning the GPU driver has corrected correctable errors. Xid 92 is informational only -- no action is required.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeIgnoreNoActionRequired,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is IGNORE_NO_ACTION_REQUIRED without REBOOT_SYSTEM/HARDWARE_INSPECTION
		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 94 indicates a contained ECC error, successfully suppressed.
			// "NVIDIA Xid 94, 95: Contained/uncontained", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-94-95-contained-uncontained (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)

			// NOTE: The official nvidia debugging guideline explains Xid 94 indicates that the application that encountered the error must be restarted.
			// All other applications can keep running as is until there is a convenient time to reset the GPU or reboot for row remapping to activate.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)

			// NOTE: The official nvidia memory error doc explains Xid 94 indicates a contained ECC error has occurred.
			// Row-remapping requires GPU reset or system reboot to take effect.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)

			// NOTE: The official nvidia k8s device plugin does not implement Xid 94 as an application-level error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)

			// NOTE: The imbue-ai GPU health check implements Xid 94 as a hardware issue.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)

			// NOTE: The AWS support doc does not mention Xid 94.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)

			// NOTE: The Google Cloud doc explains Xid 94 requires stopping the workloads and resetting the GPU.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)

			// NOTE: The Alibaba Cloud doc explains Xid 94 indicates that the uncorrectable ECC error is successfully suppressed, only affecting the faulty application,
			// recommending to submit a ticket for technical support.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)

			// NOTE: The Tencent Cloud doc explains Xid 94 indicates contained ECC error, recommending application restarts. If the issue persists, reboot the system.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)

			// NOTE: The Azure HPC GPU node problem detector does not implement Xid 94 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)

			// NOTE: DeepSeek AI paper explains Xid 94 indicates GPU memory ECC error, recommending GPU reset for optimal performance.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 94, marked as critical in GPUd, indicates contained ECC errors with row-remapping successfully suppressing the errors. User applications can keep running, but the faulty application must be restarted. Stop the workloads and reset the GPU or reboot the system. This ensures retirement or remapping is permanently effective.",
			// Xid 94, marked as critical in GPUd, indicates contained ECC errors with row-remapping successfully suppressing the errors. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeIgnoreNoActionRequired,
			},
		},
		// TODO
		// ignore the first few occurrences and then suggest reboot
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM but no immediate reboot is required
		EventType: apiv1.EventTypeCritical,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid 94, 95: Contained/uncontained", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-94-95-contained-uncontained (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)

			// NOTE: The official nvidia debugging guideline explains Xid 95 requires immediate system reboot (when MIG is disabled),
			// since there is an uncorrectable uncontained ECC error.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)

			// NOTE: The official nvidia memory error doc explains Xid 95 indicates an uncontained ECC error has occurred.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)

			// NOTE: The official nvidia k8s device plugin does not implement Xid 95 as an application-level error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)

			// NOTE: The imbue-ai GPU health check implements Xid 95 as a hardware issue.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)

			// NOTE: The AWS support doc does not mention Xid 95.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)

			// NOTE: The Google Cloud doc explains Xid 95 requires stopping the workloads and resetting the GPU.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)

			// NOTE: The Alibaba Cloud doc explains Xid 95 indicates that the uncorrectable ECC error fails to be suppressed, affecting other applications,
			// recommending to submit a ticket for technical support.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)

			// NOTE: The Tencent Cloud doc explains Xid 95 indicates uncontained ECC error, recommending system reboot.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)

			// NOTE: The Azure HPC GPU node problem detector does not implement Xid 95 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)

			// NOTE: DeepSeek AI paper explains Xid 95 indicates GPU memory ECC error, recommending GPU reset for optimal performance.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 95, marked as critical in GPUd, indicates uncontained ECC errors with row-remapping, failing to suppress the errors. In addition to the faulty application, other applications are affected. Stop the workloads and reset the GPU or reboot the system to clear this uncontained ECC error. If MIG is disabled, the node should be rebooted immediately since there is an uncorrectable uncontained ECC error.
			// Xid 95, marked as critical in GPUd, indicates uncontained ECC errors with row-remapping, failing to suppress the errors. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		// Xids whose GPUd.RepairActions is HARDWARE_INSPECTION
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// if nvidia says only possible reason is hw, then we do hard inspections directly
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			// Xid 110 indicates a security fault error, labeling a hardware failure as an only possible reason, thus we recommend submitting a ticket for hardware inspection.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is HARDWARE_INSPECTION
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 119 indicates GSP module failures to respond to RPC messages,
			// recommending GPU reset or node power cycle if the issue persists.
			// "Xid 119, 120: GSP RPC Timeout / GSP Error", https//docs.nvidia.com/deploy/xid-errors/index.html#xid-119-120-gsp-rpc-timeout-gsp-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 119.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 119 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 119 as a hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc explains Xid 119 requires GSP being turned off.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc explains Xid 119 requires stopping the workloads and rebooting the system.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc does not mention Xid 119.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: This Alibaba Cloud support doc explains Xid 119 requires disabling the GSP component.
			// "Xid 119/120 error occurs", https://help.aliyun.com/zh/egs/support/a-gpu-has-fallen-off-the-bus-due-to-an-xid-119-or-xid-120-error (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc explains Xid 119 requires disabling the GSP component.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 119 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper explains Xid 119 requires diagnosing for potential RMA.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 119, marked as critical in GPUd, indicates GSP module failures (e.g., GSP core timed out responding to RPC messages). Stop the workloads and reboot the system.
			// Xid 119, marked as critical in GPUd, indicates GSP module failures. If the same Xid is reported again after rebooting the system, the GSP module should be disabled. If the issue persists, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 120 indicates GSP module failures to respond to RPC messages,
			// recommending GPU reset or node power cycle if the issue persists.
			// "Xid 119, 120: GSP RPC Timeout / GSP Error", https//docs.nvidia.com/deploy/xid-errors/index.html#xid-119-120-gsp-rpc-timeout-gsp-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 120.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 120 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 120 as a hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc explains Xid 120 requires GSP being turned off.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc explains Xid 120 requires stopping the workloads and rebooting the system.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc does not mention Xid 120.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: This Alibaba Cloud support doc explains Xid 120 requires disabling the GSP component.
			// "Xid 119/120 error occurs", https://help.aliyun.com/zh/egs/support/a-gpu-has-fallen-off-the-bus-due-to-an-xid-119-or-xid-120-error (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 120.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector implements Xid 120 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper does not mention Xid 120.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 120, marked as critical in GPUd, indicates GSP module failures (e.g., GSP core timed out responding to RPC messages). Stop the workloads and reboot the system.
			// Xid 120, marked as critical in GPUd, indicates GSP module failures. If the same Xid is reported again after rebooting the system, the GSP module should be disabled. If the issue persists, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 121 indicates corrected errors on the C2C NVLink connection to a Grace CPU, with no operational impact,
			// recommending the GPU reset to retrain the link.
			// "NVIDIA Xid 121: C2C Link corrected error", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-121-c2c-link-corrected-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 121.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 121.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 121 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement Xid 121 detection.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 121.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 121.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc does not mention Xid 121.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 121.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement Xid 121 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper does not mention Xid 121.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 121, marked as non-critical in GPUd, indicates corrected errors on the C2C NVLink connection to a Grace CPU, with no operational impact. Stop the workloads and reboot the system to retrain the link when it's convenient.
			// Xid 121, marked as non-critical in GPUd, indicates corrected errors on the C2C NVLink connection to a Grace CPU. If the same Xid is returned after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 123 indicates potential hardware or driver errors.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline explains Xid 123 requires GPU reset that reports the Xid error.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 123.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 123 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check implements Xid 123 detection as a hardware error.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 123.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 123.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc does not mention Xid 123.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 123.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement Xid 123 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper does not mention Xid 123.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 123, marked as non-critical in GPUd, indicates SPI PMU RPC write failures. Stop the workloads and reboot the system.
			// Xid 123, marked as non-critical in GPUd, indicates SPI PMU RPC write failures. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true, // only because it requires reboot

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 137 indicates a user application error, recommending cuda-memcheck or CUDA-GDB for debugging.
			// "NVIDIA Xid 137: NVLink FLA privilege error", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-137-nvlink-fla-privilege-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 137.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 137.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 137 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement Xid 137 detection.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 137.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 137.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc does not mention Xid 137.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 137.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement Xid 137 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper does not mention Xid 137.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 137, marked as non-critical in GPUd, indicates illegal NVLink peer-to-peer access by an applicable unit on the chip, typically application-level bugs, but can also be driver bugs or hardware issues. To debug, refer to cuda-memcheck https://developer.nvidia.com/cuda-memcheck or CUDA-GDB https://docs.nvidia.com/cuda/cuda-gdb/index.html.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is IGNORE_NO_ACTION_REQUIRED without REBOOT_SYSTEM/HARDWARE_INSPECTION
		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 140 indicates uncorrectable GPU memory errors, which may impact the dynamic page offlining or row remapping,
			// recommending GPU reset if the issue persists.
			// "NVIDIA Xid 140: ECC unrecovered error", https://docs.nvidia.com/deploy/xid-errors/index.html#xid-140-ecc-unrecovered-error (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			//
			// NOTE: The official nvidia debugging guideline does not mention Xid 140.
			// "NVIDIA GPU debug guidelines, https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia memory error doc does not mention Xid 140.
			// "NVIDIA GPU Memory Error Management", https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html (accessed on Nov 3, 2024)
			//
			// NOTE: The official nvidia k8s device plugin does not treat Xid 140 as an application error.
			// "NVIDIA/k8s-device-plugin health check", https://github.com/NVIDIA/k8s-device-plugin/blob/v0.17.0/internal/rm/health.go#L65-L71 (Aug 2024)
			//
			// NOTE: The imbue-ai GPU health check does not implement Xid 140 detection.
			// "imbue-ai/cluster-health health check", https://github.com/imbue-ai/cluster-health/blob/8f5964ac620931138ed29f43557557048e826cd7/health_checks/health_checks.py#L769-L782 (Aug 2024)
			//
			// NOTE: The AWS support doc does not mention Xid 140.
			// "Troubleshoot Xid errors", https://repost.aws/knowledge-center/ec2-linux-troubleshoot-xid-errors (accessed on Nov 3, 2024)
			//
			// NOTE: The Google Cloud doc does not mention Xid 140.
			// "Xid messages", https://cloud.google.com/compute/docs/troubleshooting/troubleshooting-gpus#xid_messages (accessed on Nov 3, 2024)
			//
			// NOTE: The Alibaba Cloud doc does not mention Xid 140.
			// "Diagnose GPU-accelerated nodes", https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/use-node-diagnosis-to-self-troubleshoot-gpu-node-problems (accessed on Nov 3, 2024)
			//
			// NOTE: The Tencent Cloud doc does not mention Xid 140.
			// "How to handle common Xid events", https://cloud.tencent.com/document/product/560/106781 (accessed on Nov 3, 2024)
			//
			// NOTE: The Azure HPC GPU node problem detector does not implement Xid 140 detection.
			// "Azure HPC NPD", https://github.com/Azure/azurehpc/blob/master/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L293 (accessed on Nov 3, 2024)
			//
			// NOTE: DeepSeek AI paper does not mention Xid 140.
			// "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning", https://arxiv.org/abs/2408.14158v1 (Aug 2024)
			//
			// Xid 140, marked as critical in GPUd, indicates uncorrectable errors in GPU memory, interrupting the GPU driver's ability to mark the pages for dynamic page offlining or row remapping. Reset the GPU or reboot the system.",
			// Xid 140, marked as critical in GPUd, indicates uncorrectable errors in GPU memory. If the same Xid is reported again after rebooting the system, the GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,

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

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// e.g., "Error status 0x... while polling for FSP boot complete"
			// "GPU_INIT_ERROR in driver", https://github.com/NVIDIA/open-gpu-kernel-modules/blob/main/src/nvidia/src/kernel/gpu/fsp/arch/blackwell/kern_fsp_gb202.c#L84`,
			//
			// Xid 143, marked as critical in GPUd, indicates GPU initialization failure. GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: true,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeFatal,

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
