package firmware

import (
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvidiatestutil "github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

// fakeNVML serves fixed devices; collectGPU calls nothing else.
type fakeNVML struct {
	nvidianvml.Instance
	devices map[string]device.Device
}

func (f *fakeNVML) Devices() map[string]device.Device { return f.devices }

type fakeGPU struct {
	busID   string
	version string
	ret     nvml.Return
}

func newFakeNVML(gpus ...fakeGPU) *fakeNVML {
	devs := make(map[string]device.Device, len(gpus))
	for i, g := range gpus {
		uuid := fmt.Sprintf("GPU-%04d", i)
		dev := &nvmlmock.Device{
			GetVbiosVersionFunc: func() (string, nvml.Return) { return g.version, g.ret },
		}
		devs[uuid] = nvidiatestutil.NewMockDeviceWithIDs(dev, "", "", "", g.busID, uuid, "", 0, 0)
	}
	return &fakeNVML{Instance: nvidianvml.NewNoOp(), devices: devs}
}

func TestCollectGPU_NoDevices(t *testing.T) {
	assert.Nil(t, collectGPU(nil))
	assert.Nil(t, collectGPU(nvidianvml.NewNoOp()))
	assert.Nil(t, collectGPU(&fakeNVML{Instance: nvidianvml.NewNoOp()}))
}

func TestCollectGPU_NamesFollowPCIBusID(t *testing.T) {
	// Ten GPUs inserted out of order; the map hides the insertion order anyway.
	var gpus []fakeGPU
	for _, bus := range []int{0x9a, 0x1b, 0xdb, 0x5d, 0x0f, 0xbb, 0x3c, 0x4d, 0x6e, 0x7f} {
		gpus = append(gpus, fakeGPU{
			busID:   fmt.Sprintf("0000:%02x:00.0", bus),
			version: fmt.Sprintf("97.00.%02X.00.01", bus),
			ret:     nvml.SUCCESS,
		})
	}

	found := collectGPU(newFakeNVML(gpus...))
	require.Len(t, found, 10)

	wantBuses := []int{0x0f, 0x1b, 0x3c, 0x4d, 0x5d, 0x6e, 0x7f, 0x9a, 0xbb, 0xdb}
	for i, bus := range wantBuses {
		assert.Equal(t, instance{
			component: apiv1.FirmwareComponentGPU,
			version:   fmt.Sprintf("97.00.%02X.00.01", bus),
			name:      fmt.Sprintf("gpu%d", i+1),
		}, found[i])
	}
}

func TestCollectGPU_FailedReadKeepsOtherNames(t *testing.T) {
	found := collectGPU(newFakeNVML(
		fakeGPU{busID: "0000:03:00.0", version: "97.10.4A.00.0B", ret: nvml.SUCCESS},
		fakeGPU{busID: "0000:01:00.0", version: " 97.10.4A.00.0B\n", ret: nvml.SUCCESS},
		fakeGPU{busID: "0000:02:00.0", ret: nvml.ERROR_GPU_IS_LOST},
		fakeGPU{busID: "0000:04:00.0", version: "  ", ret: nvml.SUCCESS},
		fakeGPU{busID: "0000:05:00.0", ret: nvml.ERROR_NOT_SUPPORTED},
	))

	assert.Equal(t, []instance{
		{component: apiv1.FirmwareComponentGPU, version: "97.10.4A.00.0B", name: "gpu1"},
		{component: apiv1.FirmwareComponentGPU, version: "97.10.4A.00.0B", name: "gpu3"},
	}, found)
}

func TestCollectGPU_SkipsNilDevice(t *testing.T) {
	inst := newFakeNVML(fakeGPU{busID: "0000:01:00.0", version: "96.00.00.00.01", ret: nvml.SUCCESS})
	inst.devices["GPU-nil"] = nil

	assert.Equal(t, []instance{
		{component: apiv1.FirmwareComponentGPU, version: "96.00.00.00.01", name: "gpu1"},
	}, collectGPU(inst))
}
