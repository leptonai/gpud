package sxid

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

// SubSystem is the Prometheus subsystem name for SXID metrics.
const SubSystem = "accelerator_nvidia_sxid"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	// Severity is intentionally not a label: kernel lines often lack the
	// Non-fatal/Fatal keyword (e.g. "SXid ...: 20034, Data {0x...}"), so a
	// per-line severity would be partly invented. Severity is a function of
	// the code anyway — grade by the sxid label against the fabric-manager
	// guide matrix.
	metricSXIDErrs = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "errors_total",
			Help: "counts SXID error events per SXID code and reporting NVSwitch " +
				"(device_pci). Repeats of the same switch and code within the same " +
				"kernel-log second collapse into one count. Link- and engine-level details " +
				"stay in the raw kernel log line, which GPUd logs on every match.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey,
			"sxid", // label is the SXID error code
			// label is the reporting NVSwitch PCI address, kept in the
			// kernel-log form with the "PCI:" prefix, e.g. "PCI:0000:05:00.0".
			"device_pci",
		},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricSXIDErrs,
	)
}

// recordSXIDErrsMetric counts one SXID event in the
// accelerator_nvidia_sxid_errors_total metric.
// Aggregation does not happen here: the caller only invokes it after the
// event survives the event-store dedup (same kernel-log second and same
// switch fold into one insert), so the counter never tracks raw log volume.
func recordSXIDErrsMetric(sxid int, devicePCI string) {
	metricSXIDErrs.With(prometheus.Labels{
		"sxid":       strconv.Itoa(sxid),
		"device_pci": devicePCI,
	}).Inc()
}
