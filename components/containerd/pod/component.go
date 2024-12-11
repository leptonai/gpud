// Package pod tracks the current pods from the containerd CRI.
package pod

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	containerd_pod_id "github.com/leptonai/gpud/components/containerd/pod/id"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, containerd_pod_id.Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  getDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller
}

func (c *component) Name() string { return containerd_pod_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", containerd_pod_id.Name)
		return []components.State{
			{
				Name:    containerd_pod_id.Name,
				Healthy: true,
				Reason:  query.ErrNoData.Error(),
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if last.Error != nil {
		// this is the error from "ListSandboxStatus"
		//
		// e.g.,
		// rpc error: code = Unimplemented desc = unknown service runtime.v1.RuntimeService
		reason := "failed gRPC call to the containerd socket"
		st, ok := status.FromError(last.Error)
		if ok {
			if st.Code() == codes.Unimplemented {
				reason += "; no CRI configured for containerd"
			}
		}

		return []components.State{
			{
				Name:    containerd_pod_id.Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  reason,
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Name:    containerd_pod_id.Name,
				Healthy: true,
				Reason:  "no output",
			},
		}, nil
	}

	output, ok := last.Output.(*Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T", last.Output)
	}
	return output.States()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(containerd_pod_id.Name)

	return nil
}
