package types

import (
	"testing"
)

func TestIBPort_IsIBPort(t *testing.T) {
	tests := []struct {
		name      string
		linkLayer string
		expected  bool
	}{
		{
			name:      "infiniband lowercase",
			linkLayer: "infiniband",
			expected:  true,
		},
		{
			name:      "infiniband uppercase",
			linkLayer: "INFINIBAND",
			expected:  true,
		},
		{
			name:      "infiniband mixed case",
			linkLayer: "InfiniBand",
			expected:  true,
		},
		{
			name:      "ethernet",
			linkLayer: "Ethernet",
			expected:  false,
		},
		{
			name:      "empty string",
			linkLayer: "",
			expected:  false,
		},
		{
			name:      "unknown",
			linkLayer: "Unknown",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := IBPort{
				LinkLayer: tt.linkLayer,
			}
			result := port.IsIBPort()
			if result != tt.expected {
				t.Errorf("IsIBPort() = %v, expected %v for LinkLayer %q", result, tt.expected, tt.linkLayer)
			}
		})
	}
}
