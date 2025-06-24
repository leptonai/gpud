package infiniband

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_infiniband"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricIbLinkedDowned = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "link_downed",
			Help:      "tracks counters/link_downed - Number of times the link has gone down due to error thresholds being exceeded",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "device_port"}, // label is device name + "_" + port name (e.g., "mlx5_0_1")
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricIbLinkedDowned,
	)
}
