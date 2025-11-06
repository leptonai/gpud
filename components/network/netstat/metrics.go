package netstat

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "network_netstat"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	// metricTCPRetransSegmentsTotal tracks the cumulative number of TCP segments retransmitted.
	// This counter increases when TCP needs to retransmit data due to packet loss or timeouts,
	// indicating network reliability issues or congestion.
	//
	// Source: /proc/net/snmp Tcp:RetransSegs
	// Upstream metric: node_netstat_Tcp_RetransSegs
	// Reference: https://github.com/prometheus/node_exporter/blob/master/collector/netstat_linux.go
	metricTCPRetransSegmentsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: SubSystem,
			Name:      "tcp_retrans_segments_total",
			Help:      "Number of TCP segments retransmitted (from /proc/net/snmp Tcp:RetransSegs, upstream: node_netstat_Tcp_RetransSegs)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	// metricTCPExtSegmentRetransmitsTotal tracks TCP retransmission events including fast retransmits.
	// This is a more detailed counter than TCPRetransSegments, capturing additional retransmission
	// types such as SACK-based fast retransmits. High values indicate packet loss on the network.
	//
	// Source: /proc/net/netstat TcpExt:TCPSegRetrans
	// Upstream metric: node_netstat_TcpExt_TCPSegRetrans
	// Reference: https://github.com/prometheus/node_exporter/blob/master/collector/netstat_linux.go
	metricTCPExtSegmentRetransmitsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: SubSystem,
			Name:      "tcp_ext_segment_retransmits_total",
			Help:      "TCP retransmission events including fast retransmits (from /proc/net/netstat TcpExt:TCPSegRetrans, upstream: node_netstat_TcpExt_TCPSegRetrans)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	// metricUDPInErrorsTotal tracks UDP datagrams that could not be delivered to applications.
	// This includes malformed packets, checksum errors, and packets with no listening socket.
	// High values may indicate network corruption or misconfigured services.
	//
	// Source: /proc/net/snmp Udp:InErrors
	// Upstream metric: node_netstat_Udp_InErrors
	// Reference: https://github.com/prometheus/node_exporter/blob/master/collector/netstat_linux.go
	metricUDPInErrorsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: SubSystem,
			Name:      "udp_in_errors_total",
			Help:      "UDP packets that could not be delivered to an application (from /proc/net/snmp Udp:InErrors, upstream: node_netstat_Udp_InErrors)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	// metricUDPRcvbufErrorsTotal tracks UDP datagrams dropped due to receive buffer exhaustion.
	// This happens when UDP packets arrive faster than the application can read them, causing
	// the receive buffer to overflow. Increasing SO_RCVBUF or reading data faster can help.
	//
	// Source: /proc/net/snmp Udp:RcvbufErrors
	// Upstream metric: node_netstat_Udp_RcvbufErrors
	// Reference: https://github.com/prometheus/node_exporter/blob/master/collector/netstat_linux.go
	metricUDPRcvbufErrorsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: SubSystem,
			Name:      "udp_rcvbuf_errors_total",
			Help:      "UDP packets dropped due to receive buffer full (from /proc/net/snmp Udp:RcvbufErrors, upstream: node_netstat_Udp_RcvbufErrors)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)

	// metricUDPSndbufErrorsTotal tracks UDP datagrams dropped due to send buffer exhaustion.
	// This occurs when the application tries to send UDP packets faster than the kernel can
	// transmit them. Increasing SO_SNDBUF or reducing send rate can help mitigate this.
	//
	// Source: /proc/net/snmp Udp:SndbufErrors
	// Upstream metric: node_netstat_Udp_SndbufErrors
	// Reference: https://github.com/prometheus/node_exporter/blob/master/collector/netstat_linux.go
	metricUDPSndbufErrorsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: SubSystem,
			Name:      "udp_sndbuf_errors_total",
			Help:      "UDP packets dropped due to send buffer full (from /proc/net/snmp Udp:SndbufErrors, upstream: node_netstat_Udp_SndbufErrors)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricTCPRetransSegmentsTotal,
		metricTCPExtSegmentRetransmitsTotal,
		metricUDPInErrorsTotal,
		metricUDPRcvbufErrorsTotal,
		metricUDPSndbufErrorsTotal,
	)
}
