// Package xid provides the NVIDIA XID error details.
package xid

import (
	"fmt"
	"sort"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// Defines the Xid error information that is static.
type Detail struct {
	// Code is the error code of the Xid error, as documented in
	// https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html.
	Code int `json:"code"`

	// Description is the description of the Xid error, as documented in
	// https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html.
	Description string `json:"description"`

	// SubCode is populated for NVLink (144-150) XIDs after decoding intrinfo bits 20-25.
	SubCode int `json:"sub_code"`
	// SubCodeDescription describes the NVLink sub-component (e.g., NETIR_LINK_EVT).
	SubCodeDescription string `json:"sub_code_description"`

	// SuggestedActionsByGPUd is the suggested actions by GPUd.
	SuggestedActionsByGPUd *apiv1.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`

	// EventType is the type of the event.
	// The xid component health state is set to "Unhealthy"
	// if this event type is "Critical" or "Fatal".
	EventType apiv1.EventType `json:"event_type"`
}

type catalogEntry struct {
	Code                    int
	Mnemonic                string
	Description             string
	ImmediateResolution     string
	InvestigatoryResolution string
}

type nvlinkRule struct {
	Xid               int
	Unit              string
	IntrinfoPatternV1 string
	IntrinfoPatternV2 string
	ErrorStatus       uint32
	Resolution        string
	Investigatory     string
	Severity          string
	Action2           string
	HwSw              string
	LocalRemote       string
}

var (
	detailsWithSubCodes map[int]map[int]Detail
	nvlinkRulesByXID    = indexNVLinkRules()
)

// Returns the error if found.
// Otherwise, returns false.
func GetDetail(id int) (*Detail, bool) {
	e, ok := details[id]
	return &e, ok
}

// GetDetailWithSubCode returns the XID detail for a given base code and subcode.
// For XIDs 144-150, subcode information is derived from NVIDIA's NVLink catalog.
func GetDetailWithSubCode(id int, subCode int) (*Detail, bool) {
	if subMap, ok := detailsWithSubCodes[id]; ok {
		if detail, ok := subMap[subCode]; ok {
			copy := detail
			return &copy, true
		}
		if detail, ok := subMap[0]; ok {
			copy := detail
			return &copy, true
		}
	}
	return GetDetail(id)
}

// make sure we do not have unknown event type
func init() {
	detailsWithSubCodes = buildNVLinkSubCodeDetails()
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
		Code:        1,
		Description: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	2: {
		Code:        2,
		Description: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	3: {
		Code:        3,
		Description: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	4: {
		Code:        4,
		Description: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	5: {
		Code:        5,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	6: {
		Code:        6,
		Description: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	7: {
		Code:        7,
		Description: "Invalid or corrupted push buffer address",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	8: {
		Code:        8,
		Description: "GPU stopped processing",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	9: {
		Code:        9,
		Description: "Driver error programming GPU",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	10: {
		Code:        10,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	11: {
		Code:        11,
		Description: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	12: {
		Code:        12,
		Description: "Driver error handling GPU exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	13: {
		Code:        13,
		Description: "Graphics Engine Exception",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 13 is a potential hw/driver/user app/system memory corruption/bus/thermal issue/fb corruption.
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

		// Xids whose GPUd.RepairActions is IGNORE_NO_ACTION_REQUIRED without REBOOT_SYSTEM/HARDWARE_INSPECTION
		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,
	},
	14: {
		Code:        14,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	15: {
		Code:        15,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	16: {
		Code:        16,
		Description: "Display engine hung",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	17: {
		Code:        17,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	18: {
		Code:        18,
		Description: "Bus mastering disabled in PCI Config Space",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	19: {
		Code:        19,
		Description: "Display Engine error",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	20: {
		Code:        20,
		Description: "Invalid or corrupted Mpeg push buffer",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	21: {
		Code:        21,
		Description: "Invalid or corrupted Motion Estimation push buffer",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	22: {
		Code:        22,
		Description: "Invalid or corrupted Video Processor push buffer",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	23: {
		Code:        23,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	24: {
		Code:        24,
		Description: "GPU semaphore timeout",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	25: {
		Code:        25,
		Description: "Invalid or illegal push buffer stream",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	26: {
		Code:        26,
		Description: "Framebuffer timeout",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	27: {
		Code:        27,
		Description: "Video processor exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	28: {
		Code:        28,
		Description: "Video processor exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	29: {
		Code:        29,
		Description: "Video processor exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	30: {
		Code:        30,
		Description: "GPU semaphore access error",

		// if nvidia says this can be only because of driver error, then we only reboot
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/xid-errors/index.html (accessed on Nov 3, 2024)
			// "NVIDIA Xid Errors", https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf (Sep 2024)
			// Xid 30 indicates GPU semaphore access error, labeling a driver error as an only possible reason, thus we recommend rebooting the system.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	31: {
		Code:        31,
		Description: "GPU memory page fault",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 31 as a user application issue, but can also be driver bugs or hardware issues.
			// This event is logged when MMU reports a fault when an illegal address access is made by an application unit on the chip.
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

		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,
	},
	32: {
		Code:        32,
		Description: "Invalid or corrupted push buffer stream",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 32 is a DMA controller error which manages the communication between the NVIDIA driver and GPU over the PCI-E bus.
			// Which indicates the PCI quality issues, not the user application issues.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	33: {
		Code:        33,
		Description: "Internal micro-controller error",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	34: {
		Code:        34,
		Description: "Video processor exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	35: {
		Code:        35,
		Description: "Video processor exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	36: {
		Code:        36,
		Description: "Video processor exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	37: {
		Code:        37,
		Description: "Driver firmware error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	38: {
		Code:        38,
		Description: "Driver firmware error",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	39: {
		Code:        39,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	40: {
		Code:        40,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	41: {
		Code:        41,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	42: {
		Code:        42,
		Description: "Video processor exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	43: {
		Code:        43,
		Description: "GPU stopped processing",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 43 as a user application hitting a software induced faults.
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

		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,
	},
	44: {
		Code:        44,
		Description: "Graphics Engine fault during context switch",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	45: {
		Code:        45,
		Description: "Preemptive cleanup, due to previous errors – Most likely to see when running multiple cuda applications and hitting a DBE.",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 45 is returned when the kernel driver terminates a GPU application, as a result of a user of system action.
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

		// Xids whose GPUd.RepairActions is IGNORE_NO_ACTION_REQUIRED without REBOOT_SYSTEM/HARDWARE_INSPECTION
		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,
	},
	46: {
		Code:        46,
		Description: "GPU stopped processing",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	47: {
		Code:        47,
		Description: "Video processor exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	48: {
		Code:        48,
		Description: "Double Bit ECC Error",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 48 indicates uncorrectable double bit errors (DBE), recommending GPU reset or system reboot.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	49: {
		Code:        49,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	50: {
		Code:        50,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	51: {
		Code:        51,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	52: {
		Code:        52,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	53: {
		Code:        53,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	54: {
		Code:        54,
		Description: "Auxiliary power is not connected to the GPU board",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	55: {
		Code:        55,
		Description: "Unused",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	56: {
		Code:        56,
		Description: "Display Engine error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	57: {
		Code:        57,
		Description: "Error programming video memory interface",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	58: {
		Code:        58,
		Description: "Unstable video memory interface detected",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	59: {
		Code:        59,
		Description: "Internal micro-controller error (older drivers)",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	60: {
		Code:        60,
		Description: "Video processor exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	61: {
		Code:        61,
		Description: "Internal micro-controller breakpoint/warning (newer drivers)",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	62: {
		Code:        62,
		Description: "Internal micro-controller halt (newer drivers)",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	63: {
		Code:        63,
		Description: "ECC page retirement or row remapping recording event",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 63 indicates ECC page retirement recording event for legacy GPUs or row-remapping recording event for A100.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	64: {
		Code:        64,
		Description: "ECC page retirement or row remapper recording failure",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 64 indicates ECC page retirement recording failure or row-remapping recording failure.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	65: {
		Code:        65,
		Description: "Video processor exception",

		// "Triggered when the GPU handles memory ECC errors on the GPU"
		// "most instances can be resolved by simply resetting the GPU to retain optimal performance."
		// ref. "Fire-Flyer AI-HPC: A Cost-Effective Software-Hardware Co-Design for Deep Learning" https://arxiv.org/abs/2408.14158
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// Row-remapping happened (Xid 65, see https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html) -- user applications can keep running, but to achieve optimal performance, reset the GPU or reboot the system when convenient.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM but no immediate reboot is required
		EventType: apiv1.EventTypeCritical,
	},
	66: {
		Code:        66,
		Description: "Illegal access by driver",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	67: {
		Code:        67,
		Description: "Illegal access by driver",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	68: {
		Code:        68,
		Description: "NVDEC0 Exception",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,

		// May skip marking the GPU device as unhealthy if the error is the application error
		// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/v0.16.0/internal/rm/health.go#L62-L76
		// TODO: verify whether this is still true https://github.com/NVIDIA/k8s-device-plugin/issues/945
	},
	69: {
		Code:        69,
		Description: "Graphics Engine class error",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	70: {
		Code:        70,
		Description: "CE3: Unknown Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	71: {
		Code:        71,
		Description: "CE4: Unknown Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	72: {
		Code:        72,
		Description: "CE5: Unknown Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	73: {
		Code:        73,
		Description: "NVENC2 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	74: {
		Code:        74,
		Description: "NVLINK Error",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 74 indicates a connection problem between GPUs, or NVSwitch over NVLink.
			// GPU reset or system reboot is needed to clear the error.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	75: {
		Code:        75,
		Description: "CE6: Unknown Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	76: {
		Code:        76,
		Description: "CE7: Unknown Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	77: {
		Code:        77,
		Description: "CE8: Unknown Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	78: {
		Code:        78,
		Description: "vGPU Start Error",

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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	79: {
		Code:        79,
		Description: "GPU has fallen off the bus",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 79 indicates GPUs not being accessible, due to the PCI express connection failures.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	80: {
		Code:        80,
		Description: "Corrupted data sent to GPU",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	81: {
		Code:        81,
		Description: "VGA Subsystem Error",

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

		// Xids whose GPUd.RepairActions is HARDWARE_INSPECTION
		EventType: apiv1.EventTypeFatal,
	},
	82: {
		Code:        82,
		Description: "NVJPG0 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	83: {
		Code:        83,
		Description: "NVDEC1 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	84: {
		Code:        84,
		Description: "NVDEC2 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	85: {
		Code:        85,
		Description: "CE9: Unknown Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	86: {
		Code:        86,
		Description: "OFA Exception",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	87: {
		Code:        87,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	88: {
		Code:        88,
		Description: "NVDEC3 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	89: {
		Code:        89,
		Description: "NVDEC4 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	90: {
		Code:        90,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	91: {
		Code:        91,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	92: {
		Code:        92,
		Description: "High single-bit ECC error rate",

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

		// Xids whose GPUd.RepairActions is IGNORE_NO_ACTION_REQUIRED without REBOOT_SYSTEM/HARDWARE_INSPECTION
		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,
	},
	93: {
		Code:        93,
		Description: "Non-fatal violation of provisioned InfoROM wear limit",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	94: {
		Code:        94,
		Description: "Contained ECC error",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 94 indicates a contained ECC error, successfully suppressed.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM but no immediate reboot is required
		EventType: apiv1.EventTypeCritical,
	},
	95: {
		Code:        95,
		Description: "Uncontained ECC error",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 95 indicates a uncontained ECC error.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		// Xids whose GPUd.RepairActions is HARDWARE_INSPECTION
		EventType: apiv1.EventTypeFatal,
	},
	96: {
		Code:        96,
		Description: "NVDEC5 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	97: {
		Code:        97,
		Description: "NVDEC6 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	98: {
		Code:        98,
		Description: "NVDEC7 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	99: {
		Code:        99,
		Description: "NVJPG1 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	100: {
		Code:        100,
		Description: "NVJPG2 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	101: {
		Code:        101,
		Description: "NVJPG3 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	102: {
		Code:        102,
		Description: "NVJPG4 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	103: {
		Code:        103,
		Description: "NVJPG5 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	104: {
		Code:        104,
		Description: "NVJPG6 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	105: {
		Code:        105,
		Description: "NVJPG7 Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	106: {
		Code:        106,
		Description: "SMBPBI Test Message",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	107: {
		Code:        107,
		Description: "SMBPBI Test Message Silent",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	108: {
		Code:        108,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	109: {
		Code:        109,
		Description: "Context Switch Timeout Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	110: {
		Code:        110,
		Description: "Security Fault Error",

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

		// Xids whose GPUd.RepairActions is HARDWARE_INSPECTION
		EventType: apiv1.EventTypeFatal,
	},
	111: {
		Code:        111,
		Description: "Display Bundle Error Event",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	112: {
		Code:        112,
		Description: "Display Supervisor Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	113: {
		Code:        113,
		Description: "DP Link Training Erro",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	114: {
		Code:        114,
		Description: "Display Pipeline Underflow Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	115: {
		Code:        115,
		Description: "Display Core Channel Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	116: {
		Code:        116,
		Description: "Display Window Channel Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	117: {
		Code:        117,
		Description: "Display Cursor Channel Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	118: {
		Code:        118,
		Description: "Display Pixel Pipeline Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	119: {
		Code:        119,
		Description: "GSP RPC Timeout",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 119 indicates GSP module failures to respond to RPC messages,
			// recommending GPU reset or node power cycle if the issue persists.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	120: {
		Code:        120,
		Description: "GSP Error",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 120 indicates GSP module failures to respond to RPC messages,
			// recommending GPU reset or node power cycle if the issue persists.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	121: {
		Code:        121,
		Description: "C2C Link Error",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 121 indicates corrected errors on the C2C NVLink connection to a Grace CPU, with no operational impact,
			// recommending the GPU reset to retrain the link.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	122: {
		Code:        122,
		Description: "SPI PMU RPC Read Failure",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	123: {
		Code:        123,
		Description: "SPI PMU RPC Write Failure",
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	124: {
		Code:        124,
		Description: "SPI PMU RPC Erase Failure",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	125: {
		Code:        125,
		Description: "Inforom FS Failure",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	126: {
		Code:        126,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	127: {
		Code:        127,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	128: {
		Code:        128,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	129: {
		Code:        129,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	130: {
		Code:        130,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	131: {
		Code:        131,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	132: {
		Code:        132,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	134: {
		Code:        134,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	135: {
		Code:        135,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	136: {
		Code:        136,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	137: {
		Code:        137,
		Description: "NVLink FLA privilege error",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 137 indicates a user application error, recommending cuda-memcheck or CUDA-GDB for debugging.
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

		// Xids whose GPUd.RepairActions is IGNORE_NO_ACTION_REQUIRED without REBOOT_SYSTEM/HARDWARE_INSPECTION
		// Xids whose GPUd.RepairActions is CHECK_USER_APP_AND_GPU without REBOOT_SYSTEM/HARDWARE_INSPECTION
		EventType: apiv1.EventTypeWarning,
	},
	138: {
		Code:        138,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	139: {
		Code:        139,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	140: {
		Code:        140,
		Description: "Unrecovered ECC Error",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// NOTE: The official nvidia doc explains Xid 140 indicates uncorrectable GPU memory errors, which may impact the dynamic page offlining or row remapping,
			// recommending GPU reset if the issue persists.
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

		// Xids whose GPUd.RepairActions is REBOOT_SYSTEM
		EventType: apiv1.EventTypeFatal,
	},
	141: {
		Code:        141,
		Description: "Reserved",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	142: {
		Code:        142,
		Description: "Unrecovered ECC Error",

		SuggestedActionsByGPUd: nil,

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeWarning,
	},
	143: {
		Code:        143,
		Description: "GPU Initialization Failure",

		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			// e.g., "Error status 0x... while polling for FSP boot complete"
			// "GPU_INIT_ERROR in driver", https://github.com/NVIDIA/open-gpu-kernel-modules/blob/main/src/nvidia/src/kernel/gpu/fsp/arch/blackwell/kern_fsp_gb202.c#L84`,
			//
			// Xid 143, marked as critical in GPUd, indicates GPU initialization failure. GPU hardware should be inspected and repaired.
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeHardwareInspection,
			},
		},

		// Xids whose GPUd.RepairActions is empty
		EventType: apiv1.EventTypeFatal,
	},

	144: {
		Code:                   144,
		Description:            "NVLINK: SAW Error",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	145: {
		Code:                   145,
		Description:            "NVLINK: RLW Error",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	146: {
		Code:                   146,
		Description:            "NVLINK: TLW Error",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	147: {
		Code:                   147,
		Description:            "NVLINK: TREX Error",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	148: {
		Code:                   148,
		Description:            "NVLINK: NVLPW_CTRL Error",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	149: {
		Code:                   149,
		Description:            "NVLINK: NETIR Error",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	150: {
		Code:                   150,
		Description:            "NVLINK: MSE Error",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	151: {
		Code:        151,
		Description: "Key rotation Error",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	152: {
		Code:        152,
		Description: "DLA SMMU Error",

		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeInfo, // ignore for now
	},
	153: {
		Code:                   153,
		Description:            "DLA timeout Error",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeInfo, // ignore for now
	},
	154: {
		Code:                   154,
		Description:            "GPU Recovery Action Changed",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	155: {
		Code:        155,
		Description: "NVLINK: SW Defined Error",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	156: {
		Code:        156,
		Description: "Resource Retirement Event",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	157: {
		Code:                   157,
		Description:            "Resource Retirement Failure",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeInfo, // ignore for now
	},
	158: {
		Code:        158,
		Description: "GPU Fatal Timeout",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	159: {
		Code:                   159,
		Description:            "CHI Non-Data Error",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	160: {
		Code:                   160,
		Description:            "Channel Retirement Event",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	161: {
		Code:                   161,
		Description:            "Channel Retirement Failure",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeInfo, // ignore for now
	},
	162: {
		Code:                   162,
		Description:            "Power Smoothing HW Circuitry capability reengaged",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeInfo, // ignore for now
	},
	163: {
		Code:        163,
		Description: "Power Smoothing HW Circuitry capability disengaged",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	164: {
		Code:                   164,
		Description:            "Power Smoothing HW Circuitry low lifetime reached",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeInfo, // ignore for now
	},
	165: {
		Code:                   165,
		Description:            "Power Smoothing HW Circuitry lifetime exhausted",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeInfo, // ignore for now
	},
	166: {
		Code:        166,
		Description: "CC traffic seen prior to link properly being configured for encrypted traffic",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	167: {
		Code:        167,
		Description: "PCIE_FATAL_TIMEOUT",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	168: {
		Code:                   168,
		Description:            "Errors found in WPR (write protected region)",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	169: {
		Code:        169,
		Description: "Internal micro-controller halt",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	170: {
		Code:        170,
		Description: "Interrupt seen in CC mode",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeCheckUserAppAndGPU,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	171: {
		Code:                   171,
		Description:            "Additional to Xid 48 providing more details on particulars of fault to differentiate DRAM/SRAM",
		SuggestedActionsByGPUd: nil,
		EventType:              apiv1.EventTypeWarning,
	},
	172: {
		Code:        172,
		Description: "Additional to Xid 48 providing more details on particulars of fault to differentiate DRAM/SRAM",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
	173: {
		Code:        173,
		Description: "C2C Fatal Link Failure",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
		EventType: apiv1.EventTypeWarning,
	},
}

func detailFromNVLinkInfo(info *XidExtractedInfo) (*Detail, bool) {
	base, ok := GetDetail(info.Xid)
	if !ok {
		return nil, false
	}

	detail := *base
	if subMap, ok := detailsWithSubCodes[info.Xid]; ok {
		if subDetail, ok := subMap[info.SubCode]; ok {
			detail = subDetail
		}
	}

	if rule, ok := lookupNVLinkRule(info); ok {
		severityEvent := eventTypeFromSeverity(rule.Severity)
		if severityEvent == apiv1.EventTypeUnknown {
			severityEvent = eventTypeFromImmediateBucket(rule.Resolution)
		}
		if severityEvent != apiv1.EventTypeUnknown {
			detail.EventType = severityEvent
		}
		if actions := suggestedActionsFromBucket(rule.Resolution); actions != nil {
			detail.SuggestedActionsByGPUd = copySuggestedActions(actions)
		}
	}

	detail.SubCode = info.SubCode
	detail.SubCodeDescription = info.SubCodeName
	detail.EventType = maxEventType(detail.EventType, eventTypeFromLogSeverity(info.Severity))
	if detail.SuggestedActionsByGPUd == nil {
		detail.SuggestedActionsByGPUd = copySuggestedActions(base.SuggestedActionsByGPUd)
	}
	return &detail, true
}

func buildNVLinkSubCodeDetails() map[int]map[int]Detail {
	result := make(map[int]map[int]Detail)
	for _, rule := range nvlinkRules {
		if rule.Xid < 144 || rule.Xid > 150 {
			continue
		}
		subCode, ok := subCodeFromRule(rule)
		if !ok {
			continue
		}
		if _, ok := result[rule.Xid]; !ok {
			result[rule.Xid] = make(map[int]Detail)
		}

		aggregated, exists := result[rule.Xid][subCode]
		if !exists {
			base, ok := GetDetail(rule.Xid)
			if !ok {
				continue
			}
			aggregated = *base
			aggregated.SubCode = subCode
			aggregated.SubCodeDescription = rule.Unit
			aggregated.EventType = apiv1.EventTypeUnknown
		}

		ruleEvent := eventTypeFromSeverity(rule.Severity)
		if ruleEvent == apiv1.EventTypeUnknown {
			ruleEvent = eventTypeFromImmediateBucket(rule.Resolution)
		}
		aggregated.EventType = maxEventType(aggregated.EventType, ruleEvent)
		if actions := suggestedActionsFromBucket(rule.Resolution); actions != nil {
			aggregated.SuggestedActionsByGPUd = mergeSuggestedActions(aggregated.SuggestedActionsByGPUd, actions)
		}

		result[rule.Xid][subCode] = aggregated
	}

	applyOperationalOverrides(result)
	return result
}

func applyOperationalOverrides(result map[int]map[int]Detail) {
	if subMap, ok := result[149]; ok {
		if detail, ok := subMap[4]; ok {
			detail.EventType = apiv1.EventTypeFatal
			detail.SubCodeDescription = "NETIR_LINK_EVT/NETIR_LINK_DOWN (cartridge error)"
			detail.Description = "NVLINK: NETIR Link Event - Possible NVLink cartridge error (contact provider)"
			detail.SuggestedActionsByGPUd = &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}}
			subMap[4] = detail
		}
		if detail, ok := subMap[10]; ok {
			detail.EventType = apiv1.EventTypeFatal
			detail.SubCodeDescription = "NETIR_LINK_EVT/NETIR_LINK_DOWN (PHY timeout)"
			detail.Description = "NVLINK: NETIR Link Event - Physical layer retransmission timeout (contact provider)"
			detail.SuggestedActionsByGPUd = &apiv1.SuggestedActions{RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}}
			subMap[10] = detail
		}
	}
}

func indexNVLinkRules() map[int][]nvlinkRule {
	indexed := make(map[int][]nvlinkRule)
	for _, rule := range nvlinkRules {
		indexed[rule.Xid] = append(indexed[rule.Xid], rule)
	}
	return indexed
}

func lookupNVLinkRule(info *XidExtractedInfo) (*nvlinkRule, bool) {
	rules := nvlinkRulesByXID[info.Xid]
	for i := range rules {
		rule := &rules[i]
		if !unitMatches(rule.Unit, info.SubCodeName) {
			continue
		}
		if rule.ErrorStatus != info.ErrorStatus {
			continue
		}
		if patternMatches(rule.IntrinfoPatternV2, info.Intrinfo) || patternMatches(rule.IntrinfoPatternV1, info.Intrinfo) {
			return rule, true
		}
	}
	return nil, false
}

func subCodeFromRule(rule nvlinkRule) (int, bool) {
	if value, ok := sampleFromPattern(rule.IntrinfoPatternV2); ok {
		return int((value >> 20) & 0x3F), true
	}
	if value, ok := sampleFromPattern(rule.IntrinfoPatternV1); ok {
		return int((value >> 20) & 0x3F), true
	}
	return 0, false
}

func sampleFromPattern(pattern string) (uint32, bool) {
	if pattern == "" {
		return 0, false
	}
	if len(pattern) != 32 {
		return 0, false
	}
	var value uint32
	for idx, r := range pattern {
		bit := 31 - idx
		switch r {
		case '1':
			value |= 1 << bit
		case '0', '-':
		default:
			return 0, false
		}
	}
	return value, true
}

func patternMatches(pattern string, intrinfo uint32) bool {
	if pattern == "" {
		return true
	}
	if len(pattern) != 32 {
		return false
	}
	for idx, r := range pattern {
		bit := uint(31 - idx)
		switch r {
		case '1':
			if ((intrinfo >> bit) & 1) == 0 {
				return false
			}
		case '0':
			if ((intrinfo >> bit) & 1) == 1 {
				return false
			}
		case '-':
			continue
		default:
			return false
		}
	}
	return true
}

func unitMatches(ruleUnit, logUnit string) bool {
	canonicalLog := normalizeUnit(logUnit)
	if canonicalLog == "" {
		return false
	}
	for _, alias := range unitAliases(ruleUnit) {
		if normalizeUnit(alias) == canonicalLog {
			return true
		}
	}
	return false
}

func unitAliases(ruleUnit string) []string {
	aliases := strings.FieldsFunc(ruleUnit, func(r rune) bool {
		switch r {
		case '/', ',', '(', ')', ' ':
			return true
		default:
			return false
		}
	})
	if len(aliases) == 0 {
		aliases = []string{ruleUnit}
	}
	aliases = append(aliases, ruleUnit)
	return aliases
}

func normalizeUnit(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r
		}
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '_' {
			return r
		}
		return -1
	}, s)
}

func eventTypeFromImmediateBucket(bucket string) apiv1.EventType {
	switch bucket {
	case "CONTACT_SUPPORT", "CHECK_MECHANICALS", "WORKFLOW_NVLINK_ERR", "WORKFLOW_NVLINK5_ERR", "XID_154", "XID_154_EVAL", "RESTART_BM":
		return apiv1.EventTypeFatal
	case "RESET_GPU", "RESTART_APP", "RESTART_VM", "CHECK_UVM", "WORKFLOW_XID_48", "WORKFLOW_XID_45", "UPDATE_SWFW":
		return apiv1.EventTypeCritical
	case "IGNORE", "":
		return apiv1.EventTypeInfo
	default:
		return apiv1.EventTypeWarning
	}
}

func eventTypeFromSeverity(severity string) apiv1.EventType {
	switch strings.TrimSpace(strings.ToLower(severity)) {
	case "fatal", "fatal**", "link fatal", "link fatal?":
		return apiv1.EventTypeFatal
	case "non-fatal", "non-fatal*":
		return apiv1.EventTypeWarning
	default:
		return apiv1.EventTypeUnknown
	}
}

func eventTypeFromLogSeverity(severity string) apiv1.EventType {
	switch strings.TrimSpace(strings.ToLower(severity)) {
	case "fatal":
		return apiv1.EventTypeFatal
	case "nonfatal", "non-fatal":
		return apiv1.EventTypeWarning
	default:
		return apiv1.EventTypeUnknown
	}
}

func maxEventType(a, b apiv1.EventType) apiv1.EventType {
	if severityRank(b) > severityRank(a) {
		return b
	}
	return a
}

func severityRank(t apiv1.EventType) int {
	switch t {
	case apiv1.EventTypeInfo:
		return 1
	case apiv1.EventTypeWarning:
		return 2
	case apiv1.EventTypeCritical:
		return 3
	case apiv1.EventTypeFatal:
		return 4
	default:
		return 0
	}
}

func suggestedActionsFromBucket(bucket string) *apiv1.SuggestedActions {
	switch bucket {
	case "CONTACT_SUPPORT", "CHECK_MECHANICALS", "WORKFLOW_NVLINK_ERR", "WORKFLOW_NVLINK5_ERR", "XID_154", "XID_154_EVAL":
		return newSuggestedActions(apiv1.RepairActionTypeHardwareInspection)
	case "RESET_GPU", "RESTART_BM", "RESTART_VM", "CHECK_UVM":
		return newSuggestedActions(apiv1.RepairActionTypeRebootSystem)
	case "RESTART_APP", "WORKFLOW_XID_45", "WORKFLOW_XID_48", "UPDATE_SWFW":
		return newSuggestedActions(apiv1.RepairActionTypeCheckUserAppAndGPU)
	case "IGNORE", "":
		return newSuggestedActions(apiv1.RepairActionTypeIgnoreNoActionRequired)
	default:
		return nil
	}
}

func newSuggestedActions(actions ...apiv1.RepairActionType) *apiv1.SuggestedActions {
	if len(actions) == 0 {
		return nil
	}
	out := make([]apiv1.RepairActionType, len(actions))
	copy(out, actions)
	return &apiv1.SuggestedActions{RepairActions:slices.Clone(actions)}
}

func mergeSuggestedActions(base, addition *apiv1.SuggestedActions) *apiv1.SuggestedActions {
	if base == nil {
		return copySuggestedActions(addition)
	}
	if addition == nil {
		return copySuggestedActions(base)
	}
	set := make(map[apiv1.RepairActionType]struct{})
	for _, act := range base.RepairActions {
		set[act] = struct{}{}
	}
	for _, act := range addition.RepairActions {
		set[act] = struct{}{}
	}
	actions := make([]apiv1.RepairActionType, 0, len(set))
	for act := range set {
		actions = append(actions, act)
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i] < actions[j]
	})
	return &apiv1.SuggestedActions{RepairActions: actions}
}

func copySuggestedActions(src *apiv1.SuggestedActions) *apiv1.SuggestedActions {
	if src == nil {
		return nil
	}
	actions := make([]apiv1.RepairActionType, len(src.RepairActions))
	copy(actions, src.RepairActions)
	return &apiv1.SuggestedActions{RepairActions:slices.Clone(src.RepairActions)}
}
