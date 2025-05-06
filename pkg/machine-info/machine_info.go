// Package machineinfo provides information about the machine.
package machineinfo

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/asn"
	pkgcontainerd "github.com/leptonai/gpud/pkg/containerd"
	"github.com/leptonai/gpud/pkg/disk"
	pkgdisk "github.com/leptonai/gpud/pkg/disk"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
	pkgnetutillatencyedge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/version"
)

func GetMachineInfo(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
	hostname, _ := os.Hostname()
	info := &apiv1.MachineInfo{
		GPUdVersion: version.Version,

		GPUDriverVersion:        nvmlInstance.DriverVersion(),
		CUDAVersion:             nvmlInstance.CUDAVersion(),
		ContainerRuntimeVersion: "",
		KernelVersion:           pkghost.KernelVersion(),
		OSImage:                 pkghost.OSName(),
		OperatingSystem:         runtime.GOOS,
		SystemUUID:              pkghost.SystemUUID(),
		MachineID:               pkghost.OSMachineID(),
		BootID:                  pkghost.BootID(),
		Hostname:                hostname,
		Uptime:                  metav1.NewTime(time.Unix(int64(pkghost.BootTimeUnixSeconds()), 0)),

		CPUInfo: GetMachineCPUInfo(),
	}

	var err error
	info.GPUInfo, err = GetMachineGPUInfo(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine gpu info: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if runtime.GOOS == "linux" {
		info.DiskInfo, err = GetMachineDiskInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get machine disk info: %w", err)
		}

		if pkgcontainerd.CheckContainerdInstalled() && pkgcontainerd.CheckContainerdRunning(ctx) {
			containerdVersion, err := pkgcontainerd.GetVersion(ctx, pkgcontainerd.DefaultContainerRuntimeEndpoint)
			if err != nil {
				log.Logger.Warnw("failed to check containerd version", "error", err)
			} else {
				if !strings.HasPrefix(containerdVersion, "containerd://") {
					containerdVersion = "containerd://" + containerdVersion
				}
				info.ContainerRuntimeVersion = containerdVersion
			}
		}
	}

	return info, nil
}

// GetSystemResourceMemoryTotal returns the system memory resource of the machine
// for the total memory size, using the type defined in "corev1.ResourceName"
// in https://pkg.go.dev/k8s.io/api/core/v1#ResourceName.
// It represents the Memory, in bytes (500Gi = 500GiB = 500 * 1024 * 1024 * 1024).
// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
func GetSystemResourceMemoryTotal() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get memory: %w", err)
	}

	qty := resource.NewQuantity(int64(vm.Total), resource.DecimalSI)
	return qty.String(), nil
}

// GetSystemResourceRootVolumeTotal returns the system root disk resource of the machine
// for the total disk size, using the type defined in "corev1.ResourceName"
// in https://pkg.go.dev/k8s.io/api/core/v1#ResourceName.
// It represents the Volume size, in bytes (e,g. 5Gi = 5GiB = 5 * 1024 * 1024 * 1024).
// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
func GetSystemResourceRootVolumeTotal() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	usage, err := pkgdisk.GetUsage(ctx, "/")
	if err != nil {
		return "", fmt.Errorf("failed to get disk usage: %w", err)
	}

	qty := resource.NewQuantity(int64(usage.TotalBytes), resource.DecimalSI)
	return qty.String(), nil
}

// GetSystemResourceLogicalCores returns the system CPU resource of the machine
// with the logical core counts, using the type defined in "corev1.ResourceName"
// in https://pkg.go.dev/k8s.io/api/core/v1#ResourceName.
// It represents the CPU, in cores (500m = .5 cores).
// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
func GetSystemResourceLogicalCores() (string, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// counting the number of logical CPU cores available to the system
	// same as "nproc --all"
	cnt, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get CPU cores count: %w", err)
	}

	qty := resource.NewQuantity(int64(cnt), resource.DecimalSI)
	return qty.String(), int64(cnt), nil
}

func GetMachineCPUInfo() *apiv1.MachineCPUInfo {
	return &apiv1.MachineCPUInfo{
		Architecture: runtime.GOARCH,
	}
}

func GetMachineNetwork() *apiv1.MachineNetwork {
	publicIP, _ := netutil.PublicIP()
	return &apiv1.MachineNetwork{
		PublicIP:  publicIP,
		PrivateIP: "",
	}
}

func GetProvider(publicIP string) string {
	if publicIP == "" {
		return ""
	}
	asnResult, err := asn.GetASLookup(publicIP)
	if err != nil {
		return ""
	}
	return asnResult.AsnName
}

func GetMachineLocation() *apiv1.MachineLocation {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	latencies, err := pkgnetutillatencyedge.Measure(ctx)
	if err != nil || len(latencies) == 0 {
		return nil
	}

	closest := latencies.Closest()
	return &apiv1.MachineLocation{
		Region: closest.RegionCode,
	}
}

// GetSystemResourceGPUCount returns the system GPU resource of the machine
// with the GPU count, using the type defined in "corev1.ResourceName"
// in https://pkg.go.dev/k8s.io/api/core/v1#ResourceName.
// It represents the GPU count with the key "nvidia.com/gpu" or "nvidia.com/gpu.count".
// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
//
// This is different from the device count in DCGM.
// ref. "CountDevEntry" in "nvvs/plugin_src/software/Software.cpp"
// ref. https://github.com/NVIDIA/DCGM/blob/903d745504f50153be8293f8566346f9de3b3c93/nvvs/plugin_src/software/Software.cpp#L220-L249
func GetSystemResourceGPUCount(nvmlInstance nvidianvml.Instance) (string, error) {
	deviceCount := len(nvmlInstance.Devices())
	if deviceCount == 0 {
		return "0", nil
	}

	qty := resource.NewQuantity(int64(deviceCount), resource.DecimalSI)
	return qty.String(), nil
}

func GetMachineGPUInfo(nvmlInstance nvidianvml.Instance) (*apiv1.MachineGPUInfo, error) {
	info := &apiv1.MachineGPUInfo{
		Product:      nvmlInstance.ProductName(),
		Manufacturer: nvmlInstance.Brand(),
		Architecture: nvmlInstance.Architecture(),
	}

	for uuid, dev := range nvmlInstance.Devices() {
		if info.Memory == "" {
			gpuMemory, err := nvidianvml.GetMemory(uuid, dev)
			if err != nil {
				return nil, err
			}

			qty := resource.NewQuantity(int64(gpuMemory.TotalBytes), resource.DecimalSI)
			info.Memory = qty.String()
		}

		serialID, err := nvidianvml.GetSerial(uuid, dev)
		if err != nil {
			return nil, err
		}

		minorID, err := nvidianvml.GetMinorID(uuid, dev)
		if err != nil {
			return nil, err
		}

		info.GPUs = append(info.GPUs, apiv1.MachineGPUInstance{
			UUID:    uuid,
			SN:      serialID,
			MinorID: strconv.Itoa(minorID),
		})
	}

	return info, nil
}

func GetMachineDiskInfo(ctx context.Context) (*apiv1.MachineDiskInfo, error) {
	blks, err := disk.GetBlockDevicesWithLsblk(
		ctx,
		disk.WithFstype(func(fs string) bool {
			return fs == "" ||
				fs == "ext4" ||
				fs == "LVM2_member" ||
				fs == "linux_raid_member"
		}),
		disk.WithDeviceType(func(dt string) bool {
			return dt == "disk" || dt == "lvm" || dt == "part"
		},
		))
	if err != nil {
		return nil, err
	}
	flattened := blks.Flatten()

	rs := make([]apiv1.MachineDiskDevice, 0, len(flattened))
	for _, bd := range flattened {
		rs = append(rs, apiv1.MachineDiskDevice{
			Name:       bd.Name,
			Type:       bd.Type,
			Size:       int64(bd.Size),
			Rota:       bd.Rota,
			Serial:     bd.Serial,
			WWN:        bd.WWN,
			Vendor:     bd.Vendor,
			Model:      bd.Model,
			Rev:        bd.Rev,
			MountPoint: bd.MountPoint,
			FSType:     bd.FSType,
			PartUUID:   bd.PartUUID,
			Parents:    bd.Parents,
			Children:   bd.Children,
		})
	}

	info := &apiv1.MachineDiskInfo{
		BlockDevices: rs,
	}

	if runtime.GOOS == "linux" {
		_, serr := os.Stat("/var/lib/kubelet")
		if serr != nil && !os.IsNotExist(serr) {
			return nil, serr
		}
		if serr == nil {
			out, err := pkgdisk.FindMnt(ctx, "/var/lib/kubelet")
			if err != nil {
				return nil, err
			}
			if len(out.Filesystems) > 0 && len(out.Filesystems[0].Sources) > 0 {
				info.ContainerRootDisk = out.Filesystems[0].Sources[0]
			}
		}
	}

	return info, nil
}
