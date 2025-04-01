// Package processes provides the NVIDIA processes metrics collection and reporting.
package processes

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_processes"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-processes",
	}

	runningProcesses = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "running_total",
			Help:      "tracks the current per-GPU process counter",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
	runningProcessesTotalAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	runningProcessesTotalAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_total")
}

func ReadRunningProcessesTotal(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return runningProcessesTotalAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetRunningProcessesTotal(ctx context.Context, gpuID string, processes int, currentTime time.Time) error {
	runningProcesses.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(float64(processes))

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

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(runningProcesses); err != nil {
		return err
	}
	return nil
}
