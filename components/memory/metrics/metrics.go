// Package metrics implements the memory metrics collection and reporting.
package metrics

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "memory"

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

	totalBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total memory in bytes",
		},
	)
	totalBytesAverager = components_metrics.NewNoOpAverager()

	availableBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "available_bytes",
			Help:      "tracks the available memory in bytes",
		},
	)

	usedBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the used memory in bytes",
		},
	)
	usedBytesAverager = components_metrics.NewNoOpAverager()
	usedBytesAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_average",
			Help:      "tracks the used memory in bytes with average for the last period",
		},
		[]string{"last_period"},
	)
	usedBytesEMA = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_ema",
			Help:      "tracks the used memory in bytes with exponential moving average",
		},
		[]string{"ema_period"},
	)

	usedPercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the percentage of memory used",
		},
	)
	usedPercentAverager = components_metrics.NewNoOpAverager()
	usedPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent_avg",
			Help:      "tracks the percentage of memory used with average for the last period",
		},
		[]string{"last_period"},
	)
	usedPercentEMA = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent_ema",
			Help:      "tracks the percentage of memory used with exponential moving average",
		},
		[]string{"ema_period"},
	)

	freeBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the free memory in bytes",
		},
	)
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	totalBytesAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_total_bytes")
	usedBytesAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_used_bytes")
	usedPercentAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_used_percent")
}

func ReadTotalBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return totalBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetTotalBytes(ctx context.Context, bytes float64, currentTime time.Time) error {
	totalBytes.Set(bytes)

	if err := totalBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	return nil
}

func SetAvailableBytes(bytes float64) {
	availableBytes.Set(bytes)
}

func SetUsedBytes(ctx context.Context, bytes float64, currentTime time.Time) error {
	usedBytes.Set(bytes)

	if err := usedBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedBytesAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
		)
		if err != nil {
			return err
		}
		usedBytesAverage.WithLabelValues(duration.String()).Set(avg)

		ema, err := usedBytesAverager.EMA(
			ctx,
			components_metrics.WithEMAPeriod(duration),
		)
		if err != nil {
			return err
		}
		usedBytesEMA.WithLabelValues(duration.String()).Set(ema)
	}

	return nil
}

func SetUsedPercent(ctx context.Context, pct float64, currentTime time.Time) error {
	usedPercent.Set(pct)

	if err := usedPercentAverager.Observe(ctx, pct, components_metrics.WithCurrentTime(currentTime)); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedPercentAverager.Avg(ctx, components_metrics.WithSince(currentTime.Add(-duration)))
		if err != nil {
			return err
		}
		usedPercentAverage.WithLabelValues(duration.String()).Set(avg)

		ema, err := usedPercentAverager.EMA(ctx, components_metrics.WithEMAPeriod(duration))
		if err != nil {
			return err
		}
		usedPercentEMA.WithLabelValues(duration.String()).Set(ema)
	}

	return nil
}

func SetFreeBytes(bytes float64) {
	freeBytes.Set(bytes)
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	InitAveragers(dbRW, dbRO, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(totalBytes); err != nil {
		return err
	}
	if err := reg.Register(availableBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytesEMA); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}
	if err := reg.Register(usedPercentEMA); err != nil {
		return err
	}
	if err := reg.Register(freeBytes); err != nil {
		return err
	}
	return nil
}
