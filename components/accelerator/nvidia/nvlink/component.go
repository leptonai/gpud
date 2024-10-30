// Package nvlink monitors the NVIDIA per-GPU nvlink devices.
package nvlink

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_metrics_nvlink "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/nvlink"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"github.com/prometheus/client_golang/prometheus"
)

const Name = "accelerator-nvidia-nvlink"

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.DefaultPoller.Start(cctx, cfg.Query, Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.DefaultPoller,
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
	last, err := c.poller.Last()
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
	if allOutput.NVML == nil {
		return []components.State{
			{
				Healthy: false,
				Reason:  "no nvml data",
			},
		}, nil
	}
	output := ToOutput(allOutput)
	return output.States()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	featureEnableds, err := nvidia_query_metrics_nvlink.ReadFeatureEnabled(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read total bytes: %w", err)
	}
	replayErrors, err := nvidia_query_metrics_nvlink.ReadReplayErrors(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read replay errors: %w", err)
	}
	recoveryErrors, err := nvidia_query_metrics_nvlink.ReadRecoveryErrors(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read recovery errors: %w", err)
	}
	crcErrors, err := nvidia_query_metrics_nvlink.ReadCRCErrors(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read crc errors: %w", err)
	}
	rxBytes, err := nvidia_query_metrics_nvlink.ReadRxBytes(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read rx bytes: %w", err)
	}
	txBytes, err := nvidia_query_metrics_nvlink.ReadTxBytes(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read tx bytes: %w", err)
	}

	ms := make([]components.Metric, 0, len(featureEnableds)+len(replayErrors)+len(recoveryErrors)+len(crcErrors)+len(rxBytes)+len(txBytes))
	for _, m := range featureEnableds {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range replayErrors {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range recoveryErrors {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range crcErrors {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range rxBytes {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range txBytes {
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
	_ = c.poller.Stop(Name)

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	c.gatherer = reg
	return nvidia_query_metrics_nvlink.Register(reg, db, tableName)
}
