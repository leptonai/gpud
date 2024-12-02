package fd

import (
	"testing"
)

func TestCalculateUsedPercent(t *testing.T) {
	tests := []struct {
		name     string
		usage    uint64
		limit    uint64
		expected float64
	}{
		{"Zero usage", 0, 100, 0},
		{"Half usage", 50, 100, 50},
		{"Full usage", 100, 100, 100},
		{"Over usage", 150, 100, 150},
		{"Zero limit", 50, 0, 0},
		{"Large numbers", 1000000, 10000000, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcUsagePct(tt.usage, tt.limit)
			if result != tt.expected {
				t.Errorf("calculateUsedPercent(%d, %d) = %f; want %f", tt.usage, tt.limit, result, tt.expected)
			}
		})
	}
}
