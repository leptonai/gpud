// Package remappedrows provides the NVIDIA row remapping metrics collection and reporting.
package remappedrows

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_remapped_rows"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	uncorrectableErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "due_to_uncorrectable_errors",
			Help:      "tracks the number of rows remapped due to uncorrectable errors",
		},
		[]string{"gpu_id"},
	)
	uncorrectableErrorsAverager = components_metrics.NewNoOpAverager()

	remappingPending = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "remapping_pending",
			Help:      "set to 1 if this GPU requires a reset to actually remap the row",
		},
		[]string{"gpu_id"},
	)
	remappingPendingAverager = components_metrics.NewNoOpAverager()

	remappingFailed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "remapping_failed",
			Help:      "set to 1 if a remapping has failed in the past",
		},
		[]string{"gpu_id"},
	)
	remappingFailedAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	uncorrectableErrorsAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_due_to_uncorrectable_errors")
	remappingPendingAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_remapping_pending")
	remappingFailedAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_remapping_failed")
}

func ReadRemappedDueToUncorrectableErrors(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return uncorrectableErrorsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadRemappingPending(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return remappingPendingAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadRemappingFailed(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return remappingFailedAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetRemappedDueToUncorrectableErrors(ctx context.Context, gpuID string, cnt uint32, currentTime time.Time) error {
	uncorrectableErrors.WithLabelValues(gpuID).Set(float64(cnt))

	if err := uncorrectableErrorsAverager.Observe(
		ctx,
		float64(cnt),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetRemappingPending(ctx context.Context, gpuID string, pending bool, currentTime time.Time) error {
	v := float64(0)
	if pending {
		v = float64(1)
	}
	remappingPending.WithLabelValues(gpuID).Set(v)

	if err := remappingPendingAverager.Observe(
		ctx,
		v,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetRemappingFailed(ctx context.Context, gpuID string, failed bool, currentTime time.Time) error {
	v := float64(0)
	if failed {
		v = float64(1)
	}
	remappingFailed.WithLabelValues(gpuID).Set(v)

	if err := remappingFailedAverager.Observe(
		ctx,
		v,
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
	if err := reg.Register(uncorrectableErrors); err != nil {
		return err
	}
	if err := reg.Register(remappingPending); err != nil {
		return err
	}
	if err := reg.Register(remappingFailed); err != nil {
		return err
	}
	return nil
}
