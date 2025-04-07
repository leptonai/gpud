package latency

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "network_latency"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "network-latency",
	}

	metricEdgeInMilliseconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "edge_in_milliseconds",
			Help:      "tracks the edge latency in milliseconds",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is provider region
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(metricEdgeInMilliseconds)
}
