package machineinfo

import (
	"os"
	"strconv"
	"testing"

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
	t.Logf("mem: %s", memQty.String())
}

func TestGetSystemResourceLogicalCores(t *testing.T) {
	cpu, cnt, err := GetSystemResourceLogicalCores()
	assert.NoError(t, err)

	cpuQty, err := resource.ParseQuantity(cpu)
	assert.NoError(t, err)
	t.Logf("cpu: %s", cpuQty.String())
	t.Logf("cnt: %d", cnt)
}

func TestGetMachineNetwork(t *testing.T) {
	if os.Getenv("TEST_MACHINE_NETWORK") != "true" {
		t.Skip("TEST_MACHINE_NETWORK is not set")
	}
	network := GetMachineNetwork()
	assert.NotNil(t, network)
	t.Logf("network: %+v", network)
}

func TestGetMachineLocation(t *testing.T) {
	if os.Getenv("TEST_MACHINE_LOCATION") != "true" {
		t.Skip("TEST_MACHINE_LOCATION is not set")
	}
	location := GetMachineLocation()
	t.Logf("location: %+v", location)
}

func TestGetSystemResourceGPUCount(t *testing.T) {
	nvmlInstance, err := nvidianvml.New()
	assert.NoError(t, err)
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
		assert.Equal(t, gpuCnt, "0")
	} else {
		assert.Equal(t, gpuCnt, strconv.Itoa(devCnt))
	}
}
