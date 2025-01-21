// Package hwslowdown monitors NVIDIA GPU hardware clock events of all GPUs, such as HW Slowdown events.
package hwslowdown

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	nvidia_hw_slowdown_id "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/id"
	nvidia_hw_slowdown_state "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/state"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_metrics_clock "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/clock"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultStateHWSlowdownEvaluationWindow is the window to evaluate the HW slowdown state.
	DefaultStateHWSlowdownEvaluationWindow = 10 * time.Minute

	// DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute is the threshold frequency of the HW slowdown events per minute.
	// If the evaluation window is 10 minutes and for the last 10-minute, 6 events are found, the state is considered unhealthy, where the ratio is 0.6 = 6 / 10.
	// This is to avoid false positives when the HW slowdown events are rare.
	DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute = 0.6
)

func New(ctx context.Context, cfg nvidia_common.Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.SetDefaultPoller(
		nvidia_query.WithDBRW(cfg.Query.State.DBRW),
		nvidia_query.WithDBRO(cfg.Query.State.DBRO),
		nvidia_query.WithNvidiaSMICommand(cfg.NvidiaSMICommand),
		nvidia_query.WithNvidiaSMIQueryCommand(cfg.NvidiaSMIQueryCommand),
		nvidia_query.WithIbstatCommand(cfg.IbstatCommand),
		nvidia_query.WithInfinibandClassDirectory(cfg.InfinibandClassDirectory),
	)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, nvidia_hw_slowdown_id.Name)

	return &component{
		stateHWSlowdownEvaluationWindow:                  DefaultStateHWSlowdownEvaluationWindow,
		stateHWSlowdownEventsThresholdFrequencyPerMinute: DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,

		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),

		readEvents: func(ctx context.Context, since time.Time) ([]nvidia_hw_slowdown_state.Event, error) {
			// the default nvidia poller persists the events to the storage
			// so we can just read from the storage
			return nvidia_hw_slowdown_state.ReadEvents(
				ctx,
				cfg.Query.State.DBRO,

				nvidia_hw_slowdown_state.WithSince(since),

				// in order to dedup nvidia-smi events and prioritize nvml events
				// otherwise, we have deduplicate objects from nvml and nvidia-smi
				// deprecate this once we removed nvidia-smi dependency
				nvidia_hw_slowdown_state.WithDedupDataSource(true),
			)
		},
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	stateHWSlowdownEvaluationWindow                  time.Duration
	stateHWSlowdownEventsThresholdFrequencyPerMinute float64

	rootCtx  context.Context
	cancel   context.CancelFunc
	poller   query.Poller
	gatherer prometheus.Gatherer

	readEvents func(ctx context.Context, since time.Time) ([]nvidia_hw_slowdown_state.Event, error)
}

func (c *component) Name() string { return nvidia_hw_slowdown_id.Name }

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

	events, err := c.readEvents(ctx, since)
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
		min := int(event.Timestamp / 60) // unix seconds to minutes
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
		},
	}, nil
}

const (
	EventNameHWSlowdown = "hw_slowdown"
	EventKeyGPUUUID     = "gpu_uuid"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	events, err := c.readEvents(ctx, since)
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		log.Logger.Debugw("no event found for /events", "component", c.Name(), "since", humanize.Time(since))
		return nil, nil
	}

	log.Logger.Debugw("found events", "component", c.Name(), "since", humanize.Time(since), "count", len(events))
	convertedEvents := make([]components.Event, 0, len(events))
	for _, event := range events {
		convertedEvents = append(convertedEvents, components.Event{
			Time:    metav1.Time{Time: time.Unix(event.Timestamp, 0).UTC()},
			Name:    EventNameHWSlowdown,
			Type:    components.EventTypeWarning,
			Message: strings.Join(event.Reasons, ", "),
			ExtraInfo: map[string]string{
				EventKeyGPUUUID: event.GPUUUID,
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

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return nvidia_query_metrics_clock.Register(reg, dbRW, dbRO, tableName)
}
