package lib

import (
	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Op struct {
	nvmlLib nvml.Interface

	initReturn        *nvml.Return
	propertyExtractor nvinfo.PropertyExtractor
	devicesToReturn   []device.Device

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
	devGetRemappedRows func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return)
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
	if op.nvmlLib == nil {
		op.nvmlLib = nvml.New()
	}
}

// Specifies the NVML library instance.
// Otherwise, defaults to the NVML library instance returned by nvml.New().
func WithNVML(nvmlLib nvml.Interface) OpOption {
	return func(op *Op) {
		op.nvmlLib = nvmlLib
	}
}

// Specifies the return value of the NVML library's Init() function.
// Otherwise, defaults to the return value of the NVML library's Init() function.
func WithInitReturn(initReturn nvml.Return) OpOption {
	return func(op *Op) {
		op.initReturn = &initReturn
	}
}

// Specifies the property extractor for the NVML library.
func WithPropertyExtractor(propertyExtractor nvinfo.PropertyExtractor) OpOption {
	return func(op *Op) {
		op.propertyExtractor = propertyExtractor
	}
}

func WithDevice(dev device.Device) OpOption {
	return func(op *Op) {
		op.devicesToReturn = append(op.devicesToReturn, dev)
	}
}

// Specifies the function to get the remapped rows of the device.
// Otherwise, defaults to the function returned by device.GetRemappedRows().
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
func WithDeviceGetRemappedRows(f func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return)) OpOption {
	return func(op *Op) {
		op.devGetRemappedRows = f
	}
}
