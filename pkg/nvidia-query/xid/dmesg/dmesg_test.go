package dmesg

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
			name:     "error example with PCI prefix",
			input:    "[111111111.111] NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
			expected: 79,
		},
		{
			name:     "error example without timestamp",
			input:    "NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus.",
			expected: 79,
		},
		{
			name:     "error example with channel",
			input:    "[...] NVRM: Xid (0000:03:00): 14, Channel 00000001",
			expected: 14,
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

func TestExtractNVRMXidDeviceUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "device ID without PCI prefix",
			input:    "[...] NVRM: Xid (0000:03:00): 14, Channel 00000001",
			expected: "0000:03:00",
		},
		{
			name:     "device ID with PCI prefix",
			input:    "[...] NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
			expected: "PCI:0000:05:00",
		},
		{
			name:     "device ID without PCI prefix without timestamp",
			input:    "NVRM: Xid (0000:03:00): 14, Channel 00000001",
			expected: "0000:03:00",
		},
		{
			name:     "device ID with PCI prefix without timestamp",
			input:    "NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
			expected: "PCI:0000:05:00",
		},
		{
			name:     "no device ID",
			input:    "Regular log content without Xid",
			expected: "",
		},
		{
			name:     "malformed device ID",
			input:    "NVRM: Xid (invalid): some error",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractNVRMXidDeviceUUID(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractNVRMXidDeviceUUID(%q) = %q, want %q", tt.input, result, tt.expected)
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
		expectedXid    int
		expectedDevice string
	}{
		{
			name:           "valid XID error with PCI prefix",
			input:          "NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
			expectNil:      false,
			expectedXid:    79,
			expectedDevice: "PCI:0000:05:00",
		},
		{
			name:           "valid XID error without PCI prefix",
			input:          "[...] NVRM: Xid (0000:03:00): 14, Channel 00000001",
			expectNil:      false,
			expectedXid:    14,
			expectedDevice: "0000:03:00",
		},
		{
			name:      "no XID error",
			input:     "Regular log content without Xid errors",
			expectNil: true,
		},
		{
			name:      "invalid XID number",
			input:     "NVRM: Xid error: xyz, invalid data",
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

			if result.Xid != tt.expectedXid {
				t.Errorf("Match(%q).Xid = %d, want %d", tt.input, result.Xid, tt.expectedXid)
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
