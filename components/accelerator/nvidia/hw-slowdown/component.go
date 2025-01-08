// Package hwslowdown monitors NVIDIA GPU hardware clock events of all GPUs, such as HW Slowdown events.
package hwslowdown

import (
	"context"
	"database/sql"
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
	nvidia_query.SetDefaultPoller(cfg.Query.State.DBRW, cfg.Query.State.DBRO)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, nvidia_hw_slowdown_id.Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),
		dbRO:    cfg.Query.State.DBRO,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx  context.Context
	cancel   context.CancelFunc
	poller   query.Poller
	gatherer prometheus.Gatherer
	dbRO     *sql.DB
}

func (c *component) Name() string { return nvidia_hw_slowdown_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return nil, nil
}

const (
	EventNameHWSlowdown = "hw_slowdown"
	EventKeyGPUUUID     = "gpu_uuid"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	// the default nvidia poller persists the events to the storage
	// so we can just read from the storage
	events, err := nvidia_clock_events_state.ReadEvents(
		ctx,
		c.dbRO,
		nvidia_clock_events_state.WithSince(since),

		// in order to dedup nvidia-smi events and prioritize nvml events
		nvidia_clock_events_state.WithDedupDataSource(true),
	)
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
			Type:    components.EventTypeInfo,
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
