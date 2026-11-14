package nvml

import (
	"context"
	"testing"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestArchitectureIdentity(t *testing.T) {
	for _, tc := range []struct {
		name      string
		arch      nvml.DeviceArchitecture
		ret       nvml.Return
		want      string
		fallback  bool
		wantError bool
	}{
		{name: "Rubin without CUDA or product name", arch: nvml.DEVICE_ARCH_RUBIN, want: "rubin"},
		{name: "NVML Ampere code 7 is not CUDA Volta", arch: nvml.DEVICE_ARCH_AMPERE, want: "ampere"},
		{name: "NVML Ada code 8 is not CUDA Ampere", arch: nvml.DEVICE_ARCH_ADA, want: "ada-lovelace"},
		{name: "unknown enum falls back", arch: nvml.DEVICE_ARCH_UNKNOWN, want: "hopper", fallback: true},
		{name: "unsupported falls back", ret: nvml.ERROR_NOT_SUPPORTED, want: "hopper", fallback: true},
		{name: "missing API falls back", ret: nvml.ERROR_FUNCTION_NOT_FOUND, want: "hopper", fallback: true},
		{name: "lost device is not a fallback", ret: nvml.ERROR_GPU_IS_LOST, wantError: true},
		{name: "reset required is not a fallback", ret: nvml.ERROR_RESET_REQUIRED, wantError: true},
		{name: "unknown error is not a fallback", ret: nvml.ERROR_UNKNOWN, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fallbackCalled := false
			dev := testutil.NewMockDevice(&mock.Device{
				GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) { return tc.arch, tc.ret },
				GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
					fallbackCalled = true
					return 9, 0, nvml.SUCCESS
				},
			}, "", "", "", "")
			arch, err := GetArchFamily(dev)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, arch)
			require.Equal(t, tc.fallback, fallbackCalled)
		})
	}
	// NVML's value 13 must never be mistaken for a CUDA compute major.
	require.Equal(t, "undefined", getArchFamily(13, 0))
}

func TestRubinInstanceGenericProduct(t *testing.T) {
	for _, tc := range []struct {
		name, override  string
		link            nvidiaproduct.HostLink
		wantFabricState bool
		wantExternal    bool
		wantFMIfFabric  bool
	}{
		{name: "Vera Rubin NVL72 tray", link: nvidiaproduct.HostLinkC2C, wantFabricState: true, wantExternal: true},
		{name: "Rubin NVL8 baseboard", link: nvidiaproduct.HostLinkPCIe, wantFabricState: true, wantFMIfFabric: true},
		{name: "no host-link evidence", link: nvidiaproduct.HostLinkUnknown, wantFabricState: true},
		// The override simulates another product, so hardware evidence is ignored.
		{name: "override", override: "NVIDIA H100 PCIe", link: nvidiaproduct.HostLinkC2C},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey("Rubin identity enables health checks without a marketing name", t, func() {
				dev := testutil.NewMockDevice(&mock.Device{
					GetNameFunc:         func() (string, nvml.Return) { return "NVIDIA Graphics Device", nvml.SUCCESS },
					GetUUIDFunc:         func() (string, nvml.Return) { return "GPU-rubin", nvml.SUCCESS },
					GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) { return nvml.DEVICE_ARCH_RUBIN, nvml.SUCCESS },
					GetBrandFunc:        func() (nvml.BrandType, nvml.Return) { return nvml.BRAND_NVIDIA, nvml.SUCCESS },
				}, "", "", "", "0000:01:00.0")
				lib := newMockLibrary("610.10", 13040, []nvlibdevice.Device{dev}, nil)
				mockey.Mock(nvmllib.New).Return(lib, nil).Build()
				mockey.Mock(detectHostLink).Return(tc.link).Build()
				inst, err := newInstance(context.Background(), nil, &FailureInjectorConfig{GPUProductNameOverride: tc.override})
				require.NoError(t, err)
				require.Equal(t, "rubin", inst.Architecture())
				caps := inst.Capabilities()
				require.Equal(t, tc.wantFabricState, inst.FabricStateSupported())
				require.Equal(t, tc.wantExternal, caps.ExternalFabricCandidate)
				require.Equal(t, tc.wantFMIfFabric, caps.FabricManagerIfFabric)
				// No Rubin system is known to need FM unconditionally.
				require.False(t, inst.FabricManagerSupported())
				require.True(t, inst.GetMemoryErrorManagementCapabilities().RowRemapping)
			})
		})
	}
}

func TestHostLinkFromC2CMode(t *testing.T) {
	require.Equal(t, nvidiaproduct.HostLinkC2C, hostLinkFromC2CMode(nvml.C2cModeInfo_v1{IsC2cEnabled: 1}, nvml.SUCCESS))
	require.Equal(t, nvidiaproduct.HostLinkPCIe, hostLinkFromC2CMode(nvml.C2cModeInfo_v1{}, nvml.SUCCESS))
	require.Equal(t, nvidiaproduct.HostLinkPCIe, hostLinkFromC2CMode(nvml.C2cModeInfo_v1{}, nvml.ERROR_NOT_SUPPORTED))
	require.Equal(t, nvidiaproduct.HostLinkUnknown, hostLinkFromC2CMode(nvml.C2cModeInfo_v1{}, nvml.ERROR_GPU_IS_LOST))
}

func TestRubinInstanceHostLinkFallbackAcrossDevices(t *testing.T) {
	mockey.PatchConvey("host-link detection falls back when the first device fails", t, func() {
		makeDev := func(uuid, bus string) nvlibdevice.Device {
			return testutil.NewMockDevice(&mock.Device{
				GetNameFunc:         func() (string, nvml.Return) { return "NVIDIA Graphics Device", nvml.SUCCESS },
				GetUUIDFunc:         func() (string, nvml.Return) { return uuid, nvml.SUCCESS },
				GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) { return nvml.DEVICE_ARCH_RUBIN, nvml.SUCCESS },
				GetBrandFunc:        func() (nvml.BrandType, nvml.Return) { return nvml.BRAND_NVIDIA, nvml.SUCCESS },
			}, "", "", "", bus)
		}
		dev0, dev1 := makeDev("GPU-0", "0000:01:00.0"), makeDev("GPU-1", "0000:02:00.0")
		lib := newMockLibrary("610.10", 13040, []nvlibdevice.Device{dev0, dev1}, nil)
		mockey.Mock(nvmllib.New).Return(lib, nil).Build()
		calls := 0
		mockey.Mock(detectHostLink).To(func(_ nvmllib.Library, _ nvlibdevice.Device) nvidiaproduct.HostLink {
			calls++
			if calls == 1 {
				return nvidiaproduct.HostLinkUnknown
			}
			return nvidiaproduct.HostLinkC2C
		}).Build()
		inst, err := newInstance(context.Background(), nil, nil)
		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.True(t, inst.Capabilities().ExternalFabricCandidate)
	})
}
