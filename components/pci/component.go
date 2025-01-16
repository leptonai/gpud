// Package pci tracks the PCI devices and their Access Control Services (ACS) status.
package pci

import (
	"context"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/pci/id"
	"github.com/leptonai/gpud/components/pci/state"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/poller"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, id.Name)

	return &component{
		cfg:     cfg,
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  getDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	cfg     Config
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  poller.Poller
}

func (c *component) Name() string { return id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return nil, nil
}

const EventNameACSEnabled = "acs_enabled"

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	evs, err := state.ReadEvents(
		ctx,
		c.cfg.Query.State.DBRO,
		state.WithSince(since),
	)
	if err != nil {
		return nil, err
	}
	if len(evs) == 0 {
		return nil, nil
	}

	events := make([]components.Event, 0, len(evs))
	for _, ev := range evs {
		events = append(events, components.Event{
			Name:    EventNameACSEnabled,
			Time:    metav1.Time{Time: time.Unix(ev.UnixSeconds, 0)},
			Type:    components.EventTypeWarning,
			Message: strings.Join(ev.Reasons, ", "),
		})
	}
	return events, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(id.Name)

	return nil
}
