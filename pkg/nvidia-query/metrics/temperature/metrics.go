// Package temperature provides the NVIDIA temperature metrics collection and reporting.
package temperature

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_temperature"

// Used for tracking the past x-minute averages.
var defaultPeriods = []time.Duration{5 * time.Minute}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-temperature",
	}

	currentCelsius = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_celsius",
			Help:      "tracks the current temperature in celsius",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	currentCelsiusAverager = components_metrics.NewNoOpAverager()
	currentCelsiusAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_celsius_avg",
			Help:      "tracks the current temperature in celsius with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	thresholdSlowdownCelsius = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "slowdown_threshold_celsius",
			Help:      "tracks the threshold temperature in celsius for slowdown",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	thresholdSlowdownCelsiusAverager = components_metrics.NewNoOpAverager()

	slowdownUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "slowdown_used_percent",
			Help:      "tracks the percentage of slowdown used",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	slowdownUsedPercentAverager = components_metrics.NewNoOpAverager()
	slowdownUsedPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "slowdown_used_percent_avg",
			Help:      "tracks the percentage of slowdown used with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	currentCelsiusAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_current_celsius")
	thresholdSlowdownCelsiusAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_slowdown_threshold_celsius")
	slowdownUsedPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_slowdown_used_percent")
}

func ReadCurrentCelsius(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return currentCelsiusAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadThresholdSlowdownCelsius(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return thresholdSlowdownCelsiusAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadSlowdownUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return slowdownUsedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetCurrentCelsius(ctx context.Context, gpuID string, temp float64, currentTime time.Time) error {
	currentCelsius.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(temp)

	if err := currentCelsiusAverager.Observe(
		ctx,
		temp,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := currentCelsiusAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		currentCelsiusAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID, "last_period": duration.String()}).Set(avg)
	}

	return nil
}

func SetThresholdSlowdownCelsius(ctx context.Context, gpuID string, temp float64, currentTime time.Time) error {
	thresholdSlowdownCelsius.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(temp)

	if err := thresholdSlowdownCelsiusAverager.Observe(
		ctx,
		temp,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetSlowdownUsedPercent(ctx context.Context, gpuID string, pct float64, currentTime time.Time) error {
	slowdownUsedPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(pct)

	if err := slowdownUsedPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := slowdownUsedPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		slowdownUsedPercentAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID, "last_period": duration.String()}).Set(avg)
	}

	return nil
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(currentCelsius); err != nil {
		return err
	}
	if err := reg.Register(currentCelsiusAverage); err != nil {
		return err
	}
	if err := reg.Register(thresholdSlowdownCelsius); err != nil {
		return err
	}
	if err := reg.Register(slowdownUsedPercent); err != nil {
		return err
	}
	if err := reg.Register(slowdownUsedPercentAverage); err != nil {
		return err
	}
	return nil
}
