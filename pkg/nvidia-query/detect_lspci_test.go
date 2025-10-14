package query

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_isNVIDIAGPUPCI(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "A100 GPU 3D controller",
			line:     "000b:00:00.0 3D controller: NVIDIA Corporation GA100 [A100 SXM4 80GB] (rev a1)",
			expected: true,
		},
		{
			name:     "H100 GPU 3D controller",
			line:     "0001:00:00.0 3D controller: NVIDIA Corporation GH100 [H100 PCIe] (rev a1)",
			expected: true,
		},
		{
			name:     "uppercase 3D CONTROLLER",
			line:     "000b:00:00.0 3D CONTROLLER: NVIDIA Corporation GA100 [A100 SXM4 80GB] (rev a1)",
			expected: false, // function checks for exact case "3D controller"
		},
		{
			name:     "NVSwitch bridge should not match",
			line:     "0005:00:00.0 Bridge [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)",
			expected: false,
		},
		{
			name:     "PCI bridge should not match",
			line:     "0018:00:00.0 PCI bridge [0604]: NVIDIA Corporation Device [10de:22b1]",
			expected: false,
		},
		{
			name:     "non-NVIDIA 3D controller should not match",
			line:     "0000:01:00.0 3D controller: AMD Corporation Device",
			expected: false,
		},
		{
			name:     "empty line should not match",
			line:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNVIDIAGPUPCI(tt.line)
			require.Equal(t, tt.expected, result, "isNVIDIAGPUPCI(%q) = %v, expected %v", tt.line, result, tt.expected)
		})
	}
}
