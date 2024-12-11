package dmesg

import (
	memory_dmesg "github.com/leptonai/gpud/components/memory/dmesg"
	memory_id "github.com/leptonai/gpud/components/memory/id"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"

	"k8s.io/utils/ptr"
)

const (
	// e.g.,
	// Out of memory: Killed process 123, UID 48, (httpd).
	//
	// NOTE: this is often followed by a line like:
	// [Sun Dec  8 09:23:39 2024] oom_reaper: reaped process 345646 (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0
	// (to reap the memory used by the OOM victim)
	EventMemoryOOM = "memory_oom"

	// e.g.,
	// oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),
	EventMemoryOOMKillConstraint = "memory_oom_kill_constraint"

	// e.g.,
	// postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0
	EventMemoryOOMKiller = "memory_oom_killer"

	// e.g.,
	// Memory cgroup out of memory: Killed process 123, UID 48, (httpd).
	EventMemoryOOMCgroup = "memory_oom_cgroup"

	// e.g.,
	// [...] EDAC MC0: 1 CE memory read error
	// [...] EDAC MC1: 128 CE memory read error on CPU_SrcID#1_Ha#0_Chan#1_DIMM#1
	//
	// ref.
	// https://serverfault.com/questions/682909/how-to-find-faulty-memory-module-from-mce-message
	// https://github.com/Azure/azurehpc/blob/2d57191cb35ed638525ba9424cc2aa1b5abe1c05/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L51C20-L51C40
	EventMemoryEDACCorrectableErrors = "memory_edac_correctable_errors"
)

func DefaultDmesgFiltersForMemory() []*query_log_common.Filter {
	return []*query_log_common.Filter{
		{
			Name:            EventMemoryOOM,
			Regex:           ptr.To(memory_dmesg.RegexOOM),
			OwnerReferences: []string{memory_id.Name},
		},
		{
			Name:            EventMemoryOOMKillConstraint,
			Regex:           ptr.To(memory_dmesg.RegexOOMKillConstraint),
			OwnerReferences: []string{memory_id.Name},
		},
		{
			Name:            EventMemoryOOMKiller,
			Regex:           ptr.To(memory_dmesg.RegexOOMKiller),
			OwnerReferences: []string{memory_id.Name},
		},
		{
			Name:            EventMemoryOOMCgroup,
			Regex:           ptr.To(memory_dmesg.RegexOOMCgroup),
			OwnerReferences: []string{memory_id.Name},
		},
		{
			Name:            EventMemoryEDACCorrectableErrors,
			Regex:           ptr.To(memory_dmesg.RegexEDACCorrectableErrors),
			OwnerReferences: []string{memory_id.Name},
		},
	}
}
