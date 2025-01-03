// Package os queries the host OS information (e.g., kernel version).
package os

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	os_id "github.com/leptonai/gpud/components/os/id"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/log"

	"github.com/dustin/go-humanize"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, os_id.Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  getDefaultPoller(),
		cfg:     cfg,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller
	cfg     Config
}

func (c *component) Name() string { return os_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", os_id.Name)
		return []components.State{
			{
				Name:    os_id.Name,
				Healthy: true,
				Reason:  query.ErrNoData.Error(),
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if last.Error != nil {
		return []components.State{
			{
				Name:    os_id.Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Name:    os_id.Name,
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
	rebootEvents, err := state.GetRebootEvents(ctx, c.cfg.Query.State.DBRW, since)
	if err != nil {
		return nil, err
	}
	if len(rebootEvents) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	evs := make([]components.Event, 0, len(rebootEvents))
	for _, event := range rebootEvents {
		rebootedAt := time.Unix(event.UnixSeconds, 0)
		rebootedAtHumanized := humanize.RelTime(rebootedAt, now, "ago", "from now")

		evs = append(evs, components.Event{
			Time:    metav1.Time{Time: rebootedAt},
			Name:    "reboot",
			Type:    components.EventTypeWarning,
			Message: fmt.Sprintf("system reboot detected (%s)", rebootedAtHumanized),
		})
	}
	return evs, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(os_id.Name)

	return nil
}

var _ components.OutputProvider = (*component)(nil)

func (c *component) Output() (any, error) {
	last, err := c.poller.Last()
	if err != nil {
		return nil, err
	}
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", os_id.Name)
		return nil, query.ErrNoData
	}
	if last.Error != nil {
		return nil, last.Error
	}
	if last.Output == nil {
		return nil, nil
	}

	output, ok := last.Output.(*Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T, expected *os.Output", last.Output)
	}
	return output, nil
}
