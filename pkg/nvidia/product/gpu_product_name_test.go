package product

import (
	"testing"
)

func TestSanitizeProductName(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal gpu name",
			input:    "NVIDIA H100 80GB HBM3",
			expected: "NVIDIA-H100-80GB-HBM3",
		},
		{
			name:     "gpu name with special characters",
			input:    "NVIDIA H100-PCIe (ID=0x2331)",
			expected: "NVIDIA-H100-PCIe-ID0x2331",
		},
		{
			name:     "gpu name with leading and trailing whitespace",
			input:    "  NVIDIA A100 80GB  ",
			expected: "NVIDIA-A100-80GB",
		},
		{
			name:     "gpu name with multiple consecutive spaces",
			input:    "NVIDIA   GeForce   RTX   3090",
			expected: "NVIDIA-GeForce-RTX-3090",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "string with only special characters",
			input:    "!@#$%^&*()",
			expected: "",
		},
		{
			name:     "gpu name with periods and underscores",
			input:    "NVIDIA_Tesla.V100",
			expected: "NVIDIA_Tesla.V100",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeProductName(tc.input)
			if result != tc.expected {
				t.Errorf("SanitizeProductName(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}
