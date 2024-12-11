package dmesg

import (
	"context"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	memory_dmesg "github.com/leptonai/gpud/components/memory/dmesg"
	memory_id "github.com/leptonai/gpud/components/memory/id"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"

	"k8s.io/utils/ptr"
)

func DefaultLogFilters(ctx context.Context) ([]*query_log_common.Filter, error) {
	defaultFilters := []*query_log_common.Filter{
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

	defaultFilters = append(defaultFilters, DefaultDmesgFiltersForFileDescriptor()...)
	defaultFilters = append(defaultFilters, DefaultDmesgFiltersForNvidia()...)
	for i := range defaultFilters {
		if err := defaultFilters[i].Compile(); err != nil {
			return nil, err
		}
	}
	return defaultFilters, nil
}
