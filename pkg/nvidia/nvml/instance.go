package nvml

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// FailureInjectorConfig holds configuration for test failure injection
type FailureInjectorConfig struct {
	GPUUUIDsWithGPULost                           []string
	GPUUUIDsWithGPURequiresReset                  []string
	GPUUUIDsWithFabricStateHealthSummaryUnhealthy []string

	// GPUProductNameOverride overrides the detected GPU product name.
	// This is useful for testing fabric state failure injection on systems where
	// the actual GPU (e.g., H100-PCIe) doesn't support fabric state monitoring.
	// Set this to a product name like "H100-SXM" to simulate a fabric-capable GPU.
	// When set, this affects FabricStateSupported(), FabricManagerSupported(),
	// and memory management capabilities detection.
	GPUProductNameOverride string

	// NVMLDeviceGetDevicesError when true simulates Device().GetDevices() failure.
	// This is useful for testing the "Unable to determine the device handle for GPU: Unknown Error"
	// scenario that occurs when NVML library loads but device enumeration fails (e.g., Xid 79).
	// When enabled, gpud continues running but all nvidia components report unhealthy.
	// ref. https://github.com/leptonai/gpud/pull/1180
	NVMLDeviceGetDevicesError bool
}

var _ Instance = &instance{}

// Instance is the interface for the NVML library connector.
type Instance interface {
	// NVMLExists returns true if the NVML library is installed.
	NVMLExists() bool

	// Library returns the NVML library.
	Library() nvmllib.Library

	// Devices returns the current devices in the system.
	// The key is the UUID of the GPU device.
	Devices() map[string]device.Device

	// ProductName returns the product name of the GPU.
	// Note that some machines have nvml library but the driver is not installed,
	// returning empty value for the GPU product name.
	ProductName() string

	// Architecture returns the architecture of the GPU.
	// Early drivers may report "NVIDIA-Graphics-Device" as the product name;
	// architecture identification does not depend on that name.
	Architecture() string

	// Brand returns the brand of the GPU.
	Brand() string

	// DriverVersion returns the driver version of the GPU.
	DriverVersion() string

	// DriverMajor returns the major version of the driver.
	DriverMajor() int

	// CUDAVersion returns the CUDA version of the GPU.
	CUDAVersion() string

	// FabricManagerSupported returns true if the fabric manager is supported.
	FabricManagerSupported() bool

	// FabricStateSupported returns true if NVML fabric state telemetry is
	// available for the product (e.g. GB200 via nvmlDeviceGetGpuFabricInfo*).
	FabricStateSupported() bool

	// GetMemoryErrorManagementCapabilities returns the memory error management capabilities of the GPU.
	GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities

	// Capabilities returns the resolved product, architecture and host-link
	// capabilities, including fabric topology hints that a product name alone
	// cannot establish (for example Rubin NVL72 versus Rubin NVL8).
	Capabilities() nvidiaproduct.Capabilities

	// Shutdown shuts down the NVML library.
	Shutdown() error

	// InitError returns any error that occurred during NVML initialization.
	// If initialization succeeded, this returns nil.
	// Components should check this and report unhealthy if non-nil.
	// This typically occurs when NVML library loads but device enumeration fails,
	// for example: "error getting device handle for index '4': Unknown Error"
	// which corresponds to nvidia-smi showing:
	// "Unable to determine the device handle for GPU0000:XX:00.0: Unknown Error"
	InitError() error
}

// New creates a new instance of the NVML library.
// If NVML is not installed, it returns no-op nvml instance.
func New() (Instance, error) {
	return newInstance(context.TODO(), nil, nil)
}

// NewWithExitOnSuccessfulLoad creates a new instance of the NVML library.
// If NVML is not installed, it returns no-op nvml instance.
// It also calls the exit function when NVML is successfully loaded.
// The exit function is only called when the NVML library is not found.
// Other errors are returned as is.
func NewWithExitOnSuccessfulLoad(ctx context.Context) (Instance, error) {
	return newInstance(ctx, refreshNVMLAndExit, nil)
}

// NewWithFailureInjector creates a new instance with failure injection configuration.
func NewWithFailureInjector(failureInjector *FailureInjectorConfig) (Instance, error) {
	return newInstance(context.TODO(), nil, failureInjector)
}

// refreshNVMLAndExit exits 0 if NVML is successfully loaded.
// Otherwise, keeps retrying until NVML is successfully loaded.
func refreshNVMLAndExit(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Logger.Infow("starting NVML load refresh loop")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		log.Logger.Debugw("retrying NVML load")
		_, err := nvmllib.New()
		if err == nil {
			log.Logger.Infow("NVML loaded successfully, now exiting")
			os.Exit(0)
		}
		log.Logger.Debugw("NVML load failed", "error", err)
	}
}

// ErrDeviceGetDevicesInjected is the error returned when NVMLDeviceGetDevicesError is enabled.
// This simulates the "Unable to determine the device handle for GPU: Unknown Error" scenario.
var ErrDeviceGetDevicesInjected = errors.New("error getting device handle for index '0': Unknown Error (injected for testing)")

// newInstance creates a new instance of the NVML library.
// If NVML is not installed, it returns no-op nvml instance.
// The "refreshNVML" function is only called when the NVML library is not found.
func newInstance(refreshCtx context.Context, refreshNVML func(context.Context), failureInjector *FailureInjectorConfig) (Instance, error) {
	// Build library options for failure injection
	var libOpts []nvmllib.OpOption
	if failureInjector != nil && failureInjector.NVMLDeviceGetDevicesError {
		log.Logger.Warnw("NVML Device().GetDevices() error injection enabled for testing")
		libOpts = append(libOpts, nvmllib.WithDeviceGetDevicesError(ErrDeviceGetDevicesInjected))
	}

	nvmlLib, err := nvmllib.New(libOpts...)
	if err != nil {
		if errors.Is(err, nvmllib.ErrNVMLNotFound) {
			if refreshNVML != nil {
				go refreshNVML(refreshCtx)
			}
			return NewNoOp(), nil
		}
		return nil, err
	}

	log.Logger.Debugw("checking if nvml exists from info library")
	nvmlExists, nvmlExistsMsg := nvmlLib.Info().HasNvml()
	if !nvmlExists {
		return nil, fmt.Errorf("nvml not found: %s", nvmlExistsMsg)
	}

	log.Logger.Debugw("getting driver version from nvml library")
	driverVersion, err := GetSystemDriverVersion(nvmlLib.NVML())
	if err != nil {
		return nil, err
	}
	driverMajor, _, _, err := ParseDriverVersion(driverVersion)
	if err != nil {
		return nil, err
	}

	cudaVersion, err := getCUDAVersion(nvmlLib.NVML())
	if err != nil {
		return nil, err
	}
	log.Logger.Debugw("successfully initialized NVML", "driverVersion", driverVersion, "cudaVersion", cudaVersion)

	// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
	// "error getting device handle for index '6': Unknown Error"
	// When this happens, nvidia-smi also shows:
	// "Unable to determine the device handle for GPU0000:XX:00.0: Unknown Error"
	// Instead of failing gpud entirely, we return an errored instance so gpud continues
	// running but all nvidia components report unhealthy with this error.
	devices, err := nvmlLib.Device().GetDevices()
	if err != nil {
		log.Logger.Warnw("NVML device enumeration failed, returning errored instance",
			"error", err,
			"hint", "nvidia-smi may show 'Unable to determine the device handle for GPU: Unknown Error'",
		)
		return NewErrored(err), nil
	}
	log.Logger.Debugw("got devices from device library", "numDevices", len(devices))

	productName := ""
	archFamily := ""
	brand := ""

	devs := make(map[string]device.Device)
	if len(devices) > 0 {
		name, ret := devices[0].GetName()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
		}
		productName = name
		log.Logger.Debugw("detected product name", "productName", productName)

		// Apply product name override for testing if configured
		// This allows simulating different GPU types (e.g., H100-SXM on H100-PCIe hardware)
		// to test fabric state failure injection on systems that don't natively support it.
		if failureInjector != nil && failureInjector.GPUProductNameOverride != "" {
			log.Logger.Warnw("overriding GPU product name for testing",
				"original", productName,
				"override", failureInjector.GPUProductNameOverride)
			productName = failureInjector.GPUProductNameOverride
		}

		for _, dev := range devices {
			uuid, ret := dev.GetUUID()
			if ret != nvml.SUCCESS {
				return nil, fmt.Errorf("failed to get device uuid: %v", nvml.ErrorString(ret))
			}
			busID, err := dev.GetPCIBusID()
			if err != nil {
				return nil, err
			}

			// Build device options based on driver version and failure injector
			var opts []device.OpOption

			// Always pass driver major version so devices can gate V3 fabric API calls.
			// nvmlDeviceGetGpuFabricInfoV requires driver >= 550; calling it on older
			// drivers (e.g., 535.x) causes a symbol lookup crash.
			opts = append(opts, device.WithDriverMajor(driverMajor))

			if failureInjector != nil {
				// Check if this UUID should inject GPU Lost error
				for _, injectedUUID := range failureInjector.GPUUUIDsWithGPULost {
					if uuid == injectedUUID {
						opts = append(opts, device.WithGPULost())
						break
					}
				}
				// Check if this UUID should inject GPU Requires Reset error
				for _, injectedUUID := range failureInjector.GPUUUIDsWithGPURequiresReset {
					if uuid == injectedUUID {
						opts = append(opts, device.WithGPURequiresReset())
						break
					}
				}
				// Check if this UUID should inject Fabric Health Unhealthy
				for _, injectedUUID := range failureInjector.GPUUUIDsWithFabricStateHealthSummaryUnhealthy {
					if uuid == injectedUUID {
						opts = append(opts, device.WithFabricHealthUnhealthy())
						break
					}
				}
			}

			devs[uuid] = device.New(dev, busID, opts...)
		}

		var err error
		for _, dev := range devs {
			archFamily, err = GetArchFamily(dev)
			if err != nil {
				return nil, err
			}

			brand, err = GetBrand(dev)
			if err != nil {
				return nil, err
			}
		}
	}

	capabilityArch := archFamily
	hostLink := nvidiaproduct.HostLinkUnknown
	if failureInjector != nil && failureInjector.GPUProductNameOverride != "" {
		// An override simulates another product; real architecture or host-link
		// evidence would otherwise outrank the simulated name.
		capabilityArch = ""
	} else {
		for _, dev := range devices {
			if hostLink = detectHostLink(nvmlLib, dev); hostLink != nvidiaproduct.HostLinkUnknown {
				break
			}
		}
	}
	caps := nvidiaproduct.CapabilitiesForGPU(productName, capabilityArch, hostLink)

	return &instance{
		nvmlLib:              nvmlLib,
		nvmlExists:           nvmlExists,
		nvmlExistsMsg:        nvmlExistsMsg,
		driverVersion:        driverVersion,
		driverMajor:          driverMajor,
		cudaVersion:          cudaVersion,
		devices:              devs,
		sanitizedProductName: nvidiaproduct.SanitizeProductName(productName),
		architecture:         archFamily,
		brand:                brand,
		caps:                 caps,
	}, nil
}

// detectHostLink reads NVLink-C2C mode, the NVML signal that separates a
// superchip compute tray (Grace/Vera CPU over C2C) from an x86 PCIe host.
// GPUs in one system share the same host attachment, so one device suffices.
func detectHostLink(lib nvmllib.Library, dev nvlibdevice.Device) nvidiaproduct.HostLink {
	// go-nvml binds this symbol directly. Older drivers lack it, and calling a
	// missing symbol terminates the process during lazy linking.
	if lib.NVML().Extensions().LookupSymbol("nvmlDeviceGetC2cModeInfoV") != nil {
		return nvidiaproduct.HostLinkUnknown
	}
	// The go-nvml handler always calls into the C library, so mocks cannot
	// drive it; the decision itself lives in hostLinkFromC2CMode.
	return hostLinkFromC2CMode(dev.GetC2cModeInfoV().V1())
}

func hostLinkFromC2CMode(info nvml.C2cModeInfo_v1, ret nvml.Return) nvidiaproduct.HostLink {
	switch {
	case ret == nvml.SUCCESS && info.IsC2cEnabled != 0:
		return nvidiaproduct.HostLinkC2C
	case ret == nvml.SUCCESS,
		// NVML returns NOT_SUPPORTED when the device has no C2C feature,
		// which means the GPU is attached to its host over PCIe.
		ret == nvml.ERROR_NOT_SUPPORTED:
		return nvidiaproduct.HostLinkPCIe
	default:
		// Lost or failing GPUs prove nothing about topology.
		return nvidiaproduct.HostLinkUnknown
	}
}

var _ Instance = &instance{}

type instance struct {
	nvmlLib nvmllib.Library

	nvmlExists    bool
	nvmlExistsMsg string

	driverVersion string
	driverMajor   int
	cudaVersion   string

	devices map[string]device.Device

	sanitizedProductName string
	architecture         string
	brand                string

	caps nvidiaproduct.Capabilities
}

func (inst *instance) NVMLExists() bool {
	return inst.nvmlExists
}

func (inst *instance) Library() nvmllib.Library {
	return inst.nvmlLib
}

func (inst *instance) Devices() map[string]device.Device {
	return inst.devices
}

func (inst *instance) ProductName() string {
	return inst.sanitizedProductName
}

func (inst *instance) Architecture() string {
	return inst.architecture
}

func (inst *instance) Brand() string {
	return inst.brand
}

func (inst *instance) DriverVersion() string {
	return inst.driverVersion
}

func (inst *instance) DriverMajor() int {
	return inst.driverMajor
}

func (inst *instance) CUDAVersion() string {
	return inst.cudaVersion
}

func (inst *instance) FabricManagerSupported() bool {
	return inst.caps.FabricManager
}

func (inst *instance) FabricStateSupported() bool {
	return inst.caps.FabricState
}

func (inst *instance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return inst.caps.MemoryManagement
}

func (inst *instance) Capabilities() nvidiaproduct.Capabilities {
	return inst.caps
}

func (inst *instance) Shutdown() error {
	ret := inst.nvmlLib.Shutdown()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to shutdown nvml library: %s", ret)
	}
	return nil
}

func (inst *instance) InitError() error { return nil }

var _ Instance = &noOpInstance{}

func NewNoOp() Instance {
	return &noOpInstance{}
}

type noOpInstance struct{}

func (inst *noOpInstance) NVMLExists() bool                  { return false }
func (inst *noOpInstance) Library() nvmllib.Library          { return nil }
func (inst *noOpInstance) Devices() map[string]device.Device { return nil }
func (inst *noOpInstance) ProductName() string               { return "" }
func (inst *noOpInstance) Architecture() string              { return "" }
func (inst *noOpInstance) Brand() string                     { return "" }
func (inst *noOpInstance) DriverVersion() string             { return "" }
func (inst *noOpInstance) DriverMajor() int                  { return 0 }
func (inst *noOpInstance) CUDAVersion() string               { return "" }
func (inst *noOpInstance) FabricManagerSupported() bool      { return false }
func (inst *noOpInstance) FabricStateSupported() bool        { return false }
func (inst *noOpInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (inst *noOpInstance) Capabilities() nvidiaproduct.Capabilities {
	return nvidiaproduct.Capabilities{}
}
func (inst *noOpInstance) Shutdown() error  { return nil }
func (inst *noOpInstance) InitError() error { return nil }

var _ Instance = &erroredInstance{}

// NewErrored creates an Instance that represents a failed NVML initialization.
// gpud run continues even when NVML fails, but all nvidia accelerator components
// will report unhealthy with this error.
// This typically happens when:
// - nvidia-smi shows: "Unable to determine the device handle for GPU0000:XX:00.0: Unknown Error"
// - NVML returns: "error getting device handle for index 'N': Unknown Error"
func NewErrored(initErr error) Instance {
	return &erroredInstance{initErr: initErr}
}

// erroredInstance represents a failed NVML initialization state.
// NVMLExists() returns true because the library loaded, but InitError() returns the error.
// Components should check InitError() and report unhealthy if non-nil.
type erroredInstance struct {
	initErr error
}

func (inst *erroredInstance) NVMLExists() bool                  { return true }
func (inst *erroredInstance) Library() nvmllib.Library          { return nil }
func (inst *erroredInstance) Devices() map[string]device.Device { return nil }
func (inst *erroredInstance) ProductName() string               { return "" }
func (inst *erroredInstance) Architecture() string              { return "" }
func (inst *erroredInstance) Brand() string                     { return "" }
func (inst *erroredInstance) DriverVersion() string             { return "" }
func (inst *erroredInstance) DriverMajor() int                  { return 0 }
func (inst *erroredInstance) CUDAVersion() string               { return "" }
func (inst *erroredInstance) FabricManagerSupported() bool      { return false }
func (inst *erroredInstance) FabricStateSupported() bool        { return false }
func (inst *erroredInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (inst *erroredInstance) Capabilities() nvidiaproduct.Capabilities {
	return nvidiaproduct.Capabilities{}
}
func (inst *erroredInstance) Shutdown() error  { return nil }
func (inst *erroredInstance) InitError() error { return inst.initErr }
