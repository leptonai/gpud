package disk

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "disk"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "disk",
	}

	metricTotalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)

	metricFreeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)

	metricUsedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)

	metricUsedBytesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_percent",
			Help:      "tracks the current disk bytes usage percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)

	metricUsedInodesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_inodes_percent",
			Help:      "tracks the current disk inodes usage percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricTotalBytes,
		metricFreeBytes,
		metricUsedBytes,
		metricUsedBytesPercent,
		metricUsedInodesPercent,
	)
}
