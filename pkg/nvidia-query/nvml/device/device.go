// Package device provides a wrapper around the "github.com/NVIDIA/go-nvlib/pkg/nvlib/device".Device
// type that adds a PCIBusID and UUID method, with support for test failure injection.
package device

import (
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Device is a wrapper around the "github.com/NVIDIA/go-nvlib/pkg/nvlib/device".Device
// type that adds a PCIBusID and UUID method, plus GetFabricState for fabric health queries.
type Device interface {
	device.Device
	PCIBusID() string
	UUID() string
	GetFabricState() (FabricState, error)
}

var _ Device = &nvDevice{}

// MinDriverVersionForV3FabricAPI is the minimum NVIDIA driver major version
// required for nvmlDeviceGetGpuFabricInfoV (V3 fabric state API).
// This API was introduced in driver 550 (see NVML changelog:
// https://docs.nvidia.com/deploy/nvml-api/change-log.html).
// Calling this function on older drivers (e.g., 535.x) causes a symbol lookup
// error that crashes the process.
const MinDriverVersionForV3FabricAPI = 550

type nvDevice struct {
	device.Device
	busID       string
	uuid        string
	driverMajor int // Driver major version, used to gate V3 fabric API calls
}

func (d *nvDevice) PCIBusID() string {
	return d.busID
}

func (d *nvDevice) UUID() string {
	return d.uuid
}

func New(dev device.Device, busID string, opts ...OpOption) Device {
	op := &Op{}
	op.applyOpts(opts)

	// Fetch UUID from device
	uuid, ret := dev.GetUUID()
	if ret != nvml.SUCCESS {
		panic(fmt.Sprintf("failed to get device UUID: %v", nvml.ErrorString(ret)))
	}

	// Create the base device
	baseDevice := &nvDevice{Device: dev, busID: busID, uuid: uuid, driverMajor: op.DriverMajor}

	// If ANY test flags are set, wrap with testDevice
	if op.GPULost || op.GPURequiresReset || op.FabricHealthUnhealthy {
		return &testDevice{
			Device:                baseDevice,
			gpuLost:               op.GPULost,
			gpuRequiresReset:      op.GPURequiresReset,
			fabricHealthUnhealthy: op.FabricHealthUnhealthy,
		}
	}

	return baseDevice
}

// Op struct holds options for device creation
type Op struct {
	// DriverMajor is the major version of the NVIDIA driver.
	// Used to gate V3 fabric API calls which require driver >= 550.
	DriverMajor int
	// GPULost indicates that all device methods should return nvml.ERROR_GPU_IS_LOST
	GPULost bool
	// GPURequiresReset indicates that all device methods should return nvml.ERROR_RESET_REQUIRED
	GPURequiresReset bool
	// FabricHealthUnhealthy indicates that GetGpuFabricState should return SUCCESS but with unhealthy status
	FabricHealthUnhealthy bool
}

// OpOption is a function that configures the Op struct
type OpOption func(*Op)

// applyOpts applies the provided options to the Op struct
func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
}

// WithGPULost returns an OpOption that enables GPU Lost error injection
func WithGPULost() OpOption {
	return func(op *Op) {
		op.GPULost = true
	}
}

// WithGPURequiresReset returns an OpOption that enables GPU Requires Reset error injection
func WithGPURequiresReset() OpOption {
	return func(op *Op) {
		op.GPURequiresReset = true
	}
}

// WithFabricHealthUnhealthy returns an OpOption that enables Fabric Health Unhealthy injection
func WithFabricHealthUnhealthy() OpOption {
	return func(op *Op) {
		op.FabricHealthUnhealthy = true
	}
}

// WithDriverMajor returns an OpOption that sets the driver major version.
// This is used to gate V3 fabric API calls which require driver >= 550.
func WithDriverMajor(major int) OpOption {
	return func(op *Op) {
		op.DriverMajor = major
	}
}
