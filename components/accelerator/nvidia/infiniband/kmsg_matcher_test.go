package infiniband

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, tt.want, HasPCIPowerInsufficient(tt.line), "HasPCIPowerInsufficient(%q)", tt.line)
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
		assert.Equal(t, tt.want, HasPortModuleHighTemperature(tt.line), "HasPortModuleHighTemperature(%q)", tt.line)
	}
}

// TestHasAccessRegFailed tests detection of ACCESS_REG command failures.
// These errors occur on systems with restricted InfiniBand Physical Functions (PFs),
// such as NVIDIA DGX or Umbriel systems with ConnectX-7 adapters.
// ref. https://github.com/prometheus/node_exporter/issues/3434
// ref. https://github.com/leptonai/gpud/issues/1164
func TestHasAccessRegFailed(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		// Real examples from production systems
		{line: "mlx5_cmd_out_err:838:(pid 1441871): ACCESS_REG(0x805) op_mod(0x1) failed, status bad resource(0x5), syndrome (0x305684), err(-22)", want: true},
		{line: "mlx5_core 0000:d2:00.0: mlx5_cmd_out_err:838:(pid 268269): ACCESS_REG(0x805) op_mod(0x1) failed", want: true},
		{line: "[Sun Dec 1 14:54:40 2024] mlx5_core 0000:d2:00.0: mlx5_cmd_out_err:838:(pid 268269): ACCESS_REG(0x805) op_mod(0x1) failed", want: true},
		{line: "hostname kernel: mlx5_core 0000:5c:00.0: mlx5_cmd_out_err:838:(pid 123): ACCESS_REG(0x805) op_mod(0x0) failed", want: true},
		// Variations
		{line: "mlx5_cmd_out_err: ACCESS_REG failed", want: true},
		{line: "something before mlx5_cmd_out_err:123:(pid 456): ACCESS_REG(0x123) failed something after", want: true},
		// Should not match
		{line: "mlx5_cmd_out_err:838:(pid 1441871): OTHER_CMD(0x123) failed", want: false},
		{line: "mlx5_core: ACCESS_REG successful", want: false},
		{line: "some other log message", want: false},
		{line: "", want: false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, HasAccessRegFailed(tt.line), "HasAccessRegFailed(%q)", tt.line)
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
			name:        "ACCESS_REG failed",
			line:        "mlx5_cmd_out_err:838:(pid 1441871): ACCESS_REG(0x805) op_mod(0x1) failed, status bad resource(0x5), syndrome (0x305684), err(-22)",
			wantEvent:   eventAccessRegFailed,
			wantMessage: messageAccessRegFailed,
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
			name:        "With timestamp - ACCESS_REG failed",
			line:        "[Sun Dec 1 14:54:40 2024] mlx5_core 0000:d2:00.0: mlx5_cmd_out_err:838:(pid 268269): ACCESS_REG(0x805) op_mod(0x1) failed",
			wantEvent:   eventAccessRegFailed,
			wantMessage: messageAccessRegFailed,
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
			assert.Equal(t, tt.wantEvent, gotEvent)
			assert.Equal(t, tt.wantMessage, gotMessage)
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
			assert.Equal(t, tt.want, HasPCIPowerInsufficient(tt.line), "HasPCIPowerInsufficient(%q)", tt.line)
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
			assert.Equal(t, tt.want, HasPortModuleHighTemperature(tt.line), "HasPortModuleHighTemperature(%q)", tt.line)
		}
	})
}

// TestLogLineProcessor tests the Match function with sample log lines
func TestLogLineProcessor(t *testing.T) {
	t.Parallel()

	// Test direct matching of log lines
	tests := []struct {
		name         string
		logLine      string
		expectMatch  bool
		expectedName string
		expectedMsg  string
	}{
		{
			name:         "PCI power insufficient",
			logLine:      "mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W).",
			expectMatch:  true,
			expectedName: "pci_power_insufficient",
			expectedMsg:  "Insufficient power on MLX5 PCIe slot",
		},
		{
			name:         "Port module high temperature",
			logLine:      "mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature",
			expectMatch:  true,
			expectedName: "port_module_high_temperature",
			expectedMsg:  "Overheated MLX5 adapter",
		},
		{
			// ACCESS_REG failures on restricted PFs (DGX, Umbriel, GB200)
			// ref. https://github.com/prometheus/node_exporter/issues/3434
			// ref. https://github.com/leptonai/gpud/issues/1164
			name:         "ACCESS_REG failed on restricted PF",
			logLine:      "mlx5_cmd_out_err:838:(pid 1441871): ACCESS_REG(0x805) op_mod(0x1) failed, status bad resource(0x5), syndrome (0x305684), err(-22)",
			expectMatch:  true,
			expectedName: "access_reg_failed",
			expectedMsg:  "MLX5 ACCESS_REG command failed - device may have restricted PF access",
		},
		{
			name:         "No match",
			logLine:      "Some unrelated log line",
			expectMatch:  false,
			expectedName: "",
			expectedMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, msg := Match(tt.logLine)
			if tt.expectMatch {
				assert.Equal(t, tt.expectedName, name)
				assert.Equal(t, tt.expectedMsg, msg)
			} else {
				assert.Empty(t, name)
				assert.Empty(t, msg)
			}
		})
	}
}

// Add more test variations for Match function
func TestLogLineProcessorWithMoreExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		logLine      string
		expectMatch  bool
		expectedName string
		expectedMsg  string
	}{
		{
			name:         "PCI power insufficient with different format",
			logLine:      "mlx5_core: mlx5_pcie_event: Detected insufficient power on the PCIe slot (15W).",
			expectMatch:  true,
			expectedName: "pci_power_insufficient",
			expectedMsg:  "Insufficient power on MLX5 PCIe slot",
		},
		{
			name:         "Non-matching InfiniBand line",
			logLine:      "mlx5_core: some other info that doesn't match patterns",
			expectMatch:  false,
			expectedName: "",
			expectedMsg:  "",
		},
		{
			name:         "Completely unrelated log",
			logLine:      "kernel: CPU temperature threshold exceeded",
			expectMatch:  false,
			expectedName: "",
			expectedMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, msg := Match(tt.logLine)
			if tt.expectMatch {
				assert.Equal(t, tt.expectedName, name)
				assert.Equal(t, tt.expectedMsg, msg)
			} else {
				assert.Empty(t, name)
				assert.Empty(t, msg)
			}
		})
	}
}
