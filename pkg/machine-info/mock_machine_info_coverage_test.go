package machineinfo

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidiamemory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	componentcontainerd "github.com/leptonai/gpud/components/containerd"
	componenttailscale "github.com/leptonai/gpud/components/tailscale"
	"github.com/leptonai/gpud/pkg/disk"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	nvidiadevice "github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvidiatestutil "github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

type machineInfoCoverageDevice struct {
	*nvidiatestutil.MockDevice

	uuid   string
	busID  string
	busErr error

	serial    string
	serialRet nvml.Return

	minor    int
	minorRet nvml.Return

	board    uint32
	boardRet nvml.Return
}

func newMachineInfoCoverageDevice() *machineInfoCoverageDevice {
	base := nvidiatestutil.NewMockDeviceWithIDs(
		&nvmlmock.Device{},
		"hopper",
		"Tesla",
		"9.0",
		"0000:01:00.0",
		"GPU-test-uuid",
		"GPU-SERIAL-1",
		7,
		42,
	)
	return &machineInfoCoverageDevice{
		MockDevice: base,
		uuid:       "GPU-test-uuid",
		busID:      "0000:01:00.0",
		serial:     "GPU-SERIAL-1",
		serialRet:  nvml.SUCCESS,
		minor:      7,
		minorRet:   nvml.SUCCESS,
		board:      42,
		boardRet:   nvml.SUCCESS,
	}
}

func (d *machineInfoCoverageDevice) PCIBusID() string { return d.busID }
func (d *machineInfoCoverageDevice) UUID() string     { return d.uuid }
func (d *machineInfoCoverageDevice) GetPCIBusID() (string, error) {
	return d.busID, d.busErr
}
func (d *machineInfoCoverageDevice) GetSerial() (string, nvml.Return) {
	return d.serial, d.serialRet
}
func (d *machineInfoCoverageDevice) GetMinorNumber() (int, nvml.Return) {
	return d.minor, d.minorRet
}
func (d *machineInfoCoverageDevice) GetBoardId() (uint32, nvml.Return) {
	return d.board, d.boardRet
}
func (d *machineInfoCoverageDevice) GetFabricState() (nvidiadevice.FabricState, error) {
	return nvidiadevice.FabricState{
		ClusterUUID:   "",
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED,
	}, nil
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "kubelet" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return true }
func (fakeFileInfo) Sys() interface{}   { return nil }

func TestGetMachineGPUInfo_CoverageBranches_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetMachineGPUInfo handles not-supported serial/minor/board", t, func() {
		dev := newMachineInfoCoverageDevice()
		dev.serialRet = nvml.ERROR_NOT_SUPPORTED
		dev.minorRet = nvml.ERROR_NOT_SUPPORTED
		dev.boardRet = nvml.ERROR_NOT_SUPPORTED

		mockey.Mock(nvidiamemory.GetMemory).To(
			func(uuid string, dev nvidiadevice.Device, productName string, getVirtualMemoryFunc nvidiamemory.GetVirtualMemoryFunc) (nvidiamemory.Memory, error) {
				return nvidiamemory.Memory{TotalBytes: 80 * 1024 * 1024 * 1024}, nil
			},
		).Build()

		nvmlInstance := &mockNvmlInstanceForMockey{
			productName:  "NVIDIA H100 80GB HBM3",
			brand:        "Tesla",
			architecture: "hopper",
			devices: map[string]nvidiadevice.Device{
				dev.uuid: dev,
			},
		}

		info, err := GetMachineGPUInfo(nvmlInstance)
		require.NoError(t, err)
		require.Len(t, info.GPUs, 1)
		assert.Equal(t, "-1", info.GPUs[0].MinorID)
		assert.Equal(t, uint32(0), info.GPUs[0].BoardID)
		assert.Equal(t, dev.uuid, info.GPUs[0].UUID)
		assert.Equal(t, "0000:01:00.0", info.GPUs[0].BusID)
		assert.NotEmpty(t, info.Memory)
	})

	mockey.PatchConvey("GetMachineGPUInfo returns memory error", t, func() {
		dev := newMachineInfoCoverageDevice()
		mockey.Mock(nvidiamemory.GetMemory).To(
			func(uuid string, dev nvidiadevice.Device, productName string, getVirtualMemoryFunc nvidiamemory.GetVirtualMemoryFunc) (nvidiamemory.Memory, error) {
				return nvidiamemory.Memory{}, errors.New("memory read failed")
			},
		).Build()

		nvmlInstance := &mockNvmlInstanceForMockey{
			productName: "NVIDIA H100 80GB HBM3",
			devices: map[string]nvidiadevice.Device{
				dev.uuid: dev,
			},
		}

		_, err := GetMachineGPUInfo(nvmlInstance)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "memory read failed")
	})

	mockey.PatchConvey("GetMachineGPUInfo returns serial error", t, func() {
		dev := newMachineInfoCoverageDevice()
		dev.serialRet = nvml.ERROR_UNKNOWN
		mockey.Mock(nvidiamemory.GetMemory).To(
			func(uuid string, dev nvidiadevice.Device, productName string, getVirtualMemoryFunc nvidiamemory.GetVirtualMemoryFunc) (nvidiamemory.Memory, error) {
				return nvidiamemory.Memory{TotalBytes: 1}, nil
			},
		).Build()

		nvmlInstance := &mockNvmlInstanceForMockey{
			productName: "NVIDIA H100 80GB HBM3",
			devices: map[string]nvidiadevice.Device{
				dev.uuid: dev,
			},
		}

		_, err := GetMachineGPUInfo(nvmlInstance)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get serial id")
	})

	mockey.PatchConvey("GetMachineGPUInfo returns PCI bus id error", t, func() {
		dev := newMachineInfoCoverageDevice()
		dev.busErr = errors.New("pci bus id failed")
		mockey.Mock(nvidiamemory.GetMemory).To(
			func(uuid string, dev nvidiadevice.Device, productName string, getVirtualMemoryFunc nvidiamemory.GetVirtualMemoryFunc) (nvidiamemory.Memory, error) {
				return nvidiamemory.Memory{TotalBytes: 1}, nil
			},
		).Build()

		nvmlInstance := &mockNvmlInstanceForMockey{
			productName: "NVIDIA H100 80GB HBM3",
			devices: map[string]nvidiadevice.Device{
				dev.uuid: dev,
			},
		}

		_, err := GetMachineGPUInfo(nvmlInstance)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pci bus id failed")
	})
}

func TestGetMachineDiskInfo_LinuxBranches_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetMachineDiskInfo succeeds with NFS and kubelet root disk", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(disk.GetBlockDevicesWithLsblk).To(func(ctx context.Context, opts ...disk.OpOption) (disk.BlockDevices, error) {
			return disk.BlockDevices{
				{Name: "/dev/sda", Type: "disk", MountPoint: "", Size: disk.CustomUint64{Uint64: 100}},
				{Name: "/dev/sda1", Type: "part", MountPoint: "/", FSType: "ext4", FSUsed: disk.CustomUint64{Uint64: 50}, Size: disk.CustomUint64{Uint64: 100}},
			}, nil
		}).Build()
		mockey.Mock(disk.GetPartitions).To(func(ctx context.Context, opts ...disk.OpOption) (disk.Partitions, error) {
			return disk.Partitions{
				{
					Device:     "10.0.0.10:/nfs",
					Fstype:     "nfs",
					MountPoint: "/mnt/nfs",
					Usage: &disk.Usage{
						TotalBytes: 1000,
						UsedBytes:  100,
					},
				},
			}, nil
		}).Build()
		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			if name == "/var/lib/kubelet" {
				return fakeFileInfo{}, nil
			}
			return nil, os.ErrNotExist
		}).Build()
		mockey.Mock(disk.FindMnt).To(func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
			return &disk.FindMntOutput{
				Filesystems: []disk.FoundMnt{
					{Sources: []string{"/dev/nvme0n1p1"}},
				},
			}, nil
		}).Build()

		info, err := GetMachineDiskInfo(context.Background())
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "/dev/nvme0n1p1", info.ContainerRootDisk)
		assert.GreaterOrEqual(t, len(info.BlockDevices), 2)
	})

	mockey.PatchConvey("GetMachineDiskInfo returns lsblk error", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(disk.GetBlockDevicesWithLsblk).To(func(ctx context.Context, opts ...disk.OpOption) (disk.BlockDevices, error) {
			return nil, errors.New("lsblk failed")
		}).Build()
		_, err := GetMachineDiskInfo(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lsblk failed")
	})

	mockey.PatchConvey("GetMachineDiskInfo returns partition error", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(disk.GetBlockDevicesWithLsblk).To(func(ctx context.Context, opts ...disk.OpOption) (disk.BlockDevices, error) {
			return disk.BlockDevices{{Name: "/dev/sda1", MountPoint: "/", FSType: "ext4"}}, nil
		}).Build()
		mockey.Mock(disk.GetPartitions).To(func(ctx context.Context, opts ...disk.OpOption) (disk.Partitions, error) {
			return nil, errors.New("partitions failed")
		}).Build()

		_, err := GetMachineDiskInfo(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "partitions failed")
	})

	mockey.PatchConvey("GetMachineDiskInfo returns kubelet stat error", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(disk.GetBlockDevicesWithLsblk).To(func(ctx context.Context, opts ...disk.OpOption) (disk.BlockDevices, error) {
			return disk.BlockDevices{{Name: "/dev/sda1", MountPoint: "/", FSType: "ext4"}}, nil
		}).Build()
		mockey.Mock(disk.GetPartitions).To(func(ctx context.Context, opts ...disk.OpOption) (disk.Partitions, error) {
			return nil, nil
		}).Build()
		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return nil, errors.New("stat failed")
		}).Build()

		_, err := GetMachineDiskInfo(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stat failed")
	})

	mockey.PatchConvey("GetMachineDiskInfo returns findmnt error", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(disk.GetBlockDevicesWithLsblk).To(func(ctx context.Context, opts ...disk.OpOption) (disk.BlockDevices, error) {
			return disk.BlockDevices{{Name: "/dev/sda1", MountPoint: "/", FSType: "ext4"}}, nil
		}).Build()
		mockey.Mock(disk.GetPartitions).To(func(ctx context.Context, opts ...disk.OpOption) (disk.Partitions, error) {
			return nil, nil
		}).Build()
		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return fakeFileInfo{}, nil
		}).Build()
		mockey.Mock(disk.FindMnt).To(func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
			return nil, errors.New("findmnt failed")
		}).Build()

		_, err := GetMachineDiskInfo(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "findmnt failed")
	})
}

func TestGetMachineInfo_LinuxBranches_WithMockey(t *testing.T) {
	baseNVML := &mockNvmlInstanceForMockey{
		driverVersion: "550.90.07",
		cudaVersion:   "12.4",
		productName:   "NVIDIA H100 80GB HBM3",
		architecture:  "hopper",
		brand:         "Tesla",
		devices:       map[string]nvidiadevice.Device{},
		nvmlExists:    true,
	}

	mockey.PatchConvey("GetMachineInfo returns disk info error", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(GetMachineGPUInfo).To(func(nvidianvml.Instance) (*apiv1.MachineGPUInfo, error) {
			return &apiv1.MachineGPUInfo{}, nil
		}).Build()
		mockey.Mock(GetMachineDiskInfo).To(func(ctx context.Context) (*apiv1.MachineDiskInfo, error) {
			return nil, errors.New("disk info failed")
		}).Build()

		_, err := GetMachineInfo(baseNVML)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get machine disk info")
	})

	mockey.PatchConvey("GetMachineInfo sets containerd and tailscale versions", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(GetMachineGPUInfo).To(func(nvidianvml.Instance) (*apiv1.MachineGPUInfo, error) {
			return &apiv1.MachineGPUInfo{}, nil
		}).Build()
		mockey.Mock(GetMachineDiskInfo).To(func(ctx context.Context) (*apiv1.MachineDiskInfo, error) {
			return &apiv1.MachineDiskInfo{}, nil
		}).Build()
		mockey.Mock(componentcontainerd.CheckContainerdInstalled).To(func() bool { return true }).Build()
		mockey.Mock(componentcontainerd.CheckContainerdRunning).To(func(ctx context.Context) bool { return true }).Build()
		mockey.Mock(componentcontainerd.GetVersion).To(func(ctx context.Context, endpoint string) (string, error) {
			return "1.7.0", nil
		}).Build()
		mockey.Mock(componenttailscale.CheckTailscaleInstalled).To(func() bool { return true }).Build()
		mockey.Mock(componenttailscale.GetTailscaleVersion).To(func() (string, error) {
			return " 1.68.2 \n", nil
		}).Build()

		info, err := GetMachineInfo(baseNVML)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "containerd://1.7.0", info.ContainerRuntimeVersion)
		assert.Equal(t, "1.68.2", info.TailscaleVersion)
	})

	mockey.PatchConvey("GetMachineInfo tolerates containerd and tailscale errors", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(GetMachineGPUInfo).To(func(nvidianvml.Instance) (*apiv1.MachineGPUInfo, error) {
			return &apiv1.MachineGPUInfo{}, nil
		}).Build()
		mockey.Mock(GetMachineDiskInfo).To(func(ctx context.Context) (*apiv1.MachineDiskInfo, error) {
			return &apiv1.MachineDiskInfo{}, nil
		}).Build()
		mockey.Mock(componentcontainerd.CheckContainerdInstalled).To(func() bool { return true }).Build()
		mockey.Mock(componentcontainerd.CheckContainerdRunning).To(func(ctx context.Context) bool { return true }).Build()
		mockey.Mock(componentcontainerd.GetVersion).To(func(ctx context.Context, endpoint string) (string, error) {
			return "", errors.New("containerd version failed")
		}).Build()
		mockey.Mock(componenttailscale.CheckTailscaleInstalled).To(func() bool { return true }).Build()
		mockey.Mock(componenttailscale.GetTailscaleVersion).To(func() (string, error) {
			return "", errors.New("tailscale version failed")
		}).Build()

		info, err := GetMachineInfo(baseNVML)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "", info.ContainerRuntimeVersion)
		assert.Equal(t, "", info.TailscaleVersion)
	})
}
