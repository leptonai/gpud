package dmesg

import (
	nvidia_error "github.com/leptonai/gpud/components/accelerator/nvidia/error"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_peermem "github.com/leptonai/gpud/components/accelerator/nvidia/query/peermem"
	nvidia_query_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/query/sxid"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	query_log_filter "github.com/leptonai/gpud/components/query/log/filter"

	"k8s.io/utils/ptr"
)

func init() {
	if nvidia_query.SMIExists() {
		defaultFilters = append(defaultFilters, DefaultDmesgFiltersForNvidia()...)
	}
	for i := range defaultFilters {
		if err := defaultFilters[i].Compile(); err != nil {
			panic(err)
		}
	}
}

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
)

func DefaultDmesgFiltersForNvidia() []*query_log_filter.Filter {
	return []*query_log_filter.Filter{
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
			Name:  EventNvidiaPeermemInvalidContext,
			Regex: ptr.To(nvidia_query_peermem.RegexNvidiaPeermemInvalidContext),

			// to prevent circular dependency, do not import "nvidia_peermem.Name"
			OwnerReferences: []string{"accelerator-nvidia-peermem"},
		},
	}
}
