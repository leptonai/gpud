// Package hwslowdown monitors NVIDIA GPU hardware clock events of all GPUs, such as HW Slowdown events.
package hwslowdown

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	nvidia_hw_slowdown_id "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/id"
	"github.com/leptonai/gpud/pkg/common"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidia_query_metrics_clock "github.com/leptonai/gpud/pkg/nvidia-query/metrics/clock"
	"github.com/leptonai/gpud/pkg/query"
)

const (
	// DefaultStateHWSlowdownEvaluationWindow is the window to evaluate the HW slowdown state.
	DefaultStateHWSlowdownEvaluationWindow = 10 * time.Minute

	// DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute is the threshold frequency of the HW slowdown events per minute.
	// If the evaluation window is 10 minutes and for the last 10-minute, 6 events are found, the state is considered unhealthy, where the ratio is 0.6 = 6 / 10.
	// This is to avoid false positives when the HW slowdown events are rare.
	DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute = 0.6
)

func New(ctx context.Context, cfg nvidia_common.Config, eventsStore events_db.Store) (components.Component, error) {
	if nvidia_query.GetDefaultPoller() == nil {
		return nil, nvidia_query.ErrDefaultPollerNotSet
	}

	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, nvidia_hw_slowdown_id.Name)

	return &component{
		stateHWSlowdownEvaluationWindow:                  DefaultStateHWSlowdownEvaluationWindow,
		stateHWSlowdownEventsThresholdFrequencyPerMinute: DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,

		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),

		eventsStore: eventsStore,
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	stateHWSlowdownEvaluationWindow                  time.Duration
	stateHWSlowdownEventsThresholdFrequencyPerMinute float64

	rootCtx  context.Context
	cancel   context.CancelFunc
	poller   query.Poller
	gatherer prometheus.Gatherer

	eventsStore events_db.Store
}

func (c *component) Name() string { return nvidia_hw_slowdown_id.Name }

func (c *component) Start() error { return nil }

const (
	StateKeyHWSlowdown = "hw_slowdown"
)

func (c *component) States(ctx context.Context) ([]components.State, error) {
	if c.stateHWSlowdownEvaluationWindow == 0 {
		log.Logger.Debugw("no time window to evaluate /states", "component", c.Name())
		return []components.State{
			{
				Name:    StateKeyHWSlowdown,
				Healthy: true,
			},
		}, nil
	}

	since := time.Now().UTC().Add(-c.stateHWSlowdownEvaluationWindow)

	events, err := c.eventsStore.Get(ctx, since)
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		log.Logger.Debugw("no event found for /states", "component", c.Name(), "since", humanize.Time(since))
		return []components.State{
			{
				Name:    StateKeyHWSlowdown,
				Healthy: true,
			},
		}, nil
	}

	eventsByMinute := make(map[int]struct{})
	for _, event := range events {
		min := int(event.Time.Unix() / 60) // unix seconds to minutes
		eventsByMinute[min] = struct{}{}
	}

	totalEvents := len(eventsByMinute)
	minutes := c.stateHWSlowdownEvaluationWindow.Minutes()
	freqPerMin := float64(totalEvents) / minutes

	if freqPerMin < c.stateHWSlowdownEventsThresholdFrequencyPerMinute {
		log.Logger.Debugw("hw slowdown events count is less than threshold", "component", c.Name(), "since", humanize.Time(since), "count", len(eventsByMinute), "threshold", c.stateHWSlowdownEventsThresholdFrequencyPerMinute)
		return []components.State{
			{
				Name:    StateKeyHWSlowdown,
				Healthy: true,
				Reason:  fmt.Sprintf("hw slowdown events frequency per minute %.2f (total events per minute count %d) is less than threshold %.2f for the last %s", freqPerMin, len(eventsByMinute), c.stateHWSlowdownEventsThresholdFrequencyPerMinute, c.stateHWSlowdownEvaluationWindow),
			},
		}, nil
	}

	return []components.State{
		{
			Name:    StateKeyHWSlowdown,
			Healthy: false,
			Reason:  fmt.Sprintf("hw slowdown events frequency per minute %.2f (total events per minute count %d) exceeded threshold %.2f for the last %s", freqPerMin, len(eventsByMinute), c.stateHWSlowdownEventsThresholdFrequencyPerMinute, c.stateHWSlowdownEvaluationWindow),
			SuggestedActions: &common.SuggestedActions{
				RepairActions: []common.RepairActionType{
					common.RepairActionTypeHardwareInspection,
				},
				Descriptions: []string{
					"Hardware slowdown are often caused by GPU overheating or power supply unit (PSU) failing, please do a hardware inspection to mitigate the issue",
				},
			},
		},
	}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventsStore.Get(ctx, since)
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

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return nvidia_query_metrics_clock.Register(reg, dbRW, dbRO, tableName)
}
