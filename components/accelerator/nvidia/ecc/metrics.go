// Package ecc provides the NVIDIA ECC metrics collection and reporting.
package ecc

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_ecc"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-ecc",
	}

	metricAggregateTotalCorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "aggregate_total_corrected",
			Help:      "tracks the current aggregate total corrected",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricAggregateTotalUncorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "aggregate_total_uncorrected",
			Help:      "tracks the current aggregate total uncorrected",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricVolatileTotalCorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "volatile_total_corrected",
			Help:      "tracks the current volatile total corrected",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)

	metricVolatileTotalUncorrected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "volatile_total_uncorrected",
			Help:      "tracks the current volatile total uncorrected",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricAggregateTotalCorrected,
		metricAggregateTotalUncorrected,
		metricVolatileTotalCorrected,
		metricVolatileTotalUncorrected,
	)
}
