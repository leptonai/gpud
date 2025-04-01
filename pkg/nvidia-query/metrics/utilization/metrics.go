// Package utilization provides the NVIDIA GPU utilization metrics collection and reporting.
package utilization

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_utilization"

// Used for tracking the past x-minute averages.
var defaultPeriods = []time.Duration{5 * time.Minute}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-utilization",
	}

	gpuUtilPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "gpu_util_percent",
			Help:      "tracks the current GPU utilization/used percent",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	gpuUtilPercentAverager = components_metrics.NewNoOpAverager()
	gpuUtilPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "gpu_util_percent_avg",
			Help:      "tracks the GPU utilization percentage with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	memoryUtilPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "memory_util_percent",
			Help:      "tracks the current GPU memory utilization percent",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	memoryUtilPercentAverager = components_metrics.NewNoOpAverager()
	memoryUtilPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "memory_util_percent_avg",
			Help:      "tracks the GPU memory utilization percentage with average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey, "last_period"}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	gpuUtilPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_gpu_util_percent")
	memoryUtilPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_memory_util_percent")
}

func ReadGPUUtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuUtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadMemoryUtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return memoryUtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetGPUUtilPercent(ctx context.Context, gpuID string, pct uint32, currentTime time.Time) error {
	gpuUtilPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(float64(pct))

	if err := gpuUtilPercentAverager.Observe(
		ctx,
		float64(pct),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := gpuUtilPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		gpuUtilPercentAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID, "last_period": duration.String()}).Set(avg)
	}

	return nil
}

func SetMemoryUtilPercent(ctx context.Context, gpuID string, pct uint32, currentTime time.Time) error {
	memoryUtilPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(float64(pct))

	if err := memoryUtilPercentAverager.Observe(
		ctx,
		float64(pct),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := memoryUtilPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		memoryUtilPercentAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID, "last_period": duration.String()}).Set(avg)
	}

	return nil
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(gpuUtilPercent); err != nil {
		return err
	}
	if err := reg.Register(gpuUtilPercentAverage); err != nil {
		return err
	}
	if err := reg.Register(memoryUtilPercent); err != nil {
		return err
	}
	if err := reg.Register(memoryUtilPercentAverage); err != nil {
		return err
	}
	return nil
}
