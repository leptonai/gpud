// Package power provides the NVIDIA power usage metrics collection and reporting.
package power

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_power"

// Used for tracking the past x-minute averages.
var defaultPeriods = []time.Duration{5 * time.Minute}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-power",
	}

	currentUsageMilliWatts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_usage_milli_watts",
			Help:      "tracks the current power in milliwatts",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	currentUsageMilliWattsAverager = components_metrics.NewNoOpAverager()
	currentUsageMilliWattsAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_usage_milli_watts_avg",
			Help:      "tracks the current power in milliwatts with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	enforcedLimitMilliWatts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "enforced_limit_milli_watts",
			Help:      "tracks the enforced power limit in milliwatts",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	enforcedLimitMilliWattsAverager = components_metrics.NewNoOpAverager()

	usedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the percentage of power used",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	usedPercentAverager = components_metrics.NewNoOpAverager()
	usedPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent_avg",
			Help:      "tracks the used power in percent with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	currentUsageMilliWattsAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_current_usage_milli_watts")
	enforcedLimitMilliWattsAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_enforced_limit_milli_watts")
	usedPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_used_percent")
}

func ReadCurrentUsageMilliWatts(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return currentUsageMilliWattsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadEnforcedLimitMilliWatts(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return enforcedLimitMilliWattsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetUsageMilliWatts(ctx context.Context, gpuID string, milliWatts float64, currentTime time.Time) error {
	currentUsageMilliWatts.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(milliWatts)

	if err := currentUsageMilliWattsAverager.Observe(
		ctx,
		milliWatts,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := currentUsageMilliWattsAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		currentUsageMilliWattsAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID, "last_period": duration.String()}).Set(avg)
	}

	return nil
}

func SetEnforcedLimitMilliWatts(ctx context.Context, gpuID string, milliWatts float64, currentTime time.Time) error {
	enforcedLimitMilliWatts.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(milliWatts)

	if err := enforcedLimitMilliWattsAverager.Observe(
		ctx,
		milliWatts,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
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

	if err := reg.Register(currentUsageMilliWatts); err != nil {
		return err
	}
	if err := reg.Register(currentUsageMilliWattsAverage); err != nil {
		return err
	}
	if err := reg.Register(enforcedLimitMilliWatts); err != nil {
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
