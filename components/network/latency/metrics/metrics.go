// Package metrics implements the network latency metrics collection and reporting.
package metrics

import (
	"context"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "network_latency"

var (
	edgeInMilliseconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "edge_in_milliseconds",
			Help:      "tracks the edge latency in milliseconds",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is provider region
	).MustCurryWith(prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "network-latency",
	})
	edgeInMillisecondsAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	edgeInMillisecondsAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_edge_in_milliseconds")
}

func ReadEdgeInMilliseconds(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return edgeInMillisecondsAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetEdgeInMilliseconds(ctx context.Context, providerRegion string, latencyInMilliseconds float64, currentTime time.Time) error {
	edgeInMilliseconds.With(prometheus.Labels{pkgmetrics.MetricLabelKey: providerRegion}).Set(latencyInMilliseconds)

	if err := edgeInMillisecondsAverager.Observe(
		ctx,
		latencyInMilliseconds,
		components_metrics.WithMetricSecondaryName(providerRegion),
	); err != nil {
		return err
	}

	return nil
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(edgeInMilliseconds); err != nil {
		return err
	}
	return nil
}
