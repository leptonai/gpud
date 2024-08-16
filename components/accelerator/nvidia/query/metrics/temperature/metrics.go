package temperature

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_temperature"

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

	currentCelsius = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_celsius",
			Help:      "tracks the current temperature in celsius",
		},
		[]string{"gpu_id"},
	)
	currentCelsiusAverager = components_metrics.NewNoOpAverager()
	currentCelsiusAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_celsius_avg",
			Help:      "tracks the current temperature in celsius with average for the last period",
		},
		[]string{"gpu_id", "last_period"},
	)
	currentCelsiusEMA = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_celsius_ema",
			Help:      "tracks the current temperature in celsius with exponential moving average",
		},
		[]string{"gpu_id", "ema_period"},
	)

	thresholdSlowdownCelsius = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "slowdown_threshold_celsius",
			Help:      "tracks the threshold temperature in celsius for slowdown",
		},
		[]string{"gpu_id"},
	)
	thresholdSlowdownCelsiusAverager = components_metrics.NewNoOpAverager()

	slowdownUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "slowdown_used_percent",
			Help:      "tracks the percentage of slowdown used",
		},
		[]string{"gpu_id"},
	)
	slowdownUsedPercentAverager = components_metrics.NewNoOpAverager()
	slowdownUsedPercentAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "slowdown_used_percent_avg",
			Help:      "tracks the percentage of slowdown used with average for the last period",
		},
		[]string{"gpu_id", "last_period"},
	)
	slowdownUsedPercentEMA = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "slowdown_used_percent_ema",
			Help:      "tracks the percentage of slowdown used with exponential moving average",
		},
		[]string{"gpu_id", "ema_period"},
	)
)

func InitAveragers(db *sql.DB, tableName string) {
	currentCelsiusAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_current_celsius")
	thresholdSlowdownCelsiusAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_slowdown_threshold_celsius")
	slowdownUsedPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_slowdown_used_percent")
}

func ReadCurrentCelsius(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return currentCelsiusAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadThresholdSlowdownCelsius(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return thresholdSlowdownCelsiusAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadSlowdownUsedPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return slowdownUsedPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetCurrentCelsius(ctx context.Context, gpuID string, temp float64, currentTime time.Time) error {
	currentCelsius.WithLabelValues(gpuID).Set(temp)

	if err := currentCelsiusAverager.Observe(
		ctx,
		temp,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := currentCelsiusAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		currentCelsiusAverage.WithLabelValues(gpuID, duration.String()).Set(avg)

		ema, err := currentCelsiusAverager.EMA(
			ctx,
			components_metrics.WithEMAPeriod(duration),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		currentCelsiusEMA.WithLabelValues(gpuID, duration.String()).Set(ema)
	}

	return nil
}

func SetThresholdSlowdownCelsius(ctx context.Context, gpuID string, temp float64, currentTime time.Time) error {
	thresholdSlowdownCelsius.WithLabelValues(gpuID).Set(temp)

	if err := thresholdSlowdownCelsiusAverager.Observe(
		ctx,
		temp,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetSlowdownUsedPercent(ctx context.Context, gpuID string, pct float64, currentTime time.Time) error {
	slowdownUsedPercent.WithLabelValues(gpuID).Set(pct)

	if err := slowdownUsedPercentAverager.Observe(
		ctx,
		pct,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := slowdownUsedPercentAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		slowdownUsedPercentAverage.WithLabelValues(gpuID, duration.String()).Set(avg)

		ema, err := slowdownUsedPercentAverager.EMA(
			ctx,
			components_metrics.WithEMAPeriod(duration),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		slowdownUsedPercentEMA.WithLabelValues(gpuID, duration.String()).Set(ema)
	}

	return nil
}

func Register(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	InitAveragers(db, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(currentCelsius); err != nil {
		return err
	}
	if err := reg.Register(currentCelsiusAverage); err != nil {
		return err
	}
	if err := reg.Register(currentCelsiusEMA); err != nil {
		return err
	}
	if err := reg.Register(thresholdSlowdownCelsius); err != nil {
		return err
	}
	if err := reg.Register(slowdownUsedPercent); err != nil {
		return err
	}
	if err := reg.Register(slowdownUsedPercentAverage); err != nil {
		return err
	}
	if err := reg.Register(slowdownUsedPercentEMA); err != nil {
		return err
	}
	return nil
}
