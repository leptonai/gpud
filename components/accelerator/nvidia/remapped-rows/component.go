// Package remappedrows tracks the NVIDIA per-GPU remapped rows.
package remappedrows

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_metrics_remapped_rows "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/remapped-rows"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"github.com/prometheus/client_golang/prometheus"
)

const Name = "accelerator-nvidia-remapped-rows"

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
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),
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

	allOutput, ok := last.Output.(*nvidia_query.Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T", last.Output)
	}
	if lerr := c.poller.LastError(); lerr != nil {
		log.Logger.Warnw("last query failed -- returning cached, possibly stale data", "error", lerr)
	}
	lastSuccessPollElapsed := time.Now().UTC().Sub(allOutput.Time)
	if lastSuccessPollElapsed > 2*c.poller.Config().Interval.Duration {
		log.Logger.Warnw("last poll is too old", "elapsed", lastSuccessPollElapsed, "interval", c.poller.Config().Interval.Duration)
	}

	if allOutput.SMIExists && len(allOutput.SMIQueryErrors) > 0 {
		cs := make([]components.State, 0)
		for _, e := range allOutput.SMIQueryErrors {
			cs = append(cs, components.State{
				Name:    Name,
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

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	remappedDueToUncorrectableErrors, err := nvidia_query_metrics_remapped_rows.ReadRemappedDueToUncorrectableErrors(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read remapped due to uncorrectable errors: %w", err)
	}
	remappingPending, err := nvidia_query_metrics_remapped_rows.ReadRemappingPending(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read remapping pending: %w", err)
	}
	remappingFailed, err := nvidia_query_metrics_remapped_rows.ReadRemappingFailed(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read remapping failed: %w", err)
	}

	ms := make([]components.Metric, 0, len(remappedDueToUncorrectableErrors)+len(remappingPending)+len(remappingFailed))
	for _, m := range remappedDueToUncorrectableErrors {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range remappingPending {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range remappingFailed {
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

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return nvidia_query_metrics_remapped_rows.Register(reg, dbRW, dbRO, tableName)
}
