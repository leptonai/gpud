package dmesg

import (
	nvidia_error "github.com/leptonai/gpud/components/accelerator/nvidia/error"
	nvidia_nccl_id "github.com/leptonai/gpud/components/accelerator/nvidia/nccl/id"
	nvidia_peermem_id "github.com/leptonai/gpud/components/accelerator/nvidia/peermem/id"
	nvidia_query_nccl "github.com/leptonai/gpud/components/accelerator/nvidia/query/nccl"
	nvidia_query_peermem "github.com/leptonai/gpud/components/accelerator/nvidia/query/peermem"
	nvidia_query_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/query/sxid"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"

	"k8s.io/utils/ptr"
)

const (
	// e.g.,
	// [...] NVRM: Xid (0000:03:00): 14, Channel 00000001
	// [...] NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.
	// NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus.
	//
	// ref.
	// https://docs.nvidia.com/deploy/pdf/XID_Errors.pdf
	EventNvidiaNVRMXid = "nvidia_nvrm_xid"

	// e.g.,
	// [111111111.111] nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)
	// [131453.740743] nvidia-nvswitch0: SXid (PCI:0000:00:00.0): 20034, Fatal, Link 30 LTSSM Fault Up
	//
	// ref.
	// "D.4 Non-Fatal NVSwitch SXid Errors"
	// https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf
	EventNvidiaNVSwitchSXid = "nvidia_nvswitch_sxid"

	// repeated messages may indicate more persistent issue on the inter-GPU communication
	// e.g.,
	// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
	// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
	// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
	EventNvidiaPeermemInvalidContext = "nvidia_peermem_invalid_context"

	// repeated messages may indicate GPU communication issues, which may happen due to fabric manager issues
	// e.g.,
	// [Thu Oct 10 03:06:53 2024] pt_main_thread[2536443]: segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2[7f7c7ac00000+d3d3000]
	EventNvidiaNCCLSegfaultInLibnccl = "nvidia_nccl_segfault_in_libnccl"
)

func DefaultDmesgFiltersForNvidia() []*query_log_common.Filter {
	return []*query_log_common.Filter{
		{
			Name:            EventNvidiaNVRMXid,
			Regex:           ptr.To(nvidia_query_xid.RegexNVRMXidDmesg),
			OwnerReferences: []string{nvidia_error.Name},
		},
		{
			Name:            EventNvidiaNVSwitchSXid,
			Regex:           ptr.To(nvidia_query_sxid.RegexNVSwitchSXidDmesg),
			OwnerReferences: []string{nvidia_error.Name},
		},
		{
			Name:            EventNvidiaPeermemInvalidContext,
			Regex:           ptr.To(nvidia_query_peermem.RegexInvalidContext),
			OwnerReferences: []string{nvidia_peermem_id.Name},
		},
		{
			Name:            EventNvidiaNCCLSegfaultInLibnccl,
			Regex:           ptr.To(nvidia_query_nccl.RegexSegfaultInLibnccl),
			OwnerReferences: []string{nvidia_nccl_id.Name},
		},
	}
}
