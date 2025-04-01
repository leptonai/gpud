// Package metrics implements the FUSE connections metrics collection and reporting.
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

const SubSystem = "fuse"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "fuse",
	}

	connsCongestedPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_congested_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is device name
	).MustCurryWith(componentLabel)
	connsCongestedPctAverager = components_metrics.NewNoOpAverager()

	connsMaxBackgroundPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_max_background_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is device name
	).MustCurryWith(componentLabel)
	connsMaxBackgroundPctAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	connsCongestedPctAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_connections_congested_percent_against_threshold")
	connsMaxBackgroundPctAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_connections_max_background_percent_against_threshold")
}

func ReadConnectionsCongestedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return connsCongestedPctAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadConnectionsMaxBackgroundPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return connsMaxBackgroundPctAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetConnectionsCongestedPercent(ctx context.Context, deviceName string, pct float64, currentTime time.Time) error {
	connsCongestedPct.With(prometheus.Labels{pkgmetrics.MetricLabelKey: deviceName}).Set(pct)

	if err := connsCongestedPctAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(deviceName),
	); err != nil {
		return err
	}

	return nil
}

func SetConnectionsMaxBackgroundPercent(ctx context.Context, deviceName string, pct float64, currentTime time.Time) error {
	connsMaxBackgroundPct.With(prometheus.Labels{pkgmetrics.MetricLabelKey: deviceName}).Set(pct)

	if err := connsMaxBackgroundPctAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(deviceName),
	); err != nil {
		return err
	}

	return nil
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(connsCongestedPct); err != nil {
		return err
	}
	if err := reg.Register(connsMaxBackgroundPct); err != nil {
		return err
	}
	return nil
}
