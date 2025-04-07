package remappedrows

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_remapped_rows"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-remapped-rows",
	}

	metricUncorrectableErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "due_to_uncorrectable_errors",
			Help:      "tracks the number of rows remapped due to uncorrectable errors",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricRemappingPending = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "remapping_pending",
			Help:      "set to 1 if this GPU requires a reset to actually remap the row",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricRemappingFailed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "remapping_failed",
			Help:      "set to 1 if a remapping has failed in the past",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricUncorrectableErrors,
		metricRemappingPending,
		metricRemappingFailed,
	)
}
