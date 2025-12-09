package infiniband

import (
	"regexp"
)

const (
	// e.g.,
	// [...] ny2g1r12hh2 kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W).
	//
	// ref.
	// https://github.com/torvalds/linux/blob/ac9c34d1e45a4c25174ced4fc0cfc33ff3ed08c7/drivers/net/ethernet/mellanox/mlx5/core/events.c#L295-L299
	// https://docs.nvidia.com/networking/display/cx5vpisd/troubleshooting
	eventPCIPowerInsufficient   = "pci_power_insufficient"
	regexPCIPowerInsufficient   = `Detected insufficient power on the PCIe slot \(([0-9]+W)\)`
	messagePCIPowerInsufficient = "Insufficient power on MLX5 PCIe slot"

	// e.g.,
	// [...] mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature
	//
	// ref.
	// https://github.com/torvalds/linux/blob/ac9c34d1e45a4c25174ced4fc0cfc33ff3ed08c7/drivers/net/ethernet/mellanox/mlx5/core/events.c#L252-L254
	// https://forums.developer.nvidia.com/t/plug-in-the-connectx-5-first-time-is-very-hot-after-few-minutes/206632
	eventPortModuleHighTemperature   = "port_module_high_temperature"
	regexPortModuleHighTemperature   = `Port module event.*High Temperature`
	messagePortModuleHighTemperature = "Overheated MLX5 adapter"

	// ACCESS_REG (opcode 0x805) is an mlx5 firmware command used to read/write
	// hardware registers on Mellanox/NVIDIA InfiniBand adapters. When the mlx5
	// driver attempts to access registers on certain Physical Functions (PFs)
	// that are restricted by firmware (common on NVIDIA DGX, Umbriel systems,
	// and other converged systems with ConnectX-7), the kernel logs errors like:
	//
	// e.g.,
	// [...] mlx5_cmd_out_err:838:(pid 1441871): ACCESS_REG(0x805) op_mod(0x1) failed, status bad resource(0x5), syndrome (0x305684), err(-22)
	// [...] mlx5_core 0000:d2:00.0: mlx5_cmd_out_err:838:(pid 268269): ACCESS_REG(0x805) op_mod(0x1) failed
	//
	// These errors flood dmesg/kmsg when monitoring tools (like gpud, node_exporter)
	// read InfiniBand counter files from /sys/class/infiniband/mlx5_*/ports/*/counters/.
	// The counter reads trigger the driver to issue ACCESS_REG commands, which fail
	// on restricted PFs that are managed by the system firmware rather than the OS.
	//
	// WHY THIS HAPPENS:
	// On converged GPU+network systems (DGX, Umbriel, GB200), some InfiniBand ports
	// are "internal" (for NVLink/NVSwitch fabric) and their PFs have restricted access.
	// The firmware controls these ports directly, blocking userspace register access.
	//
	// SOLUTION:
	// Use --infiniband-exclude-devices flag to exclude problematic devices from monitoring.
	// Example: --infiniband-exclude-devices=mlx5_0,mlx5_1
	//
	// ref.
	// https://github.com/prometheus/node_exporter/issues/3434
	// https://github.com/leptonai/gpud/issues/1164
	// https://github.com/torvalds/linux/blob/master/drivers/net/ethernet/mellanox/mlx5/core/cmd.c
	eventAccessRegFailed   = "access_reg_failed"
	regexAccessRegFailed   = `mlx5_cmd_out_err.*ACCESS_REG.*failed`
	messageAccessRegFailed = "MLX5 ACCESS_REG command failed - device may have restricted PF access"
)

var (
	compiledPCIPowerInsufficient      = regexp.MustCompile(regexPCIPowerInsufficient)
	compiledPortModuleHighTemperature = regexp.MustCompile(regexPortModuleHighTemperature)
	compiledAccessRegFailed           = regexp.MustCompile(regexAccessRegFailed)
)

// HasPCIPowerInsufficient returns true if the line indicates that the power inefficient event has been detected.
// ref. https://github.com/torvalds/linux/blob/ac9c34d1e45a4c25174ced4fc0cfc33ff3ed08c7/drivers/net/ethernet/mellanox/mlx5/core/events.c#L295-L299
func HasPCIPowerInsufficient(line string) bool {
	if match := compiledPCIPowerInsufficient.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// HasPortModuleHighTemperature returns true if the line indicates that the port module high temperature event has been detected.
// ref. https://github.com/torvalds/linux/blob/ac9c34d1e45a4c25174ced4fc0cfc33ff3ed08c7/drivers/net/ethernet/mellanox/mlx5/core/events.c#L252-L254
func HasPortModuleHighTemperature(line string) bool {
	if match := compiledPortModuleHighTemperature.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// HasAccessRegFailed returns true if the line indicates that an ACCESS_REG command failed.
// This typically happens on systems with restricted InfiniBand Physical Functions (PFs),
// such as NVIDIA DGX or Umbriel systems with ConnectX-7 adapters.
//
// ref.
// https://github.com/prometheus/node_exporter/issues/3434
// https://github.com/leptonai/gpud/issues/1164
func HasAccessRegFailed(line string) bool {
	if match := compiledAccessRegFailed.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func Match(line string) (eventName string, message string) {
	for _, m := range getMatches() {
		if m.check(line) {
			return m.eventName, m.message
		}
	}
	return "", ""
}

type match struct {
	check     func(string) bool
	eventName string
	regex     string
	message   string
}

func getMatches() []match {
	return []match{
		{check: HasPCIPowerInsufficient, eventName: eventPCIPowerInsufficient, regex: regexPCIPowerInsufficient, message: messagePCIPowerInsufficient},
		{check: HasPortModuleHighTemperature, eventName: eventPortModuleHighTemperature, regex: regexPortModuleHighTemperature, message: messagePortModuleHighTemperature},
		{check: HasAccessRegFailed, eventName: eventAccessRegFailed, regex: regexAccessRegFailed, message: messageAccessRegFailed},
	}
}
