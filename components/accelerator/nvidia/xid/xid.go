// Package xid provides the NVIDIA XID error details.
package xid

import (
	"fmt"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// Defines the Xid error information that is static.
type Detail struct {
	Xid  int    `json:"xid"`
	Name string `json:"name"`

	// SuggestedActionsByGPUd is the suggested actions by GPUd.
	SuggestedActionsByGPUd *apiv1.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`
	// CriticalErrorMarkedByGPUd is true if the GPUd marks this Xid as a critical error.
	// You may use this field to decide whether to alert or not.
	CriticalErrorMarkedByGPUd bool `json:"critical_error_marked_by_gpud"`
	// EventType is the type of the event.
	EventType apiv1.EventType `json:"event_type"`
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
// See https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages
// and https://docs.nvidia.com/deploy/xid-errors/index.html for more details.
var details = map[int]Detail{
	1: {
		Xid:  1,
		Name: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	2: {
		Xid:  2,
		Name: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	3: {
		Xid:  3,
		Name: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	4: {
		Xid:  4,
		Name: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	5: {
		Xid:  5,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	6: {
		Xid:  6,
		Name: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	7: {
		Xid:  7,
		Name: "Invalid or corrupted push buffer address",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	8: {
		Xid:  8,
		Name: "GPU stopped processing",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	9: {
		Xid:  9,
		Name: "Driver error programming GPU",

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
	},
	10: {
		Xid:  10,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	11: {
		Xid:  11,
		Name: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	12: {
		Xid:  12,
		Name: "Driver error handling GPU exception",

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
	},
	13: {
		Xid:  13,
		Name: "Graphics Engine Exception",

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
	},
	14: {
		Xid:  14,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	15: {
		Xid:  15,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	16: {
		Xid:  16,
		Name: "Display engine hung",

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
	},
	17: {
		Xid:  17,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	18: {
		Xid:  18,
		Name: "Bus mastering disabled in PCI Config Space",

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
	},
	19: {
		Xid:  19,
		Name: "Display Engine error",

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
	},
	20: {
		Xid:  20,
		Name: "Invalid or corrupted Mpeg push buffer",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	21: {
		Xid:  21,
		Name: "Invalid or corrupted Motion Estimation push buffer",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	22: {
		Xid:  22,
		Name: "Invalid or corrupted Video Processor push buffer",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	23: {
		Xid:  23,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	24: {
		Xid:  24,
		Name: "GPU semaphore timeout",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	25: {
		Xid:  25,
		Name: "Invalid or illegal push buffer stream",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	26: {
		Xid:  26,
		Name: "Framebuffer timeout",

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
	},
	27: {
		Xid:  27,
		Name: "Video processor exception",

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
	},
	28: {
		Xid:  28,
		Name: "Video processor exception",

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
	},
	29: {
		Xid:  29,
		Name: "Video processor exception",

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
	},
	30: {
		Xid:  30,
		Name: "GPU semaphore access error",

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
	},
	31: {
		Xid:  31,
		Name: "GPU memory page fault",

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
	},
	32: {
		Xid:  32,
		Name: "Invalid or corrupted push buffer stream",

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
	},
	33: {
		Xid:  33,
		Name: "Internal micro-controller error",

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
	},
	34: {
		Xid:  34,
		Name: "Video processor exception",

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
	},
	35: {
		Xid:  35,
		Name: "Video processor exception",

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
	},
	36: {
		Xid:  36,
		Name: "Video processor exception",

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
	},
	37: {
		Xid:  37,
		Name: "Driver firmware error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	38: {
		Xid:  38,
		Name: "Driver firmware error",

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
	},
	39: {
		Xid:  39,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	40: {
		Xid:  40,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	41: {
		Xid:  41,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	42: {
		Xid:  42,
		Name: "Video processor exception",

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
	},
	43: {
		Xid:  43,
		Name: "GPU stopped processing",

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
	},
	44: {
		Xid:  44,
		Name: "Graphics Engine fault during context switch",

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
	},
	45: {
		Xid:  45,
		Name: "Preemptive cleanup, due to previous errors – Most likely to see when running multiple cuda applications and hitting a DBE.",

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
	},
	46: {
		Xid:  46,
		Name: "GPU stopped processing",

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
	},
	47: {
		Xid:  47,
		Name: "Video processor exception",

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
	},
	48: {
		Xid:  48,
		Name: "Double Bit ECC Error",
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
	},
	49: {
		Xid:  49,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	50: {
		Xid:  50,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	51: {
		Xid:  51,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	52: {
		Xid:  52,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	53: {
		Xid:  53,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	54: {
		Xid:  54,
		Name: "Auxiliary power is not connected to the GPU board",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	55: {
		Xid:  55,
		Name: "Unused",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	56: {
		Xid:  56,
		Name: "Display Engine error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	57: {
		Xid:  57,
		Name: "Error programming video memory interface",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	58: {
		Xid:  58,
		Name: "Unstable video memory interface detected",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	59: {
		Xid:  59,
		Name: "Internal micro-controller error (older drivers)",

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
	},
	60: {
		Xid:  60,
		Name: "Video processor exception",

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
	},
	61: {
		Xid:  61,
		Name: "Internal micro-controller breakpoint/warning (newer drivers)",

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
	},
	62: {
		Xid:  62,
		Name: "Internal micro-controller halt (newer drivers)",

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
	},
	63: {
		Xid:  63,
		Name: "ECC page retirement or row remapping recording event",

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
	},
	64: {
		Xid:  64,
		Name: "ECC page retirement or row remapper recording failure",

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
	},
	65: {
		Xid:  65,
		Name: "Video processor exception",

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
	},
	66: {
		Xid:  66,
		Name: "Illegal access by driver",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	67: {
		Xid:  67,
		Name: "Illegal access by driver",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	68: {
		Xid:  68,
		Name: "NVDEC0 Exception",

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

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		// TODO: verify whether this is still true https://github.com/NVIDIA/k8s-device-plugin/issues/945
	},
	69: {
		Xid:  69,
		Name: "Graphics Engine class error",

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
	},
	70: {
		Xid:  70,
		Name: "CE3: Unknown Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	71: {
		Xid:  71,
		Name: "CE4: Unknown Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	72: {
		Xid:  72,
		Name: "CE5: Unknown Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	73: {
		Xid:  73,
		Name: "NVENC2 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	74: {
		Xid:  74,
		Name: "NVLINK Error",

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
	},
	75: {
		Xid:  75,
		Name: "CE6: Unknown Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	76: {
		Xid:  76,
		Name: "CE7: Unknown Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	77: {
		Xid:  77,
		Name: "CE8: Unknown Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	78: {
		Xid:  78,
		Name: "vGPU Start Error",

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
	},
	79: {
		Xid:  79,
		Name: "GPU has fallen off the bus",

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
	},
	80: {
		Xid:  80,
		Name: "Corrupted data sent to GPU",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	81: {
		Xid:  81,
		Name: "VGA Subsystem Error",

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
	},
	82: {
		Xid:  82,
		Name: "NVJPG0 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	83: {
		Xid:  83,
		Name: "NVDEC1 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	84: {
		Xid:  84,
		Name: "NVDEC2 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	85: {
		Xid:  85,
		Name: "CE9: Unknown Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	86: {
		Xid:  86,
		Name: "OFA Exception",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	87: {
		Xid:  87,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	88: {
		Xid:  88,
		Name: "NVDEC3 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	89: {
		Xid:  89,
		Name: "NVDEC4 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	90: {
		Xid:  90,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	91: {
		Xid:  91,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	92: {
		Xid:  92,
		Name: "High single-bit ECC error rate",

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
	},
	93: {
		Xid:  93,
		Name: "Non-fatal violation of provisioned InfoROM wear limit",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	94: {
		Xid:  94,
		Name: "Contained ECC error",

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
	},
	95: {
		Xid:  95,
		Name: "Uncontained ECC error",

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
	},
	96: {
		Xid:  96,
		Name: "NVDEC5 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	97: {
		Xid:  97,
		Name: "NVDEC6 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	98: {
		Xid:  98,
		Name: "NVDEC7 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	99: {
		Xid:  99,
		Name: "NVJPG1 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	100: {
		Xid:  100,
		Name: "NVJPG2 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	101: {
		Xid:  101,
		Name: "NVJPG3 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	102: {
		Xid:  102,
		Name: "NVJPG4 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	103: {
		Xid:  103,
		Name: "NVJPG5 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	104: {
		Xid:  104,
		Name: "NVJPG6 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	105: {
		Xid:  105,
		Name: "NVJPG7 Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	106: {
		Xid:  106,
		Name: "SMBPBI Test Message",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	107: {
		Xid:  107,
		Name: "SMBPBI Test Message Silent",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	108: {
		Xid:  108,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	109: {
		Xid:  109,
		Name: "Context Switch Timeout Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	110: {
		Xid:  110,
		Name: "Security Fault Error",

		// if nvidia says only possible reason is hw, then we do hard inspections directly
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			// Xid 110 indicates a security fault error, labeling a hardware failure as an only possible reason, thus we recommend reboot first and then hardware inspection if the issue persists.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem, // reboot first, and then if it happens again, inspect hardware
				apiv1.RepairActionTypeHardwareInspection,
			},
		},
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is HARDWARE_INSPECTION
		EventType: apiv1.EventTypeFatal,
	},
	111: {
		Xid:  111,
		Name: "Display Bundle Error Event",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	112: {
		Xid:  112,
		Name: "Display Supervisor Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	113: {
		Xid:  113,
		Name: "DP Link Training Erro",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	114: {
		Xid:  114,
		Name: "Display Pipeline Underflow Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	115: {
		Xid:  115,
		Name: "Display Core Channel Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	116: {
		Xid:  116,
		Name: "Display Window Channel Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	117: {
		Xid:  117,
		Name: "Display Cursor Channel Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	118: {
		Xid:  118,
		Name: "Display Pixel Pipeline Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	119: {
		Xid:  119,
		Name: "GSP RPC Timeout",

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
	},
	120: {
		Xid:  120,
		Name: "GSP Error",

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
	},
	121: {
		Xid:  121,
		Name: "C2C Link Error",

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
	},
	122: {
		Xid:  122,
		Name: "SPI PMU RPC Read Failure",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	123: {
		Xid:  123,
		Name: "SPI PMU RPC Write Failure",
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
	},
	124: {
		Xid:  124,
		Name: "SPI PMU RPC Erase Failure",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	125: {
		Xid:  125,
		Name: "Inforom FS Failure",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	126: {
		Xid:  126,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	127: {
		Xid:  127,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	128: {
		Xid:  128,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	129: {
		Xid:  129,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	130: {
		Xid:  130,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	131: {
		Xid:  131,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	132: {
		Xid:  132,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	134: {
		Xid:  134,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	135: {
		Xid:  135,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	136: {
		Xid:  136,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	137: {
		Xid:  137,
		Name: "NVLink FLA privilege error",

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
	},
	138: {
		Xid:  138,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	139: {
		Xid:  139,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	140: {
		Xid:  140,
		Name: "Unrecovered ECC Error",

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
	},
	141: {
		Xid:  141,
		Name: "Reserved",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	142: {
		Xid:  142,
		Name: "Unrecovered ECC Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	143: {
		Xid:  143,
		Name: "GPU Initialization Failure",

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
	},

	144: {
		Xid:                       144,
		Name:                      "NVLINK: SAW Error",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	145: {
		Xid:                       145,
		Name:                      "NVLINK: RLW Error",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	146: {
		Xid:                       146,
		Name:                      "NVLINK: TLW Error",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	147: {
		Xid:                       147,
		Name:                      "NVLINK: TREX Error",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	148: {
		Xid:                       148,
		Name:                      "NVLINK: NVLPW_CTRL Error",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	149: {
		Xid:                       149,
		Name:                      "NVLINK: NETIR Error",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	150: {
		Xid:                       150,
		Name:                      "NVLINK: MSE Error",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	151: {
		Xid:  151,
		Name: "Key rotation Error",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	152: {
		Xid:  152,
		Name: "DLA SMMU Error",

		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeInfo, // ignore for now
	},
	153: {
		Xid:                       153,
		Name:                      "DLA timeout Error",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeInfo, // ignore for now
	},
	154: {
		Xid:                       154,
		Name:                      "GPU Recovery Action Changed",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	155: {
		Xid:  155,
		Name: "NVLINK: SW Defined Error",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	156: {
		Xid:  156,
		Name: "Resource Retirement Event",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	157: {
		Xid:                       157,
		Name:                      "Resource Retirement Failure",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeInfo, // ignore for now
	},
	158: {
		Xid:  158,
		Name: "GPU Fatal Timeout",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	159: {
		Xid:                       159,
		Name:                      "CHI Non-Data Error",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	160: {
		Xid:                       160,
		Name:                      "Channel Retirement Event",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	161: {
		Xid:                       161,
		Name:                      "Channel Retirement Failure",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeInfo, // ignore for now
	},
	162: {
		Xid:                       162,
		Name:                      "Power Smoothing HW Circuitry capability reengaged",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeInfo, // ignore for now
	},
	163: {
		Xid:  163,
		Name: "Power Smoothing HW Circuitry capability disengaged",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	164: {
		Xid:                       164,
		Name:                      "Power Smoothing HW Circuitry low lifetime reached",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeInfo, // ignore for now
	},
	165: {
		Xid:                       165,
		Name:                      "Power Smoothing HW Circuitry lifetime exhausted",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeInfo, // ignore for now
	},
	166: {
		Xid:  166,
		Name: "CC traffic seen prior to link properly being configured for encrypted traffic",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	167: {
		Xid:  167,
		Name: "PCIE_FATAL_TIMEOUT",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	168: {
		Xid:                       168,
		Name:                      "Errors found in WPR (write protected region)",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	169: {
		Xid:  169,
		Name: "Internal micro-controller halt",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	170: {
		Xid:  170,
		Name: "Interrupt seen in CC mode",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	171: {
		Xid:                       171,
		Name:                      "Additional to Xid 48 providing more details on particulars of fault to differentiate DRAM/SRAM",
		SuggestedActionsByGPUd:    nil,
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	172: {
		Xid:  172,
		Name: "Additional to Xid 48 providing more details on particulars of fault to differentiate DRAM/SRAM",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
	173: {
		Xid:  173,
		Name: "C2C Fatal Link Failure",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		CriticalErrorMarkedByGPUd: false,
		EventType:                 apiv1.EventTypeWarning,
	},
}
