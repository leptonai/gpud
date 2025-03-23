// Package lib implements the NVIDIA Management Library (NVML) interface.
// See https://docs.nvidia.com/deploy/nvml-api/nvml-api-reference.html#nvml-api-reference for more details.
package lib

import (
	"fmt"

	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Library interface {
	NVML() nvml.Interface
	GetDevices() ([]nvml.Device, error)
	Info() nvinfo.Interface
	Shutdown() nvml.Return
}

var _ Library = &nvmlInterface{}
var _ nvml.Interface = &nvmlInterface{}

type nvmlInterface struct {
	nvml.Interface

	info nvinfo.Interface

	initReturn        *nvml.Return
	propertyExtractor nvinfo.PropertyExtractor

	getRemappedRowsForAllDevs              func() (int, int, bool, bool, nvml.Return)
	getCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)
}

func (n *nvmlInterface) NVML() nvml.Interface {
	return n
}

func (n *nvmlInterface) GetDevices() ([]nvml.Device, error) {
	count, ret := n.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device count: %v", ret)
	}

	devices := make([]nvml.Device, count)
	for i := range count {
		dev, ret := n.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get device handle by index: %v", ret)
		}
		devices[i] = &nvmlDevice{
			Device:                                 dev,
			getRemappedRowsForAllDevs:              n.getRemappedRowsForAllDevs,
			getCurrentClocksEventReasonsForAllDevs: n.getCurrentClocksEventReasonsForAllDevs,
		}
	}

	return devices, nil
}

func (n *nvmlInterface) Info() nvinfo.Interface {
	return n.info
}

func (n *nvmlInterface) Shutdown() nvml.Return {
	return n.Interface.Shutdown()
}

// New creates a new NVML instance and returns nil if NVML is not supported.
func New(opts ...OpOption) Library {
	options := &Op{}
	options.applyOpts(opts)

	nvInterface := &nvmlInterface{
		Interface: options.nvmlLib,

		initReturn:        options.initReturn,
		propertyExtractor: options.propertyExtractor,

		getRemappedRowsForAllDevs:              options.devGetRemappedRowsForAllDevs,
		getCurrentClocksEventReasonsForAllDevs: options.devGetCurrentClocksEventReasonsForAllDevs,
	}

	infoOpts := []nvinfo.Option{
		nvinfo.WithNvmlLib(nvInterface),
	}
	if nvInterface.propertyExtractor != nil {
		infoOpts = append(infoOpts, nvinfo.WithPropertyExtractor(nvInterface.propertyExtractor))
	}
	nvInterface.info = nvinfo.New(infoOpts...)

	return nvInterface
}

func (n *nvmlInterface) Init() nvml.Return {
	if n.initReturn != nil {
		return *n.initReturn
	}
	return n.Interface.Init()
}

var _ nvml.Device = &nvmlDevice{}

type nvmlDevice struct {
	nvml.Device
	getRemappedRowsForAllDevs              func() (int, int, bool, bool, nvml.Return)
	getCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)
}

func (d *nvmlDevice) GetRemappedRows() (int, int, bool, bool, nvml.Return) {
	// no injected remapped rows
	// thus just passthrough to call the underlying nvml.Device.GetRemappedRows()
	if d.getRemappedRowsForAllDevs == nil {
		return d.Device.GetRemappedRows()
	}
	return d.getRemappedRowsForAllDevs()
}

func (d *nvmlDevice) GetCurrentClocksEventReasons() (uint64, nvml.Return) {
	// no injected current clocks event reasons
	// thus just passthrough to call the underlying nvml.Device.GetCurrentClocksEventReasons()
	if d.getCurrentClocksEventReasonsForAllDevs == nil {
		return d.Device.GetCurrentClocksEventReasons()
	}
	return d.getCurrentClocksEventReasonsForAllDevs()
}
