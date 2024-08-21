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

// Used for tracking the past x-minute averages + EMAs.
var defaultPeriods = []time.Duration{5 * time.Minute}

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
	runningPIDsAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "running_pids_avg",
			Help:      "tracks the total number of running pids with average for the last period",
		},
		[]string{"last_period"},
	)

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
	usedPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent_avg",
			Help:      "tracks the file descriptor usage percentage with average for the last period",
		},
		[]string{"last_period"},
	)
)

func InitAveragers(db *sql.DB, tableName string) {
	runningPIDsAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_running_pids")
	limitAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_limit")
	usedPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_used_percent")
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

	for _, duration := range defaultPeriods {
		avg, err := runningPIDsAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
		)
		if err != nil {
			return err
		}
		runningPIDsAverage.WithLabelValues(duration.String()).Set(avg)
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

	for _, duration := range defaultPeriods {
		avg, err := usedPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
		)
		if err != nil {
			return err
		}
		usedPercentAverage.WithLabelValues(duration.String()).Set(avg)
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
	if err := reg.Register(runningPIDsAverage); err != nil {
		return err
	}
	if err := reg.Register(limit); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}
	if err := reg.Register(usedPercentAverage); err != nil {
		return err
	}
	return nil
}
