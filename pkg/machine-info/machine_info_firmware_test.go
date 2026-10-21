package machineinfo

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidiamemory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	configcommon "github.com/leptonai/gpud/pkg/config/common"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/netutil"
	nvidiadevice "github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

func TestGetMachineInfo_FirmwareSnapshot(t *testing.T) {
	snapshot := []apiv1.MachineFirmware{{
		Component:     apiv1.FirmwareComponentGPU,
		Version:       "97.10.4A.00.0B",
		Instances:     1,
		InstanceNames: []string{"gpu1"},
		LastUpdated:   metav1.NewTime(time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)),
	}}

	mockey.PatchConvey("getMachineInfo attaches the cached firmware without reading it", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "darwin" }).Build()
		mockey.Mock(pkghost.KernelVersion).To(func() string { return "5.15.0-generic" }).Build()
		mockey.Mock(cpu.CountsWithContext).To(func(ctx context.Context, logical bool) (int, error) { return 16, nil }).Build()
		mockey.Mock(mem.VirtualMemoryWithContext).To(func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil
		}).Build()
		mockey.Mock(netutil.GetPrivateIPs).To(func(opts ...netutil.OpOption) (netutil.InterfaceAddrs, error) {
			return netutil.InterfaceAddrs{}, nil
		}).Build()
		mockey.Mock(nvidiamemory.GetMemory).To(
			func(uuid string, dev nvidiadevice.Device, productName string, getVirtualMemoryFunc nvidiamemory.GetVirtualMemoryFunc) (nvidiamemory.Memory, error) {
				return nvidiamemory.Memory{TotalBytes: 80 * 1024 * 1024 * 1024}, nil
			},
		).Build()

		// The coverage device has no GetVbiosVersionFunc, so any VBIOS read
		// from getMachineInfo would panic.
		dev := newMachineInfoCoverageDevice()
		nvmlInstance := &mockNvmlInstanceForMockey{
			driverMajor:  550,
			productName:  "NVIDIA H100 80GB HBM3",
			brand:        "Tesla",
			architecture: "hopper",
			devices:      map[string]nvidiadevice.Device{dev.uuid: dev},
			nvmlExists:   true,
		}

		orig := getFirmwareSnapshot
		defer func() { getFirmwareSnapshot = orig }()

		getFirmwareSnapshot = func() []apiv1.MachineFirmware { return snapshot }
		info, err := getMachineInfo(nvmlInstance, configcommon.ContainerdConfig{})
		require.NoError(t, err)
		assert.Equal(t, snapshot, info.Firmware)

		getFirmwareSnapshot = func() []apiv1.MachineFirmware { return nil }
		info, err = getMachineInfo(nvmlInstance, configcommon.ContainerdConfig{})
		require.NoError(t, err)
		assert.Nil(t, info.Firmware)
	})
}
