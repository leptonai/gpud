package types

import (
	"testing"
)

func TestExpectedPortStates_IsZero(t *testing.T) {
	tests := []struct {
		name     string
		eps      *ExpectedPortStates
		expected bool
	}{
		{
			name:     "nil pointer",
			eps:      nil,
			expected: true,
		},
		{
			name:     "zero ports and zero rate",
			eps:      &ExpectedPortStates{AtLeastPorts: 0, AtLeastRate: 0},
			expected: true,
		},
		{
			name:     "negative ports and zero rate",
			eps:      &ExpectedPortStates{AtLeastPorts: -1, AtLeastRate: 0},
			expected: true,
		},
		{
			name:     "zero ports and positive rate",
			eps:      &ExpectedPortStates{AtLeastPorts: 0, AtLeastRate: 100},
			expected: true,
		},
		{
			name:     "positive ports and zero rate",
			eps:      &ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 0},
			expected: true,
		},
		{
			name:     "positive ports and positive rate",
			eps:      &ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			expected: false,
		},
		{
			name:     "valid configuration - 8 ports at 400 Gb/s",
			eps:      &ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.eps.IsZero()
			if result != tt.expected {
				t.Errorf("IsZero() = %v, expected %v for %+v", result, tt.expected, tt.eps)
			}
		})
	}
}
