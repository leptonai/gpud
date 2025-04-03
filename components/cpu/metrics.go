package cpu

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "cpu"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "cpu",
	}

	// ref. https://www.digitalocean.com/community/tutorials/load-average-in-linux
	loadAverage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "load_average",
			Help:      "tracks the load average for the last period",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is last period
	).MustCurryWith(componentLabel)

	usedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the current file descriptor usage percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
)

var _ components.PromRegisterer = &component{}

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	return Register(reg, dbRW, dbRO, tableName)
}

func Register(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	if err := reg.Register(loadAverage); err != nil {
		return err
	}
	if err := reg.Register(usedPercent); err != nil {
		return err
	}
	return nil
}
