package dmesg

import (
	"context"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"
)

func DefaultLogFilters(ctx context.Context) ([]*query_log_common.Filter, error) {
	defaultFilters := DefaultDmesgFiltersForMemory()
	defaultFilters = append(defaultFilters, DefaultDmesgFiltersForCPU()...)
	defaultFilters = append(defaultFilters, DefaultDmesgFiltersForFileDescriptor()...)

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if err != nil {
		return nil, err
	}
	if nvidiaInstalled {
		defaultFilters = append(defaultFilters, DefaultDmesgFiltersForNvidia()...)
	}

	for i := range defaultFilters {
		if err := defaultFilters[i].Compile(); err != nil {
			return nil, err
		}
	}
	return defaultFilters, nil
}
