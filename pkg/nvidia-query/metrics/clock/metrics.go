// Package clock provides the NVIDIA clock metrics collection and reporting.
package clock

import (
	"context"
	"database/sql"
	"sync"
	"time"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_clock"

var (
	initOnce sync.Once

	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	hwSlowdown = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "hw_slowdown",
			Help:      "tracks hardware slowdown event -- HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw",
		},
		[]string{"gpu_id"},
	)
	hwSlowdownAverager = components_metrics.NewNoOpAverager()

	hwSlowdownThermal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "hw_slowdown_thermal",
			Help:      "tracks hardware thermal slowdown event -- HW Thermal Slowdown is engaged (temperature being too high",
		},
		[]string{"gpu_id"},
	)
	hwSlowdownThermalAverager = components_metrics.NewNoOpAverager()

	hwSlowdownPowerBrake = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "hw_slowdown_power_brake",
			Help:      "tracks hardware power brake slowdown event -- HW Power Brake Slowdown is engaged (External Power Brake Assertion being triggered)",
		},
		[]string{"gpu_id"},
	)
	hwSlowdownPowerBrakeAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(dbRW *sql.DB, dbRO *sql.DB, tableName string) {
	initOnce.Do(func() {
		hwSlowdownAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_hw_slowdown")
		hwSlowdownThermalAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_hw_slowdown_thermal")
		hwSlowdownPowerBrakeAverager = components_metrics.NewAverager(dbRW, dbRO, tableName, SubSystem+"_hw_slowdown_power_brake")
	})
}

func ReadHWSlowdown(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return hwSlowdownAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadHWSlowdownThermal(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return hwSlowdownThermalAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadHWSlowdownPowerBrake(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return hwSlowdownPowerBrakeAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetHWSlowdown(ctx context.Context, gpuID string, b bool, currentTime time.Time) error {
	v := float64(0.0)
	if b {
		v = float64(1.0)
	}
	hwSlowdown.WithLabelValues(gpuID).Set(v)

	if err := hwSlowdownAverager.Observe(
		ctx,
		v,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetHWSlowdownThermal(ctx context.Context, gpuID string, b bool, currentTime time.Time) error {
	v := float64(0.0)
	if b {
		v = float64(1.0)
	}
	hwSlowdownThermal.WithLabelValues(gpuID).Set(v)

	if err := hwSlowdownThermalAverager.Observe(
		ctx,
		v,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetHWSlowdownPowerBrake(ctx context.Context, gpuID string, b bool, currentTime time.Time) error {
	v := float64(0.0)
	if b {
		v = float64(1.0)
	}
	hwSlowdownPowerBrake.WithLabelValues(gpuID).Set(v)

	if err := hwSlowdownPowerBrakeAverager.Observe(
		ctx,
		v,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
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
	if err := reg.Register(hwSlowdown); err != nil {
		return err
	}
	if err := reg.Register(hwSlowdownThermal); err != nil {
		return err
	}
	if err := reg.Register(hwSlowdownPowerBrake); err != nil {
		return err
	}

	return nil
}
