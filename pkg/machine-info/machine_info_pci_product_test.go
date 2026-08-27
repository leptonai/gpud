package machineinfo

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nvidiapci "github.com/leptonai/gpud/pkg/nvidia/pci"
)

// stubListNVIDIAGPUDevices replaces the PCI device listing used by the
// product-name fallback and returns a restore function.
func stubListNVIDIAGPUDevices(devs []nvidiapci.GPUDevice, err error) func() {
	original := listNVIDIAGPUDevicesFunc
	listNVIDIAGPUDevicesFunc = func() ([]nvidiapci.GPUDevice, error) {
		return devs, err
	}
	return func() { listNVIDIAGPUDevicesFunc = original }
}

// TestGetMachineGPUInfo_PCIProductFallback covers LEP-6173: when the NVIDIA
// driver is installed and managed by the GPU Operator, the NVML userspace
// library is unavailable to gpud during initial node discovery, so the NVML
// product name is empty. The PCI sysfs fallback must still report the GPU
// product so the control plane can recognize the accelerator type.
func TestGetMachineGPUInfo_PCIProductFallback(t *testing.T) {
	restore := stubListNVIDIAGPUDevices([]nvidiapci.GPUDevice{
		{Address: "0000:19:00.0", DeviceID: "2330"}, // H100 SXM5
		{Address: "0000:9b:00.0", DeviceID: "2330"},
	}, nil)
	defer restore()

	// mockNvmlInstance.ProductName() returns "" (NVML unavailable)
	info, err := GetMachineGPUInfo(&mockNvmlInstance{})
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "NVIDIA-H100-80GB-HBM3", info.Product)
	// no NVML devices, so no per-GPU entries and no memory info
	assert.Empty(t, info.GPUs)
	assert.Empty(t, info.Memory)
}

// TestGetMachineGPUInfo_PCIProductFallbackH100PCIe verifies the PCIe variant
// maps to the H100 PCIe product.
func TestGetMachineGPUInfo_PCIProductFallbackH100PCIe(t *testing.T) {
	restore := stubListNVIDIAGPUDevices([]nvidiapci.GPUDevice{
		{Address: "0000:19:00.0", DeviceID: "2331"},
	}, nil)
	defer restore()

	info, err := GetMachineGPUInfo(&mockNvmlInstance{})
	require.NoError(t, err)
	assert.Equal(t, "NVIDIA-H100-PCIe", info.Product)
}

// TestGetMachineGPUInfo_PCIProductFallbackPrefersFirstKnownDevice verifies
// that unknown device IDs are skipped in favor of the first known GPU.
func TestGetMachineGPUInfo_PCIProductFallbackPrefersFirstKnownDevice(t *testing.T) {
	restore := stubListNVIDIAGPUDevices([]nvidiapci.GPUDevice{
		{Address: "0000:01:00.0", DeviceID: "ffff"}, // unknown device ID
		{Address: "0000:19:00.0", DeviceID: "2335"}, // H200
	}, nil)
	defer restore()

	info, err := GetMachineGPUInfo(&mockNvmlInstance{})
	require.NoError(t, err)
	assert.Equal(t, "NVIDIA-H200", info.Product)
}

// TestGetMachineGPUInfo_PCIProductFallbackUnknownDevice verifies that an
// NVIDIA GPU with an unmapped PCI device ID leaves the product empty
// rather than reporting a wrong product.
func TestGetMachineGPUInfo_PCIProductFallbackUnknownDevice(t *testing.T) {
	restore := stubListNVIDIAGPUDevices([]nvidiapci.GPUDevice{
		{Address: "0000:19:00.0", DeviceID: "ffff"},
	}, nil)
	defer restore()

	info, err := GetMachineGPUInfo(&mockNvmlInstance{})
	require.NoError(t, err)
	assert.Empty(t, info.Product)
}

// TestGetMachineGPUInfo_PCIProductFallbackListError verifies that a sysfs
// read failure degrades to an empty product without failing the request.
func TestGetMachineGPUInfo_PCIProductFallbackListError(t *testing.T) {
	restore := stubListNVIDIAGPUDevices(nil, errors.New("sysfs not available"))
	defer restore()

	info, err := GetMachineGPUInfo(&mockNvmlInstance{})
	require.NoError(t, err)
	assert.Empty(t, info.Product)
}

// TestGetMachineGPUInfo_NVMLProductTakesPrecedence verifies the PCI
// fallback does not override the NVML-detected product name.
func TestGetMachineGPUInfo_NVMLProductTakesPrecedence(t *testing.T) {
	restore := stubListNVIDIAGPUDevices([]nvidiapci.GPUDevice{
		{Address: "0000:19:00.0", DeviceID: "2330"},
	}, nil)
	defer restore()

	info, err := GetMachineGPUInfo(&mockNvmlInstanceWithProduct{productName: "NVIDIA-H100-NVL"})
	require.NoError(t, err)
	assert.Equal(t, "NVIDIA-H100-NVL", info.Product)
}

type mockNvmlInstanceWithProduct struct {
	mockNvmlInstance
	productName string
}

func (m *mockNvmlInstanceWithProduct) ProductName() string { return m.productName }
