// Package metrics implements the disk metrics collection and reporting.
package metrics

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "disk"

// Used for tracking the past x-minute averages.
var defaultPeriods = []time.Duration{5 * time.Minute}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "disk",
	}

	totalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is mount point
	).MustCurryWith(componentLabel)
	totalBytesAverager = components_metrics.NewNoOpAverager()

	freeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is mount point
	).MustCurryWith(componentLabel)

	usedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is mount point
	).MustCurryWith(componentLabel)
	usedBytesAverager = components_metrics.NewNoOpAverager()
	usedBytesAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_avg",
			Help:      "tracks the disk bytes usage with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is mount point
	).MustCurryWith(componentLabel)

	usedBytesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_percent",
			Help:      "tracks the current disk bytes usage percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is mount point
	).MustCurryWith(componentLabel)
	usedBytesPercentAverager = components_metrics.NewNoOpAverager()
	usedBytesPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_percent_avg",
			Help:      "tracks the disk bytes usage percentage with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is mount point
	).MustCurryWith(componentLabel)

	usedInodesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_inodes_percent",
			Help:      "tracks the current disk inodes usage percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is mount point
	).MustCurryWith(componentLabel)
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	totalBytesAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_total_bytes")
	usedBytesAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_used_bytes")
	usedBytesPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_used_bytes_percent")
}

func ReadTotalBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return totalBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedBytesPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedBytesPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetTotalBytes(ctx context.Context, mountPoint string, bytes float64, currentTime time.Time) error {
	totalBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: mountPoint}).Set(bytes)

	if err := totalBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(mountPoint),
	); err != nil {
		return err
	}

	return nil
}

func SetFreeBytes(mountPoint string, bytes float64) {
	freeBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: mountPoint}).Set(bytes)
}

func SetUsedBytes(ctx context.Context, mountPoint string, bytes float64, currentTime time.Time) error {
	usedBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: mountPoint}).Set(bytes)

	if err := usedBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(mountPoint),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedBytesAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(mountPoint),
		)
		if err != nil {
			return err
		}
		usedBytesAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: mountPoint, "last_period": duration.String()}).Set(avg)
	}

	return nil
}

func SetUsedBytesPercent(ctx context.Context, mountPoint string, pct float64, currentTime time.Time) error {
	usedBytesPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: mountPoint}).Set(pct)

	if err := usedBytesPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(mountPoint),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedBytesPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(mountPoint),
		)
		if err != nil {
			return err
		}
		usedBytesPercentAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: mountPoint, "last_period": duration.String()}).Set(avg)
	}

	return nil
}

func SetUsedInodesPercent(mountPoint string, pct float64) {
	usedInodesPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: mountPoint}).Set(pct)
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(totalBytes); err != nil {
		return err
	}
	if err := reg.Register(freeBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytesAverage); err != nil {
		return err
	}
	if err := reg.Register(usedBytesPercent); err != nil {
		return err
	}
	if err := reg.Register(usedBytesPercentAverage); err != nil {
		return err
	}
	if err := reg.Register(usedInodesPercent); err != nil {
		return err
	}
	return nil
}
