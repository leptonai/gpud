// Package metrics implements the disk metrics collection and reporting.
package metrics

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "disk"

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

	totalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total bytes of the disk",
		},
		[]string{"mount_point"},
	)
	totalBytesAverager = components_metrics.NewNoOpAverager()

	freeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{"mount_point"},
	)

	usedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{"mount_point"},
	)
	usedBytesAverager = components_metrics.NewNoOpAverager()
	usedBytesAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_avg",
			Help:      "tracks the disk bytes usage with average for the last period",
		},
		[]string{"mount_point", "last_period"},
	)

	usedBytesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_percent",
			Help:      "tracks the current disk bytes usage percentage",
		},
		[]string{"mount_point"},
	)
	usedBytesPercentAverager = components_metrics.NewNoOpAverager()
	usedBytesPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_percent_avg",
			Help:      "tracks the disk bytes usage percentage with average for the last period",
		},
		[]string{"mount_point", "last_period"},
	)

	usedInodesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_inodes_percent",
			Help:      "tracks the current disk inodes usage percentage",
		},
		[]string{"mount_point"},
	)
)

func InitAveragers(db *sql.DB, tableName string) {
	totalBytesAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_total_bytes")
	usedBytesAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_used_bytes")
	usedBytesPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_used_bytes_percent")
}

func ReadTotalBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return totalBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedBytesPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedBytesPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetTotalBytes(ctx context.Context, mountPoint string, bytes float64, currentTime time.Time) error {
	totalBytes.WithLabelValues(mountPoint).Set(bytes)

	if err := totalBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(mountPoint),
	); err != nil {
		return err
	}

	return nil
}

func SetFreeBytes(mountPoint string, bytes float64) {
	freeBytes.WithLabelValues(mountPoint).Set(bytes)
}

func SetUsedBytes(ctx context.Context, mountPoint string, bytes float64, currentTime time.Time) error {
	usedBytes.WithLabelValues(mountPoint).Set(bytes)

	if err := usedBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(mountPoint),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedBytesAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(mountPoint),
		)
		if err != nil {
			return err
		}
		usedBytesAverage.WithLabelValues(mountPoint, duration.String()).Set(avg)
	}

	return nil
}

func SetUsedBytesPercent(ctx context.Context, mountPoint string, pct float64, currentTime time.Time) error {
	usedBytesPercent.WithLabelValues(mountPoint).Set(pct)

	if err := usedBytesPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(mountPoint),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedBytesPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(mountPoint),
		)
		if err != nil {
			return err
		}
		usedBytesPercentAverage.WithLabelValues(mountPoint, duration.String()).Set(avg)
	}

	return nil
}

func SetUsedInodesPercent(mountPoint string, pct float64) {
	usedInodesPercent.WithLabelValues(mountPoint).Set(pct)
}

func Register(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	InitAveragers(db, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(totalBytes); err != nil {
		return err
	}
	if err := reg.Register(freeBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytesAverage); err != nil {
		return err
	}
	if err := reg.Register(usedBytesPercent); err != nil {
		return err
	}
	if err := reg.Register(usedBytesPercentAverage); err != nil {
		return err
	}
	if err := reg.Register(usedInodesPercent); err != nil {
		return err
	}
	return nil
}
