// Package error implements NVIDIA GPU driver error detector.
package error

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/poller"
)

const Name = "accelerator-nvidia-error"

func New(ctx context.Context, cfg Config) components.Component {
	cfg.PollerConfig.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.SetDefaultPoller(cfg.PollerConfig.State.DBRW, cfg.PollerConfig.State.DBRO)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.PollerConfig, Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  poller.Poller
}

func (c *component) Name() string { return Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.LastSuccess()
	if err == poller.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", Name)
		return []components.State{
			{
				Name:    Name,
				Healthy: true,
				Reason:  poller.ErrNoData.Error(),
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	allOutput, ok := last.Output.(*nvidia_query.Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T", last.Output)
	}
	if lerr := c.poller.LastError(); lerr != nil {
		log.Logger.Warnw("last query failed -- returning cached, possibly stale data", "error", lerr)
	}
	lastSuccessPollElapsed := time.Now().UTC().Sub(allOutput.Time)
	if lastSuccessPollElapsed > 2*c.poller.Config().Interval.Duration {
		log.Logger.Warnw("last poll is too old", "elapsed", lastSuccessPollElapsed, "interval", c.poller.Config().Interval.Duration)
	}

	if allOutput.SMIExists && len(allOutput.SMIQueryErrors) > 0 {
		cs := make([]components.State, 0)
		for _, e := range allOutput.SMIQueryErrors {
			cs = append(cs, components.State{
				Name:    Name,
				Healthy: false,
				Error:   e,
				Reason:  "nvidia-smi query failed with " + e,
				ExtraInfo: map[string]string{
					nvidia_query.StateKeySMIExists: fmt.Sprintf("%v", allOutput.SMIExists),
				},
			})
		}
		return cs, nil
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
