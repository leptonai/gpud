package lib

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Op struct {
	getDeviceCount   func() (int, nvml.Return)
	getDeviceByIndex func(int) (nvml.Device, nvml.Return)

	initReturn *nvml.Return

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
	devGetRemappedRowsForAllDevs func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return)

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
	devGetCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
	if op.getDeviceCount == nil {
		op.getDeviceCount = nvml.DeviceGetCount
	}
	if op.getDeviceByIndex == nil {
		op.getDeviceByIndex = nvml.DeviceGetHandleByIndex
	}
}

// Specifies the function for all devices to get the device count.
// Otherwise, defaults to the function returned by nvml.DeviceGetCount().
func WithGetDeviceCount(f func() (int, nvml.Return)) OpOption {
	return func(op *Op) {
		op.getDeviceCount = f
	}
}

// Specifies the function for all devices to get the device by index.
// Otherwise, defaults to the function returned by nvml.DeviceGetHandleByIndex().
func WithGetDeviceByIndex(f func(int) (nvml.Device, nvml.Return)) OpOption {
	return func(op *Op) {
		op.getDeviceByIndex = f
	}
}

// Specifies the return value of the NVML library's Init() function.
// Otherwise, defaults to the return value of the NVML library's Init() function.
func WithInitReturn(initReturn nvml.Return) OpOption {
	return func(op *Op) {
		op.initReturn = &initReturn
	}
}

// Specifies the function for all devices to get the remapped rows of the device.
// Otherwise, defaults to the function returned by device.GetRemappedRows().
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
func WithDeviceGetRemappedRowsForAllDevs(f func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return)) OpOption {
	return func(op *Op) {
		op.devGetRemappedRowsForAllDevs = f
	}
}

// Specifies the function for all devices  to get the current clocks event reasons of the device.
// Otherwise, defaults to the function returned by device.GetCurrentClocksEventReasons().
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
func WithDeviceGetCurrentClocksEventReasonsForAllDevs(f func() (uint64, nvml.Return)) OpOption {
	return func(op *Op) {
		op.devGetCurrentClocksEventReasonsForAllDevs = f
	}
}
