package hwslowdown

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_clock"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-hw-slowdown",
	}

	metricHWSlowdown = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "hw_slowdown",
			Help:      "tracks hardware slowdown event -- HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricHWSlowdownThermal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "hw_slowdown_thermal",
			Help:      "tracks hardware thermal slowdown event -- HW Thermal Slowdown is engaged (temperature being too high",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricHWSlowdownPowerBrake = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "hw_slowdown_power_brake",
			Help:      "tracks hardware power brake slowdown event -- HW Power Brake Slowdown is engaged (External Power Brake Assertion being triggered)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func init() {
	prometheus.MustRegister(metricHWSlowdown)
	prometheus.MustRegister(metricHWSlowdownThermal)
	prometheus.MustRegister(metricHWSlowdownPowerBrake)
}
