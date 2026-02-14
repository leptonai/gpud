package machineinfo

import (
	"context"
	"errors"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/asn"
	"github.com/leptonai/gpud/pkg/disk"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/netutil"
	"github.com/leptonai/gpud/pkg/netutil/latency"
	pkgnetutillatencyedge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiapci "github.com/leptonai/gpud/pkg/nvidia/pci"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
	"github.com/leptonai/gpud/pkg/providers"
	pkgprovidersall "github.com/leptonai/gpud/pkg/providers/all"
)

// mockNvmlInstanceForMockey implements the nvidianvml.Instance interface for mockey tests
type mockNvmlInstanceForMockey struct {
	driverVersion   string
	cudaVersion     string
	productName     string
	architecture    string
	brand           string
	devices         map[string]device.Device
	nvmlExists      bool
	driverMajor     int
	fabricSupported bool
}

func (m *mockNvmlInstanceForMockey) NVMLExists() bool                  { return m.nvmlExists }
func (m *mockNvmlInstanceForMockey) Library() nvmllib.Library          { return nil }
func (m *mockNvmlInstanceForMockey) Devices() map[string]device.Device { return m.devices }
func (m *mockNvmlInstanceForMockey) ProductName() string               { return m.productName }
func (m *mockNvmlInstanceForMockey) Architecture() string              { return m.architecture }
func (m *mockNvmlInstanceForMockey) Brand() string                     { return m.brand }
func (m *mockNvmlInstanceForMockey) DriverVersion() string             { return m.driverVersion }
func (m *mockNvmlInstanceForMockey) DriverMajor() int                  { return m.driverMajor }
func (m *mockNvmlInstanceForMockey) CUDAVersion() string               { return m.cudaVersion }
func (m *mockNvmlInstanceForMockey) FabricManagerSupported() bool      { return m.fabricSupported }
func (m *mockNvmlInstanceForMockey) FabricStateSupported() bool        { return m.fabricSupported }
func (m *mockNvmlInstanceForMockey) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNvmlInstanceForMockey) Shutdown() error  { return nil }
func (m *mockNvmlInstanceForMockey) InitError() error { return nil }

// TestGetMachineInfo_WithMockedDependencies tests GetMachineInfo with all dependencies mocked
func TestGetMachineInfo_WithMockedDependencies(t *testing.T) {
	mockey.PatchConvey("GetMachineInfo with mocked dependencies", t, func() {
		// Mock host package functions
		mockey.Mock(pkghost.KernelVersion).To(func() string {
			return "5.15.0-generic"
		}).Build()

		mockey.Mock(pkghost.OSName).To(func() string {
			return "Ubuntu 22.04.2 LTS"
		}).Build()

		mockey.Mock(pkghost.SystemUUID).To(func() string {
			return "test-system-uuid-1234"
		}).Build()

		mockey.Mock(pkghost.OSMachineID).To(func() string {
			return "test-machine-id-5678"
		}).Build()

		mockey.Mock(pkghost.BootID).To(func() string {
			return "test-boot-id-9012"
		}).Build()

		mockey.Mock(pkghost.BootTimeUnixSeconds).To(func() uint64 {
			return uint64(time.Now().Add(-24 * time.Hour).Unix())
		}).Build()

		mockey.Mock(pkghost.CPUModelName).To(func() string {
			return "Intel(R) Xeon(R) CPU @ 2.20GHz"
		}).Build()

		mockey.Mock(pkghost.CPUVendorID).To(func() string {
			return "GenuineIntel"
		}).Build()

		// Mock cpu.CountsWithContext
		mockey.Mock(cpu.CountsWithContext).To(func(ctx context.Context, logical bool) (int, error) {
			return 16, nil
		}).Build()

		// Mock mem.VirtualMemoryWithContext
		mockey.Mock(mem.VirtualMemoryWithContext).To(func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{
				Total: 64 * 1024 * 1024 * 1024, // 64GB
			}, nil
		}).Build()

		// Mock netutil.GetPrivateIPs
		mockey.Mock(netutil.GetPrivateIPs).To(func(opts ...netutil.OpOption) (netutil.InterfaceAddrs, error) {
			return netutil.InterfaceAddrs{}, nil
		}).Build()

		// Create mock NVML instance
		mockInstance := &mockNvmlInstanceForMockey{
			driverVersion: "550.90.07",
			cudaVersion:   "12.4",
			productName:   "NVIDIA H100 80GB HBM3",
			architecture:  "hopper",
			brand:         "Tesla",
			devices:       map[string]device.Device{},
			nvmlExists:    true,
			driverMajor:   550,
		}

		info, err := GetMachineInfo(mockInstance)
		require.NoError(t, err)
		require.NotNil(t, info)

		// Verify mocked values
		assert.Equal(t, "550.90.07", info.GPUDriverVersion)
		assert.Equal(t, "12.4", info.CUDAVersion)
		assert.Equal(t, "5.15.0-generic", info.KernelVersion)
		assert.Equal(t, "Ubuntu 22.04.2 LTS", info.OSImage)
		assert.Equal(t, runtime.GOOS, info.OperatingSystem)
		assert.Equal(t, "test-system-uuid-1234", info.SystemUUID)
		assert.Equal(t, "test-machine-id-5678", info.MachineID)
		assert.Equal(t, "test-boot-id-9012", info.BootID)
		assert.NotNil(t, info.CPUInfo)
		assert.Equal(t, "Intel(R) Xeon(R) CPU @ 2.20GHz", info.CPUInfo.Type)
		assert.Equal(t, "GenuineIntel", info.CPUInfo.Manufacturer)
		assert.Equal(t, int64(16), info.CPUInfo.LogicalCores)
		assert.NotNil(t, info.MemoryInfo)
		assert.Equal(t, uint64(64*1024*1024*1024), info.MemoryInfo.TotalBytes)
	})
}

// TestGetMachineCPUInfo_WithMockedCPU tests GetMachineCPUInfo with mocked CPU functions
func TestGetMachineCPUInfo_WithMockedCPU(t *testing.T) {
	mockey.PatchConvey("GetMachineCPUInfo with mocked CPU count success", t, func() {
		mockey.Mock(pkghost.CPUModelName).To(func() string {
			return "AMD EPYC 7763 64-Core Processor"
		}).Build()

		mockey.Mock(pkghost.CPUVendorID).To(func() string {
			return "AuthenticAMD"
		}).Build()

		mockey.Mock(cpu.CountsWithContext).To(func(ctx context.Context, logical bool) (int, error) {
			return 128, nil
		}).Build()

		cpuInfo := GetMachineCPUInfo()
		require.NotNil(t, cpuInfo)
		assert.Equal(t, "AMD EPYC 7763 64-Core Processor", cpuInfo.Type)
		assert.Equal(t, "AuthenticAMD", cpuInfo.Manufacturer)
		assert.Equal(t, runtime.GOARCH, cpuInfo.Architecture)
		assert.Equal(t, int64(128), cpuInfo.LogicalCores)
	})

	mockey.PatchConvey("GetMachineCPUInfo with CPU count error", t, func() {
		mockey.Mock(pkghost.CPUModelName).To(func() string {
			return "Test CPU"
		}).Build()

		mockey.Mock(pkghost.CPUVendorID).To(func() string {
			return "TestVendor"
		}).Build()

		mockey.Mock(cpu.CountsWithContext).To(func(ctx context.Context, logical bool) (int, error) {
			return 0, errors.New("failed to get CPU count")
		}).Build()

		cpuInfo := GetMachineCPUInfo()
		require.NotNil(t, cpuInfo)
		assert.Equal(t, "Test CPU", cpuInfo.Type)
		assert.Equal(t, int64(0), cpuInfo.LogicalCores) // Error case sets to 0
	})
}

// TestGetMachineMemoryInfo_WithMockedMemory tests GetMachineMemoryInfo with mocked memory functions
func TestGetMachineMemoryInfo_WithMockedMemory(t *testing.T) {
	mockey.PatchConvey("GetMachineMemoryInfo with mocked memory success", t, func() {
		mockey.Mock(mem.VirtualMemoryWithContext).To(func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{
				Total:       256 * 1024 * 1024 * 1024, // 256GB
				Available:   128 * 1024 * 1024 * 1024,
				Used:        128 * 1024 * 1024 * 1024,
				UsedPercent: 50.0,
			}, nil
		}).Build()

		memInfo := GetMachineMemoryInfo()
		require.NotNil(t, memInfo)
		assert.Equal(t, uint64(256*1024*1024*1024), memInfo.TotalBytes)
	})

	mockey.PatchConvey("GetMachineMemoryInfo with memory error", t, func() {
		mockey.Mock(mem.VirtualMemoryWithContext).To(func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
			return nil, errors.New("failed to get memory info")
		}).Build()

		memInfo := GetMachineMemoryInfo()
		require.NotNil(t, memInfo)
		assert.Equal(t, uint64(0), memInfo.TotalBytes)
	})
}

// TestGetProvider_WithMockedProviderDetection tests GetProvider with mocked provider detection
func TestGetProvider_WithMockedProviderDetection(t *testing.T) {
	mockey.PatchConvey("GetProvider with successful AWS detection", t, func() {
		mockey.Mock(pkgprovidersall.Detect).To(func(ctx context.Context) (*providers.Info, error) {
			return &providers.Info{
				Provider:      "aws",
				PublicIP:      "54.123.45.67",
				PrivateIP:     "172.16.0.10",
				VMEnvironment: "AWS",
				InstanceID:    "i-1234567890abcdef0",
			}, nil
		}).Build()

		provider := GetProvider("1.2.3.4")
		require.NotNil(t, provider)
		assert.Equal(t, "aws", provider.Provider)
		assert.Equal(t, "54.123.45.67", provider.PublicIP)
		assert.Equal(t, "172.16.0.10", provider.PrivateIP)
		assert.Equal(t, "i-1234567890abcdef0", provider.InstanceID)
	})

	mockey.PatchConvey("GetProvider with detection failure, fallback to ASN", t, func() {
		mockey.Mock(pkgprovidersall.Detect).To(func(ctx context.Context) (*providers.Info, error) {
			return nil, errors.New("provider detection failed")
		}).Build()

		mockey.Mock(asn.GetASLookup).To(func(ip string) (*asn.ASLookupResponse, error) {
			return &asn.ASLookupResponse{
				Asn:     "15169",
				AsnName: "GOOGLE",
				Country: "us",
				IP:      ip,
			}, nil
		}).Build()

		mockey.Mock(asn.NormalizeASNName).To(func(asnName string) string {
			return "gcp"
		}).Build()

		provider := GetProvider("8.8.8.8")
		require.NotNil(t, provider)
		assert.Equal(t, "gcp", provider.Provider)
		assert.Equal(t, "8.8.8.8", provider.PublicIP)
	})

	mockey.PatchConvey("GetProvider with nil provider info returns unknown", t, func() {
		mockey.Mock(pkgprovidersall.Detect).To(func(ctx context.Context) (*providers.Info, error) {
			return nil, nil
		}).Build()

		mockey.Mock(asn.GetASLookup).To(func(ip string) (*asn.ASLookupResponse, error) {
			return nil, errors.New("ASN lookup failed")
		}).Build()

		provider := GetProvider("192.168.1.1")
		require.NotNil(t, provider)
		assert.Equal(t, "unknown", provider.Provider)
		assert.Equal(t, "192.168.1.1", provider.PublicIP)
	})

	mockey.PatchConvey("GetProvider with empty public IP", t, func() {
		mockey.Mock(pkgprovidersall.Detect).To(func(ctx context.Context) (*providers.Info, error) {
			return &providers.Info{
				Provider: "",
			}, nil
		}).Build()

		provider := GetProvider("")
		require.NotNil(t, provider)
		assert.Equal(t, "unknown", provider.Provider)
	})

	mockey.PatchConvey("GetProvider with unknown provider and successful ASN lookup", t, func() {
		mockey.Mock(pkgprovidersall.Detect).To(func(ctx context.Context) (*providers.Info, error) {
			return &providers.Info{
				Provider: "",
				PublicIP: "",
			}, nil
		}).Build()

		mockey.Mock(asn.GetASLookup).To(func(ip string) (*asn.ASLookupResponse, error) {
			return &asn.ASLookupResponse{
				Asn:     "16509",
				AsnName: "AMAZON-02",
				Country: "us",
				IP:      ip,
			}, nil
		}).Build()

		mockey.Mock(asn.NormalizeASNName).To(func(asnName string) string {
			return "aws"
		}).Build()

		provider := GetProvider("52.94.76.1")
		require.NotNil(t, provider)
		assert.Equal(t, "aws", provider.Provider)
	})

	mockey.PatchConvey("GetProvider with ASN returning empty normalized name", t, func() {
		mockey.Mock(pkgprovidersall.Detect).To(func(ctx context.Context) (*providers.Info, error) {
			return &providers.Info{
				Provider: "",
			}, nil
		}).Build()

		mockey.Mock(asn.GetASLookup).To(func(ip string) (*asn.ASLookupResponse, error) {
			return &asn.ASLookupResponse{
				Asn:     "12345",
				AsnName: "some-unknown-provider",
				Country: "xx",
				IP:      ip,
			}, nil
		}).Build()

		mockey.Mock(asn.NormalizeASNName).To(func(asnName string) string {
			return ""
		}).Build()

		provider := GetProvider("10.0.0.1")
		require.NotNil(t, provider)
		assert.Equal(t, "some-unknown-provider", provider.Provider)
	})
}

// TestGetMachineLocation_WithMockedLatency tests GetMachineLocation with mocked latency measurement
func TestGetMachineLocation_WithMockedLatency(t *testing.T) {
	mockey.PatchConvey("GetMachineLocation with successful measurement", t, func() {
		mockey.Mock(pkgnetutillatencyedge.Measure).To(func(ctx context.Context, opts ...pkgnetutillatencyedge.OpOption) (latency.Latencies, error) {
			return latency.Latencies{
				{
					Provider:            "tailscale-derp",
					RegionName:          "US East (Virginia)",
					RegionCode:          "us-east-1",
					Latency:             metav1.Duration{Duration: 10 * time.Millisecond},
					LatencyMilliseconds: 10,
				},
				{
					Provider:            "tailscale-derp",
					RegionName:          "US West (Oregon)",
					RegionCode:          "us-west-2",
					Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
					LatencyMilliseconds: 50,
				},
			}, nil
		}).Build()

		location := GetMachineLocation()
		require.NotNil(t, location)
		assert.Equal(t, "us-east-1", location.Region)
	})

	mockey.PatchConvey("GetMachineLocation with measurement error", t, func() {
		mockey.Mock(pkgnetutillatencyedge.Measure).To(func(ctx context.Context, opts ...pkgnetutillatencyedge.OpOption) (latency.Latencies, error) {
			return nil, errors.New("network timeout")
		}).Build()

		location := GetMachineLocation()
		assert.Nil(t, location)
	})

	mockey.PatchConvey("GetMachineLocation with empty latencies", t, func() {
		mockey.Mock(pkgnetutillatencyedge.Measure).To(func(ctx context.Context, opts ...pkgnetutillatencyedge.OpOption) (latency.Latencies, error) {
			return latency.Latencies{}, nil
		}).Build()

		location := GetMachineLocation()
		assert.Nil(t, location)
	})

	mockey.PatchConvey("GetMachineLocation with closest region being second", t, func() {
		mockey.Mock(pkgnetutillatencyedge.Measure).To(func(ctx context.Context, opts ...pkgnetutillatencyedge.OpOption) (latency.Latencies, error) {
			return latency.Latencies{
				{
					Provider:            "tailscale-derp",
					RegionName:          "EU West (Frankfurt)",
					RegionCode:          "eu-west-1",
					Latency:             metav1.Duration{Duration: 100 * time.Millisecond},
					LatencyMilliseconds: 100,
				},
				{
					Provider:            "tailscale-derp",
					RegionName:          "Asia Pacific (Tokyo)",
					RegionCode:          "ap-northeast-1",
					Latency:             metav1.Duration{Duration: 5 * time.Millisecond},
					LatencyMilliseconds: 5,
				},
			}, nil
		}).Build()

		location := GetMachineLocation()
		require.NotNil(t, location)
		assert.Equal(t, "ap-northeast-1", location.Region)
	})
}

// TestGetSystemResourceGPUCount_WithMockedNVML tests GetSystemResourceGPUCount with mocked NVML
func TestGetSystemResourceGPUCount_WithMockedNVML(t *testing.T) {
	mockey.PatchConvey("GetSystemResourceGPUCount with devices from NVML", t, func() {
		// Create mock devices
		mockDevices := make(map[string]device.Device)
		mockDevices["GPU-uuid-1"] = nil
		mockDevices["GPU-uuid-2"] = nil
		mockDevices["GPU-uuid-3"] = nil
		mockDevices["GPU-uuid-4"] = nil

		mockInstance := &mockNvmlInstanceForMockey{
			devices: mockDevices,
		}

		count, err := GetSystemResourceGPUCount(mockInstance)
		require.NoError(t, err)
		assert.Equal(t, "4", count)
	})

	mockey.PatchConvey("GetSystemResourceGPUCount with no devices falls back to PCI", t, func() {
		mockInstance := &mockNvmlInstanceForMockey{
			devices: map[string]device.Device{},
		}

		mockey.Mock(nvidiapci.ListPCIGPUs).To(func(ctx context.Context) ([]string, error) {
			return []string{
				"0000:00:1e.0 3D controller: NVIDIA Corporation H100 [NVIDIA H100 80GB HBM3]",
				"0000:00:1f.0 3D controller: NVIDIA Corporation H100 [NVIDIA H100 80GB HBM3]",
			}, nil
		}).Build()

		count, err := GetSystemResourceGPUCount(mockInstance)
		require.NoError(t, err)
		assert.Equal(t, "2", count)
	})

	mockey.PatchConvey("GetSystemResourceGPUCount with no devices and PCI error", t, func() {
		mockInstance := &mockNvmlInstanceForMockey{
			devices: map[string]device.Device{},
		}

		mockey.Mock(nvidiapci.ListPCIGPUs).To(func(ctx context.Context) ([]string, error) {
			return nil, errors.New("PCI detection failed")
		}).Build()

		count, err := GetSystemResourceGPUCount(mockInstance)
		require.NoError(t, err)
		assert.Equal(t, "0", count)
	})

	mockey.PatchConvey("GetSystemResourceGPUCount with 8 GPUs", t, func() {
		mockDevices := make(map[string]device.Device)
		for i := 0; i < 8; i++ {
			mockDevices["GPU-uuid-"+string(rune('0'+i))] = nil
		}

		mockInstance := &mockNvmlInstanceForMockey{
			devices: mockDevices,
		}

		count, err := GetSystemResourceGPUCount(mockInstance)
		require.NoError(t, err)
		assert.Equal(t, "8", count)
	})
}

// TestGetSystemResourceRootVolumeTotal_WithMockedDisk tests GetSystemResourceRootVolumeTotal
func TestGetSystemResourceRootVolumeTotal_WithMockedDisk(t *testing.T) {
	mockey.PatchConvey("GetSystemResourceRootVolumeTotal success", t, func() {
		mockey.Mock(disk.GetUsage).To(func(ctx context.Context, path string) (*disk.Usage, error) {
			return &disk.Usage{
				TotalBytes: 500 * 1024 * 1024 * 1024, // 500GB
				FreeBytes:  200 * 1024 * 1024 * 1024,
				UsedBytes:  300 * 1024 * 1024 * 1024,
			}, nil
		}).Build()

		volume, err := GetSystemResourceRootVolumeTotal()
		require.NoError(t, err)
		assert.NotEmpty(t, volume)
		// The value should be parseable and non-zero
		// DecimalSI format may use different suffixes (k, M, G, etc.)
	})

	mockey.PatchConvey("GetSystemResourceRootVolumeTotal error", t, func() {
		mockey.Mock(disk.GetUsage).To(func(ctx context.Context, path string) (*disk.Usage, error) {
			return nil, errors.New("disk not accessible")
		}).Build()

		volume, err := GetSystemResourceRootVolumeTotal()
		require.Error(t, err)
		assert.Empty(t, volume)
		assert.Contains(t, err.Error(), "failed to get disk usage")
	})
}

// TestGetMachineNICInfo_WithMockedNetutil tests GetMachineNICInfo with mocked network interfaces
func TestGetMachineNICInfo_WithMockedNetutil(t *testing.T) {
	mockey.PatchConvey("GetMachineNICInfo with multiple interfaces", t, func() {
		mockey.Mock(netutil.GetPrivateIPs).To(func(opts ...netutil.OpOption) (netutil.InterfaceAddrs, error) {
			return netutil.InterfaceAddrs{}, nil
		}).Build()

		nicInfo := GetMachineNICInfo()
		require.NotNil(t, nicInfo)
		assert.NotNil(t, nicInfo.PrivateIPInterfaces)
	})

	mockey.PatchConvey("GetMachineNICInfo with error", t, func() {
		mockey.Mock(netutil.GetPrivateIPs).To(func(opts ...netutil.OpOption) (netutil.InterfaceAddrs, error) {
			return nil, errors.New("failed to get network interfaces")
		}).Build()

		nicInfo := GetMachineNICInfo()
		require.NotNil(t, nicInfo)
		// Even with error, should return empty slice not nil
		assert.NotNil(t, nicInfo.PrivateIPInterfaces)
	})
}

func TestGetMachineNICInfo_LeptonAndENIIncluded_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetMachineNICInfo keeps host interfaces and skips virtual CNI prefixes", t, func() {
		mockey.Mock(net.Interfaces).To(func() ([]net.Interface, error) {
			return []net.Interface{
				{Name: "lepton0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}},
				{Name: "eni0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x66}},
				{Name: "veth123", Flags: net.FlagUp},
				{Name: "cni0", Flags: net.FlagUp},
				{Name: "flannel.1", Flags: net.FlagUp},
				{Name: "tunl0", Flags: net.FlagUp},
				{Name: "vxlan.calico", Flags: net.FlagUp},
			}, nil
		}).Build()

		mockey.Mock((*net.Interface).Addrs).To(func(ifi *net.Interface) ([]net.Addr, error) {
			switch ifi.Name {
			case "lepton0":
				return []net.Addr{&net.IPNet{IP: net.ParseIP("10.50.85.108"), Mask: net.CIDRMask(24, 32)}}, nil
			case "eni0":
				return []net.Addr{&net.IPNet{IP: net.ParseIP("10.60.1.2"), Mask: net.CIDRMask(24, 32)}}, nil
			default:
				return []net.Addr{&net.IPNet{IP: net.ParseIP("10.244.0.1"), Mask: net.CIDRMask(24, 32)}}, nil
			}
		}).Build()

		nicInfo := GetMachineNICInfo()
		require.NotNil(t, nicInfo)
		require.Len(t, nicInfo.PrivateIPInterfaces, 2)

		got := map[string]string{}
		for _, iface := range nicInfo.PrivateIPInterfaces {
			got[iface.Interface] = iface.IP
		}

		require.Equal(t, "10.50.85.108", got["lepton0"])
		require.Equal(t, "10.60.1.2", got["eni0"])
		_, hasVeth := got["veth123"]
		require.False(t, hasVeth)
	})
}

// TestGetMachineGPUInfo_WithNoDevices tests GetMachineGPUInfo with no devices
func TestGetMachineGPUInfo_WithNoDevices(t *testing.T) {
	mockey.PatchConvey("GetMachineGPUInfo with no devices", t, func() {
		mockInstance := &mockNvmlInstanceForMockey{
			productName:  "",
			architecture: "",
			brand:        "",
			devices:      map[string]device.Device{},
		}

		info, err := GetMachineGPUInfo(mockInstance)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Empty(t, info.GPUs)
		assert.Empty(t, info.Memory)
	})
}

// TestGetProvider_NebiusSpecialCase tests the Nebius provider special case
func TestGetProvider_NebiusSpecialCase(t *testing.T) {
	mockey.PatchConvey("GetProvider with Nebius from ASN", t, func() {
		mockey.Mock(pkgprovidersall.Detect).To(func(ctx context.Context) (*providers.Info, error) {
			return &providers.Info{
				Provider: "",
			}, nil
		}).Build()

		mockey.Mock(asn.GetASLookup).To(func(ip string) (*asn.ASLookupResponse, error) {
			return &asn.ASLookupResponse{
				Asn:     "12345",
				AsnName: "nebiuscloud",
				Country: "ru",
				IP:      ip,
			}, nil
		}).Build()

		mockey.Mock(asn.NormalizeASNName).To(func(asnName string) string {
			return "nebius"
		}).Build()

		// We need to also mock the Nebius instance ID lookup
		// Since we can't easily mock the nebius package, we verify the provider is correctly set
		provider := GetProvider("1.2.3.4")
		require.NotNil(t, provider)
		assert.Equal(t, "nebius", provider.Provider)
	})
}

// TestGetMachineDiskInfo_WithMockedDisk tests GetMachineDiskInfo on Linux
func TestGetMachineDiskInfo_WithMockedDisk(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Test only runs on Linux or macOS")
	}

	mockey.PatchConvey("GetMachineDiskInfo with mocked lsblk", t, func() {
		mockey.Mock(disk.GetBlockDevicesWithLsblk).To(func(ctx context.Context, opts ...disk.OpOption) (disk.BlockDevices, error) {
			return disk.BlockDevices{
				{
					Name:       "nvme0n1",
					Type:       "disk",
					Size:       disk.CustomUint64{Uint64: 1000000000000},
					MountPoint: "/",
					FSType:     "ext4",
				},
			}, nil
		}).Build()

		ctx := context.Background()
		info, err := GetMachineDiskInfo(ctx)

		// The actual behavior depends on OS, but we verify no panic
		if err != nil {
			t.Logf("GetMachineDiskInfo returned error (expected on some systems): %v", err)
		} else {
			require.NotNil(t, info)
		}
	})
}
