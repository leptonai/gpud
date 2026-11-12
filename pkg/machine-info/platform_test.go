package machineinfo

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type platformDevice struct {
	device.Device
	info      nvml.PlatformInfo
	ret       nvml.Return
	module    int
	moduleRet nvml.Return
}

func (d *platformDevice) GetPlatformInfo() (nvml.PlatformInfo, nvml.Return) { return d.info, d.ret }
func (d *platformDevice) GetModuleId() (int, nvml.Return)                   { return d.module, d.moduleRet }

func TestPlatformIndependentAPIsAndRawCoordinates(t *testing.T) {
	d := &platformDevice{info: nvml.PlatformInfo{SlotNumber: 0, TrayIndex: 7, HostId: 1, PeerType: 0, ModuleId: 255}, module: 3}
	copy(d.info.ChassisSerialNumber[:], "rack-test")
	p := getGPUPlatformInfo(d, 580)
	require.Equal(t, "rack-test", p.ChassisSerial)
	require.Equal(t, uint8(0), *p.SlotNumber)
	require.Equal(t, uint8(255), *p.ModuleID)
	require.Equal(t, uint32(3), *p.DeviceModuleID)
	d.ret = nvml.ERROR_NOT_SUPPORTED
	p = getGPUPlatformInfo(d, 580)
	require.Nil(t, p.SlotNumber)
	require.Equal(t, uint32(3), *p.DeviceModuleID)
	require.NotEmpty(t, p.Errors["platform"])
	d.ret = nvml.SUCCESS
	d.moduleRet = nvml.ERROR_GPU_IS_LOST
	p = getGPUPlatformInfo(d, 580)
	require.NotNil(t, p.TrayIndex)
	require.Nil(t, p.DeviceModuleID)
	require.NotEmpty(t, p.Errors["deviceModuleId"])
	// No unsupported call should be made with an older driver.
	require.Nil(t, getGPUPlatformInfo(nil, 535))
}
