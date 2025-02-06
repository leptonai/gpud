package dmesg

import (
	"context"

	query_log_common "github.com/leptonai/gpud/internal/query/log/common"
	nvidia_query "github.com/leptonai/gpud/nvidia-query"
)

func DefaultLogFilters(ctx context.Context) ([]*query_log_common.Filter, error) {
	defaultFilters := DefaultDmesgFiltersForCPU()
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
