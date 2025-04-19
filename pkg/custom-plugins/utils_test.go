package customplugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertToComponentName(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "nvidia",
			expected: "custom-plugin-nvidia",
		},
		{
			name:     "name with spaces",
			input:    "nvidia smi",
			expected: "custom-plugin-nvidia-smi",
		},
		{
			name:     "mixed case",
			input:    "Nvidia SMI",
			expected: "custom-plugin-nvidia-smi",
		},
		{
			name:     "already has prefix",
			input:    "custom-plugin-nvidia",
			expected: "custom-plugin-nvidia",
		},
		{
			name:     "whitespace trimming",
			input:    "  nvidia  ",
			expected: "custom-plugin-nvidia",
		},
		{
			name:     "mixed case with spaces and prefix",
			input:    "  Plugin-Nvidia SMI  ",
			expected: "custom-plugin-plugin-nvidia-smi",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ConvertToComponentName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
