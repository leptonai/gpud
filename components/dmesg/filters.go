package dmesg

import (
	"context"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	poller_log_common "github.com/leptonai/gpud/poller/log/common"
)

func DefaultLogFilters(ctx context.Context) ([]*poller_log_common.Filter, error) {
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
