package cpu

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	"github.com/leptonai/gpud/pkg/log"
	components_metrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "cpu"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	// ref. https://www.digitalocean.com/community/tutorials/load-average-in-linux
	loadAverage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "load_average",
			Help:      "tracks the load average for the last period",
		},
		[]string{"last_period"},
	)

	usedPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the current file descriptor usage percentage",
		},
	)
)

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	c.loadAverage5minMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_load_average_5min")
	c.usedPercentMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_used_percent")

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(loadAverage); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}

	return nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	loadAverage5mins, err := c.readLoadAverage5mins(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read load average 5mins: %w", err)
	}

	usedPercents, err := c.readUsedPercents(ctx, since)
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

func (c *component) setLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func (c *component) setLoadAverage(ctx context.Context, duration time.Duration, avg float64, currentTime time.Time) error {
	loadAverage.WithLabelValues(duration.String()).Set(avg)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	switch duration {
	case 5 * time.Minute:
		if err := c.loadAverage5minMetricsStore.Observe(
			ctx,
			avg,
			components_metrics.WithCurrentTime(currentTime),
		); err != nil {
			return err
		}

	default:
	}

	return nil
}

func (c *component) setUsedPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	usedPercent.Set(pct)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.usedPercentMetricsStore.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) readLoadAverage5mins(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.loadAverage5minMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.usedPercentMetricsStore.Read(ctx, components_metrics.WithSince(since))
}
