// Package metrics implements the file descriptor metrics collection and reporting.
package metrics

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "fd"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	allocatedFileHandles = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "allocated_file_handles",
			Help:      "tracks the total number of allocated file handles (e.g., /proc/sys/fs/file-nr on Linux)",
		},
	)
	allocatedFileHandlesAverager = components_metrics.NewNoOpAverager()

	runningPIDs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "running_pids",
			Help:      "tracks the total number of running pids, current file descriptor usage",
		},
	)
	runningPIDsAverager = components_metrics.NewNoOpAverager()

	limit = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "limit",
			Help:      "tracks the current system-wide file descriptor limit (e.g., /proc/sys/fs/file-max on Linux)",
		},
	)
	limitAverager = components_metrics.NewNoOpAverager()

	allocatedFileHandlesPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "allocated_file_handles_percent",
			Help:      "tracks the current file descriptor allocation percentage (allocated_file_handles / limit in percentage)",
		},
	)
	allocatedFileHandlesPercentAverager = components_metrics.NewNoOpAverager()

	usedPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the current file descriptor usage percentage (running_pids / limit in percentage)",
		},
	)
	usedPercentAverager = components_metrics.NewNoOpAverager()

	thresholdRunningPIDs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_running_pids",
			Help:      "tracks the current file descriptor threshold running pids",
		},
	)
	thresholdRunningPIDsPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_running_pids_percent",
			Help:      "tracks the current file descriptor threshold running pids percentage",
		},
	)
	thresholdUsedPercentAverager = components_metrics.NewNoOpAverager()

	thresholdAllocatedFileHandles = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_allocated_file_handles",
			Help:      "tracks the current file descriptor threshold allocated file handles",
		},
	)
	thresholdAllocatedFileHandlesPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_allocated_file_handles_percent",
			Help:      "tracks the current file descriptor threshold allocated percentage",
		},
	)
	thresholdAllocatedFileHandlesPercentAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	allocatedFileHandlesAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_allocated_file_handles")
	runningPIDsAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_running_pids")
	limitAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_limit")

	allocatedFileHandlesPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_allocated_file_handles_percent")
	usedPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_used_percent")

	thresholdUsedPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_threshold_used_percent")
	thresholdAllocatedFileHandlesPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_threshold_allocated_file_handles_percent")
}

func ReadAllocatedFileHandles(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return allocatedFileHandlesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadRunningPIDs(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return runningPIDsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadLimits(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return limitAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadAllocatedFileHandlesPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return allocatedFileHandlesPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadThresholdUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return thresholdUsedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetAllocatedFileHandles(ctx context.Context, handles float64, currentTime time.Time) error {
	allocatedFileHandles.Set(handles)

	if err := allocatedFileHandlesPercentAverager.Observe(
		ctx,
		handles,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func SetRunningPIDs(ctx context.Context, pids float64, currentTime time.Time) error {
	runningPIDs.Set(pids)

	if err := runningPIDsAverager.Observe(
		ctx,
		pids,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func SetLimit(ctx context.Context, v float64, currentTime time.Time) error {
	limit.Set(v)

	if err := limitAverager.Observe(
		ctx,
		v,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func SetAllocatedFileHandlesPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	allocatedFileHandlesPercent.Set(pct)

	if err := allocatedFileHandlesPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}
	return nil
}

func SetUsedPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	usedPercent.Set(pct)

	if err := usedPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func SetThresholdRunningPIDs(ctx context.Context, limit float64) error {
	thresholdRunningPIDs.Set(limit)
	return nil
}

func SetThresholdRunningPIDsPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	thresholdRunningPIDsPercent.Set(pct)

	if err := thresholdUsedPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func SetThresholdAllocatedFileHandles(ctx context.Context, handles float64) error {
	thresholdAllocatedFileHandles.Set(handles)
	return nil
}

func SetThresholdAllocatedFileHandlesPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	thresholdAllocatedFileHandlesPercent.Set(pct)

	if err := thresholdAllocatedFileHandlesPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
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
	if err := reg.Register(allocatedFileHandles); err != nil {
		return err
	}
	if err := reg.Register(runningPIDs); err != nil {
		return err
	}
	if err := reg.Register(limit); err != nil {
		return err
	}
	if err := reg.Register(allocatedFileHandlesPercent); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}
	if err := reg.Register(thresholdRunningPIDs); err != nil {
		return err
	}
	if err := reg.Register(thresholdRunningPIDsPercent); err != nil {
		return err
	}
	if err := reg.Register(thresholdAllocatedFileHandles); err != nil {
		return err
	}
	if err := reg.Register(thresholdAllocatedFileHandlesPercent); err != nil {
		return err
	}
	return nil
}
