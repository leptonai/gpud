// Package utilization provides the NVIDIA GPU utilization metrics collection and reporting.
package utilization

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_utilization"

// Used for tracking the past x-minute averages.
var defaultPeriods = []time.Duration{5 * time.Minute}

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	gpuUtilPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "gpu_util_percent",
			Help:      "tracks the current GPU utilization/used percent",
		},
		[]string{"gpu_id"},
	)
	gpuUtilPercentAverager = components_metrics.NewNoOpAverager()
	gpuUtilPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "gpu_util_percent_avg",
			Help:      "tracks the GPU utilization percentage with average for the last period",
		},
		[]string{"gpu_id", "last_period"},
	)

	memoryUtilPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "memory_util_percent",
			Help:      "tracks the current GPU memory utilization percent",
		},
		[]string{"gpu_id"},
	)
	memoryUtilPercentAverager = components_metrics.NewNoOpAverager()
	memoryUtilPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "memory_util_percent_avg",
			Help:      "tracks the GPU memory utilization percentage with average for the last period",
		},
		[]string{"gpu_id", "last_period"},
	)
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

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetGPUUtilPercent(ctx context.Context, gpuID string, pct uint32, currentTime time.Time) error {
	gpuUtilPercent.WithLabelValues(gpuID).Set(float64(pct))

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
		gpuUtilPercentAverage.WithLabelValues(gpuID, duration.String()).Set(avg)
	}

	return nil
}

func SetMemoryUtilPercent(ctx context.Context, gpuID string, pct uint32, currentTime time.Time) error {
	memoryUtilPercent.WithLabelValues(gpuID).Set(float64(pct))

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
		memoryUtilPercentAverage.WithLabelValues(gpuID, duration.String()).Set(avg)
	}

	return nil
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
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
