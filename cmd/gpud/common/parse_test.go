package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGPUUUIDs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single UUID",
			input:    "GPU-12345678-1234-1234-1234-123456789012",
			expected: []string{"GPU-12345678-1234-1234-1234-123456789012"},
		},
		{
			name:     "multiple UUIDs comma separated",
			input:    "GPU-12345678-1234-1234-1234-123456789012,GPU-87654321-4321-4321-4321-210987654321",
			expected: []string{"GPU-12345678-1234-1234-1234-123456789012", "GPU-87654321-4321-4321-4321-210987654321"},
		},
		{
			name:     "UUIDs with spaces",
			input:    "GPU-12345678-1234-1234-1234-123456789012, GPU-87654321-4321-4321-4321-210987654321",
			expected: []string{"GPU-12345678-1234-1234-1234-123456789012", "GPU-87654321-4321-4321-4321-210987654321"},
		},
		{
			name:     "UUIDs with extra spaces and commas",
			input:    " GPU-12345678-1234-1234-1234-123456789012 , , GPU-87654321-4321-4321-4321-210987654321 ,",
			expected: []string{"GPU-12345678-1234-1234-1234-123456789012", "GPU-87654321-4321-4321-4321-210987654321"},
		},
		{
			name:     "only commas and spaces",
			input:    ", , ,",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseGPUUUIDs(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
