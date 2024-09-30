package xid

import "testing"

func TestExtractNVRMXid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "NVRM Xid match",
			input:    "NVRM: Xid critical error: 79, details follow",
			expected: 79,
		},
		{
			name:     "No match",
			input:    "Regular log content without Xid errors",
			expected: 0,
		},
		{
			name:     "NVRM Xid with non-numeric value",
			input:    "NVRM: Xid error: xyz, invalid data",
			expected: 0,
		},
		{
			name:     "error example",
			input:    "[111111111.111] NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
			expected: 79,
		},
		{
			name:     "error example",
			input:    "NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus.",
			expected: 79,
		},
		{
			name:     "error example",
			input:    "[...] NVRM: Xid (0000:03:00): 14, Channel 00000001",
			expected: 14,
		},

		// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#id3
		{
			name:     "Contained error with MIG enabled",
			input:    "NVRM: Xid (PCI:0000:01:00 GPU-I:05): 94, pid=7194, Contained: CE User Channel (0x9). RST: No, D-RST: No",
			expected: 94,
		},
		{
			name:     "Contained error with MIG disabled",
			input:    "NVRM: Xid (PCI:0000:01:00): 94, pid=7062, Contained: CE User Channel (0x9). RST: No, D-RST: No",
			expected: 94,
		},
		{
			name:     "Uncontained error",
			input:    "NVRM: Xid (PCI:0000:01:00): 95, pid=7062, Uncontained: LTC TAG (0x2,0x0). RST: Yes, D-RST: No",
			expected: 95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractNVRMXid(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractNVRMXid(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}
