package sxid

import (
	"testing"
)

func TestExtractNVSwitchSXid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "valid NVSwitch SXid error",
			input:    "[111111111.111] nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
			expected: 12028,
		},
		{
			name:     "another valid NVSwitch SXid error",
			input:    "[131453.740743] nvidia-nvswitch0: SXid (PCI:0000:00:00.0): 20034, Fatal, Link 30 LTSSM Fault Up",
			expected: 20034,
		},
		{
			name:     "NVSwitch SXid error without timestamp",
			input:    "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error",
			expected: 12028,
		},
		{
			name:     "no match",
			input:    "Regular log content without SXid errors",
			expected: 0,
		},
		{
			name:     "NVSwitch SXid with non-numeric value",
			input:    "nvidia-nvswitch0: SXid (PCI:0000:00:00.0): xyz, Fatal error",
			expected: 0,
		},
		{
			name:     "NVSwitch SXid with data payload",
			input:    "[131453.740758] nvidia-nvswitch0: SXid (PCI:0000:a9:00.0): 20034, Data {0x50610002, 0x10100030}",
			expected: 20034,
		},
		{
			name:     "NVSwitch SXid with unknown code",
			input:    "[131453.740758] nvidia-nvswitch0: SXid (PCI:0000:a9:00.0): 11111, Data {0x50610002, 0x10100030}",
			expected: 11111,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractNVSwitchSXid(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractNVSwitchSXid(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractNVSwitchSXidDeviceUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid device ID with timestamp",
			input:    "[111111111.111] nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error",
			expected: "PCI:0000:05:00.0",
		},
		{
			name:     "valid device ID without timestamp",
			input:    "nvidia-nvswitch0: SXid (PCI:0000:00:00.0): 20034, Fatal, Link 30 LTSSM Fault Up",
			expected: "PCI:0000:00:00.0",
		},
		{
			name:     "no device ID",
			input:    "Regular log content without SXid",
			expected: "",
		},
		{
			name:     "malformed device ID",
			input:    "nvidia-nvswitch0: SXid (invalid): some error",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractNVSwitchSXidDeviceUUID(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractNVSwitchSXidDeviceUUID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		expectNil      bool
		expectedSXid   int
		expectedDevice string
	}{
		{
			name:           "valid NVSwitch SXid error",
			input:          "[111111111.111] nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error",
			expectNil:      false,
			expectedSXid:   12028,
			expectedDevice: "PCI:0000:05:00.0",
		},
		{
			name:           "another valid NVSwitch SXid error",
			input:          "[131453.740743] nvidia-nvswitch0: SXid (PCI:0000:00:00.0): 20034, Fatal, Link 30 LTSSM Fault Up",
			expectNil:      false,
			expectedSXid:   20034,
			expectedDevice: "PCI:0000:00:00.0",
		},
		{
			name:      "no SXid error",
			input:     "Regular log content without SXid errors",
			expectNil: true,
		},
		{
			name:      "invalid SXid number",
			input:     "nvidia-nvswitch0: SXid (PCI:0000:00:00.0): xyz, Fatal error",
			expectNil: true,
		},
		{
			name:      "unknown SXid code",
			input:     "[131453.740758] nvidia-nvswitch0: SXid (PCI:0000:a9:00.0): 11111, Data {0x50610002, 0x10100030}",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Match(tt.input)
			if tt.expectNil {
				if result != nil {
					t.Errorf("Match(%q) = %+v, want nil", tt.input, result)
				}
				return
			}

			if result == nil {
				t.Fatalf("Match(%q) = nil, want non-nil", tt.input)
			}

			if result.SXid != tt.expectedSXid {
				t.Errorf("Match(%q).SXid = %d, want %d", tt.input, result.SXid, tt.expectedSXid)
			}

			if result.DeviceUUID != tt.expectedDevice {
				t.Errorf("Match(%q).DeviceUUID = %q, want %q", tt.input, result.DeviceUUID, tt.expectedDevice)
			}

			if result.Detail == nil {
				t.Errorf("Match(%q).Detail = nil, want non-nil", tt.input)
			}
		})
	}
}
