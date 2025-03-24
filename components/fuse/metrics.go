package fuse

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

const SubSystem = "fuse"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	connsCongestedPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_congested_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{"device_name"},
	)

	connsMaxBackgroundPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_max_background_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{"device_name"},
	)
)

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	c.connsCongestedPctMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_connections_congested_percent_against_threshold")
	c.connsMaxBackgroundPctMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_connections_max_background_percent_against_threshold")

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(connsCongestedPct); err != nil {
		return err
	}
	if err := reg.Register(connsMaxBackgroundPct); err != nil {
		return err
	}

	return nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	congestedPercents, err := c.readConnectionsCongestedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read congested percents: %w", err)
	}
	maxBackgroundPercents, err := c.readConnectionsMaxBackgroundPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read max background percents: %w", err)
	}
	ms := make([]components.Metric, 0, len(congestedPercents)+len(maxBackgroundPercents))
	for _, m := range congestedPercents {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range maxBackgroundPercents {
		ms = append(ms, components.Metric{Metric: m})
	}

	return ms, nil
}

func (c *component) setLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func (c *component) setConnectionsCongestedPercent(ctx context.Context, deviceName string, pct float64, currentTime time.Time) error {
	connsCongestedPct.WithLabelValues(deviceName).Set(pct)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.connsCongestedPctMetricsStore.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(deviceName),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) setConnectionsMaxBackgroundPercent(ctx context.Context, deviceName string, pct float64, currentTime time.Time) error {
	connsMaxBackgroundPct.WithLabelValues(deviceName).Set(pct)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.connsMaxBackgroundPctMetricsStore.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(deviceName),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) readConnectionsCongestedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.connsCongestedPctMetricsStore.Read(ctx, components_metrics.WithSince(since))
}

func (c *component) readConnectionsMaxBackgroundPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.connsMaxBackgroundPctMetricsStore.Read(ctx, components_metrics.WithSince(since))
}
