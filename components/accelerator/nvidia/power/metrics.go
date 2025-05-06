package power

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_power"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-power",
	}

	metricCurrentUsageMilliWatts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "current_usage_milli_watts",
			Help:      "tracks the current power in milliwatts",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelNamePrefix + "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricEnforcedLimitMilliWatts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "enforced_limit_milli_watts",
			Help:      "tracks the enforced power limit in milliwatts",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelNamePrefix + "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the percentage of power used",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelNamePrefix + "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricCurrentUsageMilliWatts,
		metricEnforcedLimitMilliWatts,
		metricUsedPercent,
	)
}
