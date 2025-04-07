package fuse

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "fuse"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "fuse",
	}

	metricConnsCongestedPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_congested_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is device name
	).MustCurryWith(componentLabel)

	metricConnsMaxBackgroundPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_max_background_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is device name
	).MustCurryWith(componentLabel)
)

func init() {
	prometheus.MustRegister(metricConnsCongestedPct)
	prometheus.MustRegister(metricConnsMaxBackgroundPct)
}
