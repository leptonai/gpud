package temperature

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_temperature"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-temperature",
	}

	metricCurrentCelsius = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_celsius",
			Help:      "tracks the current temperature in celsius",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricThresholdSlowdownCelsius = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "slowdown_threshold_celsius",
			Help:      "tracks the threshold temperature in celsius for slowdown",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricSlowdownUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "slowdown_used_percent",
			Help:      "tracks the percentage of slowdown used",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func init() {
	prometheus.MustRegister(
		metricCurrentCelsius,
		metricThresholdSlowdownCelsius,
		metricSlowdownUsedPercent,
	)
}
