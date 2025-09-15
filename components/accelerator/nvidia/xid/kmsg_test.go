package xid

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractNVRMXid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
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
			result, _ := ExtractNVRMXidInfo(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractNVRMXidInfo(%q) = %d, want %d", tt.input, result, tt.expected)
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
			_, result := ExtractNVRMXidInfo(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractNVRMXidInfo(%q) = %q, want %q", tt.input, result, tt.expected)
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
		{
			name:           "XID 149 NETIR_LINK_EVT Fatal error",
			input:          "[171167.620236] NVRM: Xid (PCI:0009:01:00): 149, NETIR_LINK_EVT  Fatal   XC0 i0 Link 01 (0x021425c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectNil:      false,
			expectedXid:    149,
			expectedDevice: "PCI:0009:01:00",
		},
		{
			name:           "XID 154 GPU recovery action changed",
			input:          "[171167.621162] NVRM: Xid (PCI:0009:01:00): 154, GPU recovery action changed from 0x0 (None) to 0x4 (Drain and Reset)",
			expectNil:      false,
			expectedXid:    154,
			expectedDevice: "PCI:0009:01:00",
		},
		{
			name:           "XID 149 with different device",
			input:          "[171167.630899] NVRM: Xid (PCI:0008:01:00): 149, NETIR_LINK_EVT  Fatal   XC0 i0 Link 01 (0x021425c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectNil:      false,
			expectedXid:    149,
			expectedDevice: "PCI:0008:01:00",
		},
		{
			name:           "XID 154 with different recovery action",
			input:          "[171167.897562] NVRM: Xid (PCI:0019:01:00): 154, GPU recovery action changed from 0x4 (Drain and Reset) to 0x1 (GPU Reset Required)",
			expectNil:      false,
			expectedXid:    154,
			expectedDevice: "PCI:0019:01:00",
		},
		{
			name:           "XID 149 different link number",
			input:          "[171167.969026] NVRM: Xid (PCI:0009:01:00): 149, NETIR_LINK_EVT  Fatal   XC0 i0 Link 00 (0x021405c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectNil:      false,
			expectedXid:    149,
			expectedDevice: "PCI:0009:01:00",
		},
		{
			name:           "XID 149 Link 15",
			input:          "[171168.109293] NVRM: Xid (PCI:0009:01:00): 149, NETIR_LINK_EVT  Fatal   XC0 i0 Link 15 (0x0215e5c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectNil:      false,
			expectedXid:    149,
			expectedDevice: "PCI:0009:01:00",
		},
		{
			name:           "XID 149 Link 14",
			input:          "[171168.409446] NVRM: Xid (PCI:0008:01:00): 149, NETIR_LINK_EVT  Fatal   XC0 i0 Link 14 (0x0215c5c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectNil:      false,
			expectedXid:    149,
			expectedDevice: "PCI:0008:01:00",
		},
		{
			name:           "XID 154 GPU1 recovery action",
			input:          "[171168.537473] NVRM: Xid (PCI:0018:01:00): 154, GPU recovery action changed from 0x4 (Drain and Reset) to 0x1 (GPU Reset Required)",
			expectNil:      false,
			expectedXid:    154,
			expectedDevice: "PCI:0018:01:00",
		},
		{
			name:           "XID 154 GPU0 recovery action",
			input:          "[171168.601459] NVRM: Xid (PCI:0008:01:00): 154, GPU recovery action changed from 0x4 (Drain and Reset) to 0x1 (GPU Reset Required)",
			expectNil:      false,
			expectedXid:    154,
			expectedDevice: "PCI:0008:01:00",
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

func TestMatchDmesgWithXid119(t *testing.T) {
	t.Parallel()

	// Read the test data file
	data, err := os.ReadFile("testdata/dmesg-with-xid-119.log")
	if err != nil {
		t.Fatalf("Failed to read test data file: %v", err)
	}

	// Split the file into lines
	lines := strings.Split(string(data), "\n")

	// Find all XID errors
	var xidErrors []*XidError
	for _, line := range lines {
		if xidErr := Match(line); xidErr != nil {
			xidErrors = append(xidErrors, xidErr)
		}
	}

	// Verify we found exactly 5 XID errors
	if len(xidErrors) != 5 {
		t.Errorf("Expected 5 XID errors, got %d", len(xidErrors))
	}

	// Verify each XID error
	expectedErrors := []struct {
		xid        int
		deviceUUID string
	}{
		{119, "PCI:0000:9b:00"}, // First nvidia-smi error
		{119, "PCI:0000:9b:00"}, // Second nvidia-smi error
		{119, "PCI:0000:9b:00"}, // Third nvidia-smi error
		{119, "PCI:0000:9b:00"}, // cache_mgr_main error
		{119, "PCI:0000:9b:00"}, // gpud error
	}

	for i, expected := range expectedErrors {
		if i >= len(xidErrors) {
			t.Errorf("Missing XID error at index %d", i)
			continue
		}

		actual := xidErrors[i]
		if actual.Xid != expected.xid {
			t.Errorf("XID error %d: expected Xid %d, got %d", i, expected.xid, actual.Xid)
		}
		if actual.DeviceUUID != expected.deviceUUID {
			t.Errorf("XID error %d: expected DeviceUUID %s, got %s", i, expected.deviceUUID, actual.DeviceUUID)
		}
		if actual.Detail == nil {
			t.Errorf("XID error %d: expected non-nil Detail", i)
		}
	}
}

func TestGetMessageToInject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		xid           int
		expectedXid   int
		expectedPrio  string
		shouldContain string
	}{
		{
			name:          "known XID 63",
			xid:           63,
			expectedXid:   63,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "Row remapping event",
		},
		{
			name:          "known XID 64",
			xid:           64,
			expectedXid:   64,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "Failed to persist row remap table",
		},
		{
			name:          "known XID 69",
			xid:           69,
			expectedXid:   69,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "BAR1 access failure",
		},
		{
			name:          "known XID 74",
			xid:           74,
			expectedXid:   74,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "MMU Fault",
		},
		{
			name:          "known XID 79",
			xid:           79,
			expectedXid:   79,
			expectedPrio:  "KERN_ERR",
			shouldContain: "GPU has fallen off the bus",
		},
		{
			name:          "unknown XID 42",
			xid:           42,
			expectedXid:   42,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "unknown",
		},
		{
			name:          "unknown XID 999",
			xid:           999,
			expectedXid:   999,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMessageToInject(tt.xid)

			// Check priority
			require.Equal(t, tt.expectedPrio, result.Priority)

			// Check that message contains expected substring
			require.NotEmpty(t, result.Message)

			if tt.shouldContain != "" {
				require.Contains(t, result.Message, tt.shouldContain)
			}
		})
	}
}

func TestGetMessageToInject_XidExtraction(t *testing.T) {
	t.Parallel()

	// Test all known XIDs
	knownXids := []int{63, 64, 69, 74, 79}

	for _, xid := range knownXids {
		t.Run(fmt.Sprintf("known_xid_%d", xid), func(t *testing.T) {
			msg := GetMessageToInject(xid)
			extractedXid := ExtractNVRMXid(msg.Message)

			require.NotZero(t, extractedXid, "ExtractNVRMXid failed to extract XID from message: %s", msg.Message)
			require.Equal(t, xid, extractedXid)
		})
	}

	// Test unknown XIDs
	unknownXids := []int{1, 25, 42, 100, 999}

	for _, xid := range unknownXids {
		t.Run(fmt.Sprintf("unknown_xid_%d", xid), func(t *testing.T) {
			msg := GetMessageToInject(xid)
			extractedXid := ExtractNVRMXid(msg.Message)

			require.NotZero(t, extractedXid, "ExtractNVRMXid failed to extract XID from message: %s", msg.Message)
			require.Equal(t, xid, extractedXid)
		})
	}
}

func TestGetMessageToInject_MessageFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		xid  int
	}{
		{"known XID 63", 63},
		{"known XID 79", 79},
		{"unknown XID 42", 42},
		{"unknown XID 999", 999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := GetMessageToInject(tt.xid)

			// All messages should start with "NVRM: Xid"
			require.True(t, strings.HasPrefix(msg.Message, "NVRM: Xid"), "GetMessageToInject(%d).Message should start with 'NVRM: Xid', got: %s", tt.xid, msg.Message)

			// All messages should contain PCI device information
			require.Contains(t, msg.Message, "PCI:0000:04:00")

			// Priority should be valid
			validPriorities := []string{"KERN_WARNING", "KERN_ERR", "KERN_INFO", "KERN_DEBUG"}
			require.Contains(t, validPriorities, msg.Priority)
		})
	}
}

func TestAllKnownXidsHaveValidMessages(t *testing.T) {
	t.Parallel()

	// Test every XID in the xidExampleMsgs map
	for xid := range xidExampleMsgs {
		t.Run(fmt.Sprintf("xid_%d", xid), func(t *testing.T) {
			msg := GetMessageToInject(xid)
			extractedXid := ExtractNVRMXid(msg.Message)

			require.NotZero(t, extractedXid, "XID %d: ExtractNVRMXid failed to extract XID from message: %s", xid, msg.Message)
			require.Equal(t, xid, extractedXid)
		})
	}
}

func TestExamplesWithExtractNVRMXid(t *testing.T) {
	t.Parallel()
	for xid, m := range xidExampleMsgs {
		t.Run(fmt.Sprintf("xid_%s", m.Message), func(t *testing.T) {
			extractedXid := ExtractNVRMXid(m.Message)
			require.Equal(t, xid, extractedXid)
		})
	}
}
