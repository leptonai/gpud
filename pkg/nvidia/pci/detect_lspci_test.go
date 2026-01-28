package pci

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/file"
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

// TestListPCIGPUs_Integration is an integration test that runs when lspci is available.
// This tests the full execution path including process creation and output parsing.
func TestListPCIGPUs_Integration(t *testing.T) {
	// Check if lspci is available
	lspciPath, err := file.LocateExecutable("lspci")
	if lspciPath == "" || err != nil {
		t.Skipf("lspci not found, skipping integration test: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// This will call the actual lspci command
	gpus, err := ListPCIGPUs(ctx)

	// Should not error even if no GPUs found
	require.NoError(t, err, "ListPCIGPUs should not error")

	// Log results for manual verification
	t.Logf("Found %d NVIDIA GPU(s)", len(gpus))
	for i, gpu := range gpus {
		t.Logf("  GPU %d: %s", i+1, gpu)
		// Verify output format matches expectations
		require.Contains(t, gpu, "NVIDIA", "GPU line should contain 'NVIDIA'")
		require.Contains(t, gpu, "3D controller", "GPU line should contain '3D controller'")
	}
}

// TestListPCIGPUs_ErrorHandling tests error scenarios without requiring actual lspci.
func TestListPCIGPUs_ErrorHandling(t *testing.T) {
	t.Run("context cancellation", func(t *testing.T) {
		// Check if lspci is available first
		lspciPath, err := file.LocateExecutable("lspci")
		if lspciPath == "" || err != nil {
			t.Skip("lspci not found, skipping test")
		}

		// Create an already-canceled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = ListPCIGPUs(ctx)
		// May or may not error depending on timing, but shouldn't panic
		t.Logf("Canceled context result: %v", err)
	})
}
