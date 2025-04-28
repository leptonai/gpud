// Package machineinfo provides information about the machine.
package machineinfo

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/asn"
	pkgcontainerd "github.com/leptonai/gpud/pkg/containerd"
	pkgdisk "github.com/leptonai/gpud/pkg/disk"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
	pkgnetutillatencyedge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/version"
)

func GetMachineInfo(nvmlInstance nvidianvml.Instance) (apiv1.MachineInfo, error) {
	info := apiv1.MachineInfo{
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
		Uptime:                  metav1.NewTime(time.Unix(int64(pkghost.BootTimeUnixSeconds()), 0)),

		CPUInfo: GetMachineCPUInfo(),
	}

	var err error
	info.GPUInfo, err = GetMachineGPUInfo(nvmlInstance)
	if err != nil {
		return apiv1.MachineInfo{}, fmt.Errorf("failed to get machine gpu info: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if pkgcontainerd.CheckContainerdInstalled() && pkgcontainerd.CheckContainerdRunning(ctx) {
		version, err := pkgcontainerd.GetVersion(ctx, pkgcontainerd.DefaultContainerRuntimeEndpoint)
		if err != nil {
			log.Logger.Warnw("failed to check containerd version", "error", err)
		} else {
			if !strings.HasPrefix(version, "containerd://") {
				version = "containerd://" + version
			}
			info.ContainerRuntimeVersion = version
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

func GetMachineCPUInfo() apiv1.MachineCPUInfo {
	return apiv1.MachineCPUInfo{
		Architecture: runtime.GOARCH,
	}
}

func GetMachineNetwork() apiv1.MachineNetwork {
	publicIP, _ := netutil.PublicIP()
	return apiv1.MachineNetwork{
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

func GetMachineLocation() apiv1.MachineLocation {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	latencies, err := pkgnetutillatencyedge.Measure(ctx)
	if err != nil || len(latencies) == 0 {
		return apiv1.MachineLocation{}
	}

	closest := latencies.Closest()
	return apiv1.MachineLocation{
		Region: closest.RegionCode,
	}
}

// GetSystemResourceLogicalCores returns the system GPU resource of the machine
// with the GPU count, using the type defined in "corev1.ResourceName"
// in https://pkg.go.dev/k8s.io/api/core/v1#ResourceName.
// It represents the GPU count with the key "nvidia.com/gpu" or "nvidia.com/gpu.count".
// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
//
// This is different than the device count in DCGM.
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

func GetMachineGPUInfo(nvmlInstance nvidianvml.Instance) (apiv1.MachineGPUInfo, error) {
	info := apiv1.MachineGPUInfo{
		Product: nvmlInstance.ProductName(),
	}

	for uuid, dev := range nvmlInstance.Devices() {
		mem, err := nvidianvml.GetMemory(uuid, dev)
		if err != nil {
			return apiv1.MachineGPUInfo{}, err
		}

		qty := resource.NewQuantity(int64(mem.TotalBytes), resource.DecimalSI)
		info.Memory = qty.String()
		break
	}

	return info, nil
}
