package types_test

import (
	"testing"

	infinibandtypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
)

func TestExpectedPortStates_IsZero(t *testing.T) {
	tests := []struct {
		name     string
		eps      *infinibandtypes.ExpectedPortStates
		expected bool
	}{
		{
			name:     "nil pointer",
			eps:      nil,
			expected: true,
		},
		{
			name:     "zero ports and zero rate",
			eps:      &infinibandtypes.ExpectedPortStates{AtLeastPorts: 0, AtLeastRate: 0},
			expected: true,
		},
		{
			name:     "negative ports and zero rate",
			eps:      &infinibandtypes.ExpectedPortStates{AtLeastPorts: -1, AtLeastRate: 0},
			expected: true,
		},
		{
			name:     "zero ports and positive rate",
			eps:      &infinibandtypes.ExpectedPortStates{AtLeastPorts: 0, AtLeastRate: 100},
			expected: true,
		},
		{
			name:     "positive ports and zero rate",
			eps:      &infinibandtypes.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 0},
			expected: true,
		},
		{
			name:     "positive ports and positive rate",
			eps:      &infinibandtypes.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			expected: false,
		},
		{
			name:     "valid configuration - 8 ports at 400 Gb/s",
			eps:      &infinibandtypes.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400},
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
