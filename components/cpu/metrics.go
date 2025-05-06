package cpu

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "cpu"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "cpu",
	}

	// ref. https://www.digitalocean.com/community/tutorials/load-average-in-linux
	metricLoadAverage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "load_average",
			Help:      "tracks the load average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "load_duration"}, // label is last period
	).MustCurryWith(componentLabel)

	metricUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the current CPU usage percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricLoadAverage,
		metricUsedPercent,
	)
}
