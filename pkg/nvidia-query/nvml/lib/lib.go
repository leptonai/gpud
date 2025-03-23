// Package lib implements the NVIDIA Management Library (NVML) interface.
// See https://docs.nvidia.com/deploy/nvml-api/nvml-api-reference.html#nvml-api-reference for more details.
package lib

import (
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

var theInterface Library
var initResult nvml.Return
var shutdownResult nvml.Return
var once sync.Once
var initOnce sync.Once
var shutdownOnce sync.Once

type Library interface {
	NVML() nvml.Interface
	GetDevices() ([]nvml.Device, error)
	HasNVML() bool
	Shutdown() nvml.Return
}

var _ Library = &nvmlInterface{}

type nvmlInterface struct {
	nvml.Interface
	getDeviceCount                         func() (int, nvml.Return)
	getDeviceByIndex                       func(int) (nvml.Device, nvml.Return)
	getRemappedRowsForAllDevs              func() (int, int, bool, bool, nvml.Return)
	getCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)
}

func (n *nvmlInterface) NVML() nvml.Interface {
	return n
}

func (n *nvmlInterface) GetDevices() ([]nvml.Device, error) {
	count, ret := n.getDeviceCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device count: %v", ret)
	}

	devices := make([]nvml.Device, count)
	for i := range count {
		dev, ret := n.getDeviceByIndex(i)
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

func (n *nvmlInterface) HasNVML() bool {
	return true
}

// New creates a new NVML instance and returns nil if NVML is not supported.
func New(opts ...OpOption) Library {
	options := &Op{}
	options.applyOpts(opts)

	nvInterface := &nvmlInterface{
		getDeviceCount:                         options.getDeviceCount,
		getDeviceByIndex:                       options.getDeviceByIndex,
		getRemappedRowsForAllDevs:              options.devGetRemappedRowsForAllDevs,
		getCurrentClocksEventReasonsForAllDevs: options.devGetCurrentClocksEventReasonsForAllDevs,
	}

	return nvInterface
}

func (n *nvmlInterface) Init() nvml.Return {
	initOnce.Do(func() {
		initResult = nvml.Init()
	})
	return initResult
}

func (n *nvmlInterface) Shutdown() nvml.Return {
	shutdownOnce.Do(func() {
		shutdownResult = nvml.Shutdown()
	})
	return shutdownResult
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
