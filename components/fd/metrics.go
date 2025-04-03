package fd

import (
	"context"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "fd"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "fd",
	}

	metricAllocatedFileHandles = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "allocated_file_handles",
			Help:      "tracks the total number of allocated file handles (e.g., /proc/sys/fs/file-nr on Linux)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricRunningPIDs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "running_pids",
			Help:      "tracks the total number of running pids, current file descriptor usage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "limit",
			Help:      "tracks the current system-wide file descriptor limit (e.g., /proc/sys/fs/file-max on Linux)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricAllocatedFileHandlesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "allocated_file_handles_percent",
			Help:      "tracks the current file descriptor allocation percentage (allocated_file_handles / limit in percentage)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_percent",
			Help:      "tracks the current file descriptor usage percentage (running_pids / limit in percentage)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricThresholdRunningPIDs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_running_pids",
			Help:      "tracks the current file descriptor threshold running pids",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
	metricThresholdRunningPIDsPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_running_pids_percent",
			Help:      "tracks the current file descriptor threshold running pids percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	metricThresholdAllocatedFileHandles = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_allocated_file_handles",
			Help:      "tracks the current file descriptor threshold allocated file handles",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
	metricThresholdAllocatedFileHandlesPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "threshold_allocated_file_handles_percent",
			Help:      "tracks the current file descriptor threshold allocated percentage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
)

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	return Register(reg)
}

// TO BE DEPRECATED
func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func Register(reg *prometheus.Registry) error {
	if err := reg.Register(metricAllocatedFileHandles); err != nil {
		return err
	}
	if err := reg.Register(metricRunningPIDs); err != nil {
		return err
	}
	if err := reg.Register(metricLimit); err != nil {
		return err
	}
	if err := reg.Register(metricAllocatedFileHandlesPercent); err != nil {
		return err
	}
	if err := reg.Register(metricUsedPercent); err != nil {
		return err
	}
	if err := reg.Register(metricThresholdRunningPIDs); err != nil {
		return err
	}
	if err := reg.Register(metricThresholdRunningPIDsPercent); err != nil {
		return err
	}
	if err := reg.Register(metricThresholdAllocatedFileHandles); err != nil {
		return err
	}
	if err := reg.Register(metricThresholdAllocatedFileHandlesPercent); err != nil {
		return err
	}
	return nil
}
