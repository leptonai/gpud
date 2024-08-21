// Package processes provides the NVIDIA processes metrics collection and reporting.
package processes

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_processes"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	runningProcesses = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "running_total",
			Help:      "tracks the current per-GPU process counter",
		},
		[]string{"gpu_id"},
	)
	runningProcessesTotalAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(db *sql.DB, tableName string) {
	runningProcessesTotalAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_total")
}

func ReadRunningProcessesTotal(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return runningProcessesTotalAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetRunningProcessesTotal(ctx context.Context, gpuID string, processes int, currentTime time.Time) error {
	runningProcesses.WithLabelValues(gpuID).Set(float64(processes))

	if err := runningProcessesTotalAverager.Observe(
		ctx,
		float64(processes),
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
	if err := reg.Register(runningProcesses); err != nil {
		return err
	}
	return nil
}
