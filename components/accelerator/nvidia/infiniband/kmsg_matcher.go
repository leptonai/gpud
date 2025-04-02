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
)

var (
	compiledPCIPowerInsufficient      = regexp.MustCompile(regexPCIPowerInsufficient)
	compiledPortModuleHighTemperature = regexp.MustCompile(regexPortModuleHighTemperature)
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
	}
}
