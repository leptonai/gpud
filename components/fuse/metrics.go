package fuse

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "fuse"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "fuse",
	}

	connsCongestedPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_congested_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is device name
	).MustCurryWith(componentLabel)

	connsMaxBackgroundPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_max_background_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is device name
	).MustCurryWith(componentLabel)
)

var _ components.PromRegisterer = &component{}

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	if err := reg.Register(connsCongestedPct); err != nil {
		return err
	}
	if err := reg.Register(connsMaxBackgroundPct); err != nil {
		return err
	}

	return nil
}
