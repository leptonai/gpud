package memory

import (
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_memory"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-memory",
	}

	metricTotalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricReservedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "reserved_bytes",
			Help:      "tracks the reserved memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricUsedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the used memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricFreeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the free memory in bytes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the percentage of memory used",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricTotalBytes,
		metricReservedBytes,
		metricUsedBytes,
		metricFreeBytes,
		metricUsedPercent,
	)
}
