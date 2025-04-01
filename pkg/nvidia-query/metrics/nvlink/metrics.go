// Package nvlink provides the NVIDIA nvlink metrics collection and reporting.
package nvlink

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_nvlink"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-nvlink",
	}

	featureEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "feature_enabled",
			Help:      "tracks the NVLink feature enabled (aggregated for all links per GPU)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	featureEnabledAverager = components_metrics.NewNoOpAverager()

	replayErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "replay_errors",
			Help:      "tracks the relay errors in NVLink (aggregated for all links per GPU)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	replayErrorsAverager = components_metrics.NewNoOpAverager()

	recoveryErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "recovery_errors",
			Help:      "tracks the recovery errors in NVLink (aggregated for all links per GPU)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	recoveryErrorsAverager = components_metrics.NewNoOpAverager()

	crcErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "crc_errors",
			Help:      "tracks the CRC errors in NVLink (aggregated for all links per GPU)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	crcErrorsAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	featureEnabledAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_feature_enabled")
	replayErrorsAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_replay_errors")
	recoveryErrorsAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_recovery_errors")
	crcErrorsAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_crc_errors")
}

func ReadFeatureEnabled(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return featureEnabledAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadReplayErrors(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return replayErrorsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadRecoveryErrors(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return recoveryErrorsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadCRCErrors(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return crcErrorsAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetFeatureEnabled(ctx context.Context, gpuID string, enabled bool, currentTime time.Time) error {
	v := float64(0)
	if enabled {
		v = float64(1)
	}
	featureEnabled.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(v)

	if err := featureEnabledAverager.Observe(
		ctx,
		v,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetReplayErrors(ctx context.Context, gpuID string, errors uint64, currentTime time.Time) error {
	replayErrors.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(float64(errors))

	if err := replayErrorsAverager.Observe(
		ctx,
		float64(errors),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetRecoveryErrors(ctx context.Context, gpuID string, errors uint64, currentTime time.Time) error {
	recoveryErrors.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(float64(errors))

	if err := recoveryErrorsAverager.Observe(
		ctx,
		float64(errors),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetCRCErrors(ctx context.Context, gpuID string, errors uint64, currentTime time.Time) error {
	crcErrors.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(float64(errors))

	if err := crcErrorsAverager.Observe(
		ctx,
		float64(errors),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(featureEnabled); err != nil {
		return err
	}
	if err := reg.Register(replayErrors); err != nil {
		return err
	}
	if err := reg.Register(recoveryErrors); err != nil {
		return err
	}
	if err := reg.Register(crcErrors); err != nil {
		return err
	}
	return nil
}
