// Package lib implements the NVIDIA Management Library (NVML) interface.
// See https://docs.nvidia.com/deploy/nvml-api/nvml-api-reference.html#nvml-api-reference for more details.
package lib

import (
	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Library interface {
	NVML() nvml.Interface
	Device() device.Interface
	Info() nvinfo.Interface
	Shutdown() nvml.Return
}

var _ Library = &nvmlInterface{}
var _ nvml.Interface = &nvmlInterface{}

type nvmlInterface struct {
	nvml.Interface

	dev  *devInterface
	info nvinfo.Interface

	initReturn        *nvml.Return
	propertyExtractor nvinfo.PropertyExtractor
}

func (n *nvmlInterface) NVML() nvml.Interface {
	return n
}

func (n *nvmlInterface) Device() device.Interface {
	return n.dev
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
	}

	devLib := device.New(nvInterface.Interface)
	nvInterface.dev = &devInterface{
		Interface:                              devLib,
		devices:                                options.devicesToReturn,
		getRemappedRowsForAllDevs:              options.devGetRemappedRowsForAllDevs,
		getCurrentClocksEventReasonsForAllDevs: options.devGetCurrentClocksEventReasonsForAllDevs,
	}

	infoOpts := []nvinfo.Option{
		nvinfo.WithNvmlLib(nvInterface),
		nvinfo.WithDeviceLib(nvInterface.dev),
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

var _ device.Interface = &devInterface{}

type devInterface struct {
	device.Interface
	devices                                []device.Device
	getRemappedRowsForAllDevs              func() (int, int, bool, bool, nvml.Return)
	getCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)
}

func (d *devInterface) GetDevices() ([]device.Device, error) {
	devs := d.devices

	var err error
	if len(devs) == 0 {
		devs, err = d.Interface.GetDevices()
	}

	if err != nil {
		return nil, err
	}

	updated := make([]device.Device, len(devs))
	for i, dev := range devs {
		updated[i] = &devDevInterface{
			Device:                                 dev,
			getRemappedRowsForAllDevs:              d.getRemappedRowsForAllDevs,
			getCurrentClocksEventReasonsForAllDevs: d.getCurrentClocksEventReasonsForAllDevs,
		}
	}

	return updated, nil
}

var _ device.Device = &devDevInterface{}

type devDevInterface struct {
	device.Device
	getRemappedRowsForAllDevs              func() (int, int, bool, bool, nvml.Return)
	getCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)
}

func (d *devDevInterface) GetRemappedRows() (int, int, bool, bool, nvml.Return) {
	// no injected remapped rows
	// thus just passthrough to call the underlying device.Device.GetRemappedRows()
	if d.getRemappedRowsForAllDevs == nil {
		return d.Device.GetRemappedRows()
	}
	return d.getRemappedRowsForAllDevs()
}

func (d *devDevInterface) GetCurrentClocksEventReasons() (uint64, nvml.Return) {
	// no injected current clocks event reasons
	// thus just passthrough to call the underlying device.Device.GetCurrentClocksEventReasons()
	if d.getCurrentClocksEventReasonsForAllDevs == nil {
		return d.Device.GetCurrentClocksEventReasons()
	}
	return d.getCurrentClocksEventReasonsForAllDevs()
}
