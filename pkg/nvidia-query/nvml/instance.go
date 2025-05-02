package nvml

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

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
	// GB200 may return "NVIDIA-Graphics-Device" for the product name
	// but "blackwell" for architecture.
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

	// GetMemoryErrorManagementCapabilities returns the memory error management capabilities of the GPU.
	GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities

	// Shutdown shuts down the NVML library.
	Shutdown() error
}

// New creates a new instance of the NVML library.
// If NVML is not installed, it returns no-op nvml instance.
func New() (Instance, error) {
	return newInstance(context.TODO(), nil)
}

// NewWithExitOnSuccessfulLoad creates a new instance of the NVML library.
// If NVML is not installed, it returns no-op nvml instance.
// It also calls the exit function when NVML is successfully loaded.
// The exit function is only called when the NVML library is not found.
// Other errors are returned as is.
func NewWithExitOnSuccessfulLoad(ctx context.Context) (Instance, error) {
	return newInstance(ctx, refreshNVMLAndExit)
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

// newInstance creates a new instance of the NVML library.
// If NVML is not installed, it returns no-op nvml instance.
// The "refreshNVML" function is only called when the NVML library is not found.
func newInstance(refreshCtx context.Context, refreshNVML func(context.Context)) (Instance, error) {
	nvmlLib, err := nvmllib.New()
	if err != nil {
		if errors.Is(err, nvmllib.ErrNVMLNotFound) {
			if refreshNVML != nil {
				go refreshNVML(refreshCtx)
			}
			return NewNoOp(), nil
		}
		return nil, err
	}

	log.Logger.Infow("checking if nvml exists from info library")
	nvmlExists, nvmlExistsMsg := nvmlLib.Info().HasNvml()
	if !nvmlExists {
		return nil, fmt.Errorf("nvml not found: %s", nvmlExistsMsg)
	}

	log.Logger.Infow("getting driver version from nvml library")
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
	log.Logger.Infow("successfully initialized NVML", "driverVersion", driverVersion, "cudaVersion", cudaVersion)

	// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
	// "error getting device handle for index '6': Unknown Error"
	devices, err := nvmlLib.Device().GetDevices()
	if err != nil {
		return nil, err
	}
	log.Logger.Infow("got devices from device library", "numDevices", len(devices))

	productName := ""
	archFamily := ""
	brand := ""
	dm := make(map[string]device.Device)
	if len(devices) > 0 {
		name, ret := devices[0].GetName()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
		}
		productName = name

		var err error

		archFamily, err = GetArchFamily(devices[0])
		if err != nil {
			return nil, err
		}

		brand, err = GetBrand(devices[0])
		if err != nil {
			return nil, err
		}

		for _, dev := range devices {
			uuid, ret := dev.GetUUID()
			if ret != nvml.SUCCESS {
				return nil, fmt.Errorf("failed to get device uuid: %v", nvml.ErrorString(ret))
			}
			dm[uuid] = dev
		}
	}

	fmSupported := SupportedFMByGPUProduct(productName)
	memMgmtCaps := SupportedMemoryMgmtCapsByGPUProduct(productName)

	return &instance{
		nvmlLib:              nvmlLib,
		nvmlExists:           nvmlExists,
		nvmlExistsMsg:        nvmlExistsMsg,
		driverVersion:        driverVersion,
		driverMajor:          driverMajor,
		cudaVersion:          cudaVersion,
		devices:              dm,
		sanitizedProductName: SanitizeProductName(productName),
		architecture:         archFamily,
		brand:                brand,
		fabricMgrSupported:   fmSupported,
		memMgmtCaps:          memMgmtCaps,
	}, nil
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

	fabricMgrSupported bool
	memMgmtCaps        MemoryErrorManagementCapabilities
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
	return inst.fabricMgrSupported
}

func (inst *instance) GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities {
	return inst.memMgmtCaps
}

func (inst *instance) Shutdown() error {
	ret := inst.nvmlLib.Shutdown()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to shutdown nvml library: %s", ret)
	}
	return nil
}

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
func (inst *noOpInstance) GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities {
	return MemoryErrorManagementCapabilities{}
}
func (inst *noOpInstance) Shutdown() error { return nil }
