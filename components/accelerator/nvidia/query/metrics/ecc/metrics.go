// Package ecc provides the NVIDIA ECC metrics collection and reporting.
package ecc

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_ecc"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	aggregateTotalCorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "aggregate_total_corrected",
			Help:      "tracks the current aggregate total corrected",
		},
		[]string{"gpu_id"},
	)
	aggregateTotalCorrectedAverager = components_metrics.NewNoOpAverager()

	aggregateTotalUncorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "aggregate_total_uncorrected",
			Help:      "tracks the current aggregate total uncorrected",
		},
		[]string{"gpu_id"},
	)
	aggregateTotalUncorrectedAverager = components_metrics.NewNoOpAverager()

	volatileTotalCorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "volatile_total_corrected",
			Help:      "tracks the current volatile total corrected",
		},
		[]string{"gpu_id"},
	)
	volatileTotalCorrectedAverager = components_metrics.NewNoOpAverager()

	volatileTotalUncorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "volatile_total_uncorrected",
			Help:      "tracks the current volatile total uncorrected",
		},
		[]string{"gpu_id"},
	)
	volatileTotalUncorrectedAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	aggregateTotalCorrectedAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_aggregate_total_corrected")
	aggregateTotalUncorrectedAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_aggregate_total_uncorrected")
	volatileTotalCorrectedAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_volatile_total_corrected")
	volatileTotalUncorrectedAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_volatile_total_uncorrected")
}

func ReadAggregateTotalCorrected(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return aggregateTotalCorrectedAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadAggregateTotalUncorrected(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return aggregateTotalUncorrectedAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadVolatileTotalCorrected(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return volatileTotalCorrectedAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadVolatileTotalUncorrected(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return volatileTotalUncorrectedAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetAggregateTotalCorrected(ctx context.Context, gpuID string, cnt float64, currentTime time.Time) error {
	aggregateTotalCorrected.WithLabelValues(gpuID).Set(cnt)

	if err := aggregateTotalCorrectedAverager.Observe(
		ctx,
		cnt,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetAggregateTotalUncorrected(ctx context.Context, gpuID string, cnt float64, currentTime time.Time) error {
	aggregateTotalUncorrected.WithLabelValues(gpuID).Set(cnt)

	if err := aggregateTotalUncorrectedAverager.Observe(
		ctx,
		cnt,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetVolatileTotalCorrected(ctx context.Context, gpuID string, cnt float64, currentTime time.Time) error {
	volatileTotalCorrected.WithLabelValues(gpuID).Set(cnt)

	if err := volatileTotalCorrectedAverager.Observe(
		ctx,
		cnt,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetVolatileTotalUncorrected(ctx context.Context, gpuID string, cnt float64, currentTime time.Time) error {
	volatileTotalUncorrected.WithLabelValues(gpuID).Set(cnt)

	if err := volatileTotalUncorrectedAverager.Observe(
		ctx,
		cnt,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(aggregateTotalCorrected); err != nil {
		return err
	}
	if err := reg.Register(aggregateTotalUncorrected); err != nil {
		return err
	}
	if err := reg.Register(volatileTotalCorrected); err != nil {
		return err
	}
	if err := reg.Register(volatileTotalUncorrected); err != nil {
		return err
	}
	return nil
}
