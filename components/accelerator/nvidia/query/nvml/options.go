package nvml

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Op struct {
	gpmMetricsIDs map[nvml.GpmMetricId]struct{}
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	return nil
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
