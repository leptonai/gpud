package machineinfo

import (
	"encoding/hex"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

func getGPUPlatformInfo(dev device.Device, driverMajor int) *apiv1.GPUPlatformInfo {
	if driverMajor < minDriverMajorForNVMLPlatformInfo {
		return nil
	}
	result := &apiv1.GPUPlatformInfo{Errors: make(map[string]string)}
	platform, ret := dev.GetPlatformInfo()
	if ret == nvml.SUCCESS {
		result.ChassisSerial = strings.TrimRight(string(platform.ChassisSerialNumber[:]), "\x00")
		result.IBGUID = hex.EncodeToString(platform.IbGuid[:])
		result.Version = platform.Version
		result.SlotNumber = &platform.SlotNumber
		result.TrayIndex = &platform.TrayIndex
		result.HostID = &platform.HostId
		result.PeerType = &platform.PeerType
		result.ModuleID = &platform.ModuleId
	} else {
		result.Errors["platform"] = nvml.ErrorString(ret)
	}
	module, ret := dev.GetModuleId()
	if ret == nvml.SUCCESS {
		value := uint32(module)
		result.DeviceModuleID = &value
	} else {
		result.Errors["deviceModuleId"] = nvml.ErrorString(ret)
	}
	return result
}
