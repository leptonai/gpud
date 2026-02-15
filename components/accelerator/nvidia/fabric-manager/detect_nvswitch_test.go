package fabricmanager

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_countSMINVSwitches_A10(t *testing.T) {
	lines := filterFixture(t, "testdata/nvidia-smi.nvlink.status.a10", func(line string) bool {
		return strings.Contains(line, "GPU ") && strings.Contains(line, "NVIDIA") && strings.Contains(line, "UUID")
	})
	require.Equal(t, 0, len(lines), "A10 systems should not have NVSwitch")
}

func Test_countSMINVSwitches_A100(t *testing.T) {
	lines := filterFixture(t, "testdata/nvidia-smi.nvlink.status.a100", func(line string) bool {
		return strings.Contains(line, "GPU ") && strings.Contains(line, "NVIDIA") && strings.Contains(line, "UUID")
	})
	require.Equal(t, 8, len(lines), "DGX/HGX A100 systems typically have 8 GPUs connected via NVSwitch")
	require.Contains(t, lines[0], "NVIDIA", "Output should contain NVIDIA GPU information")
}

func Test_listPCIs_A10(t *testing.T) {
	lines := filterFixture(t, "testdata/lspci.nn.a10", func(line string) bool {
		return strings.Contains(strings.ToLower(line), DeviceVendorID) && isNVIDIANVSwitchPCI(line)
	})
	require.Equal(t, 0, len(lines), "A10 systems should not have NVSwitch bridge devices")
}

func Test_listPCIs_A100(t *testing.T) {
	lines := filterFixture(t, "testdata/lspci.nn.a100", func(line string) bool {
		return strings.Contains(strings.ToLower(line), DeviceVendorID) && isNVIDIANVSwitchPCI(line)
	})
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

// filterFixture reads a fixture file line by line and returns all lines matching the given predicate.
// This avoids shelling out to a subprocess, making the test fully deterministic.
func filterFixture(t *testing.T, relativePath string, matchFunc func(string) bool) []string {
	t.Helper()
	f, err := os.Open(relativePath)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if matchFunc(line) {
			lines = append(lines, line)
		}
	}
	require.NoError(t, scanner.Err())
	return lines
}
