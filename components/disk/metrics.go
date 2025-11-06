package disk

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "disk"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricTotalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "total_bytes",
			Help:      "tracks the total bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "mount_point"}, // label is the mount point
	).MustCurryWith(componentLabel)

	metricFreeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "free_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "mount_point"}, // label is the mount point
	).MustCurryWith(componentLabel)

	metricUsedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "used_bytes",
			Help:      "tracks the current free bytes of the disk",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "mount_point"}, // label is the mount point
	).MustCurryWith(componentLabel)

	metricPressureIOFullSecondsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "pressure_io_full_seconds_total",
			Help:      "tracks time in seconds where IO pressure stalled all non-idle tasks",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricTotalBytes,
		metricFreeBytes,
		metricUsedBytes,
		metricPressureIOFullSecondsTotal,
	)
}

func readIOPressureFullSeconds() (float64, error) {
	// Derived from node_exporter's PSI collector (collector/pressure_linux.go).
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		return 0, err
	}

	stats, err := fs.PSIStatsForResource("io")
	if err != nil {
		return 0, err
	}

	if stats.Full == nil {
		return 0, fmt.Errorf("io pressure full stats not available")
	}

	return float64(stats.Full.Total) / 1_000_000.0, nil
}

func recordIOPressureFullSeconds(value float64) {
	metricPressureIOFullSecondsTotal.With(prometheus.Labels{}).Set(value)
}
