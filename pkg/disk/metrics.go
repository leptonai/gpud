package disk

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "disk",
	}

	metricGetUsageSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gpud",
			Subsystem: "disk",
			Name:      "get_usage_seconds",
			Help:      "time taken to get disk usage",

			// want to track with lowest bound 0.001s (1ms) and highest bound 2.5s
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "mount_point"}, // label is the mount point
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricGetUsageSeconds,
	)
}
