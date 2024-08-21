// Package metrics implements the CPU metrics collection and reporting.
package metrics

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "cpu"

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

	// ref. https://www.digitalocean.com/community/tutorials/load-average-in-linux
	loadAverage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "load_average",
			Help:      "tracks the load average for the last period",
		},
		[]string{"last_period"},
	)
	loadAverage5minAverager = components_metrics.NewNoOpAverager()

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
	loadAverage5minAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_load_average_5min")
	usedPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_used_percent")
}

func ReadLoadAverage5mins(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return loadAverage5minAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetLoadAverage(ctx context.Context, duration time.Duration, avg float64, currentTime time.Time) error {
	loadAverage.WithLabelValues(duration.String()).Set(avg)

	switch duration {
	case 5 * time.Minute:
		if err := loadAverage5minAverager.Observe(
			ctx,
			avg,
			components_metrics.WithCurrentTime(currentTime),
		); err != nil {
			return err
		}

	default:
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
	if err := reg.Register(loadAverage); err != nil {
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
