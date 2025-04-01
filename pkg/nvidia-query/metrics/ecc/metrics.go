// Package ecc provides the NVIDIA ECC metrics collection and reporting.
package ecc

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_ecc"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-ecc",
	}

	aggregateTotalCorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "aggregate_total_corrected",
			Help:      "tracks the current aggregate total corrected",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	aggregateTotalCorrectedAverager = components_metrics.NewNoOpAverager()

	aggregateTotalUncorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "aggregate_total_uncorrected",
			Help:      "tracks the current aggregate total uncorrected",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	aggregateTotalUncorrectedAverager = components_metrics.NewNoOpAverager()

	volatileTotalCorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "volatile_total_corrected",
			Help:      "tracks the current volatile total corrected",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	volatileTotalCorrectedAverager = components_metrics.NewNoOpAverager()

	volatileTotalUncorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "volatile_total_uncorrected",
			Help:      "tracks the current volatile total uncorrected",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
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

func SetAggregateTotalCorrected(ctx context.Context, gpuID string, cnt float64, currentTime time.Time) error {
	aggregateTotalCorrected.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(cnt)

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
	aggregateTotalUncorrected.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(cnt)

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
	volatileTotalCorrected.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(cnt)

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
	volatileTotalUncorrected.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(cnt)

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
