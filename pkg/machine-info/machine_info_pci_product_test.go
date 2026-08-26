package machineinfo

import (
	"errors"
	"testing"

	"github.com/bytedance/mockey"
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

// TestListNVIDIAGPUDevicesFunc_OriginalCall exercises the un-stubbed
// listNVIDIAGPUDevicesFunc so that its function body (the currentGOOS
// branch and the real nvidiapci.ListNVIDIAGPUDevices call) is covered by
// the coverage profile. On non-Linux platforms the function returns
// (nil, nil) immediately; on Linux it reads /sys/bus/pci/devices, which
// either yields devices or an empty list. Either outcome is acceptable —
// the test only asserts that the call does not panic.
func TestListNVIDIAGPUDevicesFunc_OriginalCall(t *testing.T) {
	// Ensure no other test has left a stub in place.
	original := listNVIDIAGPUDevicesFunc
	defer func() { listNVIDIAGPUDevicesFunc = original }()

	devs, err := listNVIDIAGPUDevicesFunc()
	// On non-Linux: devs == nil, err == nil
	// On Linux: devs may be nil or non-nil, err may be nil or non-nil
	// (e.g., if /sys/bus/pci/devices is not mounted in the test container).
	// The only invariant is that the call must not panic.
	t.Logf("listNVIDIAGPUDevicesFunc returned %d devices, err=%v", len(devs), err)
}

// TestDetectGPUProductFromPCI_EmptyDevices verifies that
// detectGPUProductFromPCI returns an empty string when the PCI device
// list is empty (no NVIDIA GPUs on the host).
func TestDetectGPUProductFromPCI_EmptyDevices(t *testing.T) {
	restore := stubListNVIDIAGPUDevices(nil, nil)
	defer restore()

	assert.Empty(t, detectGPUProductFromPCI())
}

// TestDetectGPUProductFromPCI_SingleKnownDevice verifies the direct
// call to detectGPUProductFromPCI returns the sanitized product name
// for a known device ID.
func TestDetectGPUProductFromPCI_SingleKnownDevice(t *testing.T) {
	restore := stubListNVIDIAGPUDevices([]nvidiapci.GPUDevice{
		{Address: "0000:19:00.0", DeviceID: "2330"},
	}, nil)
	defer restore()

	assert.Equal(t, "NVIDIA-H100-80GB-HBM3", detectGPUProductFromPCI())
}

// TestListNVIDIAGPUDevicesFunc_NonLinuxReturnsNil uses mockey to force the
// non-Linux branch of listNVIDIAGPUDevicesFunc, covering the early return
// (nil, nil) that is unreachable on Linux CI without mocking.
func TestListNVIDIAGPUDevicesFunc_NonLinuxReturnsNil(t *testing.T) {
	mockey.PatchRun(func() {
		mockey.Mock(currentGOOS).To(func() string { return "darwin" }).Build()

		devs, err := listNVIDIAGPUDevicesFunc()
		assert.NoError(t, err)
		assert.Nil(t, devs)
	})
}

// TestListNVIDIAGPUDevicesFunc_LinuxCallsSysfs uses mockey to force the
// Linux branch of listNVIDIAGPUDevicesFunc and stubs the underlying
// nvidiapci.ListNVIDIAGPUDevices call, covering the real sysfs path
// deterministically on every platform.
func TestListNVIDIAGPUDevicesFunc_LinuxCallsSysfs(t *testing.T) {
	mockey.PatchRun(func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(nvidiapci.ListNVIDIAGPUDevices).To(func(dir string) ([]nvidiapci.GPUDevice, error) {
			assert.Equal(t, nvidiapci.DefaultSysfsPCIDevicesDir, dir)
			return []nvidiapci.GPUDevice{
				{Address: "0000:19:00.0", DeviceID: "2330"},
			}, nil
		}).Build()

		devs, err := listNVIDIAGPUDevicesFunc()
		assert.NoError(t, err)
		require.Len(t, devs, 1)
		assert.Equal(t, "2330", devs[0].DeviceID)
	})
}
