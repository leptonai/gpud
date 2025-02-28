// Package edge provides a client for the Tailscale DERP (Designated Edge Router Protocol) service.
package edge

import (
	"context"

	"github.com/leptonai/gpud/pkg/netutil/latency"
	"github.com/leptonai/gpud/pkg/netutil/latency/edge/derpmap"
)

type Op struct {
	verbose bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

func WithVerbose(verbose bool) OpOption {
	return func(op *Op) {
		op.verbose = verbose
	}
}

// Measure measures the latencies from local to the global edge nodes.
func Measure(ctx context.Context, opts ...OpOption) (latency.Latencies, error) {
	return measureDERP(ctx, &derpmap.DefaultDERPMap, opts...)
}
