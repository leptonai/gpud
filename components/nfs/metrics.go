package nfs

import (
	"github.com/prometheus/client_golang/prometheus"
	procnfs "github.com/prometheus/procfs/nfs"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const metricsSubsystem = "nfs"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricRPCRetransmissionsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: metricsSubsystem,
			Name:      "rpc_retransmissions_total",
			Help:      "tracks NFS RPC retransmissions observed via /proc/net/rpc/nfs (ref. node_nfs_rpc_retransmissions_total in prometheus/node_exporter/collector/nfs_linux.go)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(metricRPCRetransmissionsTotal)
}

func readDefaultRPCRetransmissions() (uint64, error) {
	// Derived from node_exporter's NFS collector (collector/nfs_linux.go)
	fs, err := procnfs.NewDefaultFS()
	if err != nil {
		return 0, err
	}

	stats, err := fs.ClientRPCStats()
	if err != nil {
		return 0, err
	}

	return stats.ClientRPC.Retransmissions, nil
}

func recordRPCRetransmissions(value uint64) {
	metricRPCRetransmissionsTotal.With(prometheus.Labels{}).Set(float64(value))
}
