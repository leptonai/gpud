package clockspeed

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_clock_speed"

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

	graphicsMHz = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "graphics_mhz",
			Help:      "tracks the current GPU clock speeds in MHz",
		},
		[]string{"gpu_id"},
	)
	graphicsMHzAverager = components_metrics.NewNoOpAverager()
	graphicsMHzAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "graphics_mhz_avg",
			Help:      "tracks the GPU clock speeds in MHz with average for the last period",
		},
		[]string{"gpu_id", "last_period"},
	)
	graphicsMHzEMA = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "graphics_mhz_ema",
			Help:      "tracks the GPU clock speeds in MHz with exponential moving average",
		},
		[]string{"gpu_id", "ema_period"},
	)

	memoryMHz = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "memory_mhz",
			Help:      "tracks the current GPU memory utilization percent",
		},
		[]string{"gpu_id"},
	)
	memoryMHzAverager = components_metrics.NewNoOpAverager()
	memoryMHzAverage  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "memory_mhz_avg",
			Help:      "tracks the GPU memory clock speed in MHz with average for the last period",
		},
		[]string{"gpu_id", "last_period"},
	)
	memoryMHzEMA = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "memory_mhz_ema",
			Help:      "tracks the GPU memory clock speed in MHz with exponential moving average",
		},
		[]string{"gpu_id", "ema_period"},
	)
)

func InitAveragers(db *sql.DB, tableName string) {
	graphicsMHzAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_graphics_mhz")
	memoryMHzAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_memory_mhz")
}

func ReadGraphicsMHzs(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return graphicsMHzAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadMemoryMHzs(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return memoryMHzAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetGraphicsMHz(ctx context.Context, gpuID string, pct uint32, currentTime time.Time) error {
	graphicsMHz.WithLabelValues(gpuID).Set(float64(pct))

	if err := graphicsMHzAverager.Observe(
		ctx,
		float64(pct),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := graphicsMHzAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		graphicsMHzAverage.WithLabelValues(gpuID, duration.String()).Set(avg)

		ema, err := graphicsMHzAverager.EMA(
			ctx,
			components_metrics.WithEMAPeriod(duration),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		graphicsMHzEMA.WithLabelValues(gpuID, duration.String()).Set(ema)
	}

	return nil
}

func SetMemoryMHz(ctx context.Context, gpuID string, pct uint32, currentTime time.Time) error {
	memoryMHz.WithLabelValues(gpuID).Set(float64(pct))

	if err := memoryMHzAverager.Observe(
		ctx,
		float64(pct),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	for _, duration := range defaultPeriods {
		avg, err := memoryMHzAverager.Avg(
			ctx,
			components_metrics.WithSince(currentTime.Add(-duration)),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		memoryMHzAverage.WithLabelValues(gpuID, duration.String()).Set(avg)

		ema, err := memoryMHzAverager.EMA(
			ctx,
			components_metrics.WithEMAPeriod(duration),
			components_metrics.WithMetricSecondaryName(gpuID),
		)
		if err != nil {
			return err
		}
		memoryMHzEMA.WithLabelValues(gpuID, duration.String()).Set(ema)
	}

	return nil
}

func Register(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	InitAveragers(db, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(graphicsMHz); err != nil {
		return err
	}
	if err := reg.Register(graphicsMHzAverage); err != nil {
		return err
	}
	if err := reg.Register(graphicsMHzEMA); err != nil {
		return err
	}
	if err := reg.Register(memoryMHz); err != nil {
		return err
	}
	if err := reg.Register(memoryMHzAverage); err != nil {
		return err
	}
	if err := reg.Register(memoryMHzEMA); err != nil {
		return err
	}
	return nil
}
