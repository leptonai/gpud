package infiniband

import "testing"

func TestHasPCIPowerInsufficient(t *testing.T) {
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
		if got := HasPCIPowerInsufficient(tt.line); got != tt.want {
			t.Errorf("HasPCIPowerInsufficient(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestHasPortModuleHighTemperature(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature", want: true},
		{line: "[Sun Dec  1 14:54:40 2024] mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature", want: true},
		{line: "hostname kernel: mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature", want: true},
		{line: "[Sun Dec  1 14:54:40 2024] hostname kernel: mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature", want: true},
		{line: "mlx5_port_module_event:2345:(pid 1234): Port module event[warning]: module 1, Cable error, High Temperature", want: true},
		{line: "mlx5_core: Port module event[error]: module 2, High Temperature detected", want: true},
		{line: "some other log message", want: false},
		{line: "mlx5_port_module_event: Port module event[error]: module 0, Cable error, Low Signal", want: false},
		{line: "", want: false},
	}
	for _, tt := range tests {
		if got := HasPortModuleHighTemperature(tt.line); got != tt.want {
			t.Errorf("HasPortModuleHighTemperature(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantEvent   string
		wantMessage string
	}{
		{
			name:        "PCI power insufficient",
			line:        "ny2g1r12hh2 kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W).",
			wantEvent:   eventPCIPowerInsufficient,
			wantMessage: messagePCIPowerInsufficient,
		},
		{
			name:        "Port module high temperature",
			line:        "mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature",
			wantEvent:   eventPortModuleHighTemperature,
			wantMessage: messagePortModuleHighTemperature,
		},
		{
			name:        "With timestamp - PCI power insufficient",
			line:        "[Sun Dec 1 14:54:40 2024] ny2g1r12hh2 kernel: mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (30W).",
			wantEvent:   eventPCIPowerInsufficient,
			wantMessage: messagePCIPowerInsufficient,
		},
		{
			name:        "With timestamp - Port module high temperature",
			line:        "[Sun Dec 1 14:54:40 2024] hostname kernel: mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature",
			wantEvent:   eventPortModuleHighTemperature,
			wantMessage: messagePortModuleHighTemperature,
		},
		{
			name:        "No match",
			line:        "some random log line with no matching patterns",
			wantEvent:   "",
			wantMessage: "",
		},
		{
			name:        "Empty line",
			line:        "",
			wantEvent:   "",
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvent, gotMessage := Match(tt.line)
			if gotEvent != tt.wantEvent {
				t.Errorf("Match() gotEvent = %v, want %v", gotEvent, tt.wantEvent)
			}
			if gotMessage != tt.wantMessage {
				t.Errorf("Match() gotMessage = %v, want %v", gotMessage, tt.wantMessage)
			}
		})
	}
}

// Edge cases and special patterns
func TestEdgeCases(t *testing.T) {
	t.Run("PCI power with different wattage formats", func(t *testing.T) {
		tests := []struct {
			line string
			want bool
		}{
			{line: "mlx5_core: Detected insufficient power on the PCIe slot (1W).", want: true},
			{line: "mlx5_core: Detected insufficient power on the PCIe slot (999W).", want: true},
			{line: "mlx5_core: Detected insufficient power on the PCIe slot (0W).", want: true},
			// Should not match - invalid format
			{line: "mlx5_core: Detected insufficient power on the PCIe slot (W).", want: false},
			{line: "mlx5_core: Detected insufficient power on the PCIe slot (27.5W).", want: false},
			{line: "mlx5_core: Detected insufficient power on the PCIe slot (27 W).", want: false},
		}

		for _, tt := range tests {
			if got := HasPCIPowerInsufficient(tt.line); got != tt.want {
				t.Errorf("HasPCIPowerInsufficient(%q) = %v, want %v", tt.line, got, tt.want)
			}
		}
	})

	t.Run("High temperature with different patterns", func(t *testing.T) {
		tests := []struct {
			line string
			want bool
		}{
			{line: "Port module event: High Temperature alert detected", want: true},
			{line: "Port module event[error]: Severe High Temperature", want: true},
			{line: "Port module event - module 0: High Temperature warning", want: true},
			// Should not match
			{line: "Port module event: Temperature normal", want: false},
			{line: "High Temperature detected but not in a port module event", want: false},
		}

		for _, tt := range tests {
			if got := HasPortModuleHighTemperature(tt.line); got != tt.want {
				t.Errorf("HasPortModuleHighTemperature(%q) = %v, want %v", tt.line, got, tt.want)
			}
		}
	})
}
