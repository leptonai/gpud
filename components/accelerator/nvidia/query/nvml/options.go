package nvml

import (
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Op struct {
	gpmSampleInteval time.Duration
	gpmMetricsIDs    map[nvml.GpmMetricId]struct{}
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	return nil
}

func WithGPMMetricsID(id nvml.GpmMetricId) OpOption {
	return func(op *Op) {
		if op.gpmMetricsIDs == nil {
			op.gpmMetricsIDs = make(map[nvml.GpmMetricId]struct{})
		}
		op.gpmMetricsIDs[id] = struct{}{}
	}
}
