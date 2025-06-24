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

// Package class implements the infiniband class sysfs interface.
package class

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

const DefaultRootDir = "/sys/class/infiniband"

// LoadDevices loads all InfiniBand devices from the given root directory.
// If rootDir is empty, the default root directory is used.
func LoadDevices(rootDir string) (Devices, error) {
	if rootDir == "" {
		rootDir = DefaultRootDir
	}
	cd, err := newClassDirInterface(rootDir)
	if err != nil {
		return nil, err
	}
	return loadDevices(cd)
}

// loadDevices returns info for all InfiniBand devices read from
// /sys/class/infiniband.
func loadDevices(cd classDirInterface) (Devices, error) {
	dirs, err := cd.listDir("")
	if err != nil {
		return nil, err
	}

	ibc := make(Devices, 0, len(dirs))
	for _, d := range dirs {
		dev, err := parseInfiniBandDevice(cd, d.Name())
		if err != nil {
			return nil, err
		}
		ibc = append(ibc, dev)
	}

	sort.Slice(ibc, func(i, j int) bool {
		return ibc[i].Name < ibc[j].Name
	})
	return ibc, nil
}

// parseInfiniBandDevice parses one InfiniBand device.
// Refer to https://www.kernel.org/doc/Documentation/ABI/stable/sysfs-class-infiniband
func parseInfiniBandDevice(cd classDirInterface, deviceName string) (Device, error) {
	device := Device{Name: deviceName}

	// fw_ver is exposed by all InfiniBand drivers since kernel version 4.10.
	value, err := cd.readFile(filepath.Join(deviceName, "fw_ver"))
	if err != nil {
		return Device{}, fmt.Errorf("failed to read HCA firmware version: %w", err)
	}
	device.FirmwareVersion = value

	// Not all InfiniBand drivers expose all of these.
	for _, f := range [...]string{"board_id", "hca_type"} {
		value, err := cd.readFile(filepath.Join(deviceName, f))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Device{}, fmt.Errorf("failed to read file %q: %w", deviceName, err)
		}

		switch f {
		case "board_id":
			device.BoardID = value
		case "hca_type":
			device.HCAType = value
		}
	}

	portsDir := filepath.Join(deviceName, "ports")
	ports, err := cd.listDir(portsDir)
	if err != nil {
		return Device{}, fmt.Errorf("failed to list InfiniBand ports at %q: %w", portsDir, err)
	}

	device.Ports = make([]Port, 0, len(ports))
	for _, d := range ports {
		port, err := parseInfiniBandPort(cd, deviceName, d.Name())
		if err != nil {
			return Device{}, err
		}
		device.Ports = append(device.Ports, *port)
	}
	sort.Slice(device.Ports, func(i, j int) bool {
		return device.Ports[i].Port < device.Ports[j].Port
	})

	return device, nil
}

// parseState parses InfiniBand state. Expected format: "<id>: <string-representation>".
func parseState(s string) (uint, string, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("failed to split %s into 'ID: NAME'", s)
	}
	name := strings.TrimSpace(parts[1])
	value, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 32)
	if err != nil {
		return 0, name, fmt.Errorf("failed to convert %s into uint", strings.TrimSpace(parts[0]))
	}
	id := uint(value)
	return id, name, nil
}

// parseRate parses rate (example: "100 Gb/sec (4X EDR)") and return it as bytes/second.
// It returns the rate in GB/s and the rate in bytes/second.
func parseRate(s string) (float64, uint64, error) {
	parts := strings.SplitAfterN(s, " ", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("failed to split %q", s)
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 32)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to convert %s into uint", strings.TrimSpace(parts[0]))
	}

	// Convert Gb/s into bytes/s
	rate := uint64(value * 125000000)
	return value, rate, nil
}

// parseInfiniBandPort scans predefined files in /sys/class/infiniband/<device>/ports/<port>
// directory and gets their contents.
func parseInfiniBandPort(cd classDirInterface, portName string, port string) (*Port, error) {
	portNumber, err := strconv.ParseUint(port, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to convert %s into uint", port)
	}
	ibp := Port{Name: portName, Port: uint(portNumber)}

	// e.g., /sys/class/infiniband/mlx5_0/ports/1
	portDir := filepath.Join(portName, "ports", port)

	content, err := cd.readFile(filepath.Join(portDir, "link_layer"))
	if err != nil {
		return nil, err
	}
	ibp.LinkLayer = strings.TrimSpace(content)

	content, err = cd.readFile(filepath.Join(portDir, "state"))
	if err != nil {
		return nil, err
	}
	id, state, err := parseState(content)
	if err != nil {
		return nil, fmt.Errorf("could not parse state file in %q: %w", portDir, err)
	}
	ibp.State = strings.TrimSpace(state)
	ibp.StateID = id

	content, err = cd.readFile(filepath.Join(portDir, "phys_state"))
	if err != nil {
		return nil, err
	}
	id, physState, err := parseState(content)
	if err != nil {
		return nil, fmt.Errorf("could not parse phys_state file in %q: %w", portDir, err)
	}
	ibp.PhysState = physState
	ibp.PhysStateID = id

	content, err = cd.readFile(filepath.Join(portDir, "rate"))
	if err != nil {
		return nil, err
	}
	ibp.RateGBSec, ibp.Rate, err = parseRate(content)
	if err != nil {
		return nil, fmt.Errorf("could not parse rate file in %q: %w", portDir, err)
	}

	// Since the HCA may have been renamed by systemd, we cannot infer the kernel driver used by the
	// device, and thus do not know what type(s) of counters should be present. Attempt to parse
	// either / both "counters" (and potentially also "counters_ext"), and "hw_counters", subject
	// to their availability on the system - irrespective of HCA naming convention.

	// e.g., /sys/class/infiniband/mlx5_0/ports/1/counters
	countersDir := filepath.Join(portDir, "counters")
	exists, err := cd.exists(countersDir)
	if exists {
		counters, err := parseInfiniBandCounters(cd, portDir)
		if err != nil {
			return nil, err
		}
		ibp.Counters = *counters
	} else if err != nil {
		return nil, err
	}

	// e.g., /sys/class/infiniband/mlx5_0/ports/1/hw_counters
	hwCountersDir := filepath.Join(portDir, "hw_counters")
	exists, err = cd.exists(hwCountersDir)
	if exists {
		hwCounters, err := parseInfiniBandHwCounters(cd, portDir)
		if err != nil {
			return nil, err
		}
		ibp.HWCounters = *hwCounters
	} else if err != nil {
		return nil, err
	}

	return &ibp, nil
}

// parseInfiniBandCounters parses the counters exposed under
// /sys/class/infiniband/<device>/ports/<port-num>/counters, which first appeared in kernel v2.6.12.
// Prior to kernel v4.5, 64-bit counters were exposed separately under the "counters_ext" directory.
func parseInfiniBandCounters(cd classDirInterface, portDir string) (*Counters, error) {
	// e.g., /sys/class/infiniband/mlx5_0/ports/1/counters
	path := filepath.Join(portDir, "counters")
	files, err := cd.listDir(path)
	if err != nil {
		return nil, err
	}

	var counters Counters
	for _, f := range files {
		if !f.Type().IsRegular() {
			continue
		}

		name := filepath.Join(path, f.Name())
		value, err := cd.readFile(name)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) || err.Error() == "operation not supported" || errors.Is(err, os.ErrInvalid) || errors.Is(err, syscall.EINVAL) {
				continue
			}
			return nil, fmt.Errorf("failed to read file %q: %w", name, err)
		}

		// According to Mellanox, the metrics port_rcv_data, port_xmit_data,
		// port_rcv_data_64, and port_xmit_data_64 "are divided by 4 unconditionally"
		// as they represent the amount of data being transmitted and received per lane.
		// Mellanox cards have 4 lanes per port, so all values must be multiplied by 4
		// to get the expected value.

		vp := newValueParser(value)

		switch f.Name() {
		case "excessive_buffer_overrun_errors":
			counters.ExcessiveBufferOverrunErrors = vp.PUInt64()
		case "link_downed":
			counters.LinkDowned = vp.PUInt64()
		case "link_error_recovery":
			counters.LinkErrorRecovery = vp.PUInt64()
		case "local_link_integrity_errors":
			counters.LocalLinkIntegrityErrors = vp.PUInt64()
		case "multicast_rcv_packets":
			counters.MulticastRcvPackets = vp.PUInt64()
		case "multicast_xmit_packets":
			counters.MulticastXmitPackets = vp.PUInt64()
		case "port_rcv_constraint_errors":
			counters.PortRcvConstraintErrors = vp.PUInt64()
		case "port_rcv_data":
			counters.PortRcvData = vp.PUInt64()
			if counters.PortRcvData != nil {
				*counters.PortRcvData *= 4
			}
		case "port_rcv_discards":
			counters.PortRcvDiscards = vp.PUInt64()
		case "port_rcv_errors":
			counters.PortRcvErrors = vp.PUInt64()
		case "port_rcv_packets":
			counters.PortRcvPackets = vp.PUInt64()
		case "port_rcv_remote_physical_errors":
			counters.PortRcvRemotePhysicalErrors = vp.PUInt64()
		case "port_rcv_switch_relay_errors":
			counters.PortRcvSwitchRelayErrors = vp.PUInt64()
		case "port_xmit_constraint_errors":
			counters.PortXmitConstraintErrors = vp.PUInt64()
		case "port_xmit_data":
			counters.PortXmitData = vp.PUInt64()
			if counters.PortXmitData != nil {
				*counters.PortXmitData *= 4
			}
		case "port_xmit_discards":
			counters.PortXmitDiscards = vp.PUInt64()
		case "port_xmit_packets":
			counters.PortXmitPackets = vp.PUInt64()
		case "port_xmit_wait":
			counters.PortXmitWait = vp.PUInt64()
		case "symbol_error":
			counters.SymbolError = vp.PUInt64()
		case "unicast_rcv_packets":
			counters.UnicastRcvPackets = vp.PUInt64()
		case "unicast_xmit_packets":
			counters.UnicastXmitPackets = vp.PUInt64()
		case "VL15_dropped":
			counters.VL15Dropped = vp.PUInt64()
		}

		if err := vp.Err(); err != nil {
			// Ugly workaround for handling https://github.com/prometheus/node_exporter/issues/966
			// when counters are `N/A (not available)`.
			// This was already patched and submitted, see
			// https://www.spinics.net/lists/linux-rdma/msg68596.html
			// Remove this as soon as the fix lands in the enterprise distros.
			if strings.Contains(value, "N/A (no PMA)") {
				continue
			}
			return nil, err
		}
	}

	return &counters, nil
}

// parseInfiniBandHwCounters parses the optional counters exposed under
// /sys/class/infiniband/<device>/ports/<port-num>/hw_counters, which first appeared in kernel v4.6.
func parseInfiniBandHwCounters(cd classDirInterface, portDir string) (*HWCounters, error) {
	var hwCounters HWCounters

	// e.g., /sys/class/infiniband/mlx5_0/ports/1/hw_counters
	path := filepath.Join(portDir, "hw_counters")
	files, err := cd.listDir(path)
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		if !f.Type().IsRegular() {
			continue
		}

		file := filepath.Join(path, f.Name())
		value, err := cd.readFile(file)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) || err.Error() == "operation not supported" || errors.Is(err, os.ErrInvalid) {
				continue
			}
			return nil, fmt.Errorf("failed to read file %q: %w", file, err)
		}

		vp := newValueParser(value)

		switch f.Name() {
		case "duplicate_request":
			hwCounters.DuplicateRequest = vp.PUInt64()
		case "implied_nak_seq_err":
			hwCounters.ImpliedNakSeqErr = vp.PUInt64()
		case "lifespan":
			hwCounters.Lifespan = vp.PUInt64()
		case "local_ack_timeout_err":
			hwCounters.LocalAckTimeoutErr = vp.PUInt64()
		case "np_cnp_sent":
			hwCounters.NpCnpSent = vp.PUInt64()
		case "np_ecn_marked_roce_packets":
			hwCounters.NpEcnMarkedRocePackets = vp.PUInt64()
		case "out_of_buffer":
			hwCounters.OutOfBuffer = vp.PUInt64()
		case "out_of_sequence":
			hwCounters.OutOfSequence = vp.PUInt64()
		case "packet_seq_err":
			hwCounters.PacketSeqErr = vp.PUInt64()
		case "req_cqe_error":
			hwCounters.ReqCqeError = vp.PUInt64()
		case "req_cqe_flush_error":
			hwCounters.ReqCqeFlushError = vp.PUInt64()
		case "req_remote_access_errors":
			hwCounters.ReqRemoteAccessErrors = vp.PUInt64()
		case "req_remote_invalid_request":
			hwCounters.ReqRemoteInvalidRequest = vp.PUInt64()
		case "resp_cqe_error":
			hwCounters.RespCqeError = vp.PUInt64()
		case "resp_cqe_flush_error":
			hwCounters.RespCqeFlushError = vp.PUInt64()
		case "resp_local_length_error":
			hwCounters.RespLocalLengthError = vp.PUInt64()
		case "resp_remote_access_errors":
			hwCounters.RespRemoteAccessErrors = vp.PUInt64()
		case "rnr_nak_retry_err":
			hwCounters.RnrNakRetryErr = vp.PUInt64()
		case "roce_adp_retrans":
			hwCounters.RoceAdpRetrans = vp.PUInt64()
		case "roce_adp_retrans_to":
			hwCounters.RoceAdpRetransTo = vp.PUInt64()
		case "roce_slow_restart":
			hwCounters.RoceSlowRestart = vp.PUInt64()
		case "roce_slow_restart_cnps":
			hwCounters.RoceSlowRestartCnps = vp.PUInt64()
		case "roce_slow_restart_trans":
			hwCounters.RoceSlowRestartTrans = vp.PUInt64()
		case "rp_cnp_handled":
			hwCounters.RpCnpHandled = vp.PUInt64()
		case "rp_cnp_ignored":
			hwCounters.RpCnpIgnored = vp.PUInt64()
		case "rx_atomic_requests":
			hwCounters.RxAtomicRequests = vp.PUInt64()
		case "rx_dct_connect":
			hwCounters.RxDctConnect = vp.PUInt64()
		case "rx_icrc_encapsulated":
			hwCounters.RxIcrcEncapsulated = vp.PUInt64()
		case "rx_read_requests":
			hwCounters.RxReadRequests = vp.PUInt64()
		case "rx_write_requests":
			hwCounters.RxWriteRequests = vp.PUInt64()
		}

		if err := vp.Err(); err != nil {
			// Ugly workaround for handling https://github.com/prometheus/node_exporter/issues/966
			// when counters are `N/A (not available)`.
			// This was already patched and submitted, see
			// https://www.spinics.net/lists/linux-rdma/msg68596.html
			// Remove this as soon as the fix lands in the enterprise distros.
			if strings.Contains(value, "N/A (no PMA)") {
				continue
			}
			return nil, err
		}
	}
	return &hwCounters, nil
}
