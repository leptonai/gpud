package os

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "os_fd"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
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

	metricZombieProcesses = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "zombie_processes",
			Help:      "tracks the total number of zombie processes",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricAllocatedFileHandles,
		metricRunningPIDs,
		metricLimit,
		metricAllocatedFileHandlesPercent,
		metricUsedPercent,
		metricThresholdRunningPIDs,
		metricThresholdRunningPIDsPercent,
		metricThresholdAllocatedFileHandles,
		metricThresholdAllocatedFileHandlesPercent,
		metricZombieProcesses,
	)
}
