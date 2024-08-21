// Package file tracks the file status.
package file

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/fd/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
)

const Name = "file"

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, Name)

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
				Name:    Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Name:    Name,
				Healthy: false,
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

	runningPIDs, err := metrics.ReadRunningPIDs(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read running pids: %w", err)
	}
	limits, err := metrics.ReadLimits(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read limits: %w", err)
	}
	usedPercents, err := metrics.ReadUsedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used percents: %w", err)
	}

	ms := make([]components.Metric, 0, len(runningPIDs)+len(limits)+len(usedPercents))
	for _, m := range runningPIDs {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range limits {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range usedPercents {
		ms = append(ms, components.Metric{Metric: m})
	}

	return ms, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(Name)

	return nil
}
