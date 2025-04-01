// Package metrics implements the CPU metrics collection and reporting.
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

const SubSystem = "cpu"

// Used for tracking the past x-minute averages.
var defaultPeriods = []time.Duration{5 * time.Minute}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "cpu",
	}

	// ref. https://www.digitalocean.com/community/tutorials/load-average-in-linux
	loadAverage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "load_average",
			Help:      "tracks the load average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is last period
	).MustCurryWith(componentLabel)
	loadAverage5minAverager = components_metrics.NewNoOpAverager()

	usedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the current file descriptor usage percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
	usedPercentAverager = components_metrics.NewNoOpAverager()
	usedPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent_avg",
			Help:      "tracks the file descriptor usage percentage with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is last period
	).MustCurryWith(componentLabel)
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	loadAverage5minAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_load_average_5min")
	usedPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_used_percent")
}

func ReadLoadAverage5mins(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return loadAverage5minAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLoadAverage(ctx context.Context, duration time.Duration, avg float64, currentTime time.Time) error {
	loadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: duration.String()}).Set(avg)

	switch duration {
	case 5 * time.Minute:
		if err := loadAverage5minAverager.Observe(
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

func SetUsedPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	usedPercent.With(prometheus.Labels{}).Set(pct)

	if err := usedPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
		)
		if err != nil {
			return err
		}
		usedPercentAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: duration.String()}).Set(avg)
	}

	return nil
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(loadAverage); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}
	if err := reg.Register(usedPercentAverage); err != nil {
		return err
	}
	return nil
}
