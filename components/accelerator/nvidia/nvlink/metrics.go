package nvlink

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_nvlink"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-nvlink",
	}

	metricFeatureEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "feature_enabled",
			Help:      "tracks the NVLink feature enabled (aggregated for all links per GPU)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricReplayErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "replay_errors",
			Help:      "tracks the relay errors in NVLink (aggregated for all links per GPU)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricRecoveryErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "recovery_errors",
			Help:      "tracks the recovery errors in NVLink (aggregated for all links per GPU)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricCRCErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "crc_errors",
			Help:      "tracks the CRC errors in NVLink (aggregated for all links per GPU)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func init() {
	prometheus.MustRegister(
		metricFeatureEnabled,
		metricReplayErrors,
		metricRecoveryErrors,
		metricCRCErrors,
	)
}
