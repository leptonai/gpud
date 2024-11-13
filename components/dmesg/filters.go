package dmesg

import (
	"context"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/components/memory"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"

	"k8s.io/utils/ptr"
)

const (
	// e.g.,
	// Out of memory: Killed process 123, UID 48, (httpd).
	EventOOMKill      = "oom_kill"
	EventOOMKillRegex = `Out of memory:`

	// e.g.,
	// oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),
	EventOOMKillConstraint      = "oom_kill_constraint"
	EventOOMKillConstraintRegex = `oom-kill:constraint=`

	// e.g.,
	// postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0
	EventOOMKiller      = "oom_killer"
	EventOOMKillerRegex = `(?i)\b(invoked|triggered) oom-killer\b`

	// e.g.,
	// Memory cgroup out of memory: Killed process 123, UID 48, (httpd).
	EventOOMCgroup      = "oom_cgroup"
	EventOOMCgroupRegex = `Memory cgroup out of memory`
)

func DefaultLogFilters(ctx context.Context) ([]*query_log_common.Filter, error) {
	defaultFilters := []*query_log_common.Filter{
		{
			Name:            EventOOMKill,
			Regex:           ptr.To(EventOOMKillRegex),
			OwnerReferences: []string{memory.Name},
		},
		{
			Name:            EventOOMKillConstraint,
			Regex:           ptr.To(EventOOMKillConstraintRegex),
			OwnerReferences: []string{memory.Name},
		},
		{
			Name:            EventOOMKiller,
			Regex:           ptr.To(EventOOMKillerRegex),
			OwnerReferences: []string{memory.Name},
		},
		{
			Name:            EventOOMCgroup,
			Regex:           ptr.To(EventOOMCgroupRegex),
			OwnerReferences: []string{memory.Name},
		},
	}

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if err != nil {
		return nil, err
	}
	if !nvidiaInstalled {
		for i := range defaultFilters {
			if err := defaultFilters[i].Compile(); err != nil {
				return nil, err
			}
		}
		return defaultFilters, nil
	}

	defaultFilters = append(defaultFilters, DefaultDmesgFiltersForNvidia()...)
	for i := range defaultFilters {
		if err := defaultFilters[i].Compile(); err != nil {
			return nil, err
		}
	}
	return defaultFilters, nil
}
