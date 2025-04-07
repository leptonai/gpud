package disk

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "disk"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "disk",
	}

	totalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)

	freeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)

	usedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)

	usedBytesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes_percent",
			Help:      "tracks the current disk bytes usage percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)

	usedInodesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_inodes_percent",
			Help:      "tracks the current disk inodes usage percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, pkgmetrics.MetricLabelKey}, // label is the mount point
	).MustCurryWith(componentLabel)
)

var _ components.PromRegisterer = &component{}

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	if err := reg.Register(totalBytes); err != nil {
		return err
	}
	if err := reg.Register(freeBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytes); err != nil {
		return err
	}
	if err := reg.Register(usedBytesPercent); err != nil {
		return err
	}
	if err := reg.Register(usedInodesPercent); err != nil {
		return err
	}
	return nil
}
