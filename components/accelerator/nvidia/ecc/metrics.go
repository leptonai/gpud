// Package ecc provides the NVIDIA ECC metrics collection and reporting.
package ecc

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
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

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	if err := reg.Register(metricAggregateTotalCorrected); err != nil {
		return err
	}
	if err := reg.Register(metricAggregateTotalUncorrected); err != nil {
		return err
	}
	if err := reg.Register(metricVolatileTotalCorrected); err != nil {
		return err
	}
	if err := reg.Register(metricVolatileTotalUncorrected); err != nil {
		return err
	}

	return nil
}
