package fabricmanager

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_countSMINVSwitches_A10(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A10 GPUs typically don't have NVSwitch in standard configurations
	command := "cat ./testdata/nvidia-smi.nvlink.status.a10"

	lines, err := countSMINVSwitches(ctx, command)
	require.NoError(t, err)
	require.Equal(t, 0, len(lines), "A10 systems should not have NVSwitch")
}

func Test_countSMINVSwitches_A100(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A100 systems with NVSwitch (DGX A100, HGX A100) have 8 GPUs
	command := "cat ./testdata/nvidia-smi.nvlink.status.a100"

	lines, err := countSMINVSwitches(ctx, command)
	require.NoError(t, err)
	require.Equal(t, 8, len(lines), "DGX/HGX A100 systems typically have 8 GPUs connected via NVSwitch")
	require.Contains(t, lines[0], "NVIDIA", "Output should contain NVIDIA GPU information")
}

func Test_listPCIs_A10(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A10 systems don't have NVSwitch
	command := "cat ./testdata/lspci.nn.a10"

	lines, err := listPCIs(ctx, command, isNVIDIANVSwitchPCI)
	require.NoError(t, err)
	require.Equal(t, 0, len(lines), "A10 systems should not have NVSwitch bridge devices")
}

func Test_listPCIs_A100(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A100 systems with NVSwitch have multiple bridge devices
	command := "cat ./testdata/lspci.nn.a100"

	lines, err := listPCIs(ctx, command, isNVIDIANVSwitchPCI)
	require.NoError(t, err)
	require.Equal(t, 6, len(lines), "DGX/HGX A100 systems typically have 6 NVSwitch bridge devices")
	require.Contains(t, lines[0], "NVIDIA", "Output should contain NVIDIA bridge information")
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
