// Package lib implements the NVIDIA Management Library (NVML) interface.
// See https://docs.nvidia.com/deploy/nvml-api/nvml-api-reference.html#nvml-api-reference for more details.
package lib

import (
	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Library interface {
	NVML() nvml.Interface
	Device() nvlibdevice.Interface
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

func (n *nvmlInterface) Device() nvlibdevice.Interface {
	return n.dev
}

func (n *nvmlInterface) Info() nvinfo.Interface {
	return n.info
}

func (n *nvmlInterface) Shutdown() nvml.Return {
	return n.Interface.Shutdown()
}

// createLibrary creates a new NVML instance and returns nil if NVML is not supported.
func createLibrary(opts ...OpOption) Library {
	options := &Op{}
	options.applyOpts(opts)

	nvInterface := &nvmlInterface{
		Interface: options.nvmlLib,

		initReturn:        options.initReturn,
		propertyExtractor: options.propertyExtractor,
	}

	devLib := nvlibdevice.New(nvInterface.Interface)
	nvInterface.dev = &devInterface{
		Interface: devLib,
		lookupSymbol: func(name string) error {
			return nvInterface.Interface.Extensions().LookupSymbol(name)
		},
		devices:                                options.devicesToReturn,
		getRemappedRowsForAllDevs:              options.devGetRemappedRowsForAllDevs,
		getCurrentClocksEventReasonsForAllDevs: options.devGetCurrentClocksEventReasonsForAllDevs,
		getDevicesError:                        options.devGetDevicesError,
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

var _ nvlibdevice.Interface = &devInterface{}

type devInterface struct {
	nvlibdevice.Interface
	lookupSymbol                           func(string) error
	devices                                []nvlibdevice.Device
	getRemappedRowsForAllDevs              func() (int, int, bool, bool, nvml.Return)
	getCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)
	getDevicesError                        error
}

func (d *devInterface) GetDevices() ([]nvlibdevice.Device, error) {
	// Check for injected error first (for testing device enumeration failures)
	if d.getDevicesError != nil {
		return nil, d.getDevicesError
	}

	devs := d.devices

	var err error
	if len(devs) == 0 {
		devs, err = d.Interface.GetDevices()
	}

	if err != nil {
		return nil, err
	}

	updated := make([]nvlibdevice.Device, len(devs))
	for i, dev := range devs {
		updated[i] = &devDevInterface{
			Device:                                 dev,
			lookupSymbol:                           d.lookupSymbol,
			getRemappedRowsForAllDevs:              d.getRemappedRowsForAllDevs,
			getCurrentClocksEventReasonsForAllDevs: d.getCurrentClocksEventReasonsForAllDevs,
		}
	}

	return updated, nil
}

var _ nvlibdevice.Device = &devDevInterface{}

type devDevInterface struct {
	nvlibdevice.Device
	lookupSymbol                           func(string) error
	getRemappedRowsForAllDevs              func() (int, int, bool, bool, nvml.Return)
	getCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)
}

func (d *devDevInterface) GetArchitecture() (nvml.DeviceArchitecture, nvml.Return) {
	// go-nvml calls this unversioned C symbol directly. Check the loaded library
	// first so an older driver cannot terminate the process during lazy linking.
	if d.lookupSymbol != nil && d.lookupSymbol("nvmlDeviceGetArchitecture") != nil {
		return nvml.DEVICE_ARCH_UNKNOWN, nvml.ERROR_FUNCTION_NOT_FOUND
	}
	return d.Device.GetArchitecture()
}

// The memory-repair queries below are newer NVML entry points. go-nvml binds
// each C symbol directly, so on a driver whose libnvidia-ml.so predates them the
// first call fails lazy symbol resolution and terminates the process instead of
// returning an error. Check the loaded library first and report
// ERROR_FUNCTION_NOT_FOUND, which callers treat as "not supported".

// GetBankRemapperStatus_v1 guards nvmlDeviceGetBankRemapperStatus_v1
// (nvml.h: %RUBIN_OR_NEWER%).
func (d *devDevInterface) GetBankRemapperStatus_v1() (nvml.EccBankRemapperStatus_v1, nvml.Return) {
	if d.lookupSymbol != nil && d.lookupSymbol("nvmlDeviceGetBankRemapperStatus_v1") != nil {
		return nvml.EccBankRemapperStatus_v1{}, nvml.ERROR_FUNCTION_NOT_FOUND
	}
	return d.Device.GetBankRemapperStatus_v1()
}

// GetRepairStatus guards nvmlDeviceGetRepairStatus (nvml.h: Ampere or newer).
func (d *devDevInterface) GetRepairStatus() (nvml.RepairStatus, nvml.Return) {
	if d.lookupSymbol != nil && d.lookupSymbol("nvmlDeviceGetRepairStatus") != nil {
		return nvml.RepairStatus{}, nvml.ERROR_FUNCTION_NOT_FOUND
	}
	return d.Device.GetRepairStatus()
}

// GetUnrepairableMemoryFlag_v1 guards nvmlDeviceGetUnrepairableMemoryFlag_v1
// (nvml.h: Hopper or newer).
func (d *devDevInterface) GetUnrepairableMemoryFlag_v1() (nvml.UnrepairableMemoryStatus_v1, nvml.Return) {
	if d.lookupSymbol != nil && d.lookupSymbol("nvmlDeviceGetUnrepairableMemoryFlag_v1") != nil {
		return nvml.UnrepairableMemoryStatus_v1{}, nvml.ERROR_FUNCTION_NOT_FOUND
	}
	return d.Device.GetUnrepairableMemoryFlag_v1()
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
