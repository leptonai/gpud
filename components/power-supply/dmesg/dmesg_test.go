package dmesg

import "testing"

func TestHasInsufficientPowerOnPCIe(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "ny2g1r12hh2 kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W).", want: true},
		{line: "[Sun Dec  1 14:54:40 2024] ny2g1r12hh2 kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W).", want: true},
		{line: "randomstring kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 123123): Detected insufficient power on the PCIe slot (27W).", want: true},
		{line: "[Sun Dec  1 14:54:40 2024] randomstring kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 123123): Detected insufficient power on the PCIe slot (27W).", want: true},
		{line: "ny2g1r12hh2 kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (30W).", want: true},
		{line: "[Sun Dec  1 14:54:40 2024] ny2g1r12hh2 kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (30W).", want: true},
		{line: "randomstring kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 123123): Detected insufficient power on the PCIe slot (30W).", want: true},
		{line: "[Sun Dec  1 14:54:40 2024] randomstring kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 123123): Detected insufficient power on the PCIe slot (30W).", want: true},
		{line: "some other log message", want: false},
		{line: "", want: false},
	}
	for _, tt := range tests {
		if got := HasInsufficientPowerOnPCIe(tt.line); got != tt.want {
			t.Errorf("HasInsufficientPowerOnPCIe(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}
