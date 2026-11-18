package nvml

import (
	"context"
	"errors"
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
		{name: "lost device uses CUDA fallback", ret: nvml.ERROR_GPU_IS_LOST, want: "hopper", fallback: true},
		{name: "reset required uses CUDA fallback", ret: nvml.ERROR_RESET_REQUIRED, want: "hopper", fallback: true},
		{name: "unknown error uses CUDA fallback", ret: nvml.ERROR_UNKNOWN, want: "hopper", fallback: true},
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
		name, driverName, override string
		link                       nvidiaproduct.HostLink
		wantRubin                  bool
		wantFabricState            bool
		wantExternal               bool
		wantExplicitNVL72          bool
	}{
		{name: "Vera Rubin NVL72 tray", link: nvidiaproduct.HostLinkC2C, wantRubin: true, wantFabricState: true, wantExternal: true},
		{name: "disabled C2C leaves topology unknown", link: nvidiaproduct.HostLinkUnknown, wantRubin: true, wantFabricState: true},
		{name: "no host-link evidence", link: nvidiaproduct.HostLinkUnknown, wantRubin: true, wantFabricState: true},
		{name: "driver declares NVL72", driverName: nvidiaproduct.VRNVL72Name, wantRubin: true, wantFabricState: true, wantExplicitNVL72: true},
		// The override simulates another product, so hardware evidence is ignored.
		{name: "override", override: "NVIDIA H100 PCIe", link: nvidiaproduct.HostLinkC2C},
		{name: "override declares NVL72", override: nvidiaproduct.VRNVL72Name, wantRubin: true, wantFabricState: true, wantExplicitNVL72: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey("Rubin identity enables health checks without a marketing name", t, func() {
				driverName := tc.driverName
				if driverName == "" {
					driverName = "NVIDIA Graphics Device"
				}
				dev := testutil.NewMockDevice(&mock.Device{
					GetNameFunc:         func() (string, nvml.Return) { return driverName, nvml.SUCCESS },
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
				wantProduct := "NVIDIA-VR-NVL72"
				if tc.override != "" {
					wantProduct = nvidiaproduct.SanitizeProductName(tc.override)
				}
				require.Equal(t, wantProduct, inst.ProductName())
				caps := inst.Capabilities()
				require.Equal(t, tc.wantRubin, caps.Rubin)
				require.Equal(t, tc.wantExplicitNVL72, caps.ExplicitNVL72)
				require.Equal(t, tc.wantFabricState, inst.FabricStateSupported())
				require.Equal(t, tc.wantExternal, caps.ExternalFabricCandidate)
				// No Rubin system is known to need FM unconditionally.
				require.False(t, inst.FabricManagerSupported())
				require.True(t, inst.GetMemoryErrorManagementCapabilities().RowRemapping)
			})
		})
	}
}

func TestHostLinkFromC2CMode(t *testing.T) {
	require.Equal(t, nvidiaproduct.HostLinkC2C, hostLinkFromC2CMode(nvml.C2cModeInfo_v1{IsC2cEnabled: 1}, nvml.SUCCESS))
	require.Equal(t, nvidiaproduct.HostLinkUnknown, hostLinkFromC2CMode(nvml.C2cModeInfo_v1{}, nvml.SUCCESS))
	require.Equal(t, nvidiaproduct.HostLinkUnknown, hostLinkFromC2CMode(nvml.C2cModeInfo_v1{}, nvml.ERROR_NOT_SUPPORTED))
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
				return hostLinkFromC2CMode(nvml.C2cModeInfo_v1{}, nvml.ERROR_NOT_SUPPORTED)
			}
			return nvidiaproduct.HostLinkC2C
		}).Build()
		inst, err := newInstance(context.Background(), nil, nil)
		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.True(t, inst.Capabilities().ExternalFabricCandidate)
	})
}

func TestDetectHostLinkUsesVersionedHandler(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing bool
		enabled uint32
		ret     nvml.Return
		want    nvidiaproduct.HostLink
	}{
		{name: "enabled", enabled: 1, want: nvidiaproduct.HostLinkC2C},
		{name: "disabled"},
		{name: "unsupported", ret: nvml.ERROR_NOT_SUPPORTED},
		{name: "query failure", ret: nvml.ERROR_UNKNOWN},
		{name: "missing symbol", missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey("C2C lookup and versioned handler", t, func() {
				called := false
				mockey.Mock(nvml.C2cModeInfoHandler.V1).To(func(nvml.C2cModeInfoHandler) (nvml.C2cModeInfo_v1, nvml.Return) {
					called = true
					return nvml.C2cModeInfo_v1{IsC2cEnabled: tc.enabled}, tc.ret
				}).Build()
				lib := &fullMockLibWrapper{nvmlIface: &mock.Interface{ExtensionsFunc: func() nvml.ExtendedInterface {
					return &mock.ExtendedInterface{LookupSymbolFunc: func(name string) error {
						require.Equal(t, "nvmlDeviceGetC2cModeInfoV", name)
						if tc.missing {
							return errors.New("missing symbol")
						}
						return nil
					}}
				}}}
				dev := testutil.NewMockDevice(&mock.Device{GetC2cModeInfoVFunc: func() nvml.C2cModeInfoHandler {
					return nvml.C2cModeInfoHandler{}
				}}, "", "", "", "")
				require.Equal(t, tc.want, detectHostLink(lib, dev))
				require.Equal(t, !tc.missing, called)
			})
		})
	}
}

func TestLegacyStartupDoesNotQueryC2C(t *testing.T) {
	for _, name := range []string{"NVIDIA H100", "NVIDIA GB200", "NVIDIA GB300"} {
		t.Run(name, func(t *testing.T) {
			mockey.PatchConvey("C2C query is only activated on Rubin", t, func() {
				dev := testutil.NewMockDevice(&mock.Device{
					GetNameFunc:         func() (string, nvml.Return) { return name, nvml.SUCCESS },
					GetUUIDFunc:         func() (string, nvml.Return) { return "GPU-test", nvml.SUCCESS },
					GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) { return nvml.DEVICE_ARCH_HOPPER, nvml.SUCCESS },
					GetBrandFunc:        func() (nvml.BrandType, nvml.Return) { return nvml.BRAND_NVIDIA, nvml.SUCCESS },
				}, "", "", "", "0000:01:00.0")
				lib := newMockLibrary("610.10", 13040, []nvlibdevice.Device{dev}, nil)
				mockey.Mock(nvmllib.New).Return(lib, nil).Build()
				mockey.Mock(detectHostLink).To(func(nvmllib.Library, nvlibdevice.Device) nvidiaproduct.HostLink {
					t.Fatal("legacy startup must not query C2C")
					return nvidiaproduct.HostLinkUnknown
				}).Build()
				inst, err := newInstance(t.Context(), nil, nil)
				require.NoError(t, err)
				require.False(t, inst.Capabilities().Rubin)
			})
		})
	}
}
