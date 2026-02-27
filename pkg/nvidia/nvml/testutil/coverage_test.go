package testutil

import (
	"testing"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateClockSpeedDevice_HelperCoverage(t *testing.T) {
	dev := CreateClockSpeedDevice(
		1234,
		nvml.SUCCESS,
		5678,
		nvml.ERROR_NOT_SUPPORTED,
		"clock-uuid",
	)

	graphicsClock, graphicsRet := dev.GetClockInfo(nvml.CLOCK_GRAPHICS)
	require.Equal(t, uint32(1234), graphicsClock)
	require.Equal(t, nvml.SUCCESS, graphicsRet)

	memClock, memRet := dev.GetClockInfo(nvml.CLOCK_MEM)
	require.Equal(t, uint32(5678), memClock)
	require.Equal(t, nvml.ERROR_NOT_SUPPORTED, memRet)

	unknownClock, unknownRet := dev.GetClockInfo(nvml.ClockType(999))
	require.Equal(t, uint32(0), unknownClock)
	require.Equal(t, nvml.ERROR_UNKNOWN, unknownRet)

	uuid, uuidRet := dev.GetUUID()
	require.Equal(t, nvml.SUCCESS, uuidRet)
	require.Equal(t, "clock-uuid", uuid)
}

func TestCreateGPMSupportedDevice_HelperCoverage(t *testing.T) {
	dev := CreateGPMSupportedDevice(
		"gpm-supported-uuid",
		nvml.GpmSupport{IsSupportedDevice: 1},
		nvml.SUCCESS,
	)

	support, ret := dev.GpmQueryDeviceSupport()
	require.Equal(t, nvml.SUCCESS, ret)
	assert.EqualValues(t, 1, support.IsSupportedDevice)

	uuid, uuidRet := dev.GetUUID()
	require.Equal(t, nvml.SUCCESS, uuidRet)
	require.Equal(t, "gpm-supported-uuid", uuid)
}

func TestCreateGPMSampleDevice_HelperCoverage(t *testing.T) {
	dev := CreateGPMSampleDevice("gpm-sample-uuid", nvml.ERROR_NOT_SUPPORTED)

	support, supportRet := dev.GpmQueryDeviceSupport()
	require.Equal(t, nvml.SUCCESS, supportRet)
	assert.EqualValues(t, 1, support.IsSupportedDevice)

	sampleRet := dev.GpmSampleGet(nil)
	require.Equal(t, nvml.ERROR_NOT_SUPPORTED, sampleRet)

	uuid, uuidRet := dev.GetUUID()
	require.Equal(t, nvml.SUCCESS, uuidRet)
	require.Equal(t, "gpm-sample-uuid", uuid)
}

func TestCreateGSPFirmwareDevice_HelperCoverage(t *testing.T) {
	dev := CreateGSPFirmwareDevice("gsp-uuid", true, false, nvml.SUCCESS)

	enabled, supported, ret := dev.GetGspFirmwareMode()
	require.Equal(t, nvml.SUCCESS, ret)
	assert.True(t, enabled)
	assert.False(t, supported)

	uuid, uuidRet := dev.GetUUID()
	require.Equal(t, nvml.SUCCESS, uuidRet)
	require.Equal(t, "gsp-uuid", uuid)
}

func TestCreateMemoryDevice_HelperCoverage(t *testing.T) {
	memV2 := nvml.Memory_v2{
		Version:  1,
		Total:    16 * 1024,
		Reserved: 512,
		Free:     4 * 1024,
		Used:     12 * 1024,
	}
	mem := nvml.Memory{
		Total: 16 * 1024,
		Free:  4 * 1024,
		Used:  12 * 1024,
	}

	dev := CreateMemoryDevice("memory-uuid", memV2, nvml.SUCCESS, mem, nvml.ERROR_NOT_SUPPORTED)

	gotV2, gotV2Ret := dev.GetMemoryInfo_v2()
	require.Equal(t, nvml.SUCCESS, gotV2Ret)
	assert.Equal(t, memV2, gotV2)

	gotLegacy, gotLegacyRet := dev.GetMemoryInfo()
	require.Equal(t, nvml.ERROR_NOT_SUPPORTED, gotLegacyRet)
	assert.Equal(t, mem, gotLegacy)

	uuid, uuidRet := dev.GetUUID()
	require.Equal(t, nvml.SUCCESS, uuidRet)
	require.Equal(t, "memory-uuid", uuid)
}

func TestCreatePersistenceModeDevice_HelperCoverage(t *testing.T) {
	dev := CreatePersistenceModeDevice("persistence-uuid", nvml.FEATURE_ENABLED, nvml.SUCCESS)

	state, ret := dev.GetPersistenceMode()
	require.Equal(t, nvml.SUCCESS, ret)
	assert.Equal(t, nvml.FEATURE_ENABLED, state)

	uuid, uuidRet := dev.GetUUID()
	require.Equal(t, nvml.SUCCESS, uuidRet)
	require.Equal(t, "persistence-uuid", uuid)
}

func TestMockDevice_AllUtilityMethodsCoverage(t *testing.T) {
	d := NewMockDeviceWithIDs(
		nil,
		"blackwell",
		"NVIDIA",
		"12.0",
		"0000:ff:00.0",
		"GPU-UTIL-UUID",
		"GPU-UTIL-SERIAL",
		7,
		42,
	)

	arch, err := d.GetArchitectureAsString()
	require.NoError(t, err)
	assert.Equal(t, "blackwell", arch)

	brand, err := d.GetBrandAsString()
	require.NoError(t, err)
	assert.Equal(t, "NVIDIA", brand)

	cc, err := d.GetCudaComputeCapabilityAsString()
	require.NoError(t, err)
	assert.Equal(t, "12.0", cc)

	migs, err := d.GetMigDevices()
	require.NoError(t, err)
	assert.Nil(t, migs)

	profiles, err := d.GetMigProfiles()
	require.NoError(t, err)
	assert.Nil(t, profiles)

	pciBusID, err := d.GetPCIBusID()
	require.NoError(t, err)
	assert.Equal(t, "0000:ff:00.0", pciBusID)

	serial, serialRet := d.GetSerial()
	assert.Equal(t, nvml.SUCCESS, serialRet)
	assert.Equal(t, "GPU-UTIL-SERIAL", serial)

	minor, minorRet := d.GetMinorNumber()
	assert.Equal(t, nvml.SUCCESS, minorRet)
	assert.Equal(t, 7, minor)

	boardID, boardRet := d.GetBoardId()
	assert.Equal(t, nvml.SUCCESS, boardRet)
	assert.Equal(t, uint32(42), boardID)

	coherent, err := d.IsCoherent()
	require.NoError(t, err)
	assert.False(t, coherent)

	fabricAttached, err := d.IsFabricAttached()
	require.NoError(t, err)
	assert.False(t, fabricAttached)

	migCapable, err := d.IsMigCapable()
	require.NoError(t, err)
	assert.False(t, migCapable)

	migEnabled, err := d.IsMigEnabled()
	require.NoError(t, err)
	assert.False(t, migEnabled)

	require.NoError(t, d.VisitMigDevices(func(_ int, _ nvlibdevice.MigDevice) error { return nil }))
	require.NoError(t, d.VisitMigProfiles(func(_ nvlibdevice.MigProfile) error { return nil }))

	assert.Equal(t, "0000:ff:00.0", d.PCIBusID())
	assert.Equal(t, "GPU-UTIL-UUID", d.UUID())

	fabricState, err := d.GetFabricState()
	require.NoError(t, err)
	assert.EqualValues(t, nvml.GPU_FABRIC_STATE_NOT_STARTED, fabricState.State)
	assert.EqualValues(t, nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED, fabricState.HealthSummary)
}

func TestNewMockDevice_DefaultIDCoverage(t *testing.T) {
	d := NewMockDevice(nil, "hopper", "Tesla", "9.0", "0000:01:00.0")

	assert.Equal(t, "MOCK-GPU-UUID", d.UUID())
	serial, ret := d.GetSerial()
	assert.Equal(t, nvml.SUCCESS, ret)
	assert.Equal(t, "MOCK-GPU-SERIAL", serial)

	minor, minorRet := d.GetMinorNumber()
	assert.Equal(t, nvml.SUCCESS, minorRet)
	assert.Equal(t, 0, minor)

	boardID, boardRet := d.GetBoardId()
	assert.Equal(t, nvml.SUCCESS, boardRet)
	assert.Equal(t, uint32(0), boardID)
}
