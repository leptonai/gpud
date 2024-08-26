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

	gpuSMOccupancyPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "gpu_sm_occupancy_percent",
			Help:      "tracks the current GPU SM occupancy percent",
		},
		[]string{"gpu_id"},
	)
	gpuSMOccupancyPercentAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(db *sql.DB, tableName string) {
	gpuSMOccupancyPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_sm_occupancy_percent")
}

func ReadGPUUtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuSMOccupancyPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetGPUSMOccupancy(ctx context.Context, gpuID string, pct uint32, currentTime time.Time) error {
	gpuSMOccupancyPercent.WithLabelValues(gpuID).Set(float64(pct))

	if err := gpuSMOccupancyPercentAverager.Observe(
		ctx,
		float64(pct),
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
