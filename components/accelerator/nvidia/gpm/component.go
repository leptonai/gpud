// Package gpm tracks the NVIDIA per-GPU GPM metrics.
package gpm

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"
	"github.com/leptonai/gpud/internal/query"
	"github.com/leptonai/gpud/log"
	nvidia_query_metrics_gpm "github.com/leptonai/gpud/nvidia-query/metrics/gpm"
	nvidia_query_nvml "github.com/leptonai/gpud/nvidia-query/nvml"

	"github.com/prometheus/client_golang/prometheus"
)

const Name = "accelerator-nvidia-gpm"

func New(ctx context.Context, cfg nvidia_common.Config) components.Component {
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

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.LastSuccess()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", Name)
		return []components.State{
			{
				Name:    Name,
				Healthy: true,
				Reason:  query.ErrNoData.Error(),
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	gpmEvent, ok := last.Output.(*nvidia_query_nvml.GPMEvent)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T, expected nvidia_query_nvml.GPMEvent", last.Output)
	}

	o := &Output{}
	if gpmEvent != nil && len(gpmEvent.Metrics) > 0 {
		if lerr := c.poller.LastError(); lerr != nil {
			log.Logger.Warnw("last query failed -- returning cached, possibly stale data", "error", lerr)
		}
		lastSuccessPollElapsed := time.Now().UTC().Sub(gpmEvent.Time.Time)
		if lastSuccessPollElapsed > 2*c.poller.Config().Interval.Duration {
			log.Logger.Warnw("last poll is too old", "elapsed", lastSuccessPollElapsed, "interval", c.poller.Config().Interval.Duration)
		}

		o.NVMLGPMEvent = gpmEvent
	}
	return o.States()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func updateMetrics(ms []components.Metric, metrics components_metrics_state.Metrics) []components.Metric {
	for _, m := range metrics {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	return ms
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	smOccupancies, err := nvidia_query_metrics_gpm.ReadGPUSMOccupancyPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read total bytes: %w", err)
	}

	ms := make([]components.Metric, 0, len(smOccupancies))
	ms = updateMetrics(ms, smOccupancies)
	intUtils, err := nvidia_query_metrics_gpm.ReadGPUIntUtilPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("fail to read sm int util: %w", err)
	}
	ms = updateMetrics(ms, intUtils)

	anyTensorUtils, err := nvidia_query_metrics_gpm.ReadGPUAnyTensorUtilPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("fail to read sm any tensor util: %w", err)
	}
	ms = updateMetrics(ms, anyTensorUtils)

	dfmaTensorUtils, err := nvidia_query_metrics_gpm.ReadGPUDFMATensorUtilPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("fail to read sm dfma tensor util: %w", err)
	}
	ms = updateMetrics(ms, dfmaTensorUtils)

	hmmaTensorUtils, err := nvidia_query_metrics_gpm.ReadGPUHMMATensorUtilPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("fail to read sm dfma tensor util: %w", err)
	}
	ms = updateMetrics(ms, hmmaTensorUtils)

	immaTensorUtils, err := nvidia_query_metrics_gpm.ReadGPUIMMATensorUtilPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("fail to read sm dfma tensor util: %w", err)
	}
	ms = updateMetrics(ms, immaTensorUtils)

	fp64Utils, err := nvidia_query_metrics_gpm.ReadGPUFp64UtilPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("fail to read sm fp64 util: %w", err)
	}
	ms = updateMetrics(ms, fp64Utils)

	fp32Utils, err := nvidia_query_metrics_gpm.ReadGPUFp32UtilPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("fail to read sm fp64 util: %w", err)
	}
	ms = updateMetrics(ms, fp32Utils)

	fp16Utils, err := nvidia_query_metrics_gpm.ReadGPUFp16UtilPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("fail to read sm fp64 util: %w", err)
	}
	ms = updateMetrics(ms, fp16Utils)

	return ms, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(Name)

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return nvidia_query_metrics_gpm.Register(reg, dbRW, dbRO, tableName)
}
