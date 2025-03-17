package nvml

import (
	"fmt"
	"strings"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

var _ InstanceV2 = &instanceV2{}

type InstanceV2 interface {
	NVMLExists() bool
	Library() nvml_lib.Library
	Devices() map[string]device.Device
	ProductName() string
	GetMemoryErrorManagementCapabilities() MemoryErrorManagementCapabilities
	Shutdown() error
}

var ErrNVMLNotInstalled = fmt.Errorf("nvml not installed")

// NewInstanceV2 creates a new instance of the NVML library.
// If NVML is not installed, it returns `ErrNVMLNotInstalled`.
func NewInstanceV2() (InstanceV2, error) {
	nvmlLib := nvml_lib.NewDefault()
	installed, err := initAndCheckNVMLSupported(nvmlLib.NVML())
	if err != nil {
		return nil, err
	}
	if !installed {
		return nil, ErrNVMLNotInstalled
	}

	log.Logger.Infow("checking if nvml exists from info library")
	nvmlExists, nvmlExistsMsg := nvmlLib.Info().HasNvml()
	if !nvmlExists {
		return nil, fmt.Errorf("nvml not found: %s", nvmlExistsMsg)
	}

	log.Logger.Infow("getting driver version from nvml library")
	driverVersion, err := getDriverVersion(nvmlLib.NVML())
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

type instanceV2 struct {
	nvmlLib nvml_lib.Library

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

func (inst *instanceV2) Library() nvml_lib.Library {
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

var (
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#supported-gpus
	memMgmtCapAllSupported = MemoryErrorManagementCapabilities{
		ErrorContainment:     true,
		DynamicPageOfflining: true,
		RowRemapping:         true,
	}
	memMgmtCapOnlyRowRemappingSupported = MemoryErrorManagementCapabilities{
		RowRemapping: true,
	}
	gpuProductToMemMgmtCaps = map[string]MemoryErrorManagementCapabilities{
		"a100": memMgmtCapAllSupported,
		"b100": memMgmtCapAllSupported,
		"b200": memMgmtCapAllSupported,
		"h100": memMgmtCapAllSupported,
		"h200": memMgmtCapAllSupported,
		"a10":  memMgmtCapOnlyRowRemappingSupported,
	}
)

// SupportedMemoryMgmtCapsByGPUProduct returns the GPU memory error management capabilities
// based on the GPU product name.
// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#supported-gpus
func SupportedMemoryMgmtCapsByGPUProduct(gpuProductName string) MemoryErrorManagementCapabilities {
	p := strings.ToLower(gpuProductName)
	longestName, memCaps := "", MemoryErrorManagementCapabilities{}
	for k, v := range gpuProductToMemMgmtCaps {
		if !strings.Contains(p, k) {
			continue
		}
		if len(longestName) < len(k) {
			longestName = k
			memCaps = v
		}
	}
	return memCaps
}

// Contains information about the GPU's memory error management capabilities.
// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#supported-gpus
type MemoryErrorManagementCapabilities struct {
	// (If supported) GPU can limit the impact of uncorrectable ECC errors to GPU applications.
	// Existing/new workloads will run unaffected, both in terms of accuracy and performance.
	// Thus, does not require a GPU reset when memory errors occur.
	//
	// Note thtat there are some rarer cases, where uncorrectable errors are still uncontained
	// thus impacting all other workloads being procssed in the GPU.
	//
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#error-containments
	ErrorContainment bool `json:"error_containment"`

	// (If supported) GPU can dynamically mark the page containing uncorrectable errors
	// as unusable, and any existing or new workloads will not be allocating this page.
	//
	// Thus, does not require a GPU reset to recover from most uncorrectable ECC errors.
	//
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#dynamic-page-offlining
	DynamicPageOfflining bool `json:"dynamic_page_offlining"`

	// (If supported) GPU can replace degrading memory cells with spare ones
	// to avoid offlining regions of memory. And the row remapping is different
	// from dynamic page offlining which is fixed at a hardware level.
	//
	// The row remapping requires a GPU reset to take effect.
	//
	// Even for "NVIDIA GeForce RTX 4090", NVML API returns no error on the remapped rows API,
	// thus "NVML.Supported" is not a reliable way to check if row remapping is supported.
	// We track a separate boolean value based on the GPU product name.
	//
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#row-remapping
	RowRemapping bool `json:"row_remapping"`

	// Message contains the message to the user about the memory error management capabilities.
	Message string `json:"message,omitempty"`
}
