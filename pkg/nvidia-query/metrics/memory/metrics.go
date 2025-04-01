// Package memory provides the NVIDIA memory metrics collection and reporting.
package memory

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_memory"

// Used for tracking the past x-minute averages.
var defaultPeriods = []time.Duration{5 * time.Minute}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-memory",
	}

	totalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	totalBytesAverager = components_metrics.NewNoOpAverager()

	reservedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "reserved_bytes",
			Help:      "tracks the reserved memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	usedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the used memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	usedBytesAverager = components_metrics.NewNoOpAverager()
	usedBytesAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_average",
			Help:      "tracks the used memory in bytes with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	freeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the free memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	usedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the percentage of memory used",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	usedPercentAverager = components_metrics.NewNoOpAverager()
	usedPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent_avg",
			Help:      "tracks the percentage of memory used with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	totalBytesAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_total_bytes")
	usedBytesAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_used_bytes")
	usedPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_used_percent")
}

func ReadTotalBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return totalBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetTotalBytes(ctx context.Context, gpuID string, bytes float64, currentTime time.Time) error {
	totalBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(bytes)

	if err := totalBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetReservedBytes(gpuID string, bytes float64) {
	reservedBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(bytes)
}

func SetUsedBytes(ctx context.Context, gpuID string, bytes float64, currentTime time.Time) error {
	usedBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(bytes)

	if err := usedBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedBytesAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		usedBytesAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID, "last_period": duration.String()}).Set(avg)
	}

	return nil
}

func SetFreeBytes(gpuID string, bytes float64) {
	freeBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(bytes)
}

func SetUsedPercent(ctx context.Context, gpuID string, pct float64, currentTime time.Time) error {
	usedPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(pct)

	if err := usedPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		usedPercentAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID, "last_period": duration.String()}).Set(avg)
	}

	return nil
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(totalBytes); err != nil {
		return err
	}
	if err := reg.Register(reservedBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytesAverage); err != nil {
		return err
	}
	if err := reg.Register(freeBytes); err != nil {
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
