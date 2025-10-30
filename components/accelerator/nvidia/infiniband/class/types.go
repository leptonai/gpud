// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// modified from https://github.com/prometheus/procfs/blob/master/sysfs/class_infiniband.go

package class

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
)

// Counters contains counter values from files in
// /sys/class/infiniband/<Name>/ports/<Port>/counters or
// /sys/class/infiniband/<Name>/ports/<Port>/counters_ext
// for a single port of one InfiniBand device.
// ref. https://enterprise-support.nvidia.com/s/article/infiniband-port-counters
// ref. https://enterprise-support.nvidia.com/s/article/understanding-mlx5-linux-counters-and-status-parameters
type Counters struct {
	// ExcessiveBufferOverrunErrors from counters/excessive_buffer_overrun_errors - Number of times the port receiver detected an overrun of its receive buffer.
	// This indicates packets arriving faster than they can be processed, potentially causing data loss.
	ExcessiveBufferOverrunErrors *uint64 `json:"excessive_buffer_overrun_errors"`

	// LinkDowned from counters/link_downed - Number of times the link has gone down due to error thresholds being exceeded.
	// A high value indicates link instability and potential hardware or cabling issues.
	// "Total number of times the Port Training state machine has failed the link error recovery process and downed the link."
	LinkDowned *uint64 `json:"link_downed"`

	// LinkErrorRecovery from counters/link_error_recovery - Number of times the link recovered from an error condition.
	// This shows the port's ability to automatically recover from transient errors.
	LinkErrorRecovery *uint64 `json:"link_error_recovery"`

	// LocalLinkIntegrityErrors from counters/local_link_integrity_errors - Number of times the port detected a link integrity error.
	// These errors indicate problems with the physical link such as bad cables or connectors.
	LocalLinkIntegrityErrors *uint64 `json:"local_link_integrity_errors"`

	// MulticastRcvPackets from counters/multicast_rcv_packets - Total number of multicast packets received by this port.
	// Used for monitoring multicast traffic patterns and performance.
	MulticastRcvPackets *uint64 `json:"multicast_rcv_packets"`

	// MulticastXmitPackets from counters/multicast_xmit_packets - Total number of multicast packets transmitted from this port.
	// Used for monitoring outbound multicast traffic and performance.
	MulticastXmitPackets *uint64 `json:"multicast_xmit_packets"`

	// PortRcvConstraintErrors from counters/port_rcv_constraint_errors - Number of packets received that violated partition key or other constraints.
	// This indicates security policy violations or configuration issues.
	PortRcvConstraintErrors *uint64 `json:"port_rcv_constraint_errors"`

	// PortRcvData from counters/port_rcv_data - Total number of data octets (bytes) received by this port, divided by 4.
	// This is the primary metric for measuring inbound data throughput.
	PortRcvData *uint64 `json:"port_rcv_data"`

	// PortRcvDiscards from counters/port_rcv_discards - Number of inbound packets discarded due to lack of buffer space.
	// High values indicate receive buffer overflow and potential performance issues.
	PortRcvDiscards *uint64 `json:"port_rcv_discards"`

	// PortRcvErrors from counters/port_rcv_errors - Total number of packets received with errors (CRC, length, format, etc.).
	// This is a key indicator of link quality and hardware health.
	PortRcvErrors *uint64 `json:"port_rcv_errors"`

	// PortRcvPackets from counters/port_rcv_packets - Total number of packets received by this port.
	// This is the primary metric for measuring inbound packet rate.
	PortRcvPackets *uint64 `json:"port_rcv_packets"`

	// PortRcvRemotePhysicalErrors from counters/port_rcv_remote_physical_errors - Number of packets marked by the remote port as having physical layer errors.
	// This indicates problems with the remote end of the connection.
	PortRcvRemotePhysicalErrors *uint64 `json:"port_rcv_remote_physical_errors"`

	// PortRcvSwitchRelayErrors from counters/port_rcv_switch_relay_errors - Number of packets that could not be forwarded by the switch.
	// This indicates switch fabric congestion or routing issues.
	PortRcvSwitchRelayErrors *uint64 `json:"port_rcv_switch_relay_errors"`

	// PortXmitConstraintErrors from counters/port_xmit_constraint_errors - Number of outbound packets that violated partition key or other constraints.
	// This indicates attempts to send packets violating security policies.
	PortXmitConstraintErrors *uint64 `json:"port_xmit_constraint_errors"`

	// PortXmitData from counters/port_xmit_data - Total number of data octets (bytes) transmitted from this port, divided by 4.
	// This is the primary metric for measuring outbound data throughput.
	PortXmitData *uint64 `json:"port_xmit_data"`

	// PortXmitDiscards from counters/port_xmit_discards - Number of outbound packets discarded due to various reasons.
	// High values may indicate congestion or configuration issues.
	PortXmitDiscards *uint64 `json:"port_xmit_discards"`

	// PortXmitPackets from counters/port_xmit_packets - Total number of packets transmitted from this port.
	// This is the primary metric for measuring outbound packet rate.
	PortXmitPackets *uint64 `json:"port_xmit_packets"`

	// PortXmitWait from counters/port_xmit_wait - Number of ticks the port had to wait to transmit due to lack of credits.
	// High values indicate flow control issues and potential performance bottlenecks.
	PortXmitWait *uint64 `json:"port_xmit_wait"`

	// SymbolError from counters/symbol_error - Number of symbol errors detected on the physical link.
	// This indicates electrical signaling problems and is a key link health metric.
	SymbolError *uint64 `json:"symbol_error"`

	// UnicastRcvPackets from counters/unicast_rcv_packets - Total number of unicast packets received by this port.
	// Used for monitoring point-to-point traffic patterns and performance.
	UnicastRcvPackets *uint64 `json:"unicast_rcv_packets"`

	// UnicastXmitPackets from counters/unicast_xmit_packets - Total number of unicast packets transmitted from this port.
	// Used for monitoring outbound point-to-point traffic and performance.
	UnicastXmitPackets *uint64 `json:"unicast_xmit_packets"`

	// VL15Dropped from counters/VL15_dropped - Number of management packets (VL15) that were dropped.
	// VL15 is reserved for subnet management; drops indicate management plane issues.
	VL15Dropped *uint64 `json:"vl15_dropped"`
}

// HWCounters contains counter value from files in
// /sys/class/infiniband/<Name>/ports/<Port>/hw_counters
// for a single port of one InfiniBand device.
// These are hardware-specific counters typically found on modern RDMA NICs.
// ref. https://enterprise-support.nvidia.com/s/article/understanding-mlx5-linux-counters-and-status-parameters
type HWCounters struct {
	// DuplicateRequest from hw_counters/duplicate_request - Number of duplicate request packets received.
	// High values may indicate network issues or improper retry logic in applications.
	DuplicateRequest *uint64 `json:"duplicate_request"`

	// ImpliedNakSeqErr from hw_counters/implied_nak_seq_err - Number of implied NAK sequence errors.
	// These occur when the expected sequence number doesn't match, indicating packet loss or reordering.
	ImpliedNakSeqErr *uint64 `json:"implied_nak_seq_err"`

	// Lifespan from hw_counters/lifespan - Hardware-specific counter tracking connection lifespan metrics.
	// Used for monitoring connection duration and stability patterns.
	Lifespan *uint64 `json:"lifespan"`

	// LocalAckTimeoutErr from hw_counters/local_ack_timeout_err - Number of local acknowledgment timeout errors.
	// High values indicate the remote peer is not responding within expected timeframes.
	LocalAckTimeoutErr *uint64 `json:"local_ack_timeout_err"`

	// NpCnpSent from hw_counters/np_cnp_sent - Number of Congestion Notification Packets (CNP) sent by notification point.
	// Used in RoCE congestion control to signal network congestion to senders.
	NpCnpSent *uint64 `json:"np_cnp_sent"`

	// NpEcnMarkedRocePackets from hw_counters/np_ecn_marked_roce_packets - Number of RoCE packets marked with ECN (Explicit Congestion Notification).
	// Indicates congestion encountered in the network path for RoCE traffic.
	NpEcnMarkedRocePackets *uint64 `json:"np_ecn_marked_roce_packets"`

	// OutOfBuffer from hw_counters/out_of_buffer - Number of times operations failed due to insufficient buffer space.
	// High values indicate memory pressure or suboptimal buffer management.
	OutOfBuffer *uint64 `json:"out_of_buffer"`

	// OutOfSequence from hw_counters/out_of_sequence - Number of out-of-sequence packets received.
	// Indicates packet reordering in the network, which can impact performance.
	OutOfSequence *uint64 `json:"out_of_sequence"`

	// PacketSeqErr from hw_counters/packet_seq_err - Number of packet sequence errors detected.
	// These indicate problems with packet ordering or delivery reliability.
	PacketSeqErr *uint64 `json:"packet_seq_err"`

	// ReqCqeError from hw_counters/req_cqe_error - Number of completion queue errors on the requester side.
	// These indicate problems with RDMA request operations completion.
	ReqCqeError *uint64 `json:"req_cqe_error"`

	// ReqCqeFlushError from hw_counters/req_cqe_flush_error - Number of flushed completion queue entries on requester side.
	// Occurs when QP transitions to error state, flushing pending operations.
	ReqCqeFlushError *uint64 `json:"req_cqe_flush_error"`

	// ReqRemoteAccessErrors from hw_counters/req_remote_access_errors - Number of remote access errors on requester operations.
	// Indicates permission or protection violations when accessing remote memory.
	ReqRemoteAccessErrors *uint64 `json:"req_remote_access_errors"`

	// ReqRemoteInvalidRequest from hw_counters/req_remote_invalid_request - Number of invalid remote requests from requester side.
	// Indicates malformed or invalid RDMA operations attempted.
	ReqRemoteInvalidRequest *uint64 `json:"req_remote_invalid_request"`

	// RespCqeError from hw_counters/resp_cqe_error - Number of completion queue errors on the responder side.
	// These indicate problems with RDMA response operations completion.
	RespCqeError *uint64 `json:"resp_cqe_error"`

	// RespCqeFlushError from hw_counters/resp_cqe_flush_error - Number of flushed completion queue entries on responder side.
	// Occurs when QP transitions to error state, flushing pending responses.
	RespCqeFlushError *uint64 `json:"resp_cqe_flush_error"`

	// RespLocalLengthError from hw_counters/resp_local_length_error - Number of local length errors on responder side.
	// Indicates mismatched buffer sizes between local and remote operations.
	RespLocalLengthError *uint64 `json:"resp_local_length_error"`

	// RespRemoteAccessErrors from hw_counters/resp_remote_access_errors - Number of remote access errors on responder operations.
	// Indicates permission violations when remote peer attempts to access local memory.
	RespRemoteAccessErrors *uint64 `json:"resp_remote_access_errors"`

	// RnrNakRetryErr from hw_counters/rnr_nak_retry_err - Number of Receiver Not Ready (RNR) NAK retry errors.
	// Occurs when receiver lacks resources; high values indicate resource contention.
	RnrNakRetryErr *uint64 `json:"rnr_nak_retry_err"`

	// RoceAdpRetrans from hw_counters/roce_adp_retrans - Number of RoCE adaptive retransmissions.
	// Part of RoCE's adaptive retry mechanism for handling network congestion.
	RoceAdpRetrans *uint64 `json:"roce_adp_retrans"`

	// RoceAdpRetransTo from hw_counters/roce_adp_retrans_to - Number of RoCE adaptive retransmission timeouts.
	// Indicates network congestion severe enough to cause timeout-based retries.
	RoceAdpRetransTo *uint64 `json:"roce_adp_retrans_to"`

	// RoceSlowRestart from hw_counters/roce_slow_restart - Number of RoCE slow restart events.
	// Triggered by severe congestion, forcing connections to reduce transmission rate.
	RoceSlowRestart *uint64 `json:"roce_slow_restart"`

	// RoceSlowRestartCnps from hw_counters/roce_slow_restart_cnps - Number of CNPs that triggered RoCE slow restart.
	// Shows relationship between congestion signals and slow restart activation.
	RoceSlowRestartCnps *uint64 `json:"roce_slow_restart_cnps"`

	// RoceSlowRestartTrans from hw_counters/roce_slow_restart_trans - Number of RoCE slow restart transitions.
	// Tracks how often connections enter and exit slow restart mode.
	RoceSlowRestartTrans *uint64 `json:"roce_slow_restart_trans"`

	// RpCnpHandled from hw_counters/rp_cnp_handled - Number of CNP packets handled by the reaction point.
	// Shows how effectively the sender responds to congestion notifications.
	RpCnpHandled *uint64 `json:"rp_cnp_handled"`

	// RpCnpIgnored from hw_counters/rp_cnp_ignored - Number of CNP packets ignored by the reaction point.
	// High values may indicate congestion control configuration issues.
	RpCnpIgnored *uint64 `json:"rp_cnp_ignored"`

	// RxAtomicRequests from hw_counters/rx_atomic_requests - Number of atomic operation requests received.
	// Used for monitoring atomic RDMA operations like compare-and-swap, fetch-and-add.
	RxAtomicRequests *uint64 `json:"rx_atomic_requests"`

	// RxDctConnect from hw_counters/rx_dct_connect - Number of DCT (Dynamically Connected Transport) connections received.
	// DCT is used for scalable many-to-one communication patterns.
	RxDctConnect *uint64 `json:"rx_dct_connect"`

	// RxIcrcEncapsulated from hw_counters/rx_icrc_encapsulated - Number of packets received with encapsulated ICRC.
	// Used in certain tunneling scenarios to preserve end-to-end integrity.
	RxIcrcEncapsulated *uint64 `json:"rx_icrc_encapsulated"`

	// RxReadRequests from hw_counters/rx_read_requests - Number of RDMA read requests received.
	// Used for monitoring remote memory read operations initiated by peers.
	RxReadRequests *uint64 `json:"rx_read_requests"`

	// RxWriteRequests from hw_counters/rx_write_requests - Number of RDMA write requests received.
	// Used for monitoring remote memory write operations initiated by peers.
	RxWriteRequests *uint64 `json:"rx_write_requests"`
}

// Port contains info from files in
// /sys/class/infiniband/<Name>/ports/<Port>
// for a single port of one InfiniBand device.
// ref. https://enterprise-support.nvidia.com/s/article/understanding-mlx5-linux-counters-and-status-parameters
type Port struct {
	// Name is the name of the port from /sys/class/infiniband/<Name>/ports/<Port>.
	// e.g., "mlx5_0" for /sys/class/infiniband/mlx5_0/ports/1.
	Name string `json:"name"`

	// LinkLayer is the link layer from /sys/class/infiniband/<Name>/ports/<Port>/link_layer.
	// e.g., "InfiniBand" for /sys/class/infiniband/mlx5_0/ports/1/link_layer.
	LinkLayer string `json:"link_layer"`

	// Port is the port number from /sys/class/infiniband/<Name>/ports/<Port>.
	// e.g., 1 for /sys/class/infiniband/<Name>/ports/1.
	Port uint `json:"port"`

	// State is the string representation from /sys/class/infiniband/<Name>/ports/<Port>/state.
	// If "State" is "ACTIVE", the physical connection is up and working properly.
	// If "State" is "DOWN", the physical connection is down.
	// e.g., "ACTIVE" for "4: ACTIVE".
	State string `json:"state"`

	// StateID is the ID from /sys/class/infiniband/<Name>/ports/<Port>/state.
	// e.g., "4" for "4: ACTIVE".
	StateID uint `json:"state_id"`

	// PhysState is the string representation from /sys/class/infiniband/<Name>/ports/<Port>/phys_state.
	// If "PhysState" is "Polling", the "State" is likely "Down", meaning
	// the connecton is lost from this card to other cards/switches.
	// If "PhysState" is "LinkUp", the "State" is likely "Active", meaning
	// the connection is up physically thus connected to other cards/switches.
	// e.g., "LinkUp" for "5: LinkUp".
	PhysState string `json:"phys_state"`

	// PhysStateID is the ID from /sys/class/infiniband/<Name>/ports/<Port>/phys_state.
	// e.g., 5 for "5: LinkUp".
	PhysStateID uint `json:"phys_state_id"`

	// Rate is bytes/second value from /sys/class/infiniband/<Name>/ports/<Port>/rate.
	// e.g., 400 Gb/s * 125000000 = 50000000000 bytes/second.
	Rate uint64 `json:"rate"`

	// RateGBSec is the rate in GB/s.
	RateGBSec float64 `json:"rate_gb_sec"`

	// Counters is the counters from /sys/class/infiniband/<Name>/ports/<Port>/counters.
	Counters Counters `json:"counters"`

	// HWCounters is the hw counters from /sys/class/infiniband/<Name>/ports/<Port>/hw_counters.
	HWCounters HWCounters `json:"hw_counters"`
}

// Device contains info from files in /sys/class/infiniband for a
// single InfiniBand device.
// ref. https://enterprise-support.nvidia.com/s/article/understanding-mlx5-linux-counters-and-status-parameters
type Device struct {
	// Name is the name of the device from /sys/class/infiniband/<Name>.
	Name string `json:"name"`

	// BoardID is the board ID from /sys/class/infiniband/<Name>/board_id.
	// e.g., "MT_0000000838" for /sys/class/infiniband/mlx5_0/board_id.
	BoardID string `json:"board_id"`

	// FirmwareVersion is the firmware version from /sys/class/infiniband/<Name>/fw_ver.
	// e.g., "28.41.1000" for /sys/class/infiniband/mlx5_0/fw_ver.
	FirmwareVersion string `json:"fw_ver"`

	// HCAType is the HCA type from /sys/class/infiniband/<Name>/hca_type.
	// e.g., "MT4129" for /sys/class/infiniband/mlx5_0/hca_type.
	HCAType string `json:"hca_type"`

	// Ports maps the port number to the port info.
	Ports []Port `json:"ports"`
}

// Devices is a collection of every InfiniBand device in
// /sys/class/infiniband.
type Devices []Device

func (devs Devices) RenderTable(wr io.Writer) {
	for _, dev := range devs {
		dev.RenderTable(wr)
		_, _ = wr.Write([]byte("\n"))
	}
}

func (dev Device) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.Append([]string{"Device", dev.Name})
	table.Append([]string{"Board ID", dev.BoardID})
	table.Append([]string{"Firmware Version", dev.FirmwareVersion})

	for _, port := range dev.Ports {
		pfx := fmt.Sprintf("Port %d", port.Port)
		table.Append([]string{pfx + " Name", port.Name})
		table.Append([]string{pfx + " LinkLayer", port.LinkLayer})
		table.Append([]string{pfx + " State", port.State})
		table.Append([]string{pfx + " Phys State", port.PhysState})
		table.Append([]string{pfx + " Rate", fmt.Sprintf("%d Gb/sec", uint64(port.RateGBSec))})

		if port.Counters.LinkDowned != nil {
			table.Append([]string{pfx + " Link Downed", fmt.Sprintf("%d", *port.Counters.LinkDowned)})
		}
		if port.Counters.LinkErrorRecovery != nil {
			table.Append([]string{pfx + " Link Error Recovery", fmt.Sprintf("%d", *port.Counters.LinkErrorRecovery)})
		}
		if port.Counters.ExcessiveBufferOverrunErrors != nil {
			table.Append([]string{pfx + " Excessive Buffer Overrun Errors", fmt.Sprintf("%d", *port.Counters.ExcessiveBufferOverrunErrors)})
		}
	}

	table.Render()
}
