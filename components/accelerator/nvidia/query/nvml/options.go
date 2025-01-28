package nvml

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	events_db "github.com/leptonai/gpud/components/db"
)

type Op struct {
	xidEventsStore        events_db.Store
	hwslowdownEventsStore events_db.Store
	gpmMetricsIDs         map[nvml.GpmMetricId]struct{}
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	return nil
}

func WithXidEventsStore(store events_db.Store) OpOption {
	return func(op *Op) {
		op.xidEventsStore = store
	}
}

func WithHWSlowdownEventsStore(store events_db.Store) OpOption {
	return func(op *Op) {
		op.hwslowdownEventsStore = store
	}
}

func WithGPMMetricsID(ids ...nvml.GpmMetricId) OpOption {
	return func(op *Op) {
		if op.gpmMetricsIDs == nil {
			op.gpmMetricsIDs = make(map[nvml.GpmMetricId]struct{})
		}
		for _, id := range ids {
			op.gpmMetricsIDs[id] = struct{}{}
		}
	}
}
