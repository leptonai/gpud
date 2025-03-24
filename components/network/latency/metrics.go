package latency

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

const SubSystem = "network_latency"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	edgeInMilliseconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "edge_in_milliseconds",
			Help:      "tracks the edge latency in milliseconds",
		},
		[]string{"provider_region"},
	)
)

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	c.edgeInMillisecondsMetricsStore = components_metrics.NewStore(dbRW, dbRO, tableName, SubSystem+"_edge_in_milliseconds")

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(edgeInMilliseconds); err != nil {
		return err
	}

	return nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	edgeLatencies, err := c.readEdgeInMilliseconds(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read running pids: %w", err)
	}

	ms := make([]components.Metric, 0, len(edgeLatencies))
	for _, m := range edgeLatencies {
		ms = append(ms, components.Metric{Metric: m})
	}

	return ms, nil
}

func (c *component) setLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func (c *component) setEdgeInMilliseconds(ctx context.Context, providerRegion string, latencyInMilliseconds float64) error {
	edgeInMilliseconds.WithLabelValues(providerRegion).Set(latencyInMilliseconds)

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if err := c.edgeInMillisecondsMetricsStore.Observe(
		ctx,
		latencyInMilliseconds,
		components_metrics.WithMetricSecondaryName(providerRegion),
	); err != nil {
		return err
	}

	return nil
}

func (c *component) readEdgeInMilliseconds(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return c.edgeInMillisecondsMetricsStore.Read(ctx, components_metrics.WithSince(since))
}
