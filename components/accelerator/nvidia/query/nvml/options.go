package nvml

import (
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Op struct {
	gpmSampleInterval time.Duration
	gpmMetricsIDs     map[nvml.GpmMetricId]struct{}
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.gpmSampleInterval == 0 {
		op.gpmSampleInterval = time.Minute
	}

	return nil
}

func WithGPMSampleInterval(d time.Duration) OpOption {
	return func(op *Op) {
		op.gpmSampleInterval = d
	}
}

func WithGPMMetricsID(id nvml.GpmMetricId) OpOption {
	return func(op *Op) {
		if op.gpmMetricsIDs == nil {
			op.gpmMetricsIDs = make(map[nvml.GpmMetricId]struct{})
		}
		op.gpmMetricsIDs[id] = struct{}{}
	}
}
