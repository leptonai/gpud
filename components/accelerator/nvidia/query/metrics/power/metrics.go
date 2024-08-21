// Package power provides the NVIDIA power usage metrics collection and reporting.
package power

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_power"

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

	currentUsageMilliWatts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_usage_milli_watts",
			Help:      "tracks the current power in milliwatts",
		},
		[]string{"gpu_id"},
	)
	currentUsageMilliWattsAverager = components_metrics.NewNoOpAverager()
	currentUsageMilliWattsAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_usage_milli_watts_avg",
			Help:      "tracks the current power in milliwatts with average for the last period",
		},
		[]string{"gpu_id", "last_period"},
	)
	currentUsageMilliWattsEMA = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_usage_milli_watts_ema",
			Help:      "tracks the current power in milliwatts with exponential moving average",
		},
		[]string{"gpu_id", "ema_period"},
	)

	enforcedLimitMilliWatts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "enforced_limit_milli_watts",
			Help:      "tracks the enforced power limit in milliwatts",
		},
		[]string{"gpu_id"},
	)
	enforcedLimitMilliWattsAverager = components_metrics.NewNoOpAverager()

	usedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the percentage of power used",
		},
		[]string{"gpu_id"},
	)
	usedPercentAverager = components_metrics.NewNoOpAverager()
	usedPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent_avg",
			Help:      "tracks the used power in percent with average for the last period",
		},
		[]string{"gpu_id", "last_period"},
	)
	usedPercentEMA = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent_ema",
			Help:      "tracks the percentage of power used with exponential moving average",
		},
		[]string{"gpu_id", "ema_period"},
	)
)

func InitAveragers(db *sql.DB, tableName string) {
	currentUsageMilliWattsAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_current_usage_milli_watts")
	enforcedLimitMilliWattsAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_enforced_limit_milli_watts")
	usedPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_used_percent")
}

func ReadCurrentUsageMilliWatts(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return currentUsageMilliWattsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadEnforcedLimitMilliWatts(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return enforcedLimitMilliWattsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return usedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetUsageMilliWatts(ctx context.Context, gpuID string, milliWatts float64, currentTime time.Time) error {
	currentUsageMilliWatts.WithLabelValues(gpuID).Set(milliWatts)

	if err := currentUsageMilliWattsAverager.Observe(
		ctx,
		milliWatts,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := currentUsageMilliWattsAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		currentUsageMilliWattsAverage.WithLabelValues(gpuID, duration.String()).Set(avg)

		ema, err := currentUsageMilliWattsAverager.EMA(
			ctx,
			components_metrics.WithEMAPeriod(duration),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		currentUsageMilliWattsEMA.WithLabelValues(gpuID, duration.String()).Set(ema)
	}

	return nil
}

func SetEnforcedLimitMilliWatts(ctx context.Context, gpuID string, milliWatts float64, currentTime time.Time) error {
	enforcedLimitMilliWatts.WithLabelValues(gpuID).Set(milliWatts)

	if err := enforcedLimitMilliWattsAverager.Observe(
		ctx,
		milliWatts,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetUsedPercent(ctx context.Context, gpuID string, pct float64, currentTime time.Time) error {
	usedPercent.WithLabelValues(gpuID).Set(pct)

	if err := usedPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := usedPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		usedPercentAverage.WithLabelValues(gpuID, duration.String()).Set(avg)

		ema, err := usedPercentAverager.EMA(
			ctx,
			components_metrics.WithEMAPeriod(duration),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		usedPercentEMA.WithLabelValues(gpuID, duration.String()).Set(ema)
	}

	return nil
}

func Register(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	InitAveragers(db, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(currentUsageMilliWatts); err != nil {
		return err
	}
	if err := reg.Register(currentUsageMilliWattsAverage); err != nil {
		return err
	}
	if err := reg.Register(currentUsageMilliWattsEMA); err != nil {
		return err
	}
	if err := reg.Register(enforcedLimitMilliWatts); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}
	if err := reg.Register(usedPercentAverage); err != nil {
		return err
	}
	if err := reg.Register(usedPercentEMA); err != nil {
		return err
	}
	return nil
}
