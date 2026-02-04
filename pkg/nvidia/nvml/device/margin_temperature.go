package device

import (
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
)

// MinDriverVersionForMarginTemperatureAPI is the minimum NVIDIA driver major version
// required for nvmlDeviceGetMarginTemperature.
// NVML changelog (display drivers 570+): https://docs.nvidia.com/deploy/nvml-api/change-log.html
// (see "Changes between v565 and v570", which lists nvmlDeviceGetMarginTemperature).
// API reference: https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g42db93dc04fc99d253eadc2037a5232d
// Calling this function on older drivers can cause a symbol lookup error.
const MinDriverVersionForMarginTemperatureAPI = 570

var logMarginTempSkipOnce sync.Once

// GetMarginTemperature guards the margin temperature API on older drivers to prevent crashes.
func (d *nvDevice) GetMarginTemperature() (nvml.MarginTemperature, nvml.Return) {
	if d.driverMajor >= MinDriverVersionForMarginTemperatureAPI {
		return d.Device.GetMarginTemperature()
	}

	logMarginTempSkipOnce.Do(func() {
		log.Logger.Warnw("skipping margin temperature API (nvmlDeviceGetMarginTemperature) due to old driver version; requires driver >= 570",
			"driverMajor", d.driverMajor,
			"minRequired", MinDriverVersionForMarginTemperatureAPI,
		)
	})
	return nvml.MarginTemperature{}, nvml.ERROR_NOT_SUPPORTED
}
