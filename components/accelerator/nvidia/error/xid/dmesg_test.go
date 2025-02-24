package xid

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/leptonai/gpud/pkg/nvidia-query/xid"
)

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

func TestXidError_YAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		xidErr  XidError
		wantErr bool
	}{
		{
			name: "basic XID error",
			xidErr: XidError{
				Xid:        79,
				DeviceUUID: "PCI:0000:05:00",
				Detail:     &xid.Detail{Description: "GPU has fallen off the bus"},
			},
			wantErr: false,
		},
		{
			name: "XID error without detail",
			xidErr: XidError{
				Xid:        14,
				DeviceUUID: "0000:03:00",
				Detail:     nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.xidErr.YAML()
			if (err != nil) != tt.wantErr {
				t.Errorf("XidError.YAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var parsedXidErr XidError
			err = yaml.Unmarshal(got, &parsedXidErr)
			if err != nil {
				t.Errorf("Failed to parse generated YAML: %v", err)
				return
			}

			if !reflect.DeepEqual(tt.xidErr, parsedXidErr) {
				t.Errorf("XidError.YAML() roundtrip failed, got = %+v, want %+v", parsedXidErr, tt.xidErr)
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
