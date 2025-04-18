package nvml

import (
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

var _ InstanceV2 = &instanceV2{}

type InstanceV2 interface {
	NVMLExists() bool
	Library() nvmllib.Library
	Devices() map[string]device.Device
	ProductName() string
	GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities
	Shutdown() error
}

// NewInstanceV2 creates a new instance of the NVML library.
// If NVML is not installed, it returns no-op nvml instance.
func NewInstanceV2() (InstanceV2, error) {
	nvmlLib, err := nvmllib.New()
	if err != nil {
		if errors.Is(err, nvmllib.ErrNVMLNotFound) {
			return NewNoOpInstanceV2(), nil
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

	return &instanceV2{
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

var _ InstanceV2 = &instanceV2{}

type instanceV2 struct {
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

func (inst *instanceV2) NVMLExists() bool {
	return inst.nvmlExists
}

func (inst *instanceV2) Library() nvmllib.Library {
	return inst.nvmlLib
}

func (inst *instanceV2) Devices() map[string]device.Device {
	return inst.devices
}

func (inst *instanceV2) ProductName() string {
	return inst.productName
}

func (inst *instanceV2) GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities {
	return inst.memMgmtCaps
}

func (inst *instanceV2) Shutdown() error {
	ret := inst.nvmlLib.Shutdown()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to shutdown nvml library: %s", ret)
	}
	return nil
}

var _ InstanceV2 = &noOpInstanceV2{}

func NewNoOpInstanceV2() InstanceV2 {
	return &noOpInstanceV2{}
}

type noOpInstanceV2 struct{}

func (inst *noOpInstanceV2) NVMLExists() bool                  { return false }
func (inst *noOpInstanceV2) Library() nvmllib.Library          { return nil }
func (inst *noOpInstanceV2) Devices() map[string]device.Device { return nil }
func (inst *noOpInstanceV2) ProductName() string               { return "" }
func (inst *noOpInstanceV2) GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities {
	return MemoryErrorManagementCapabilities{}
}
func (inst *noOpInstanceV2) Shutdown() error { return nil }
