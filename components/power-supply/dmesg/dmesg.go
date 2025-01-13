package dmesg

import (
	"regexp"
)

const (
	// e.g.,
	// [...] ny2g1r12hh2 kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W).
	RegexInsufficientPowerOnPCIe = `Detected insufficient power on the PCIe slot \(([0-9]+W)\)`
)

var compiledInsufficientPowerOnPCIe = regexp.MustCompile(RegexInsufficientPowerOnPCIe)

// Returns true if the line indicates that the PCIe slot has insufficient power.
func HasInsufficientPowerOnPCIe(line string) bool {
	if match := compiledInsufficientPowerOnPCIe.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}
