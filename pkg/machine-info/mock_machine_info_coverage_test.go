package machineinfo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	"github.com/leptonai/gpud/pkg/providers"
	pkgprovidersall "github.com/leptonai/gpud/pkg/providers/all"
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

func TestGetDiskCommandsZeroValue(t *testing.T) {
	previous := diskCommandConfig.Swap(nil)
	t.Cleanup(func() {
		diskCommandConfig.Store(previous)
	})

	assert.Equal(t, diskCommands{}, getDiskCommands())
}

func TestDiskCommandsAtomicSnapshot(t *testing.T) {
	first := diskCommands{findmntCommand: "findmnt-a", lsblkCommand: "lsblk-a", blockdevUsageCommand: "df-a"}
	second := diskCommands{findmntCommand: "findmnt-b", lsblkCommand: "lsblk-b", blockdevUsageCommand: "df-b"}
	SetDiskCommands(first.findmntCommand, first.lsblkCommand, first.blockdevUsageCommand)
	defer SetDiskCommands("", "", "")

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10_000; i++ {
			SetDiskCommands(second.findmntCommand, second.lsblkCommand, second.blockdevUsageCommand)
			SetDiskCommands(first.findmntCommand, first.lsblkCommand, first.blockdevUsageCommand)
		}
		close(done)
	}()

	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()

	for {
		commands := getDiskCommands()
		if commands != first && commands != second {
			t.Fatalf("observed partial disk command snapshot: %+v", commands)
		}
		select {
		case <-done:
			return
		case <-timeout.C:
			t.Fatal("timed out waiting for disk command updates")
		default:
			runtime.Gosched()
		}
	}
}

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
	for _, command := range []string{"", "host-findmnt"} {
		for _, lookupErr := range []error{nil, os.ErrNotExist, context.DeadlineExceeded, errors.New("findmnt failed")} {
			mockey.PatchConvey(fmt.Sprintf("command %q lookup %v", command, lookupErr), t, func() {
				SetDiskCommands(command, "", "")
				defer SetDiskCommands("", "", "")
				mockey.Mock(currentGOOS).Return("linux").Build()
				mockey.Mock(disk.GetBlockDevicesWithLsblk).Return(disk.BlockDevices{{Name: "/dev/root", MountPoint: "/", FSType: "ext4"}}, nil).Build()
				mockey.Mock(disk.GetMounts).Return(disk.Partitions{{Device: "server:/export", Fstype: "nfs4", MountPoint: "/remote"}}, nil).Build()
				mockey.Mock(disk.FindMountSource).To(func(ctx context.Context, target, prefix string) (string, error) {
					assert.Equal(t, "/var/lib/kubelet", target)
					assert.Equal(t, command, prefix)
					_, bounded := ctx.Deadline()
					assert.True(t, bounded)
					return "/dev/root", lookupErr
				}).Build()
				info, err := GetMachineDiskInfo(context.Background())
				require.NoError(t, err)
				require.Len(t, info.BlockDevices, 2)
				if lookupErr == nil {
					assert.Equal(t, "/dev/root", info.ContainerRootDisk)
				} else {
					assert.Empty(t, info.ContainerRootDisk)
				}
			})
		}
	}
	mockey.PatchConvey("lsblk failure remains fatal", t, func() {
		mockey.Mock(currentGOOS).Return("linux").Build()
		mockey.Mock(disk.GetBlockDevicesWithLsblk).Return(nil, errors.New("lsblk failed")).Build()
		_, err := GetMachineDiskInfo(context.Background())
		require.ErrorContains(t, err, "lsblk failed")
	})
	mockey.PatchConvey("optional inventory timeout preserves local disks and caller cancellation", t, func() {
		mockey.Mock(currentGOOS).Return("linux").Build()
		mockey.Mock(disk.GetBlockDevicesWithLsblk).Return(disk.BlockDevices{{Name: "/dev/root", MountPoint: "/", FSType: "ext4"}}, nil).Build()
		mockey.Mock(disk.GetMounts).Return(nil, context.DeadlineExceeded).Build()
		mockey.Mock(disk.FindMountSource).Return("", context.DeadlineExceeded).Build()
		info, err := GetMachineDiskInfo(context.Background())
		require.NoError(t, err)
		require.Len(t, info.BlockDevices, 1)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		info, err = GetMachineDiskInfo(ctx)
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, info)
	})
	mockey.PatchConvey("caller cancellation during kubelet lookup is honored", t, func() {
		mockey.Mock(currentGOOS).Return("linux").Build()
		mockey.Mock(disk.GetBlockDevicesWithLsblk).Return(disk.BlockDevices{}, nil).Build()
		mockey.Mock(disk.GetMounts).Return(disk.Partitions{}, nil).Build()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		mockey.Mock(disk.FindMountSource).To(func(context.Context, string, string) (string, error) {
			cancel()
			return "", context.Canceled
		}).Build()
		info, err := GetMachineDiskInfo(ctx)
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, info)
	})
}

func TestMachineInfoDiskCommands_WithMockey(t *testing.T) {
	mockey.PatchConvey("machine info uses configured host disk commands", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()

		findmntPath := filepath.Join(t.TempDir(), "findmnt.sh")
		require.NoError(t, os.WriteFile(findmntPath, []byte(`case "$*" in
*--target*) printf '%s\n' '{"filesystems":[{"target":"/","source":"/dev/sda1","fstype":"ext4"}]}' ;;
*) printf '%s\n' '{"filesystems":[{"target":"/mnt/nfs","source":"server:/data","fstype":"nfs4"}]}' ;;
esac
`), 0600))
		findmnt := "sh '" + findmntPath + "'"
		lsblk := `printf '%s\n' '{"blockdevices":[{"name":"/dev/sda","type":"disk","size":1000,"mountpoint":null,"fstype":null,"children":[{"name":"/dev/sda1","type":"part","size":1000,"mountpoint":"/","fstype":"ext4","fsused":"400"}]}]}' #`
		df := `printf '%s\n' 'Filesystem Type 1B-blocks Used Available Use% Mounted on' '/dev/sda1 ext4 1000 400 600 40% /' 'server:/data nfs4 2000 1000 1000 50% /mnt/nfs' #`
		SetDiskCommands(findmnt, lsblk, df)
		defer SetDiskCommands("", "", "")

		// The configured commands run as short-lived shell fixtures; under the
		// full CI suite such a process can sporadically produce no parsed rows
		// (the same failure class as the df fixture documented below, observed
		// on findmnt or df fixtures under heavy test runner load). The probe is
		// read-only and idempotent, so retry a few times to assert the command
		// wiring rather than the subprocess race.
		var info *apiv1.MachineDiskInfo
		var err error
		for range 5 {
			info, err = GetMachineDiskInfo(context.Background())
			if err == nil && info != nil && info.ContainerRootDisk != "" && len(info.BlockDevices) == 2 {
				break
			}
		}
		require.NoError(t, err)
		require.Len(t, info.BlockDevices, 2)
		assert.Equal(t, "/dev/sda1", info.ContainerRootDisk)
		assert.Equal(t, "/dev/sda1", info.BlockDevices[0].Name)
		assert.Equal(t, "server:/data", info.BlockDevices[1].Name)

		mockey.Mock(disk.GetUsageWithCommand).To(func(ctx context.Context, target, command string) (*disk.Usage, error) {
			assert.Equal(t, "/", target)
			assert.Equal(t, df, command)
			return &disk.Usage{TotalBytes: 1000}, nil
		}).Build()

		total, err := GetSystemResourceRootVolumeTotal()
		require.NoError(t, err)
		assert.Equal(t, "1k", total)
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
		// Pin CRI-O detection so this test stays host-independent: with
		// ContainerRuntimeVersion empty, GetMachineInfo would otherwise probe
		// the host's real CRI-O endpoint and could set the version on a CRI-O box.
		mockey.Mock(componentcontainerd.CheckCRIORunning).To(func(ctx context.Context) bool { return false }).Build()
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

	mockey.PatchConvey("GetMachineInfo falls back to CRI-O version when containerd is absent", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(GetMachineGPUInfo).To(func(nvidianvml.Instance) (*apiv1.MachineGPUInfo, error) {
			return &apiv1.MachineGPUInfo{}, nil
		}).Build()
		mockey.Mock(GetMachineDiskInfo).To(func(ctx context.Context) (*apiv1.MachineDiskInfo, error) {
			return &apiv1.MachineDiskInfo{}, nil
		}).Build()
		// containerd binary present but not running (CRI-O node): containerd
		// version stays empty, so the CRI-O fallback must supply the runtime.
		mockey.Mock(componentcontainerd.CheckContainerdInstalled).To(func() bool { return true }).Build()
		mockey.Mock(componentcontainerd.CheckContainerdRunning).To(func(ctx context.Context) bool { return false }).Build()
		mockey.Mock(componentcontainerd.CheckCRIORunning).To(func(ctx context.Context) bool { return true }).Build()
		mockey.Mock(componentcontainerd.GetVersion).To(func(ctx context.Context, endpoint string) (string, error) {
			if endpoint == componentcontainerd.DefaultCRIOEndpoint {
				return "1.30.3", nil
			}
			return "", errors.New("containerd endpoint unreachable")
		}).Build()
		mockey.Mock(componenttailscale.CheckTailscaleInstalled).To(func() bool { return false }).Build()

		info, err := GetMachineInfo(baseNVML)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "cri-o://1.30.3", info.ContainerRuntimeVersion)
	})

	mockey.PatchConvey("GetMachineInfo keeps runtime empty when the CRI-O version lookup fails", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(GetMachineGPUInfo).To(func(nvidianvml.Instance) (*apiv1.MachineGPUInfo, error) {
			return &apiv1.MachineGPUInfo{}, nil
		}).Build()
		mockey.Mock(GetMachineDiskInfo).To(func(ctx context.Context) (*apiv1.MachineDiskInfo, error) {
			return &apiv1.MachineDiskInfo{}, nil
		}).Build()
		// CRI-O is live but its Version RPC fails: the runtime must stay empty
		// rather than report a version we never actually read.
		mockey.Mock(componentcontainerd.CheckContainerdInstalled).To(func() bool { return false }).Build()
		mockey.Mock(componentcontainerd.CheckContainerdRunning).To(func(ctx context.Context) bool { return false }).Build()
		mockey.Mock(componentcontainerd.CheckCRIORunning).To(func(ctx context.Context) bool { return true }).Build()
		mockey.Mock(componentcontainerd.GetVersion).To(func(ctx context.Context, endpoint string) (string, error) {
			return "", errors.New("cri-o version failed")
		}).Build()
		mockey.Mock(componenttailscale.CheckTailscaleInstalled).To(func() bool { return false }).Build()

		info, err := GetMachineInfo(baseNVML)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "", info.ContainerRuntimeVersion)
	})

	mockey.PatchConvey("GetMachineInfo does not double-prefix an already-prefixed CRI-O version", t, func() {
		mockey.Mock(currentGOOS).To(func() string { return "linux" }).Build()
		mockey.Mock(GetMachineGPUInfo).To(func(nvidianvml.Instance) (*apiv1.MachineGPUInfo, error) {
			return &apiv1.MachineGPUInfo{}, nil
		}).Build()
		mockey.Mock(GetMachineDiskInfo).To(func(ctx context.Context) (*apiv1.MachineDiskInfo, error) {
			return &apiv1.MachineDiskInfo{}, nil
		}).Build()
		mockey.Mock(componentcontainerd.CheckContainerdInstalled).To(func() bool { return false }).Build()
		mockey.Mock(componentcontainerd.CheckContainerdRunning).To(func(ctx context.Context) bool { return false }).Build()
		mockey.Mock(componentcontainerd.CheckCRIORunning).To(func(ctx context.Context) bool { return true }).Build()
		mockey.Mock(componentcontainerd.GetVersion).To(func(ctx context.Context, endpoint string) (string, error) {
			return "cri-o://1.31.0", nil
		}).Build()
		mockey.Mock(componenttailscale.CheckTailscaleInstalled).To(func() bool { return false }).Build()

		info, err := GetMachineInfo(baseNVML)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "cri-o://1.31.0", info.ContainerRuntimeVersion)
	})
}

func TestGetMachineGPUInfo_MinorAndBoardErrors_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetMachineGPUInfo returns minor id error", t, func() {
		dev := newMachineInfoCoverageDevice()
		dev.minorRet = nvml.ERROR_UNKNOWN
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
		assert.Contains(t, err.Error(), "failed to get minor id")
	})

	mockey.PatchConvey("GetMachineGPUInfo returns board id error", t, func() {
		dev := newMachineInfoCoverageDevice()
		dev.boardRet = nvml.ERROR_UNKNOWN
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
		assert.Contains(t, err.Error(), "failed to get board id")
	})
}

func TestGetMachineInfo_GPUInfoError_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetMachineInfo returns gpu info error", t, func() {
		mockey.Mock(GetMachineGPUInfo).To(func(nvidianvml.Instance) (*apiv1.MachineGPUInfo, error) {
			return nil, errors.New("gpu info failed")
		}).Build()

		_, err := GetMachineInfo(&mockNvmlInstanceForMockey{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get machine gpu info")
	})
}

func TestGetProviderWithContext_BlankProviderName_WithMockey(t *testing.T) {
	mockey.PatchConvey("whitespace-only provider name falls back to unknown", t, func() {
		mockey.Mock(pkgprovidersall.Detect).To(func(ctx context.Context) (*providers.Info, error) {
			return &providers.Info{Provider: "   "}, nil
		}).Build()

		info := GetProviderWithContext(context.Background(), "1.2.3.4")
		require.NotNil(t, info)
		assert.Equal(t, "unknown", info.Provider)
	})
}
