// Package clockspeed tracks the NVIDIA per-GPU clock speed.
package clockspeed

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_clock_speed_id "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed/id"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_metrics_clockspeed "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/clock-speed"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/poller"

	"github.com/prometheus/client_golang/prometheus"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.PollerConfig.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.SetDefaultPoller(cfg.PollerConfig.State.DBRW, cfg.PollerConfig.State.DBRO)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.PollerConfig, nvidia_clock_speed_id.Name)

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
	poller   poller.Poller
	gatherer prometheus.Gatherer
}

func (c *component) Name() string { return nvidia_clock_speed_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.LastSuccess()
	if err == poller.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", nvidia_clock_speed_id.Name)
		return []components.State{
			{
				Name:    nvidia_clock_speed_id.Name,
				Healthy: true,
				Reason:  poller.ErrNoData.Error(),
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
				Name:    nvidia_clock_speed_id.Name,
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

	graphicsMHzs, err := nvidia_query_metrics_clockspeed.ReadGraphicsMHzs(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read graphics clock speed: %w", err)
	}
	memoryMHzs, err := nvidia_query_metrics_clockspeed.ReadMemoryMHzs(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read memory clock speed: %w", err)
	}

	ms := make([]components.Metric, 0, len(graphicsMHzs)+len(memoryMHzs))
	for _, m := range graphicsMHzs {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range memoryMHzs {
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
	_ = c.poller.Stop(nvidia_clock_speed_id.Name)

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return nvidia_query_metrics_clockspeed.Register(reg, dbRW, dbRO, tableName)
}
