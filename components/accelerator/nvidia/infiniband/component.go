package infiniband

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
)

const (
	Name        = "accelerator-nvidia-infiniband"
	Description = "Monitors the infiniband status of the system. Optional, enabled if the host has NVIDIA GPUs."
)

var Tags = []string{"nvidia", "gpu", "infiniband", "ibstat"}

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.DefaultPoller.Start(cctx, cfg.Query, Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.DefaultPoller,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller
}

func (c *component) Name() string { return Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err != nil {
		return nil, err
	}
	if last == nil { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", Name)
		return nil, nil
	}
	if last.Error != nil {
		return []components.State{
			{
				Healthy: false,
				Error:   last.Error,
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Healthy: false,
				Reason:  "no output",
			},
		}, nil
	}

	allOutput, ok := last.Output.(*nvidia_query.Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T", last.Output)
	}
	if !allOutput.IbstatExists {
		return []components.State{
			{
				Name:    Name,
				Healthy: true,
				Reason:  "ibstat does not exist",
			},
		}, nil
	}
	if allOutput.IbstatExists && len(allOutput.Ibstat.Errors) > 0 {
		return []components.State{
			{
				Name:    Name,
				Healthy: false,
				Reason:  "ibstat query found errors " + strings.Join(allOutput.Ibstat.Errors, ", "),
				ExtraInfo: map[string]string{
					nvidia_query.StateKeyIbstatExists: fmt.Sprintf("%v", allOutput.IbstatExists),
				},
			},
		}, nil
	}
	output := ToOutput(allOutput)
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
	_ = c.poller.Stop(Name)

	return nil
}
