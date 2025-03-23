package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

var _ InstanceV2 = &instanceV2{}

type InstanceV2 interface {
	Library() nvml_lib.Library
	Devices() map[string]nvml.Device
	ProductName() string
	GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities
}

var ErrNVMLNotInstalled = fmt.Errorf("nvml not installed")

// NewInstanceV2 creates a new instance of the NVML library.
// If NVML is not installed, it returns `ErrNVMLNotInstalled`.
func NewInstanceV2() (InstanceV2, error) {
	nvmlLib := nvml_lib.NewDefault()
	installed, err := initAndCheckNVMLSupported(nvmlLib)
	if err != nil {
		return nil, err
	}
	if !installed {
		return nil, ErrNVMLNotInstalled
	}

	log.Logger.Infow("checking if nvml exists from info library")
	if !nvmlLib.HasNVML() {
		return nil, fmt.Errorf("nvml not found")
	}

	log.Logger.Infow("getting driver version from nvml library")
	driverVersion, err := getDriverVersion()
	if err != nil {
		return nil, err
	}
	driverMajor, _, _, err := ParseDriverVersion(driverVersion)
	if err != nil {
		return nil, err
	}

	cudaVersion, err := getCUDAVersion()
	if err != nil {
		return nil, err
	}
	log.Logger.Infow("successfully initialized NVML", "driverVersion", driverVersion, "cudaVersion", cudaVersion)

	// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
	// "error getting device handle for index '6': Unknown Error"
	devices, err := nvmlLib.GetDevices()
	if err != nil {
		return nil, err
	}
	log.Logger.Infow("got devices from device library", "numDevices", len(devices))

	productName := ""
	dm := make(map[string]nvml.Device)
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
		driverVersion: driverVersion,
		driverMajor:   driverMajor,
		cudaVersion:   cudaVersion,
		devices:       dm,
		productName:   productName,
		memMgmtCaps:   memMgmtCaps,
	}, nil
}

type instanceV2 struct {
	nvmlLib nvml_lib.Library

	driverVersion string
	driverMajor   int
	cudaVersion   string

	devices map[string]nvml.Device

	productName string
	memMgmtCaps MemoryErrorManagementCapabilities
}

func (inst *instanceV2) Library() nvml_lib.Library {
	return inst.nvmlLib
}

func (inst *instanceV2) Devices() map[string]nvml.Device {
	return inst.devices
}

func (inst *instanceV2) ProductName() string {
	return inst.productName
}

func (inst *instanceV2) GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities {
	return inst.memMgmtCaps
}
