package containerd

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "containerd"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "version",
			Help:      "tracks containerd version",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "version"}, // from "containerd --version" output
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricVersion,
	)
}
