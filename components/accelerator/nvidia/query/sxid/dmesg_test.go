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
			name:     "SXid match",
			input:    "Some log content SXid error: 31, other info",
			expected: 31,
		},
		{
			name:     "No match",
			input:    "Regular log content without Xid errors",
			expected: 0,
		},
		{
			name:     "SXid with non-numeric value",
			input:    "SXid error: abc, invalid data",
			expected: 0,
		},
		{
			name:     "error example",
			input:    "[111111111.111] nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
			expected: 12028,
		},
		{
			// ref. https://access.redhat.com/solutions/6619941
			name:     "error example",
			input:    "[131453.740743] nvidia-nvswitch0: SXid (PCI:0000:a9:00.0): 20034, Fatal, Link 30 LTSSM Fault Up",
			expected: 20034,
		},
		{
			// ref. https://access.redhat.com/solutions/6619941
			name:     "error example",
			input:    "[131453.740754] nvidia-nvswitch0: SXid (PCI:0000:a9:00.0): 20034, Severity 1 Engine instance 30 Sub-engine instance 00",
			expected: 20034,
		},
		{
			// ref. https://access.redhat.com/solutions/6619941
			name:     "error example",
			input:    "[131453.740758] nvidia-nvswitch0: SXid (PCI:0000:a9:00.0): 20034, Data {0x50610002, 0x10100030, 0x00000000, 0x10100030, 0x00000000, 0x00000000, 0x00000000, 0x00000000}p",
			expected: 20034,
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
