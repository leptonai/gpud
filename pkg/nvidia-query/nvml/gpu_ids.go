package nvml

import (
	"fmt"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func GetSerial(uuid string, dev device.Device) (string, error) {
	serialID, ret := dev.GetSerial()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get serial id: %v", nvml.ErrorString(ret))
	}
	return serialID, nil
}

func GetMinorID(uuid string, dev device.Device) (int, error) {
	minorID, ret := dev.GetMinorNumber()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get minor id: %v", nvml.ErrorString(ret))
	}
	return minorID, nil
}

func GetBoardID(uuid string, dev device.Device) (uint32, error) {
	boardID, ret := dev.GetBoardId()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get board id: %v", nvml.ErrorString(ret))
	}
	return boardID, nil
}
