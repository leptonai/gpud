package product

import (
	"testing"
)

func TestGetProductNameByPCIDeviceID(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "H100 SXM5",
			input:    "2330",
			expected: "NVIDIA H100 80GB HBM3",
		},
		{
			name:     "H100 PCIe",
			input:    "2331",
			expected: "NVIDIA H100 PCIe",
		},
		{
			name:     "H100 PCIe with 0x prefix as read from sysfs",
			input:    "0x2331",
			expected: "NVIDIA H100 PCIe",
		},
		{
			name:     "device ID uppercase",
			input:    "0x2330",
			expected: "NVIDIA H100 80GB HBM3",
		},
		{
			name:     "device ID with surrounding whitespace",
			input:    " 2330\n",
			expected: "NVIDIA H100 80GB HBM3",
		},
		{
			name:     "A100 SXM4 80GB",
			input:    "20b2",
			expected: "NVIDIA A100-SXM4-80GB",
		},
		{
			name:     "A100 PCIe 80GB",
			input:    "20b5",
			expected: "NVIDIA A100 80GB PCIe",
		},
		{
			name:     "H200 SXM",
			input:    "2335",
			expected: "NVIDIA H200",
		},
		{
			name:     "B200",
			input:    "2901",
			expected: "NVIDIA B200",
		},
		{
			name:     "GB200",
			input:    "2941",
			expected: "NVIDIA GB200",
		},
		{
			name:     "L4",
			input:    "27b8",
			expected: "NVIDIA L4",
		},
		{
			name:     "L40S",
			input:    "26b9",
			expected: "NVIDIA L40S",
		},
		{
			name:     "RTX 4090",
			input:    "2684",
			expected: "NVIDIA GeForce RTX 4090",
		},
		{
			name:     "Tesla T4",
			input:    "1eb8",
			expected: "Tesla T4",
		},
		{
			name:     "A10G",
			input:    "2237",
			expected: "NVIDIA A10G",
		},
		{
			name:     "NVSwitch device is not a GPU",
			input:    "1af1",
			expected: "",
		},
		{
			name:     "unknown device ID",
			input:    "ffff",
			expected: "",
		},
		{
			name:     "empty device ID",
			input:    "",
			expected: "",
		},
		{
			name:     "only 0x prefix",
			input:    "0x",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := GetProductNameByPCIDeviceID(tc.input); got != tc.expected {
				t.Errorf("GetProductNameByPCIDeviceID(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// TestGetProductNameByPCIDeviceID_SanitizedMatchDocumentsThat every mapped
// product name round-trips through SanitizeProductName into the sanitized
// form that downstream consumers (e.g., the Lepton control plane) match on,
// consistent with what the NVML code path reports.
func TestGetProductNameByPCIDeviceID_SanitizedMatch(t *testing.T) {
	testCases := []struct {
		deviceID string
		expected string
	}{
		{deviceID: "2330", expected: "NVIDIA-H100-80GB-HBM3"},
		{deviceID: "2331", expected: "NVIDIA-H100-PCIe"},
		{deviceID: "2335", expected: "NVIDIA-H200"},
		{deviceID: "2901", expected: "NVIDIA-B200"},
		{deviceID: "2941", expected: "NVIDIA-GB200"},
		{deviceID: "20b2", expected: "NVIDIA-A100-SXM4-80GB"},
		{deviceID: "20b5", expected: "NVIDIA-A100-80GB-PCIe"},
		{deviceID: "2684", expected: "NVIDIA-GeForce-RTX-4090"},
		{deviceID: "1eb8", expected: "Tesla-T4"},
	}

	for _, tc := range testCases {
		t.Run(tc.deviceID, func(t *testing.T) {
			if got := SanitizeProductName(GetProductNameByPCIDeviceID(tc.deviceID)); got != tc.expected {
				t.Errorf("SanitizeProductName(GetProductNameByPCIDeviceID(%q)) = %q, want %q", tc.deviceID, got, tc.expected)
			}
		})
	}
}
