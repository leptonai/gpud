// Package cpu tracks the combined usage of all CPUs (not per-CPU).
package cpu

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"strconv"
	"time"

	"github.com/leptonai/gpud/components"
	common_dmesg "github.com/leptonai/gpud/components/common/dmesg"
	cpu_id "github.com/leptonai/gpud/components/cpu/id"
	"github.com/leptonai/gpud/components/cpu/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"github.com/prometheus/client_golang/prometheus"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, cpu_id.Name)

	if runtime.GOOS == "linux" {
		if perr := common_dmesg.SetDefaultLogPoller(ctx, cfg.Query.State.DBRW, cfg.Query.State.DBRO); perr != nil {
			log.Logger.Warnw("failed to set default log poller", "error", perr)
		} else {
			common_dmesg.GetDefaultLogPoller().Start(cctx, cfg.Query, common_dmesg.Name)
		}
	}

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

func (c *component) Name() string { return cpu_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", cpu_id.Name)
		return []components.State{
			{
				Name:    cpu_id.Name,
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
				Name:    cpu_id.Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Name:    cpu_id.Name,
				Healthy: true,
				Reason:  "no output",
			},
		}, nil
	}

	output, ok := last.Output.(*Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T", last.Output)
	}
	return output.States()
}

const (
	EventKeyUnixSeconds = "unix_seconds"
	EventKeyLogLine     = "log_line"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}

	logItems, err := common_dmesg.GetDefaultLogPoller().Find(since)
	if err != nil {
		return nil, err
	}

	events := make([]components.Event, 0)
	for _, logItem := range logItems {
		name := ""
		included := false
		for _, owner := range logItem.Matched.OwnerReferences {
			if owner != cpu_id.Name {
				continue
			}
			name = logItem.Matched.Name
			included = true
		}
		if !included {
			continue
		}

		events = append(events, components.Event{
			Time: logItem.Time,
			Name: name,
			Type: components.EventTypeWarning,
			ExtraInfo: map[string]string{
				EventKeyUnixSeconds: strconv.FormatInt(logItem.Time.Unix(), 10),
				EventKeyLogLine:     logItem.Line,
			},
		})
	}

	return events, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	loadAverage5mins, err := metrics.ReadLoadAverage5mins(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read load average 5mins: %w", err)
	}

	usedPercents, err := metrics.ReadUsedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used percents: %w", err)
	}

	ms := make([]components.Metric, 0, len(loadAverage5mins)+len(usedPercents))
	for _, m := range loadAverage5mins {
		ms = append(ms, components.Metric{
			Metric: m,
		})
	}
	for _, m := range usedPercents {
		ms = append(ms, components.Metric{
			Metric: m,
		})
	}

	return ms, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	_ = c.poller.Stop(cpu_id.Name)

	if runtime.GOOS == "linux" {
		_ = common_dmesg.GetDefaultLogPoller().Stop(common_dmesg.Name)
	}

	c.cancel()
	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return metrics.Register(reg, dbRW, dbRO, tableName)
}
