// Package machineinfo provides information about the machine.
package machineinfo

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidiamemory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	componentcontainerd "github.com/leptonai/gpud/components/containerd"
	componenttailscale "github.com/leptonai/gpud/components/tailscale"
	"github.com/leptonai/gpud/pkg/asn"
	"github.com/leptonai/gpud/pkg/disk"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
	pkgnetutillatencyedge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	nvidiapci "github.com/leptonai/gpud/pkg/nvidia/pci"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
	"github.com/leptonai/gpud/pkg/providers"
	pkgprovidersall "github.com/leptonai/gpud/pkg/providers/all"
	"github.com/leptonai/gpud/pkg/providers/nebius"
	"github.com/leptonai/gpud/version"
)

const (
	diskPartitionsTimeout             = 10 * time.Second
	minDriverMajorForNVMLPlatformInfo = 570
)

type diskCommands struct {
	findmntCommand       string
	lsblkCommand         string
	blockdevUsageCommand string
}

var diskCommandConfig atomic.Pointer[diskCommands]

// SetDiskCommands configures machine-info disk collection to run in the host namespace.
func SetDiskCommands(findmnt, lsblk, blockdevUsage string) {
	diskCommandConfig.Store(&diskCommands{
		findmntCommand:       findmnt,
		lsblkCommand:         lsblk,
		blockdevUsageCommand: blockdevUsage,
	})
}

func getDiskCommands() diskCommands {
	if commands := diskCommandConfig.Load(); commands != nil {
		return *commands
	}
	return diskCommands{}
}

func currentGOOS() string {
	return runtime.GOOS
}

func GetMachineInfo(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
	hostname, _ := os.Hostname()
	info := &apiv1.MachineInfo{
		GPUdVersion: version.Version,

		GPUDriverVersion:        nvmlInstance.DriverVersion(),
		CUDAVersion:             nvmlInstance.CUDAVersion(),
		ContainerRuntimeVersion: "",
		KernelVersion:           pkghost.KernelVersion(),
		OSImage:                 pkghost.OSName(),
		OperatingSystem:         currentGOOS(),
		SystemUUID:              pkghost.SystemUUID(),
		MachineID:               pkghost.OSMachineID(),
		BootID:                  pkghost.BootID(),
		Hostname:                hostname,
		Uptime:                  metav1.NewTime(time.Unix(int64(pkghost.BootTimeUnixSeconds()), 0)),

		CPUInfo:    GetMachineCPUInfo(),
		MemoryInfo: GetMachineMemoryInfo(),
		NICInfo:    GetMachineNICInfo(),
	}

	var err error
	info.GPUInfo, err = GetMachineGPUInfo(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine gpu info: %w", err)
	}

	// The control plane records these values in the Machine CR status, so
	// that operators can see the fabric domain and the clique of each node.
	// Only NVLink fabric systems (e.g., NVIDIA GB200 NVL72) report them;
	// they stay empty on other platforms.
	info.ClusterUUID, info.CliqueID = getGPUFabricIdentifiers(nvmlInstance)
	info.ChassisSerial = getGPUChassisSerial(nvmlInstance)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if currentGOOS() == "linux" {
		info.DiskInfo, err = GetMachineDiskInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get machine disk info: %w", err)
		}

		if componentcontainerd.CheckContainerdInstalled() && componentcontainerd.CheckContainerdRunning(ctx) {
			containerdVersion, err := componentcontainerd.GetVersion(ctx, componentcontainerd.DefaultContainerRuntimeEndpoint)
			if err != nil {
				log.Logger.Warnw("failed to check containerd version", "error", err)
			} else {
				if !strings.HasPrefix(containerdVersion, "containerd://") {
					containerdVersion = "containerd://" + containerdVersion
				}
				info.ContainerRuntimeVersion = containerdVersion
			}
		}

		// On a CRI-O node the containerd binary is installed but not running,
		// so the block above leaves the runtime version empty even though the
		// node has a live runtime. Fall back to CRI-O so the reported runtime
		// reflects what actually runs the node's containers (LEP-6128).
		if info.ContainerRuntimeVersion == "" && componentcontainerd.CheckCRIORunning(ctx) {
			crioVersion, err := componentcontainerd.GetVersion(ctx, componentcontainerd.DefaultCRIOEndpoint)
			if err != nil {
				log.Logger.Warnw("failed to check cri-o version", "error", err)
			} else {
				if !strings.HasPrefix(crioVersion, "cri-o://") {
					crioVersion = "cri-o://" + crioVersion
				}
				info.ContainerRuntimeVersion = crioVersion
			}
		}

		// Collect tailscale version if installed
		if componenttailscale.CheckTailscaleInstalled() {
			tailscaleVersion, err := componenttailscale.GetTailscaleVersion()
			if err != nil {
				log.Logger.Warnw("failed to get tailscale version", "error", err)
			} else {
				info.TailscaleVersion = strings.TrimSpace(tailscaleVersion)
			}
		}

	}

	return info, nil
}

// uint64ToInt64Capped converts a uint64 to int64, capping at math.MaxInt64.
// A bare int64(v) conversion wraps values above math.MaxInt64 to negative
// numbers; capping keeps disk sizes/usage non-negative and monotonic. Real disk
// sizes never legitimately exceed ~9.2 EB, so the cap is a safety bound rather
// than an expected case. (Also satisfies CodeQL's incorrect-integer-conversion
// check by bounding the value before the conversion.)
func uint64ToInt64Capped(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// GetSystemResourceRootVolumeTotal returns the system root disk resource of the machine
// for the total disk size, using the type defined in "corev1.ResourceName"
// in https://pkg.go.dev/k8s.io/api/core/v1#ResourceName.
// It represents the Volume size, in bytes (e,g. 5Gi = 5GiB = 5 * 1024 * 1024 * 1024).
// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
func GetSystemResourceRootVolumeTotal() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	commands := getDiskCommands()
	var usage *disk.Usage
	if commands.blockdevUsageCommand == "" {
		var err error
		usage, err = disk.GetUsage(ctx, "/")
		if err != nil {
			return "", fmt.Errorf("failed to get disk usage: %w", err)
		}
	} else {
		partitions, err := disk.GetPartitions(
			ctx,
			disk.WithMountPoint(func(mountPoint string) bool { return mountPoint == "/" }),
			disk.WithBlockdevUsageCommand(commands.blockdevUsageCommand),
		)
		if err != nil {
			return "", fmt.Errorf("failed to get disk usage: %w", err)
		}
		if len(partitions) == 0 || partitions[0].Usage == nil {
			return "", fmt.Errorf("failed to get disk usage: root partition not found")
		}
		usage = partitions[0].Usage
	}

	qty := resource.NewQuantity(uint64ToInt64Capped(usage.TotalBytes), resource.DecimalSI)
	return qty.String(), nil
}

func GetMachineCPUInfo() *apiv1.MachineCPUInfo {
	info := &apiv1.MachineCPUInfo{
		Type:         pkghost.CPUModelName(),
		Manufacturer: pkghost.CPUVendorID(),
		Architecture: runtime.GOARCH,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// counting the number of logical CPU cores available to the system
	// same as "nproc --all"
	cnt, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		log.Logger.Errorw("failed to get logical CPU cores count", "error", err)
	}
	info.LogicalCores = int64(cnt)

	return info
}

func GetMachineMemoryInfo() *apiv1.MachineMemoryInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		log.Logger.Errorw("failed to get memory info", "error", err)
		return &apiv1.MachineMemoryInfo{
			TotalBytes: 0,
		}
	}

	return &apiv1.MachineMemoryInfo{
		TotalBytes: vm.Total,
	}
}

func GetMachineNICInfo() *apiv1.MachineNICInfo {
	ifaces := []apiv1.MachineNetworkInterface{}
	privateIPs, err := netutil.GetPrivateIPs(
		netutil.WithPrefixesToSkip(
			"lo",
			// Do NOT skip "eni"/"lepton": on some nodes these are host-facing NICs
			// carrying the only RFC1918 address. login_request.go relies on this list
			// as a fallback when provider metadata does not return a usable private IP.
			"cali",
			"cni",
			"docker",
			"flannel",
			"nodelocaldns",
			"tailscale",
			"tunl",
			"veth",
			"vxlan",
			"ib", // e.g., "ibp24s0" infiniband links
		),
		netutil.WithSuffixesToSkip(".calico"),
	)
	if err != nil {
		log.Logger.Errorw("failed to get private ips", "error", err)
	}

	for _, ip := range privateIPs {
		addr := ip.Addr.String()
		if addr == "" {
			continue
		}
		ifaces = append(ifaces, apiv1.MachineNetworkInterface{
			Interface: ip.Iface.Name,
			MAC:       ip.Iface.HardwareAddr.String(),
			IP:        ip.Addr.String(),
			Addr:      ip.Addr,
		})
	}

	sort.Slice(ifaces, func(i, j int) bool {
		return ifaces[i].IP < ifaces[j].IP
	})
	return &apiv1.MachineNICInfo{
		PrivateIPInterfaces: ifaces,
	}
}

// GetProvider looks up the provider of the machine.
// If the metadata service or other provider detection fails, it falls back to ASN lookup
// using the public IP address.
func GetProvider(publicIP string) *providers.Info {
	return GetProviderWithContext(context.Background(), publicIP)
}

// GetProviderWithContext passes the caller's context to metadata provider detection.
func GetProviderWithContext(ctx context.Context, publicIP string) *providers.Info {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	providerInfo, err := pkgprovidersall.Detect(ctx)
	if err != nil {
		log.Logger.Warnw("failed to detect provider", "error", err)
	} else {
		log.Logger.Debugw("provider detection result", "provider", providerInfo)
	}

	if providerInfo == nil {
		log.Logger.Debugw("providerInfo is nil, creating default")
		providerInfo = &providers.Info{
			Provider: "unknown",
		}
	}
	if providerInfo.PublicIP == "" {
		providerInfo.PublicIP = publicIP
	}
	if providerInfo.Provider == "" {
		log.Logger.Debugw("providerInfo.Provider is empty, setting to unknown")
		providerInfo.Provider = "unknown"
	}
	providerInfo.Provider = canonicalizeProviderName(providerInfo.Provider)
	if providerInfo.Provider == "" {
		providerInfo.Provider = "unknown"
	}

	log.Logger.Debugw("provider after initial detection", "provider", providerInfo.Provider, "publicIP", providerInfo.PublicIP)

	if providerInfo.Provider != "unknown" {
		log.Logger.Debugw("returning detected provider", "provider", providerInfo.Provider)
		return providerInfo
	}

	if publicIP != "" {
		// fallback to ASN lookup
		log.Logger.Debugw("fallback to ASN lookup for provider", "publicIP", publicIP)
		asnResult, err := asn.GetASLookup(publicIP)
		if err != nil {
			log.Logger.Warnw("ASN lookup failed", "error", err, "publicIP", publicIP)
			return providerInfo
		}

		normalizedProvider := asn.NormalizeASNName(asnResult.AsnName)
		log.Logger.Debugw("ASN lookup result", "asnResult", asnResult, "asnName", asnResult.AsnName, "normalizedProvider", normalizedProvider)

		// Ensure we don't set an empty provider
		if normalizedProvider != "" {
			providerInfo.Provider = normalizedProvider
		} else {
			// as lookup succeeded but normalized provider is empty
			providerInfo.Provider = asnResult.AsnName
			log.Logger.Warnw("normalized provider is empty -- fallback to raw asn name for provider name", "asnName", asnResult.AsnName)
		}
	} else {
		log.Logger.Warnw("no public IP provided for ASN lookup")
	}

	providerInfo.Provider = canonicalizeProviderName(providerInfo.Provider)
	if providerInfo.Provider == "" {
		providerInfo.Provider = "unknown"
	}

	if providerInfo.Provider == "nebius" && providerInfo.InstanceID == "" {
		instanceID, err := nebius.GetInstanceID(ctx)
		if err != nil {
			log.Logger.Warnw("failed to get Nebius instance ID", "error", err)
		} else {
			providerInfo.InstanceID = instanceID
		}
	}

	log.Logger.Debugw("GetProvider returning",
		"provider", providerInfo.Provider,
		"publicIP", providerInfo.PublicIP,
		"privateIP", providerInfo.PrivateIP,
		"instanceID", providerInfo.InstanceID,
	)

	return providerInfo
}

func canonicalizeProviderName(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return ""
	}
	normalized := asn.NormalizeASNName(provider)
	if normalized == "" {
		return provider
	}
	return normalized
}

//go:noinline
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
		// fallback to pci in case nvml/nvidia driver has not been loaded
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		devs, err := nvidiapci.ListPCIGPUs(ctx)
		if err != nil {
			log.Logger.Errorw("failed to list nvidia pci devices", "error", err)
		}
		deviceCount = len(devs)
	}
	if deviceCount == 0 {
		return "0", nil
	}

	qty := resource.NewQuantity(int64(deviceCount), resource.DecimalSI)
	return qty.String(), nil
}

// getGPUFabricIdentifiers returns the NVLink fabric cluster UUID and clique ID
// of the machine (e.g., NVIDIA GB200 NVL72). All GPUs on the same machine share
// these identifiers, so the first device with a readable fabric state is
// sufficient. Fabric state telemetry is not supported on all platforms; in
// that case this function returns empty values and does not fail.
func getGPUFabricIdentifiers(nvmlInstance nvidianvml.Instance) (string, uint32) {
	for uuid, dev := range nvmlInstance.Devices() {
		fabricState, err := dev.GetFabricState()
		if err != nil {
			log.Logger.Debugw("failed to get GPU fabric state for machine info", "uuid", uuid, "error", err)
			continue
		}
		return fabricState.ClusterUUID, fabricState.CliqueID
	}
	return "", 0
}

// getGPUChassisSerial returns the chassis serial reported by NVML platform
// info. The API first shipped with R570; calling it on older drivers can fail
// during symbol resolution before NVML can return ERROR_NOT_SUPPORTED.
func getGPUChassisSerial(nvmlInstance nvidianvml.Instance) string {
	if nvmlInstance.DriverMajor() < minDriverMajorForNVMLPlatformInfo {
		return ""
	}
	for uuid, dev := range nvmlInstance.Devices() {
		platformInfo, ret := dev.GetPlatformInfo()
		if ret != nvml.SUCCESS {
			log.Logger.Debugw("failed to get GPU platform info for machine info", "uuid", uuid, "error", nvml.ErrorString(ret))
			continue
		}
		if serial := strings.TrimRight(string(platformInfo.ChassisSerialNumber[:]), "\x00"); serial != "" {
			return serial
		}
	}
	return ""
}

func GetMachineGPUInfo(nvmlInstance nvidianvml.Instance) (*apiv1.MachineGPUInfo, error) {
	info := &apiv1.MachineGPUInfo{
		Product:      nvmlInstance.ProductName(),
		Manufacturer: nvmlInstance.Brand(),
		Architecture: nvmlInstance.Architecture(),
	}

	// NVML product detection needs the NVIDIA userspace driver library,
	// which is unavailable when the driver is installed and managed by the
	// NVIDIA GPU Operator (its toolkit is injected only into GPU workload
	// containers) or when the Operator-managed driver is still being
	// installed during initial node discovery. Fall back to PCI sysfs
	// enumeration, which needs no driver, so the GPU product is still
	// reported (LEP-6173).
	if info.Product == "" {
		info.Product = detectGPUProductFromPCI()
	}

	productName := nvmlInstance.ProductName()
	for uuid, dev := range nvmlInstance.Devices() {
		if info.Memory == "" {
			gpuMemory, err := nvidiamemory.GetMemory(uuid, dev, productName, mem.VirtualMemoryWithContext)
			if err != nil {
				return nil, err
			}

			qty := resource.NewQuantity(int64(gpuMemory.TotalBytes), resource.DecimalSI)
			info.Memory = qty.String()
		}

		serialID, ret := dev.GetSerial()
		if ret != nvml.SUCCESS {
			if ret != nvml.ERROR_NOT_SUPPORTED {
				return nil, fmt.Errorf("failed to get serial id: %v", nvml.ErrorString(ret))
			}
		}

		var minorID int
		minorID, ret = dev.GetMinorNumber()
		if ret != nvml.SUCCESS {
			if ret != nvml.ERROR_NOT_SUPPORTED {
				return nil, fmt.Errorf("failed to get minor id: %v", nvml.ErrorString(ret))
			}
			minorID = -1 // set to -1 when not supported
		}

		var boardID uint32
		boardID, ret = dev.GetBoardId()
		if ret != nvml.SUCCESS {
			if ret != nvml.ERROR_NOT_SUPPORTED {
				return nil, fmt.Errorf("failed to get board id: %v", nvml.ErrorString(ret))
			}
			boardID = 0 // set to 0 when not supported
		}

		busID, err := dev.GetPCIBusID()
		if err != nil {
			return nil, err
		}

		info.GPUs = append(info.GPUs, apiv1.MachineGPUInstance{
			UUID:    uuid,
			SN:      serialID,
			MinorID: strconv.Itoa(minorID),
			BoardID: boardID,
			BusID:   busID,
		})
	}

	return info, nil
}

// listNVIDIAGPUDevicesFunc lists NVIDIA GPU PCI devices for the product
// name fallback. PCI sysfs exists only on Linux; other platforms get no
// devices. It is a package-level variable so tests can stub PCI device
// discovery.
var listNVIDIAGPUDevicesFunc = func() ([]nvidiapci.GPUDevice, error) {
	if currentGOOS() != "linux" {
		return nil, nil
	}
	return nvidiapci.ListNVIDIAGPUDevices(nvidiapci.DefaultSysfsPCIDevicesDir)
}

// detectGPUProductFromPCI returns the sanitized GPU product name derived
// from the PCI device IDs in sysfs, or an empty string when no known
// NVIDIA GPU is found. PCI enumeration needs no NVIDIA driver, so this
// works even when the driver is absent or still being installed (e.g.,
// GPU Operator-managed drivers during initial node discovery).
func detectGPUProductFromPCI() string {
	devs, err := listNVIDIAGPUDevicesFunc()
	if err != nil {
		log.Logger.Warnw("failed to list NVIDIA GPU PCI devices for product detection", "error", err)
		return ""
	}
	for _, dev := range devs {
		name := nvidiaproduct.GetProductNameByPCIDeviceID(dev.DeviceID)
		if name == "" {
			log.Logger.Debugw("no product name mapping for NVIDIA PCI device ID", "address", dev.Address, "deviceID", dev.DeviceID)
			continue
		}
		sanitized := nvidiaproduct.SanitizeProductName(name)
		log.Logger.Infow("detected GPU product from PCI device ID (NVML unavailable)",
			"address", dev.Address,
			"deviceID", dev.DeviceID,
			"product", sanitized,
		)
		return sanitized
	}
	return ""
}

func GetMachineDiskInfo(ctx context.Context) (*apiv1.MachineDiskInfo, error) {
	commands := getDiskCommands()
	blks, err := disk.GetBlockDevicesWithLsblk(
		ctx,
		disk.WithFstype(disk.DefaultFsTypeFunc),
		disk.WithDeviceType(disk.DefaultDeviceTypeFunc),
		disk.WithFindmntCommand(commands.findmntCommand),
		disk.WithLsblkCommand(commands.lsblkCommand),
	)
	if err != nil {
		return nil, err
	}
	flattened := blks.Flatten()

	rs := make([]apiv1.MachineDiskDevice, 0, len(flattened))
	for _, bd := range flattened {
		if bd.MountPoint == "" {
			continue
		}

		rs = append(rs, apiv1.MachineDiskDevice{
			Name:       bd.Name,
			Type:       bd.Type,
			Size:       uint64ToInt64Capped(bd.Size),
			Used:       uint64ToInt64Capped(bd.FSUsed),
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

	// track nfs partitions only with available fields
	if currentGOOS() == "linux" {
		timeoutCtx, cancel := context.WithTimeout(ctx, diskPartitionsTimeout)
		nfsParts, err := disk.GetPartitions(
			timeoutCtx,
			disk.WithFstype(disk.DefaultNFSFsTypeFunc),
			disk.WithMountPoint(disk.DefaultMountPointFunc),
			disk.WithBlockdevUsageCommand(commands.blockdevUsageCommand),
		)
		cancel()
		if err != nil {
			return nil, err
		}
		for _, part := range nfsParts {
			dev := apiv1.MachineDiskDevice{
				Name:       part.Device,
				Type:       "nfs",
				MountPoint: part.MountPoint,
				FSType:     part.Fstype,
			}
			if part.Usage != nil {
				dev.Size = uint64ToInt64Capped(part.Usage.TotalBytes)
				dev.Used = uint64ToInt64Capped(part.Usage.UsedBytes)
			}
			rs = append(rs, dev)
		}
	}

	info := &apiv1.MachineDiskInfo{
		BlockDevices: rs,
	}

	if currentGOOS() == "linux" {
		kubeletRootExists := commands.findmntCommand != ""
		if !kubeletRootExists {
			_, serr := os.Stat("/var/lib/kubelet")
			if serr != nil && !os.IsNotExist(serr) {
				return nil, serr
			}
			kubeletRootExists = serr == nil
		}
		if kubeletRootExists {
			var out *disk.FindMntOutput
			var err error
			if commands.findmntCommand == "" {
				out, err = disk.FindMnt(ctx, "/var/lib/kubelet")
			} else {
				out, err = disk.FindMntWithCommand(ctx, "/var/lib/kubelet", commands.findmntCommand)
			}
			if err != nil {
				if commands.findmntCommand == "" {
					return nil, err
				}
				log.Logger.Warnw("failed to find container root disk", "error", err)
			} else if len(out.Filesystems) > 0 && len(out.Filesystems[0].Sources) > 0 {
				info.ContainerRootDisk = out.Filesystems[0].Sources[0]
			}
		}
	}

	return info, nil
}
