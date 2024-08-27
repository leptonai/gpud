// Package gpm tracks the NVIDIA GPU GPM metrics.
package gpm

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_query_metrics_gpm "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/gpm"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"github.com/prometheus/client_golang/prometheus"
)

const Name = "accelerator-nvidia-gpm"

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
	rootCtx  context.Context
	cancel   context.CancelFunc
	poller   query.Poller
	gatherer prometheus.Gatherer
}

func (c *component) Name() string { return Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	o := &Output{}

	last, err := c.poller.Last()
	if err != nil {
		return nil, err
	}
	if last == nil || last.Output == nil { // no data
		log.Logger.Debugw("no gpm event data -- this is normal when nvml has not received any registered gpm event events yet")
	} else {
		gpmEvent, ok := last.Output.(*nvidia_query_nvml.GPMEvent)
		if !ok {
			return nil, fmt.Errorf("invalid output type: %T, expected nvidia_query_nvml.GPMEvent", last.Output)
		}
		if gpmEvent != nil && len(gpmEvent.Metrics) > 0 {
			o.NVMLGPMEvent = gpmEvent
		}
	}

	return o.States()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	smOccupancies, err := nvidia_query_metrics_gpm.ReadGPUSMOccupancyPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read total bytes: %w", err)
	}

	ms := make([]components.Metric, 0, len(smOccupancies))
	for _, m := range smOccupancies {
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
	c.poller.Stop(Name)

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	c.gatherer = reg
	return nvidia_query_metrics_gpm.Register(reg, db, tableName)
}
