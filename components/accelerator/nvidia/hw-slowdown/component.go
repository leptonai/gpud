// Package hwslowdown monitors NVIDIA GPU hardware clock events of all GPUs, such as HW Slowdown events.
package hwslowdown

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_hw_slowdown_id "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/id"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_clock_events_state "github.com/leptonai/gpud/components/accelerator/nvidia/query/clock-events-state"
	nvidia_query_metrics_clock "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/clock"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.SetDefaultPoller(cfg.Query.State.DB)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, nvidia_hw_slowdown_id.Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),
		db:      cfg.Query.State.DB,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx  context.Context
	cancel   context.CancelFunc
	poller   query.Poller
	gatherer prometheus.Gatherer
	db       *sql.DB
}

func (c *component) Name() string { return nvidia_hw_slowdown_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", nvidia_hw_slowdown_id.Name)
		return []components.State{
			{
				Name:    StateNameHWSlowdown,
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
				Healthy: false,
				Error:   last.Error.Error(),
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
	if allOutput.SMIExists && len(allOutput.SMIQueryErrors) > 0 {
		cs := make([]components.State, 0)
		for _, e := range allOutput.SMIQueryErrors {
			cs = append(cs, components.State{
				Name:    StateNameHWSlowdown,
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

const (
	EventNameHWSlowdown = "hw_slowdown"
	EventKeyUnixSeconds = "unix_seconds"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	events, err := nvidia_clock_events_state.ReadEvents(ctx, c.db, nvidia_clock_events_state.WithSince(since))
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		log.Logger.Debugw("no event found", "component", c.Name(), "since", humanize.Time(since))
		return nil, nil
	}

	log.Logger.Debugw("found events", "component", c.Name(), "since", humanize.Time(since), "count", len(events))
	convertedEvents := make([]components.Event, 0, len(events))
	for _, event := range events {
		convertedEvents = append(convertedEvents, components.Event{
			Time:    metav1.Time{Time: time.Unix(event.UnixSeconds, 0).UTC()},
			Name:    EventNameHWSlowdown,
			Message: strings.Join(event.Reasons, ", "),
			ExtraInfo: map[string]string{
				EventKeyUnixSeconds: strconv.FormatInt(event.UnixSeconds, 10),
			},
		})
	}
	return convertedEvents, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	hwSlowdown, err := nvidia_query_metrics_clock.ReadHWSlowdown(ctx, since)
	if err != nil {
		return nil, err
	}
	hwSlowdownThermal, err := nvidia_query_metrics_clock.ReadHWSlowdownThermal(ctx, since)
	if err != nil {
		return nil, err
	}
	hwSlowdownPowerBrake, err := nvidia_query_metrics_clock.ReadHWSlowdownPowerBrake(ctx, since)
	if err != nil {
		return nil, err
	}

	ms := make([]components.Metric, 0, len(hwSlowdown)+len(hwSlowdownThermal)+len(hwSlowdownPowerBrake))
	for _, m := range hwSlowdown {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range hwSlowdownThermal {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range hwSlowdownPowerBrake {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}

	return ms, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	_ = c.poller.Stop(nvidia_hw_slowdown_id.Name)

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	c.gatherer = reg
	return nvidia_query_metrics_clock.Register(reg, db, tableName)
}
