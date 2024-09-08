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

	runningPIDs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "running_pids",
			Help:      "tracks the total number of running pids",
		},
	)
	runningPIDsAverager = components_metrics.NewNoOpAverager()

	limit = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "limit",
			Help:      "tracks the current file descriptor limit",
		},
	)
	limitAverager = components_metrics.NewNoOpAverager()

	usedPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the current file descriptor usage percentage",
		},
	)
	usedPercentAverager = components_metrics.NewNoOpAverager()

	thresholdLimit = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_limit",
			Help:      "tracks the current file descriptor threshold limit",
		},
	)
	thresholdUsedPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_used_percent",
			Help:      "tracks the current file descriptor threshold usage percentage",
		},
	)
	thresholdUsedPercentAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(db *sql.DB, tableName string) {
	runningPIDsAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_running_pids")
	limitAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_limit")
	usedPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_used_percent")
	thresholdUsedPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_threshold_used_percent")
}

func ReadRunningPIDs(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return runningPIDsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadLimits(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return limitAverager.Read(ctx, components_metrics.WithSince(since))
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

func SetRunningPIDs(ctx context.Context, pids float64, currentTime time.Time) error {
	runningPIDs.Set(pids)

	if err := runningPIDsAverager.Observe(
		ctx,
		pids,
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

func SetThresholdLimit(ctx context.Context, limit float64) error {
	thresholdLimit.Set(limit)
	return nil
}

func SetThresholdUsedPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	thresholdUsedPercent.Set(pct)

	if err := thresholdUsedPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
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
	if err := reg.Register(runningPIDs); err != nil {
		return err
	}
	if err := reg.Register(limit); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}
	if err := reg.Register(thresholdLimit); err != nil {
		return err
	}
	if err := reg.Register(thresholdUsedPercent); err != nil {
		return err
	}
	return nil
}
