package nvml

import (
	"testing"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

func TestProductNameInference(t *testing.T) {
	for _, tc := range []struct {
		name, product, want string
		arch                nvml.DeviceArchitecture
		archRet, nameRet    nvml.Return
		queryArch           bool
	}{
		{name: "generic Rubin", product: "NVIDIA Graphics Device", arch: nvml.DEVICE_ARCH_RUBIN, want: "NVIDIA VR NVL72", queryArch: true},
		{name: "case and whitespace", product: " nvidia graphics device ", arch: nvml.DEVICE_ARCH_RUBIN, want: "NVIDIA VR NVL72", queryArch: true},
		{name: "driver reports VR NVL72", product: "NVIDIA VR NVL72", want: "NVIDIA VR NVL72"},
		{name: "specific Rubin name", product: "NVIDIA Rubin", arch: nvml.DEVICE_ARCH_RUBIN, want: "NVIDIA Rubin"},
		{name: "specific legacy name", product: "NVIDIA H100 80GB HBM3", want: "NVIDIA H100 80GB HBM3"},
		{name: "generic Blackwell", product: "NVIDIA Graphics Device", arch: nvml.DEVICE_ARCH_BLACKWELL, want: "NVIDIA Graphics Device", queryArch: true},
		{name: "unknown architecture", product: "NVIDIA Graphics Device", arch: nvml.DEVICE_ARCH_UNKNOWN, want: "NVIDIA Graphics Device", queryArch: true},
		{name: "unsupported architecture", product: "NVIDIA Graphics Device", archRet: nvml.ERROR_NOT_SUPPORTED, want: "NVIDIA Graphics Device", queryArch: true},
		{name: "missing symbol", product: "NVIDIA Graphics Device", archRet: nvml.ERROR_FUNCTION_NOT_FOUND, want: "NVIDIA Graphics Device", queryArch: true},
		{name: "failed architecture", product: "NVIDIA Graphics Device", arch: nvml.DEVICE_ARCH_RUBIN, archRet: nvml.ERROR_GPU_IS_LOST, want: "NVIDIA Graphics Device", queryArch: true},
		{name: "empty name", arch: nvml.DEVICE_ARCH_RUBIN},
		{name: "failed name", nameRet: nvml.ERROR_GPU_IS_LOST},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey("direct queries and device-name loading agree", t, func() {
				archCalls := 0
				dev := testutil.NewMockDevice(&mock.Device{
					GetNameFunc: func() (string, nvml.Return) { return tc.product, tc.nameRet },
					GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
						archCalls++
						return tc.arch, tc.archRet
					},
				}, "", "", "", "")
				lib := newMockLibrary("620.16", 13050, []nvlibdevice.Device{dev}, nil)
				mockey.Mock(nvmllib.New).Return(lib, nil).Build()
				for _, query := range []func() (string, error){
					func() (string, error) { return GetProductName(dev) },
					LoadGPUDeviceName,
				} {
					name, err := query()
					if tc.nameRet != nvml.SUCCESS {
						require.ErrorContains(t, err, "failed to get device name")
					} else {
						require.NoError(t, err)
					}
					require.Equal(t, tc.want, name)
				}
				if tc.queryArch {
					require.Equal(t, 2, archCalls)
				} else {
					require.Zero(t, archCalls)
				}
			})
		})
	}
}
