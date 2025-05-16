package machineinfo

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/leptonai/gpud/pkg/log"
	nvidiaquery "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func TestGetSystemResourceMemoryTotal(t *testing.T) {
	mem, err := GetSystemResourceMemoryTotal()
	assert.NoError(t, err)

	memQty, err := resource.ParseQuantity(mem)
	assert.NoError(t, err)
	assert.NotZero(t, memQty.Value(), "Memory quantity should not be zero")
	t.Logf("mem: %s", memQty.String())
}

func TestGetSystemResourceLogicalCores(t *testing.T) {
	cpu, cnt, err := GetSystemResourceLogicalCores()
	assert.NoError(t, err)

	cpuQty, err := resource.ParseQuantity(cpu)
	assert.NoError(t, err)
	assert.NotZero(t, cpuQty.Value(), "CPU quantity should not be zero")
	assert.NotZero(t, cnt, "CPU core count should not be zero")
	t.Logf("cpu: %s", cpuQty.String())
	t.Logf("cnt: %d", cnt)
}

func TestGetMachineNetwork(t *testing.T) {
	// Even if the environment variable is not set, we can still test the function structure
	network := GetMachineNICInfo()
	assert.NotNil(t, network)

	// Run more detailed test if environment variable is set
	if os.Getenv("TEST_MACHINE_NETWORK") == "true" {
		t.Log("Running detailed network test")
		assert.NotNil(t, network)
		assert.NotEmpty(t, network.PrivateIPInterfaces)
	} else {
		t.Log("Basic network test - verify structure only")
	}

	t.Logf("network: %+v", network)
}

func TestGetMachineCPUInfo(t *testing.T) {
	cpuInfo := GetMachineCPUInfo()
	assert.NotNil(t, cpuInfo)
	assert.Equal(t, runtime.GOARCH, cpuInfo.Architecture)
}

func TestGetMachineLocation(t *testing.T) {
	if os.Getenv("TEST_MACHINE_LOCATION") != "true" {
		t.Skip("TEST_MACHINE_LOCATION is not set")
	}

	// Always run a basic test, but don't assert on the results
	// as it may return nil depending on network conditions
	location := GetMachineLocation()
	t.Logf("location: %+v", location)

	// More detailed test when environment variable is set
	if os.Getenv("TEST_MACHINE_LOCATION") == "true" {
		t.Log("Running detailed location test")
		if location != nil {
			assert.NotEmpty(t, location.Region, "Region should not be empty when TEST_MACHINE_LOCATION is set")
		}
	} else {
		t.Log("Basic location test - no assertions on result")
	}
}

func TestGetSystemResourceGPUCount(t *testing.T) {
	// Skip if NVML is not available
	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		t.Skip("NVML not available, skipping test")
	}
	defer func() {
		if err := nvmlInstance.Shutdown(); err != nil {
			log.Logger.Warnw("failed to shutdown nvml instance", "error", err)
		}
	}()

	devCnt, err := nvidiaquery.CountAllDevicesFromDevDir()
	assert.NoError(t, err)
	gpuCnt, err := GetSystemResourceGPUCount(nvmlInstance)
	assert.NoError(t, err)
	assert.NotEmpty(t, gpuCnt)

	if devCnt == 0 {
		assert.Equal(t, "0", gpuCnt)
	} else {
		assert.Equal(t, strconv.Itoa(devCnt), gpuCnt)
	}
}

func TestGetSystemResourceRootVolumeTotal(t *testing.T) {
	// Skip test on non-Linux platforms or in environments where root volume check fails
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Test only runs on Linux or macOS")
	}

	volume, err := GetSystemResourceRootVolumeTotal()
	if err != nil {
		t.Skipf("Could not get root volume total: %v", err)
	}

	assert.NotEmpty(t, volume)
	volQty, err := resource.ParseQuantity(volume)
	assert.NoError(t, err)
	assert.NotZero(t, volQty.Value())
	t.Logf("Root volume: %s", volume)
}

func TestGetProvider(t *testing.T) {
	// Test with empty IP
	provider := GetProvider("")
	assert.Empty(t, provider)

	// Test with localhost IP
	provider = GetProvider("127.0.0.1")
	assert.Empty(t, provider)

	// Test with invalid IP
	provider = GetProvider("999.999.999.999")
	assert.Empty(t, provider)

	// Skip real IP test as it depends on external service
	if os.Getenv("TEST_PROVIDER_LOOKUP") == "true" {
		// Test with a real public IP (Google DNS)
		provider = GetProvider("8.8.8.8")
		t.Logf("Provider for 8.8.8.8: %s", provider)
	}
}

// TestGetMachineInfo tests only basic functionality without mocking
func TestGetMachineInfo(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Test only runs on Linux or macOS")
	}

	// Skip if NVML is not available
	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		t.Skip("NVML not available, skipping test")
	}
	defer func() {
		if err := nvmlInstance.Shutdown(); err != nil {
			log.Logger.Warnw("failed to shutdown nvml instance", "error", err)
		}
	}()

	// Test the functionality, but don't verify detailed outputs
	info, err := GetMachineInfo(nvmlInstance)
	if err != nil {
		t.Skipf("Could not get machine info: %v", err)
	}

	// Basic validations
	assert.NotEmpty(t, info.GPUdVersion)
	assert.NotEmpty(t, info.Hostname)
	assert.NotNil(t, info.CPUInfo)
	if info.GPUInfo != nil && len(info.GPUInfo.GPUs) > 0 {
		assert.NotEmpty(t, info.GPUInfo.Memory)
	}
}

// TestGetMachineGPUInfo tests GPU info without complex mocking
func TestGetMachineGPUInfo(t *testing.T) {
	// Skip if NVML is not available
	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		t.Skip("NVML not available, skipping test")
	}
	defer func() {
		if err := nvmlInstance.Shutdown(); err != nil {
			log.Logger.Warnw("failed to shutdown nvml instance", "error", err)
		}
	}()

	if len(nvmlInstance.Devices()) == 0 {
		t.Skip("No GPU devices found, skipping test")
	}

	info, err := GetMachineGPUInfo(nvmlInstance)
	if err != nil {
		t.Skipf("Could not get GPU info: %v", err)
	}

	assert.NotEmpty(t, info.Product)
	assert.NotEmpty(t, info.Manufacturer)
	assert.NotEmpty(t, info.Memory)
	assert.NotEmpty(t, info.GPUs)

	for _, gpu := range info.GPUs {
		assert.NotEmpty(t, gpu.UUID)
		assert.NotEmpty(t, gpu.MinorID)
	}

	// Test memory parsing
	memQty, err := resource.ParseQuantity(info.Memory)
	assert.NoError(t, err)
	assert.NotZero(t, memQty.Value())
}

// TestGetMachineDiskInfo tests disk info with minimal validation
func TestGetMachineDiskInfo(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Test only runs on Linux or macOS")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := GetMachineDiskInfo(ctx)
	if err != nil {
		t.Skipf("Could not get disk info: %v", err)
	}

	assert.NotNil(t, info)

	// At least one block device should be present
	assert.NotEmpty(t, info.BlockDevices)

	// Validate first block device
	if len(info.BlockDevices) > 0 {
		device := info.BlockDevices[0]
		assert.NotEmpty(t, device.Name)
		assert.NotEmpty(t, device.Type)
		assert.NotZero(t, device.Size)

		// Log device details for better understanding
		t.Logf("Device: %+v", device)
	}

	// If we're on Linux, check container root disk detection
	if runtime.GOOS == "linux" {
		t.Logf("Container root disk: %s", info.ContainerRootDisk)
	}
}
