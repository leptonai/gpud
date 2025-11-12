package tailscale

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "tailscale"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricTailscaleVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "tailscale_version",
			Help:      "tracks tailscale version",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "version"}, // from "tailscale --version" output
	).MustCurryWith(componentLabel)

	metricTailscaledVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "tailscaled_version",
			Help:      "tracks tailscaled version",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "version"}, // from "tailscaled --version" output
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricTailscaleVersion,
		metricTailscaledVersion,
	)
}
