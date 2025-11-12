package kubelet

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "kubelet"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "version",
			Help:      "tracks kubelet version",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "version"}, // from "kubelet --version" output
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricVersion,
	)
}
