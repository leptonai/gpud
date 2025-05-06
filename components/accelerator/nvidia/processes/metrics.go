package processes

import (
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_processes"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-processes",
	}

	metricRunningProcesses = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "running_total",
			Help:      "tracks the current per-GPU process counter",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelNamePrefix + "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(metricRunningProcesses)
}
