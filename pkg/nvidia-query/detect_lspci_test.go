package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_listNVIDIAPCIs_A10(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := "cat ./testdata/lspci.nn.a10"

	lines, err := listPCIs(ctx, command, isNVIDIAGPUPCI)
	require.NoError(t, err)
	require.Equal(t, 1, len(lines))
	require.Contains(t, lines[0], "NVIDIA")

	lines, err = listPCIs(ctx, command, isNVIDIANVSwitchPCI)
	require.NoError(t, err)
	require.Equal(t, 0, len(lines))
}

func Test_listNVIDIAPCIs_A100(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := "cat ./testdata/lspci.nn.a100"

	lines, err := listPCIs(ctx, command, isNVIDIAGPUPCI)
	require.NoError(t, err)
	require.Equal(t, 8, len(lines))
	require.Contains(t, lines[0], "NVIDIA")

	lines, err = listPCIs(ctx, command, isNVIDIANVSwitchPCI)
	require.NoError(t, err)
	require.Equal(t, 6, len(lines))
	require.Contains(t, lines[0], "NVIDIA")
}

func Test_isNVIDIANVSwitchPCI(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "older Bridge format",
			line:     "0005:00:00.0 Bridge [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)",
			expected: true,
		},
		{
			name:     "newer GB200 PCI bridge format",
			line:     "0018:00:00.0 PCI bridge [0604]: NVIDIA Corporation Device [10de:22b1]",
			expected: true,
		},
		{
			name:     "lowercase bridge",
			line:     "0005:00:00.0 bridge [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)",
			expected: true,
		},
		{
			name:     "uppercase BRIDGE",
			line:     "0005:00:00.0 BRIDGE [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)",
			expected: true,
		},
		{
			name:     "mixed case PCI Bridge",
			line:     "0018:00:00.0 PCI Bridge [0604]: NVIDIA Corporation Device [10de:22b1]",
			expected: true,
		},
		{
			name:     "GPU 3D controller should not match",
			line:     "000b:00:00.0 3D controller: NVIDIA Corporation GA100 [A100 SXM4 80GB] (rev a1)",
			expected: false,
		},
		{
			name:     "GPU with different format should not match",
			line:     "0000:01:00.0 VGA compatible controller: NVIDIA Corporation GA102 [GeForce RTX 3090]",
			expected: false,
		},
		{
			name:     "non-NVIDIA bridge should not match",
			line:     "0000:00:1c.0 PCI bridge: Intel Corporation Device [8086:a340] (rev f0)",
			expected: false,
		},
		{
			name:     "empty line should not match",
			line:     "",
			expected: false,
		},
		{
			name:     "line with only nvidia should not match",
			line:     "NVIDIA Corporation",
			expected: false,
		},
		{
			name:     "line with only bridge should not match",
			line:     "PCI bridge Device",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNVIDIANVSwitchPCI(tt.line)
			require.Equal(t, tt.expected, result, "isNVIDIANVSwitchPCI(%q) = %v, expected %v", tt.line, result, tt.expected)
		})
	}
}

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
