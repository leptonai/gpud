// Package clock provides the NVIDIA clock metrics collection and reporting.
package clock

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_clock"

var (
	initOnce sync.Once

	hwSlowdown = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "hw_slowdown",
			Help:      "tracks hardware slowdown event -- HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-hw-slowdown",
	})
	hwSlowdownAverager = components_metrics.NewNoOpAverager()

	hwSlowdownThermal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "hw_slowdown_thermal",
			Help:      "tracks hardware thermal slowdown event -- HW Thermal Slowdown is engaged (temperature being too high",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-hw-slowdown",
	})
	hwSlowdownThermalAverager = components_metrics.NewNoOpAverager()

	hwSlowdownPowerBrake = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "hw_slowdown_power_brake",
			Help:      "tracks hardware power brake slowdown event -- HW Power Brake Slowdown is engaged (External Power Brake Assertion being triggered)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-hw-slowdown",
	})
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

func SetHWSlowdown(ctx context.Context, gpuID string, b bool, currentTime time.Time) error {
	v := float64(0.0)
	if b {
		v = float64(1.0)
	}
	hwSlowdown.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(v)

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
	hwSlowdownThermal.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(v)

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
	hwSlowdownPowerBrake.With(prometheus.Labels{pkgmetrics.MetricLabelKey: gpuID}).Set(v)

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
