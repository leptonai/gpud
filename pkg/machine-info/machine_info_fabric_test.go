package machineinfo

import (
	"context"
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/bytedance/mockey"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nvidiamemory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/netutil"
	"github.com/leptonai/gpud/pkg/nvidia/nvidiasmi"
	nvidiadevice "github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

// fabricMachineInfoDevice overrides the fabric state of the coverage device.
type fabricMachineInfoDevice struct {
	*machineInfoCoverageDevice
	fabricState nvidiadevice.FabricState
	fabricErr   error
}

func (d *fabricMachineInfoDevice) GetFabricState() (nvidiadevice.FabricState, error) {
	return d.fabricState, d.fabricErr
}

func newFabricMachineInfoDevice(fabricState nvidiadevice.FabricState, fabricErr error) *fabricMachineInfoDevice {
	return &fabricMachineInfoDevice{
		machineInfoCoverageDevice: newMachineInfoCoverageDevice(),
		fabricState:               fabricState,
		fabricErr:                 fabricErr,
	}
}

func TestGetGPUFabricIdentifiers(t *testing.T) {
	t.Run("returns identifiers from the first readable device", func(t *testing.T) {
		dev := newFabricMachineInfoDevice(nvidiadevice.FabricState{
			CliqueID:    42,
			ClusterUUID: "cluster-uuid-123",
			State:       nvml.GPU_FABRIC_STATE_COMPLETED,
			Status:      nvml.SUCCESS,
		}, nil)
		nvmlInstance := &mockNvmlInstanceForMockey{
			devices: map[string]nvidiadevice.Device{dev.uuid: dev},
		}

		clusterUUID, cliqueID := getGPUFabricIdentifiers(nvmlInstance)
		assert.Equal(t, "cluster-uuid-123", clusterUUID)
		assert.Equal(t, uint32(42), cliqueID)
	})

	t.Run("skips devices with fabric state errors", func(t *testing.T) {
		devErr := newFabricMachineInfoDevice(nvidiadevice.FabricState{}, errors.New("fabric state telemetry not supported"))
		devOK := newFabricMachineInfoDevice(nvidiadevice.FabricState{
			CliqueID:    7,
			ClusterUUID: "cluster-uuid-456",
			State:       nvml.GPU_FABRIC_STATE_COMPLETED,
			Status:      nvml.SUCCESS,
		}, nil)
		devOK.uuid = "GPU-test-uuid-2"
		nvmlInstance := &mockNvmlInstanceForMockey{
			devices: map[string]nvidiadevice.Device{
				devErr.uuid: devErr,
				devOK.uuid:  devOK,
			},
		}

		clusterUUID, cliqueID := getGPUFabricIdentifiers(nvmlInstance)
		assert.Equal(t, "cluster-uuid-456", clusterUUID)
		assert.Equal(t, uint32(7), cliqueID)
	})

	t.Run("returns empty values when no device supports fabric state", func(t *testing.T) {
		dev := newFabricMachineInfoDevice(nvidiadevice.FabricState{}, errors.New("fabric state telemetry not supported"))
		nvmlInstance := &mockNvmlInstanceForMockey{
			devices: map[string]nvidiadevice.Device{dev.uuid: dev},
		}

		clusterUUID, cliqueID := getGPUFabricIdentifiers(nvmlInstance)
		assert.Empty(t, clusterUUID)
		assert.Zero(t, cliqueID)
	})

	t.Run("returns empty values without devices", func(t *testing.T) {
		nvmlInstance := &mockNvmlInstanceForMockey{
			devices: map[string]nvidiadevice.Device{},
		}

		clusterUUID, cliqueID := getGPUFabricIdentifiers(nvmlInstance)
		assert.Empty(t, clusterUUID)
		assert.Zero(t, cliqueID)
	})
}

// TestGetMachineInfo_GPUFabricIdentifiers tests that GetMachineInfo surfaces
// the GPU fabric identifiers and chassis serial number.
func TestGetMachineInfo_GPUFabricIdentifiers(t *testing.T) {
	mockey.PatchConvey("GetMachineInfo surfaces fabric identifiers and chassis serial", t, func() {
		mockey.Mock(pkghost.KernelVersion).To(func() string { return "5.15.0-generic" }).Build()
		mockey.Mock(pkghost.OSName).To(func() string { return "Ubuntu 22.04.2 LTS" }).Build()
		mockey.Mock(cpu.CountsWithContext).To(func(ctx context.Context, logical bool) (int, error) { return 16, nil }).Build()
		mockey.Mock(mem.VirtualMemoryWithContext).To(func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil
		}).Build()
		mockey.Mock(netutil.GetPrivateIPs).To(func(opts ...netutil.OpOption) (netutil.InterfaceAddrs, error) {
			return netutil.InterfaceAddrs{}, nil
		}).Build()
		mockey.Mock(nvidiamemory.GetMemory).To(
			func(uuid string, dev nvidiadevice.Device, productName string, getVirtualMemoryFunc nvidiamemory.GetVirtualMemoryFunc) (nvidiamemory.Memory, error) {
				return nvidiamemory.Memory{TotalBytes: 186 * 1024 * 1024 * 1024}, nil
			},
		).Build()
		mockey.Mock(nvidiasmi.GetChassisSerial).To(func(ctx context.Context) (string, error) {
			return "3136434J5234567", nil
		}).Build()

		dev := newFabricMachineInfoDevice(nvidiadevice.FabricState{
			CliqueID:    42,
			ClusterUUID: "cluster-uuid-123",
			State:       nvml.GPU_FABRIC_STATE_COMPLETED,
			Status:      nvml.SUCCESS,
		}, nil)
		nvmlInstance := &mockNvmlInstanceForMockey{
			productName:  "NVIDIA GB200",
			brand:        "NVIDIA",
			architecture: "blackwell",
			devices:      map[string]nvidiadevice.Device{dev.uuid: dev},
			nvmlExists:   true,
		}

		info, err := GetMachineInfo(nvmlInstance)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "cluster-uuid-123", info.ClusterUUID)
		assert.Equal(t, uint32(42), info.CliqueID)
		assert.Equal(t, "3136434J5234567", info.ChassisSerial)
	})

	mockey.PatchConvey("GetMachineInfo tolerates chassis serial lookup failure", t, func() {
		mockey.Mock(pkghost.KernelVersion).To(func() string { return "5.15.0-generic" }).Build()
		mockey.Mock(pkghost.OSName).To(func() string { return "Ubuntu 22.04.2 LTS" }).Build()
		mockey.Mock(cpu.CountsWithContext).To(func(ctx context.Context, logical bool) (int, error) { return 16, nil }).Build()
		mockey.Mock(mem.VirtualMemoryWithContext).To(func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil
		}).Build()
		mockey.Mock(netutil.GetPrivateIPs).To(func(opts ...netutil.OpOption) (netutil.InterfaceAddrs, error) {
			return netutil.InterfaceAddrs{}, nil
		}).Build()
		mockey.Mock(nvidiamemory.GetMemory).To(
			func(uuid string, dev nvidiadevice.Device, productName string, getVirtualMemoryFunc nvidiamemory.GetVirtualMemoryFunc) (nvidiamemory.Memory, error) {
				return nvidiamemory.Memory{TotalBytes: 186 * 1024 * 1024 * 1024}, nil
			},
		).Build()
		mockey.Mock(nvidiasmi.GetChassisSerial).To(func(ctx context.Context) (string, error) {
			return "", errors.New("nvidia-smi not found")
		}).Build()

		dev := newFabricMachineInfoDevice(nvidiadevice.FabricState{}, errors.New("fabric state telemetry not supported"))
		nvmlInstance := &mockNvmlInstanceForMockey{
			productName:  "NVIDIA H100 80GB HBM3",
			brand:        "Tesla",
			architecture: "hopper",
			devices:      map[string]nvidiadevice.Device{dev.uuid: dev},
			nvmlExists:   true,
		}

		info, err := GetMachineInfo(nvmlInstance)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Empty(t, info.ClusterUUID)
		assert.Zero(t, info.CliqueID)
		assert.Empty(t, info.ChassisSerial)
	})
}
