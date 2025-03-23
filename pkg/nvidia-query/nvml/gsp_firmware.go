package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// GSPFirmwareMode is the GSP firmware mode of the device.
// ref. https://www.nvidia.com.tw/Download/driverResults.aspx/224886/tw
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g37f644e70bd4853a78ca2bbf70861f67
type GSPFirmwareMode struct {
	UUID      string `json:"uuid"`
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported"`
}

func GetGSPFirmwareMode(uuid string, dev nvml.Device) (GSPFirmwareMode, error) {
	mode := GSPFirmwareMode{
		UUID: uuid,
	}

	gspEnabled, supported, ret := dev.GetGspFirmwareMode()
	if IsNotSupportError(ret) {
		mode.Enabled = false
		mode.Supported = false
		return mode, nil
	}

	// not a "not supported" error, not a success return, thus return an error here
	if ret != nvml.SUCCESS {
		return mode, fmt.Errorf("failed to get gsp firmware mode: %v", nvml.ErrorString(ret))
	}
	mode.Enabled = gspEnabled
	mode.Supported = supported

	return mode, nil
}
