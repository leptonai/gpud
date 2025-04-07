package memory

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "memory"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "memory",
	}

	metricTotalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricAvailableBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "available_bytes",
			Help:      "tracks the available memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricUsedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the used memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the percentage of memory used",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricFreeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the free memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
)

func init() {
	prometheus.MustRegister(metricTotalBytes)
	prometheus.MustRegister(metricAvailableBytes)
	prometheus.MustRegister(metricUsedBytes)
	prometheus.MustRegister(metricUsedPercent)
	prometheus.MustRegister(metricFreeBytes)
}
