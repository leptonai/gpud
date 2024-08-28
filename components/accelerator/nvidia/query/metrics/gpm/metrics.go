// Package gpm provides the NVIDIA GPM metrics collection and reporting.
package gpm

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_gpm"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	// gpuSMOccupancyPercent is the percentage of warps that were active vs theoretical maximum (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_SM_OCCUPANCY or DCGM_FI_PROF_SM_OCCUPANCY in DCGM exporter.
	// It's the ratio of number of warps resident on an SM.
	// It's the number of resident as a ratio of the theoretical maximum number of warps per elapsed cycle.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlGpmStructs.html#group__nvmlGpmStructs_1g168f5f2704ec9871110d22aa1879aec0
	gpuSMOccupancyPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "gpu_sm_occupancy_percent",
			Help:      "tracks the current GPU SM occupancy, as a percentage of warps that were active vs theoretical maximum",
		},
		[]string{"gpu_id"},
	)
	gpuSMOccupancyPercentAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(db *sql.DB, tableName string) {
	gpuSMOccupancyPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_sm_occupancy_percent")
}

func ReadGPUSMOccupancyPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuSMOccupancyPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetGPUSMOccupancyPercent(ctx context.Context, gpuID string, pct float64, currentTime time.Time) error {
	gpuSMOccupancyPercent.WithLabelValues(gpuID).Set(pct)

	if err := gpuSMOccupancyPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func Register(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	InitAveragers(db, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(gpuSMOccupancyPercent); err != nil {
		return err
	}
	return nil
}
