package netutil

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultNetStatPath = "/proc/net/netstat"
	DefaultSNMPPath    = "/proc/net/snmp"
)

var (
	// ErrNoNetStatFile is returned when the netstat file path is empty.
	ErrNoNetStatFile = errors.New("netstat file path is required")

	// ErrNoSNMPFile is returned when the SNMP file path is empty.
	ErrNoSNMPFile = errors.New("snmp file path is required")
)

// NetStatCounters holds the subset of counters that the GPUd agent exports.
// These counters track network transmission errors and retransmissions.
type NetStatCounters struct {
	// Number of TCP segments retransmitted (from /proc/net/snmp Tcp:RetransSegs)
	// ref. "node_netstat_Tcp_RetransSegs" in prometheus/node_exporter/collector/netstat_linux.go
	TCPRetransSegments uint64
	// TCP retransmission events including fast retransmits (from /proc/net/netstat TcpExt:TCPSegRetrans)
	// ref. "node_netstat_TcpExt_TCPSegRetrans" in prometheus/node_exporter/collector/netstat_linux.go
	TcpExtSegmentRetransmits uint64
	// UDP packets that could not be delivered to an application (from /proc/net/snmp Udp:InErrors)
	// ref. "node_netstat_Udp_InErrors" in prometheus/node_exporter/collector/netstat_linux.go
	UDPInErrors uint64
	// UDP packets dropped due to receive buffer full (from /proc/net/snmp Udp:RcvbufErrors)
	// ref. "node_netstat_Udp_RcvbufErrors" in prometheus/node_exporter/collector/netstat_linux.go
	UDPRcvbufErrors uint64
	// UDP packets dropped due to send buffer full (from /proc/net/snmp Udp:SndbufErrors)
	// ref. "node_netstat_Udp_SndbufErrors" in prometheus/node_exporter/collector/netstat_linux.go
	UDPSndbufErrors uint64
}

// ReadNetStatCounters reads network statistics from the default procfs locations
// (/proc/net/netstat and /proc/net/snmp) and returns the aggregated counters.
//
// Inspired by github.com/prometheus/node_exporter/collector/netstat_linux.go which reads
// both files and merges the results.
func ReadNetStatCounters() (NetStatCounters, error) {
	return readNetStatCounters(DefaultNetStatPath, DefaultSNMPPath)
}

// readNetStatCounters aggregates counters from both netstat and snmp files.
// This is an internal function used for testing and requires both paths to be non-empty.
func readNetStatCounters(netstatPath, snmpPath string) (NetStatCounters, error) {
	if netstatPath == "" {
		return NetStatCounters{}, ErrNoNetStatFile
	}
	if snmpPath == "" {
		return NetStatCounters{}, ErrNoSNMPFile
	}

	// read /proc/net/netstat for TcpExt, IpExt, etc.
	parsed, err := readProcNetFile(netstatPath)
	if err != nil {
		return NetStatCounters{}, fmt.Errorf("netstat: %w", err)
	}

	// read /proc/net/snmp for Tcp, Udp, Ip, Icmp protocols.
	parsed2, err := readProcNetFile(snmpPath)
	if err != nil {
		return NetStatCounters{}, fmt.Errorf("snmp: %w", err)
	}

	stats := make(map[string]map[string]uint64)
	mergeProtocolMaps(stats, parsed)
	mergeProtocolMaps(stats, parsed2)

	// Extract counters with direct map lookups (defaults to 0 if not found)
	return NetStatCounters{
		TCPRetransSegments:       stats["Tcp"]["RetransSegs"],      // ref. node_netstat_Tcp_RetransSegs
		TcpExtSegmentRetransmits: stats["TcpExt"]["TCPSegRetrans"], // ref. node_netstat_TcpExt_TCPSegRetrans
		UDPInErrors:              stats["Udp"]["InErrors"],         // ref. node_netstat_Udp_InErrors
		UDPRcvbufErrors:          stats["Udp"]["RcvbufErrors"],     // ref. node_netstat_Udp_RcvbufErrors
		UDPSndbufErrors:          stats["Udp"]["SndbufErrors"],     // ref. node_netstat_Udp_SndbufErrors
	}, nil
}

func readProcNetFile(file string) (map[string]map[string]uint64, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseProcNet(f)
}

// parseProcNet converts the two-line procfs format into structured data.
// The implementation mirrors the Prometheus collector logic.
func parseProcNet(r io.Reader) (map[string]map[string]uint64, error) {
	result := make(map[string]map[string]uint64)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		header := strings.TrimSpace(scanner.Text())
		if header == "" {
			continue
		}
		fields := strings.Fields(header)
		if len(fields) == 0 {
			continue
		}

		protocolWithColon := fields[0]
		if !strings.HasSuffix(protocolWithColon, ":") {
			return nil, fmt.Errorf("protocol header %q missing trailing colon", protocolWithColon)
		}
		protocol := strings.TrimSuffix(protocolWithColon, ":")

		if !scanner.Scan() {
			return nil, fmt.Errorf("missing value line for protocol %q", protocol)
		}
		valuesLine := scanner.Text()
		values := strings.Fields(valuesLine)
		if len(fields) != len(values) {
			return nil, fmt.Errorf("field/value mismatch for protocol %q", protocol)
		}

		protoStats := make(map[string]uint64, len(fields)-1)
		for i := 1; i < len(fields); i++ {
			// Try parsing as int64 first to handle negative values (e.g., MaxConn: -1).
			// Negative values are treated as 0 since we store counters as uint64.
			// This matches the behavior of prometheus/node_exporter.
			val, err := strconv.ParseInt(values[i], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s_%s: %w", protocol, fields[i], err)
			}
			if val < 0 {
				// Negative values (like -1 for "unlimited") are stored as 0
				protoStats[fields[i]] = 0
			} else {
				protoStats[fields[i]] = uint64(val)
			}
		}

		result[protocol] = protoStats
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func mergeProtocolMaps(dst, src map[string]map[string]uint64) {
	for proto, counters := range src {
		target := dst[proto]
		if target == nil {
			target = make(map[string]uint64, len(counters))
			dst[proto] = target
		}
		for name, value := range counters {
			target[name] = value
		}
	}
}
