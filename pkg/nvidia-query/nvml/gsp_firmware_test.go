package nvml

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetGSPFirmwareMode(t *testing.T) {
	testCases := []struct {
		name                  string
		gspEnabled            bool
		gspSupported          bool
		gspFirmwareRet        nvml.Return
		expectedGSPFirmware   GSPFirmwareMode
		expectError           bool
		expectedErrorContains string
	}{
		{
			name:           "gsp enabled and supported",
			gspEnabled:     true,
			gspSupported:   true,
			gspFirmwareRet: nvml.SUCCESS,
			expectedGSPFirmware: GSPFirmwareMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   true,
				Supported: true,
			},
			expectError: false,
		},
		{
			name:           "gsp disabled but supported",
			gspEnabled:     false,
			gspSupported:   true,
			gspFirmwareRet: nvml.SUCCESS,
			expectedGSPFirmware: GSPFirmwareMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   false,
				Supported: true,
			},
			expectError: false,
		},
		{
			name:           "not supported",
			gspEnabled:     false,
			gspSupported:   false,
			gspFirmwareRet: nvml.ERROR_NOT_SUPPORTED,
			expectedGSPFirmware: GSPFirmwareMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   false,
				Supported: false,
			},
			expectError: false,
		},
		{
			name:                  "error case",
			gspEnabled:            false,
			gspSupported:          false,
			gspFirmwareRet:        nvml.ERROR_UNKNOWN,
			expectError:           true,
			expectedErrorContains: "failed to get gsp firmware mode",
		},
		{
			name:                  "GPU lost error",
			gspEnabled:            false,
			gspSupported:          false,
			gspFirmwareRet:        nvml.ERROR_GPU_IS_LOST,
			expectError:           true,
			expectedErrorContains: "gpu lost",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := testutil.CreateGSPFirmwareDevice(
				"test-uuid",
				tc.gspEnabled,
				tc.gspSupported,
				tc.gspFirmwareRet,
			)

			gspFirmware, err := GetGSPFirmwareMode("test-uuid", mockDevice)

			if tc.expectError {
				assert.Error(t, err)
				if tc.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorContains)
				}
				if tc.gspFirmwareRet == nvml.ERROR_GPU_IS_LOST {
					assert.True(t, errors.Is(err, ErrGPULost), "Expected GPU lost error")
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedGSPFirmware, gspFirmware)
			}
		})
	}
}

func TestValidateGSPFirmwareModeWithKernelConfig(t *testing.T) {
	testCases := []struct {
		name                string
		inputMode           GSPFirmwareMode
		kernelConfigContent string
		expectedMode        GSPFirmwareMode
		createFile          bool
	}{
		{
			name: "NVML enabled but kernel config disabled - should override to disabled",
			inputMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true,
				Supported: true,
			},
			kernelConfigContent: "options nvidia NVreg_EnableGpuFirmware=0\n",
			expectedMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   false, // Should be overridden to false
				Supported: true,
			},
			createFile: true,
		},
		{
			name: "NVML enabled and kernel config enabled - should remain enabled",
			inputMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true,
				Supported: true,
			},
			kernelConfigContent: "options nvidia NVreg_EnableGpuFirmware=1\n",
			expectedMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true, // Should remain true
				Supported: true,
			},
			createFile: true,
		},
		{
			name: "NVML disabled - no need to check kernel config",
			inputMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   false,
				Supported: true,
			},
			kernelConfigContent: "options nvidia NVreg_EnableGpuFirmware=1\n",
			expectedMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   false, // Should remain false (NVML takes precedence when disabled)
				Supported: true,
			},
			createFile: true,
		},
		{
			name: "Kernel config with tabs instead of spaces",
			inputMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true,
				Supported: true,
			},
			kernelConfigContent: "options	nvidia	NVreg_EnableGpuFirmware=0\n",
			expectedMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   false, // Should be overridden to false
				Supported: true,
			},
			createFile: true,
		},
		{
			name: "Kernel config with multiple parameters",
			inputMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true,
				Supported: true,
			},
			kernelConfigContent: "options nvidia NVreg_EnableGpuFirmware=0 NVreg_EnablePCIeGen3=1\n",
			expectedMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   false, // Should be overridden to false
				Supported: true,
			},
			createFile: true,
		},
		{
			name: "Kernel config with comments",
			inputMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true,
				Supported: true,
			},
			kernelConfigContent: "# This is a comment\noptions nvidia NVreg_EnableGpuFirmware=0\n# Another comment\n",
			expectedMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   false, // Should be overridden to false
				Supported: true,
			},
			createFile: true,
		},
		{
			name: "Kernel config file doesn't exist - trust NVML",
			inputMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true,
				Supported: true,
			},
			kernelConfigContent: "",
			expectedMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true, // Should remain true (trust NVML)
				Supported: true,
			},
			createFile: false,
		},
		{
			name: "Kernel config without GSP parameter - trust NVML",
			inputMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true,
				Supported: true,
			},
			kernelConfigContent: "options nvidia NVreg_EnablePCIeGen3=1\n",
			expectedMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true, // Should remain true (no GSP parameter)
				Supported: true,
			},
			createFile: true,
		},
		{
			name: "Empty kernel config file - trust NVML",
			inputMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true,
				Supported: true,
			},
			kernelConfigContent: "",
			expectedMode: GSPFirmwareMode{
				UUID:      "gpu-uuid-123",
				BusID:     "0000:01:00.0",
				Enabled:   true, // Should remain true (empty file)
				Supported: true,
			},
			createFile: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for the test
			tmpDir, err := os.MkdirTemp("", "gsp-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			configPath := filepath.Join(tmpDir, "nvidia.conf")

			if tc.createFile {
				// Create the kernel config file with test content
				err = os.WriteFile(configPath, []byte(tc.kernelConfigContent), 0644)
				require.NoError(t, err)
			}

			// Call the validation function
			result := ValidateGSPFirmwareModeWithKernelConfig(tc.inputMode, configPath)

			// Check the result
			assert.Equal(t, tc.expectedMode, result)
		})
	}
}

func TestValidateGSPFirmwareModeWithKernelConfig_RealWorldExample(t *testing.T) {
	// Test with a real-world example configuration that shows the problem
	tmpDir, err := os.MkdirTemp("", "gsp-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "nvidia.conf")

	// Create a config file that matches the user's reported configuration
	configContent := `# NVIDIA kernel module configuration
# Disable GSP firmware to avoid XID errors
options nvidia NVreg_EnableGpuFirmware=0
`
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Simulate NVML reporting GSP as enabled (the problem scenario)
	inputMode := GSPFirmwareMode{
		UUID:      "GPU-12345678-1234-1234-1234-123456789012",
		BusID:     "0000:3b:00.0",
		Enabled:   true, // NVML says enabled
		Supported: true,
	}

	// Validate against kernel config
	result := ValidateGSPFirmwareModeWithKernelConfig(inputMode, configPath)

	// Should be corrected to disabled based on kernel config
	assert.False(t, result.Enabled, "GSP should be disabled based on kernel config")
	assert.True(t, result.Supported, "GSP support status should not change")
	assert.Equal(t, inputMode.UUID, result.UUID, "UUID should not change")
	assert.Equal(t, inputMode.BusID, result.BusID, "BusID should not change")
}
