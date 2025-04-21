package nvml

import (
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

var _ Instance = &instance{}

// Instance is the interface for the NVML library connector.
type Instance interface {
	// NVMLExists returns true if NVML is installed.
	NVMLExists() bool

	// Library returns the NVML library.
	Library() nvmllib.Library

	// Devices returns the current devices in the system.
	// The key is the UUID of the GPU device.
	Devices() map[string]device.Device

	// ProductName returns the product name of the GPU.
	ProductName() string

	// DriverVersion returns the driver version of the GPU.
	DriverVersion() string

	// DriverMajor returns the major version of the driver.
	DriverMajor() int

	// CUDAVersion returns the CUDA version of the GPU.
	CUDAVersion() string

	// GetMemoryErrorManagementCapabilities returns the memory error management capabilities of the GPU.
	GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities

	// Shutdown shuts down the NVML library.
	Shutdown() error
}

// New creates a new instance of the NVML library.
// If NVML is not installed, it returns no-op nvml instance.
func New() (Instance, error) {
	nvmlLib, err := nvmllib.New()
	if err != nil {
		if errors.Is(err, nvmllib.ErrNVMLNotFound) {
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
	dm := make(map[string]device.Device)
	if len(devices) > 0 {
		name, ret := devices[0].GetName()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
		}
		productName = name

		for _, dev := range devices {
			uuid, ret := dev.GetUUID()
			if ret != nvml.SUCCESS {
				return nil, fmt.Errorf("failed to get device uuid: %v", nvml.ErrorString(ret))
			}
			dm[uuid] = dev
		}
	}
	memMgmtCaps := SupportedMemoryMgmtCapsByGPUProduct(productName)

	return &instance{
		nvmlLib:       nvmlLib,
		nvmlExists:    nvmlExists,
		nvmlExistsMsg: nvmlExistsMsg,
		driverVersion: driverVersion,
		driverMajor:   driverMajor,
		cudaVersion:   cudaVersion,
		devices:       dm,
		productName:   productName,
		memMgmtCaps:   memMgmtCaps,
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

	productName string
	memMgmtCaps MemoryErrorManagementCapabilities
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
	return inst.productName
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
func (inst *noOpInstance) DriverVersion() string             { return "" }
func (inst *noOpInstance) DriverMajor() int                  { return 0 }
func (inst *noOpInstance) CUDAVersion() string               { return "" }
func (inst *noOpInstance) GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities {
	return MemoryErrorManagementCapabilities{}
}
func (inst *noOpInstance) Shutdown() error { return nil }
